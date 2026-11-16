package state

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// SystemAPI is what Stores sends to the system; *app.SystemService has
// it.
type SystemAPI interface {
	Notify(ctx context.Context, n app.Notification) error
}

// ChatAPI is the chat service as the stores use it.
type ChatAPI interface {
	ChatsAPI
	ThreadAPI
}

// API is the services the stores read; app.Bound's services have it.
type API struct {
	Projects ProjectsAPI
	Chats    ChatAPI
	Settings SettingsAPI
	System   SystemAPI
	// Actions is what the chat screen asks of the chat service; nil when
	// Chats has it too, as *app.ChatService does.
	Actions ActionsAPI
	// Hosts and Exports are what project settings and the exports ask;
	// nil when Projects has them, and ChatExport when Chats has it, as
	// the services do.
	Hosts      HostsAPI
	Exports    ExportsAPI
	ChatExport ChatExportAPI
}

// Toast is a message for the window's toasts: a project notice, or an
// error no screen shows.
type Toast struct {
	Text    string
	Warning bool
	Error   bool
	Sticky  bool // until closed
}

// reloadWait is how long a chat list waits before it is read again, so a
// burst of statuses reads it once.
var reloadWait = 300 * time.Millisecond

// Stores is every store, and what applies the Go events to them
// (state/events.ts) and opens projects and chats (shell/open.ts).
type Stores struct {
	Nav      *Nav
	Projects *Projects
	Chats    *Chats
	Threads  *Threads
	Settings *Settings
	Shell    *Shell
	Quotes   Quotes
	Actions  *Actions
	Hosts    *Hosts
	Exports  *Exports
	// Toasts wait for the window to show them; the kit takes them with
	// TakeToasts as it builds.
	Toasts []Toast
	// WindowActive tells whether the window is shown and has the focus. A
	// chat that starts to wait gets a notification unless it is shown in
	// an active window.
	WindowActive func() bool

	post    Post
	system  SystemAPI
	states  map[id.Chat]chat.State // each busy chat's last state, to see when it starts to wait
	reloads chan id.Project
	stop    chan struct{}
	opening int // counts the OpenProject calls, so only the latest one moves
}

// NewStores makes the stores. post is how work gets back to the UI
// thread. Close stops the goroutine that reads chat lists again.
func NewStores(api API, post Post) *Stores {
	s := &Stores{
		Nav: NewNav(), Shell: NewShell(), Quotes: Quotes{},
		WindowActive: func() bool { return false },
		post:         post, system: api.System,
		states:  map[id.Chat]chat.State{},
		reloads: make(chan id.Project, 64),
		stop:    make(chan struct{}),
	}
	s.Projects = NewProjects(api.Projects, post)
	s.Threads = NewThreads(api.Chats, post, s.Error)
	s.Chats = NewChats(api.Chats, post, s.Threads)
	s.Settings = NewSettings(api.Settings, post, s.Error)
	actions := api.Actions
	if actions == nil {
		actions, _ = api.Chats.(ActionsAPI)
	}
	s.Actions = NewActions(actions, post, s.Error)
	hosts, exports, chatExport := api.Hosts, api.Exports, api.ChatExport
	if hosts == nil {
		hosts, _ = api.Projects.(HostsAPI)
	}
	if exports == nil {
		exports, _ = api.Projects.(ExportsAPI)
	}
	if chatExport == nil {
		chatExport, _ = api.Chats.(ChatExportAPI)
	}
	s.Hosts = NewHosts(hosts, post, s.Error)
	s.Exports = NewExports(exports, chatExport, post, s.Error)
	go s.runReloads()
	return s
}

// Close stops the stores' goroutine.
func (s *Stores) Close() { close(s.stop) }

// Error shows err in a toast.
func (s *Stores) Error(err error) {
	if err != nil {
		s.Toasts = append(s.Toasts, Toast{Text: errorText(err), Error: true})
	}
}

// TakeToasts returns the toasts waiting and clears them.
func (s *Stores) TakeToasts() []Toast {
	t := s.Toasts
	s.Toasts = nil
	return t
}

// report shows the error done gets, if any.
func (s *Stores) report(done <-chan error) {
	go func() {
		if err := <-done; err != nil {
			s.post(func() { s.Error(err) })
		}
	}()
}

// Apply applies a Go event to the stores, on the UI thread. The native
// Publisher posts every event here.
func (s *Stores) Apply(name string, data any) {
	switch name {
	case app.EventStatus:
		if st, ok := data.(chat.Status); ok {
			s.onStatus(st)
		}
	case app.EventDelta:
		if d, ok := data.(chat.Delta); ok {
			s.Threads.Delta(d)
		}
	case app.EventPart:
		if e, ok := data.(chat.PartDone); ok {
			s.Threads.Part(e)
		}
	case app.EventOpen:
		if o, ok := data.(app.Open); ok {
			s.OpenProject(o.Project, o.Chat)
		}
	case app.EventExport:
		if pr, ok := data.(project.ExportProgress); ok {
			s.Exports.Event(pr)
		}
	case app.EventNotice:
		if n, ok := data.(project.Notice); ok {
			s.Toasts = append(s.Toasts, Toast{Text: n.Text, Warning: n.Kind == project.NoticeDamaged, Sticky: n.Kind == project.NoticeDamaged})
		}
	}
}

