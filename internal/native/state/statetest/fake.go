// Package statetest has in-memory services for the stores, so the native
// screens can be tested headless and measured without a provider: Fake
// keeps projects, chats and messages in memory and records what the
// screens send.
package statetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Sent is a message the screens sent.
type Sent struct {
	Project id.Project
	Chat    id.Chat
	Text    string
	Quote   *chat.Quote
	ID      id.Message // the message Send made
}

// Fake is every service the stores use, in memory. Its fields are read
// and written under mu by the methods; tests set them up before the
// stores read them.
type Fake struct {
	mu       sync.Mutex
	Projects []app.ProjectItem
	Mothers  map[id.Project]id.Chat
	Chats    map[id.Project][]app.ChatItem
	Msgs     map[id.Chat][]chat.Message
	Groups   []app.ModelGroup
	SetTo    map[id.Chat]string // SetModel's last model per chat
	Levels   []string           // SetLevel's levels, in order
	sent     []Sent
	answers  []agent.Answer
	rewinds  []int
	rewound  []id.Chat // the chats of rewinds, in order
	stops    int
	retries  int
	n        int
	// Undone is what the chips' undo reverted: "memory:", "desktop:" or
	// "refresh:" and the change, true while it is undone.
	Undone map[string]bool
	// HoldUndone, when not nil, holds each answer to whether a chip's
	// change is undone until it is closed, as a slow read.
	HoldUndone chan struct{}
	// Changed makes the chips' undo of these changes find them changed
	// since, as a section edited after its chip.
	Changed map[string]bool
	// Estimate is RefreshEstimate's answer; Refreshes records each
	// refresh started, true for a full one.
	Estimate  agent.RefreshEstimate
	Refreshes []bool
	// TurnUndo is what Undo turn and Redo turn would do now; undoTurns
	// records them.
	TurnUndo  agent.TurnUndoState
	undoTurns []agent.UndoRequest
	live      map[id.Chat]agent.Live // each chat's running turn, for Snapshot
	// Bucket is the projects' files and the bottom bar's readings.
	Bucket FakeBucket
	admin  ProjectAdmin // the project screens' part (fake_project.go)
}

// New makes an empty Fake.
func New() *Fake {
	return &Fake{Mothers: map[id.Project]id.Chat{}, Chats: map[id.Project][]app.ChatItem{},
		Msgs: map[id.Chat][]chat.Message{}, SetTo: map[id.Chat]string{},
		Undone: map[string]bool{}, Changed: map[string]bool{}}
}

// API is the stores' services, all of them f.
func (f *Fake) API() state.API {
	return state.API{Projects: projects{f}, Chats: f, Settings: f, System: f, Actions: f, Bucket: bucket{f}, Status: status{f}}
}

// AddProject adds a project with its Mother chat and returns both.
func (f *Fake) AddProject(name string) (id.Project, id.Chat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	p := id.Project(fmt.Sprintf("p%02d", f.n))
	mother := chat.Chat{ID: id.Chat(string(p) + "-mother"), Kind: chat.KindMother, Title: "Mother", Model: "default", CreatedAt: time.Unix(0, 0)}
	f.Projects = append(f.Projects, app.ProjectItem{ID: p, Name: name, Folder: `C:\Projects\` + name, CreatedAt: time.Unix(0, 0)})
	f.Mothers[p] = mother.ID
	f.Chats[p] = append(f.Chats[p], app.ChatItem{Chat: mother, State: chat.StateIdle})
	return p, mother.ID
}

// AddChat adds a chat to project p and returns it.
func (f *Fake) AddChat(p id.Project, title string) id.Chat {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addChat(p, title, "").ID
}

// RemoveChat takes chat c out of project p's list, as a delete elsewhere
// does.
func (f *Fake) RemoveChat(p id.Project, c id.Chat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Chats[p] = slices.DeleteFunc(f.Chats[p], func(it app.ChatItem) bool { return it.Chat.ID == c })
}

func (f *Fake) addChat(p id.Project, title, role string) chat.Chat {
	f.n++
	c := chat.Chat{ID: id.Chat(fmt.Sprintf("%s-c%02d", p, f.n)), Kind: chat.KindChat, Title: title, Role: role, Model: "default", CreatedAt: time.Unix(0, 0)}
	f.Chats[p] = append(f.Chats[p], app.ChatItem{Chat: c, State: chat.StateIdle})
	return c
}

// AddMessages adds messages to chat c, numbering their IDs.
func (f *Fake) AddMessages(c id.Chat, ms ...chat.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range ms {
		f.n++
		if m.ID == "" {
			m.ID = id.Message(fmt.Sprintf("m%05d", f.n))
		}
		m.Chat = c
		f.Msgs[c] = append(f.Msgs[c], m)
	}
}

// Sent returns what the screens sent.
func (f *Fake) Sent() []Sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}

// Stops counts the Stop calls.
func (f *Fake) Stops() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stops
}

// projects is the Fake as ProjectsAPI, whose List and Create clash with
// the chats'.
type projects struct{ f *Fake }

func (p projects) List(context.Context) ([]app.ProjectItem, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	return slices.Clone(p.f.Projects), nil
}

func (projects) FolderWarning(context.Context) (*project.FolderWarning, error) { return nil, nil }

func (p projects) Create(_ context.Context, name string) (app.OpenedProject, error) {
	if err := p.f.Admin().CreateErr; err != nil {
		return app.OpenedProject{}, err
	}
	pid, mother := p.f.AddProject(name)
	return app.OpenedProject{ID: pid, Name: name, Mother: mother, Level: "standard"}, nil
}

func (p projects) Open(_ context.Context, pid id.Project) (app.OpenedProject, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	for _, it := range p.f.Projects {
		if it.ID == pid {
			return app.OpenedProject{ID: pid, Name: it.Name, Mother: p.f.Mothers[pid], Level: "standard"}, nil
		}
	}
	return app.OpenedProject{}, errors.New("no such project")
}

// List is ChatsAPI's.
func (f *Fake) List(_ context.Context, p id.Project) ([]app.ChatItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.Chats[p]), nil
}

func (f *Fake) Waiting(context.Context) ([]app.WaitingItem, error) { return nil, nil }

func (f *Fake) Create(_ context.Context, p id.Project, title, role string) (chat.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addChat(p, title, role), nil
}

func (f *Fake) chat(c id.Chat) chat.Chat {
	for _, items := range f.Chats {
		for _, it := range items {
			if it.Chat.ID == c {
				return it.Chat
			}
		}
	}
	return chat.Chat{ID: c}
}

func (f *Fake) Snapshot(_ context.Context, _ id.Project, c id.Chat) (app.ChatSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := f.chat(c)
	if m, ok := f.SetTo[c]; ok {
		ch.Model = m
	}
	return app.ChatSnapshot{Chat: ch, Messages: slices.Clone(f.Msgs[c]), From: 1, Live: f.live[c]}, nil
}

// SetLive sets the turn running in chat c, as a snapshot reads it.
func (f *Fake) SetLive(c id.Chat, l agent.Live) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live == nil {
		f.live = map[id.Chat]agent.Live{}
	}
	f.live[c] = l
}

func (f *Fake) Messages(_ context.Context, _ id.Project, c id.Chat, from, to int) ([]chat.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []chat.Message
	for _, m := range f.Msgs[c] {
		if m.Turn >= from && (to <= 0 || m.Turn <= to) {
			out = append(out, m)
		}
	}
	return out, nil
}

// Get is SettingsAPI's: the default settings.
func (f *Fake) Get(context.Context) (app.SettingsView, error) {
	return app.SettingsView{Settings: config.Defaults()}, nil
}

func (f *Fake) Save(_ context.Context, next config.Settings) (app.SettingsView, error) {
	return app.SettingsView{Settings: next}, nil
}

func (f *Fake) Models(context.Context) ([]app.ModelGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.Groups), nil
}

func (f *Fake) Started(context.Context) (app.Started, error) { return app.Started{}, nil }

func (f *Fake) Connect(context.Context, app.ConnectRequest) (app.ConnectResult, error) {
	return app.ConnectResult{}, errors.New("not in the fake")
}

func (f *Fake) Remove(context.Context, string) (app.SettingsView, error) {
	return app.SettingsView{Settings: config.Defaults()}, nil
}

// Notify is SystemAPI's; the fake shows nothing.
func (f *Fake) Notify(context.Context, app.Notification) error { return nil }

// Send is ActionsAPI's: it records the message.
func (f *Fake) Send(_ context.Context, p id.Project, c id.Chat, text string, quote *chat.Quote) (id.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	m := id.Message(fmt.Sprintf("sent%05d", f.n))
	f.sent = append(f.sent, Sent{Project: p, Chat: c, Text: text, Quote: quote, ID: m})
	return m, nil
}

func (f *Fake) Stop(context.Context, id.Project, id.Chat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}

// Retry counts the retries.
func (f *Fake) Retry(context.Context, id.Project, id.Chat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries++
	return nil
}

// Retries is how many times Retry was called.
func (f *Fake) Retries() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.retries
}

// flip undoes or redoes key, as the chips' undo does: already as asked is
// not done, and a change made since is left as it is.
func (f *Fake) flip(key string, redo bool) (done, changed, undone bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.Changed[key]:
		return false, true, f.Undone[key]
	case f.Undone[key] == !redo:
		return false, false, f.Undone[key]
	}
	f.Undone[key] = !redo
	return true, false, !redo
}

// IsUndone tells whether the chips' undo reverted key.
func (f *Fake) IsUndone(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Undone[key]
}

// SetUndone sets whether key is undone while the screen runs, as an
// Undo turn or Redo turn of the chip's turn does.
func (f *Fake) SetUndone(key string, undone bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Undone[key] = undone
}

// undone answers whether key is undone, once HoldUndone lets it.
func (f *Fake) undone(key string) bool {
	f.mu.Lock()
	hold := f.HoldUndone
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	return f.IsUndone(key)
}

// SetChanged makes key changed since its chip, so its undo does nothing.
func (f *Fake) SetChanged(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Changed[key] = true
}

func (f *Fake) MemoryUndone(_ context.Context, _ id.Project, r app.MemoryUndoRequest) (bool, error) {
	return f.undone("memory:" + r.Change), nil
}

// UndoMemory undoes or redoes the memory chip's change.
func (f *Fake) UndoMemory(_ context.Context, _ id.Project, r app.MemoryUndoRequest) (app.MemoryUndo, error) {
	done, changed, undone := f.flip("memory:"+r.Change, r.Redo)
	return app.MemoryUndo{Done: done, Changed: changed, Undone: undone}, nil
}

func (f *Fake) DesktopSaveUndone(_ context.Context, r app.DesktopUndoRequest) (bool, error) {
	return f.undone("desktop:" + r.Change), nil
}

// UndoDesktopSave undoes or redoes the desktop chip's save.
func (f *Fake) UndoDesktopSave(_ context.Context, r app.DesktopUndoRequest) (store.DesktopUndo, error) {
	done, changed, undone := f.flip("desktop:"+r.Change, r.Redo)
	return store.DesktopUndo{Done: done, Changed: changed, Undone: undone}, nil
}

func (f *Fake) RefreshEstimate(context.Context, id.Project, id.Chat, bool) (agent.RefreshEstimate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Estimate, nil
}

// SetEstimate sets RefreshEstimate's answer while the screen runs.
func (f *Fake) SetEstimate(e agent.RefreshEstimate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Estimate = e
}

// Refreshed is each refresh started so far, true for a full one.
func (f *Fake) Refreshed() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.Refreshes...)
}

// RefreshMemory records the refresh.
func (f *Fake) RefreshMemory(_ context.Context, _ id.Project, _ id.Chat, full bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Refreshes = append(f.Refreshes, full)
	return nil
}

func batchKey(edits []chat.MemoryDiff) string {
	k := "refresh:"
	for _, e := range edits {
		k += e.Change + ","
	}
	return k
}

func (f *Fake) RefreshUndone(_ context.Context, _ id.Project, edits []chat.MemoryDiff) (bool, error) {
	return f.undone(batchKey(edits)), nil
}

// UndoRefresh undoes or redoes the batch; a changed one names its first
// section, as the service does.
func (f *Fake) UndoRefresh(_ context.Context, _ id.Project, r app.RefreshUndoRequest) (app.RefreshUndo, error) {
	done, changed, undone := f.flip(batchKey(r.Edits), r.Redo)
	res := app.RefreshUndo{Done: done, Changed: changed, Undone: undone}
	if changed && len(r.Edits) > 0 {
		res.Sections = []string{r.Edits[0].Target}
	}
	return res, nil
}

// Answer records the answer.
func (f *Fake) Answer(_ context.Context, _ id.Project, _ id.Chat, a agent.Answer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, a)
	return nil
}

// Answers returns the answers to cards.
func (f *Fake) Answers() []agent.Answer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.answers)
}

// Rewinds returns the turns rewound to.
func (f *Fake) Rewinds() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.rewinds)
}

// Rewound returns the chats rewound, in the order of Rewinds.
func (f *Fake) Rewound() []id.Chat {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.rewound)
}

func (f *Fake) SetModel(_ context.Context, _ id.Project, c id.Chat, model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SetTo[c] = model
	return nil
}

// SetLevel records the level; one that isn't strict, standard or auto
// fails, as the chat service's does.
func (f *Fake) SetLevel(_ context.Context, _ id.Project, _ id.Chat, level string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch level {
	case "strict", "standard", "auto":
	default:
		return fmt.Errorf("no approval level %q", level)
	}
	f.Levels = append(f.Levels, level)
	return nil
}

// UndoState is TurnUndo, what Undo turn and Redo turn would do now.
func (f *Fake) UndoState(context.Context, id.Project, id.Chat, int) (agent.TurnUndoState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.TurnUndo, nil
}

// UndoTurn records the request, and an undo or redo that has changes to
// make makes them: TurnUndo's plans trade places.
func (f *Fake) UndoTurn(_ context.Context, _ id.Project, _ id.Chat, r agent.UndoRequest) (agent.TurnUndo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.undoTurns = append(f.undoTurns, r)
	u := &f.TurnUndo
	if (r.Redo && u.Redo.Changes == 0) || (!r.Redo && u.Undo.Changes == 0) {
		return agent.TurnUndo{Turn: r.Turn, Redo: r.Redo}, nil
	}
	u.Undo, u.Redo = u.Redo, u.Undo
	return agent.TurnUndo{Turn: r.Turn, Redo: r.Redo, Done: true}, nil
}

// UndoTurns is each Undo turn and Redo turn asked for.
func (f *Fake) UndoTurns() []agent.UndoRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.undoTurns)
}

func (f *Fake) last(c id.Chat) int {
	n := 0
	for _, m := range f.Msgs[c] {
		n = max(n, m.Turn)
	}
	return n
}

func (f *Fake) RewindState(_ context.Context, _ id.Project, c id.Chat, turn int) (agent.RewindState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return agent.RewindState{Turn: turn, LastTurn: f.last(c)}, nil
}

// Rewind takes the turns after turn out of the chat.
func (f *Fake) Rewind(_ context.Context, _ id.Project, c id.Chat, turn int) (agent.RewindResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	last := f.last(c)
	f.rewinds = append(f.rewinds, turn)
	f.rewound = append(f.rewound, c)
	f.Msgs[c] = slices.DeleteFunc(f.Msgs[c], func(m chat.Message) bool { return m.Turn > turn })
	return agent.RewindResult{Rewind: int64(len(f.rewinds)), Turn: turn, LastTurn: last}, nil
}

func (f *Fake) UndoRewind(context.Context, id.Project, id.Chat, int64) error { return nil }

func (f *Fake) RewindUndoState(context.Context, id.Project, id.Chat, int64) (agent.TurnUndoState, error) {
	return agent.TurnUndoState{}, nil
}

// Loop is a UI thread for tests: Post queues work, Drain runs it.
type Loop struct {
	mu   sync.Mutex
	work []func()
}

// Post queues fn.
func (l *Loop) Post(fn func()) {
	l.mu.Lock()
	l.work = append(l.work, fn)
	l.mu.Unlock()
}

// Drain runs the work queued, also what it queues, until cond holds or
// the wait runs out; it reports whether cond held. A nil cond waits for
// the queue to stay empty a moment.
func (l *Loop) Drain(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	quiet := 0
	for time.Now().Before(deadline) {
		l.mu.Lock()
		work := l.work
		l.work = nil
		l.mu.Unlock()
		for _, fn := range work {
			fn()
		}
		if cond != nil && cond() {
			return true
		}
		if len(work) == 0 {
			quiet++
			if cond == nil && quiet > 20 {
				return true
			}
			time.Sleep(time.Millisecond)
		} else {
			quiet = 0
		}
	}
	return cond == nil
}

// Text makes a user or assistant message of one text part.
func Text(turn int, role chat.Role, text string) chat.Message {
	return chat.Message{Turn: turn, Role: role, CreatedAt: time.Date(2026, 11, 16, 10, turn%60, 0, 0, time.UTC),
		Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: text}}}}
}