func (s *Stores) onStatus(st chat.Status) {
	items, listed := s.Chats.ByProject[st.Project]
	before := s.states[st.Chat]
	if st.State == chat.StateIdle {
		delete(s.states, st.Chat)
	} else {
		s.states[st.Chat] = st.State
	}
	s.Chats.Status(st)
	s.Threads.Status(st)
	// A chat:status event carries no title, so a new chat or a generated
	// title shows after the list is read again.
	if listed && (st.State == chat.StateIdle || !slices.ContainsFunc(items, func(it app.ChatItem) bool { return it.Chat.ID == st.Chat })) {
		select {
		case s.reloads <- st.Project:
		default: // a burst: the reads already due cover it
		}
	}
	if st.State == chat.StateWaiting && before != chat.StateWaiting && st.Waiting != nil && !s.onScreen(st) {
		s.notify(st)
	}
}

// runReloads reads the chat lists asked for, each once reloadWait after
// the first ask.
func (s *Stores) runReloads() {
	due := map[id.Project]time.Time{}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case p := <-s.reloads:
			if _, ok := due[p]; !ok {
				due[p] = time.Now().Add(reloadWait)
			}
		case <-timer.C:
			now := time.Now()
			for p, at := range due {
				if !at.After(now) {
					delete(due, p)
					s.post(func() { s.report(s.Chats.Load(p)) })
				}
			}
		case <-s.stop:
			timer.Stop()
			return
		}
		var next time.Time
		for _, at := range due {
			if next.IsZero() || at.Before(next) {
				next = at
			}
		}
		if !next.IsZero() {
			timer.Reset(time.Until(next))
		}
	}
}

// onScreen tells whether the user sees the chat now.
func (s *Stores) onScreen(st chat.Status) bool {
	p := s.Nav.Current()
	return p != nil && p.View == ViewChat && p.Project == st.Project && p.Chat == st.Chat && s.WindowActive()
}

// notify shows a desktop notification for a chat that waits (SPEC 8.8).
func (s *Stores) notify(st chat.Status) {
	title := ""
	if i := slices.IndexFunc(s.Chats.ByProject[st.Project], func(it app.ChatItem) bool { return it.Chat.ID == st.Chat }); i >= 0 {
		title = s.Chats.ByProject[st.Project][i].Chat.Title
	}
	if t := s.Threads.Threads[st.Chat]; title == "" && t != nil {
		title = t.Chat.Title
	}
	if title == "" {
		title = "A chat"
	}
	name := s.Projects.Opened[st.Project].Name
	if i := slices.IndexFunc(s.Projects.List, func(p app.ProjectItem) bool { return p.ID == st.Project }); name == "" && i >= 0 {
		name = s.Projects.List[i].Name
	}
	what := "approval"
	if st.Waiting.Kind == chat.PartQuestion {
		what = "answer"
	}
	body := st.Waiting.Text
	if name != "" {
		body = name + " · " + body
	}
	n := app.Notification{Project: st.Project, Chat: st.Chat, Title: fmt.Sprintf("%s is waiting for your %s", title, what), Body: body}
	go func() {
		if err := s.system.Notify(background, n); err != nil {
			s.post(func() { s.Error(err) })
		}
	}()
}

// OpenProject opens a project at chat, or else at the chat used last in
// it, or its Mother chat. It stays put when another project was asked for
// or the place changed while the project was opening.
func (s *Stores) OpenProject(p id.Project, c id.Chat) {
	s.opening++
	seq := s.opening
	from := s.Nav.Current()
	s.Projects.Open(p, func(op app.OpenedProject, err error) {
		if err != nil {
			s.Error(err)
			return
		}
		if now := s.Nav.Current(); seq != s.opening || (from != now && !SamePlace(from, now)) {
			return
		}
		if c == "" {
			c = op.Mother
			if i := slices.IndexFunc(s.Nav.MRU, func(m ChatRef) bool { return m.Project == p }); i >= 0 {
				c = s.Nav.MRU[i].Chat
			}
		}
		s.OpenChat(p, c)
		if _, ok := s.Chats.ByProject[p]; !ok {
			s.report(s.Chats.Load(p))
		}
	})
}

// OpenChat shows a chat.
func (s *Stores) OpenChat(p id.Project, c id.Chat) {
	s.Nav.Go(ChatPlace(p, c))
	s.Shell.LeftOverlay = false
	s.report(s.Threads.Open(p, c))
}

// NewChat makes a chat in the project and shows it.
func (s *Stores) NewChat(p id.Project) {
	s.Chats.Create(p, func(c id.Chat, err error) {
		if err != nil {
			s.Error(err)
			return
		}
		s.OpenChat(p, c)
	})
}

// OpenMemory opens the settings with a memory section to edit, target
// memory:<scope>/<section>: user memory in Settings, project memory in the
// project's settings (SPEC 3.3).
func (s *Stores) OpenMemory(p id.Project, target string) {
	s.Shell.MemoryFocus = &MemoryFocus{Project: p, Target: target}
	if strings.HasPrefix(target, "memory:user/") {
		s.Nav.Go(Place{View: ViewSettings, Section: "Memory"})
	} else {
		s.Nav.Go(Place{View: ViewProjectSettings, Project: p, Section: "Memory"})
	}
}

// BackToChats leaves Settings for the chat used last, or the first-run
// screen when there is none.
func (s *Stores) BackToChats() {
	if len(s.Nav.MRU) > 0 {
		m := s.Nav.MRU[0]
		s.OpenChat(m.Project, m.Chat)
		return
	}
	s.Nav.History = NoHistory()
}

// The services have what the stores read.
var (
	_ ProjectsAPI = (*app.ProjectService)(nil)
	_ ChatAPI     = (*app.ChatService)(nil)
	_ SettingsAPI = (*app.SettingsService)(nil)
	_ SystemAPI   = (*app.SystemService)(nil)

	_ HostsAPI      = (*app.ProjectService)(nil)
	_ ExportsAPI    = (*app.ProjectService)(nil)
	_ ChatExportAPI = (*app.ChatService)(nil)
)
