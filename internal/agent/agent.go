// Package agent is the orchestrator (CODE-OUTLINE 7, SPEC 3.1, 3.6, 8.3):
// one runner per chat while it has work, turns made of steps, the context
// of each request and the history window.
//
// A turn is one user message plus everything the agent does until it
// replies. Each step sends one request and runs the tool calls of the
// answer in order. The answer is kept in memory while it streams and
// written once it is complete, so a retried request leaves nothing behind.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Publisher sends chat events to the frontend: chat:delta, chat:part and
// chat:status (Q32). It must not block.
type Publisher interface {
	Delta(chat.Delta)
	Part(chat.PartDone)
	Status(chat.Status)
}

// Skills gives a chat's loaded skills and the list of available skills for
// block 1 of the context (3.1, 8.9). P1-12 implements it.
type Skills interface {
	Block(ctx context.Context, p *project.Project, c chat.Chat) (string, error)
}

// Deps are what the Orchestrator needs.
type Deps struct {
	// Context is the app context (Q20); turns stop when it is cancelled.
	// Nil means context.Background().
	Context  context.Context
	Projects *project.Manager
	Models   *provider.Registry
	Tools    *tool.Registry
	// Settings returns the settings now, so changes apply without a
	// restart (7.6).
	Settings func() config.Settings
	Events   Publisher
	Skills   Skills // nil adds none
	Log      *slog.Logger
}

// UserMessage is what the user sends. Images are already in the bucket.
type UserMessage struct {
	Text   string
	Images []chat.Image
}

var (
	ErrRefused = errors.New("agent: the app is shutting down")
	ErrEmpty   = errors.New("agent: the message is empty")
	// ErrNoRetry: Retry was called, but the chat's last turn did not stop
	// on a provider error and no request is waiting to be retried.
	ErrNoRetry = errors.New("agent: there is nothing to retry")
	ErrInTurn  = errors.New("agent: the chat is in a turn")
)

// Orchestrator runs the turns of every open chat.
type Orchestrator struct {
	d   Deps
	ctx context.Context
	pub Publisher // d.Events, safe to call under locks

	mu      sync.Mutex
	chats   map[key]*chatState
	refused bool
	busy    int           // calls in progress and runners
	idle    chan struct{} // closed when busy drops to 0
}

type key struct {
	p id.Project
	c id.Chat
}

// New returns an Orchestrator. It starts nothing.
func New(d Deps) *Orchestrator {
	if d.Context == nil {
		d.Context = context.Background()
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Settings == nil {
		d.Settings = config.Defaults
	}
	return &Orchestrator{d: d, ctx: d.Context, pub: safePublisher{d.Events, d.Log}, chats: map[key]*chatState{}}
}

// chatState is what the Orchestrator keeps per chat between turns. The
// history window lives here, so it outlasts the runner (3.6).
//
// A chat has at most one runner. It runs the queued turn, then the next
// one if a message queued another meanwhile, and ends when none is left.
type chatState struct {
	key key

	mu       sync.Mutex
	loaded   bool          // turn, seq and failed are read from the store
	turn     int           // the running or queued turn, or the last one
	seq      atomic.Uint64 // the chat's sequence number, as far as we know
	lastUser id.Message    // the newest message from Send
	runner   bool          // a runner is alive
	queued   bool          // turn waits for the runner
	running  bool
	stopping bool // Stop was called for the running turn
	gen      int  // counts runners, so a finished one leaves a newer one alone
	cancel   context.CancelCauseFunc
	retryNow func() // set while a failed request waits to be tried again
	retry    *chat.Retry
	resp     *response // the answer streaming now
	failed   bool      // the last turn stopped on an error; Retry continues it
	titling  bool      // a title request is running
	clears   int       // counts Clear and Delete, so a late title is dropped
	started  time.Time
	active   time.Time
	requests int

	// pub keeps a store write and its events together, so events go out
	// in seq order. Taken after mu, never before.
	pub sync.Mutex

	win window // used by the runner only
}

func (o *Orchestrator) state(p id.Project, c id.Chat) *chatState {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := key{p, c}
	cs := o.chats[k]
	if cs == nil {
		cs = &chatState{key: k}
		o.chats[k] = cs
	}
	return cs
}

func (o *Orchestrator) find(p id.Project, c id.Chat) *chatState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.chats[key{p, c}]
}

// enter counts a call in, unless the app is shutting down. leave counts
// it out.
func (o *Orchestrator) enter() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.refused {
		return ErrRefused
	}
	o.add()
	return nil
}

// add counts a call or runner in. o.mu is held.
func (o *Orchestrator) add() {
	if o.busy == 0 {
		o.idle = make(chan struct{})
	}
	o.busy++
}

func (o *Orchestrator) leave() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.busy--; o.busy == 0 {
		close(o.idle)
	}
}

// Send adds a user message to the chat. It joins the running turn at its
// next step, or queues a new turn (8.3). After Stop, a message starts a
// new turn. It returns once the message is stored.
func (o *Orchestrator) Send(ctx context.Context, p id.Project, c id.Chat, m UserMessage) (id.Message, error) {
	var parts []chat.Part
	if strings.TrimSpace(m.Text) != "" {
		parts = append(parts, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: m.Text}})
	}
	for _, img := range m.Images {
		parts = append(parts, chat.Part{Kind: chat.PartImage, Image: &img})
	}
	if len(parts) == 0 {
		return "", ErrEmpty
	}
	if err := o.enter(); err != nil {
		return "", err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return "", err
	}
	defer func() {
		if proj != nil {
			proj.Release()
		}
	}()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := o.load(ctx, cs, proj); err != nil {
		return "", err
	}
	join := cs.queued || cs.running && !cs.stopping
	turn := cs.turn
	if !join {
		turn++
	}
	msg, err := o.write(ctx, cs, proj, chat.Message{Chat: c, Turn: turn, Role: chat.RoleUser, Parts: parts})
	if err != nil {
		return "", err
	}
	cs.lastUser = msg.ID
	if !join {
		cs.turn, cs.queued, cs.failed = turn, true, false
		if !cs.runner {
			o.startRunner(cs, proj)
			proj = nil
		}
		o.publishStatus(cs)
	}
	return msg.ID, nil
}

// Retry tries a failed request again. While a turn waits to retry, it
// tries at once (*Retry now*); after a turn stopped on a provider error,
// it continues that turn (*Retry* on the error card, 8.3), after a restart
// too. A continued turn gets turn_max_requests again.
func (o *Orchestrator) Retry(ctx context.Context, p id.Project, c id.Chat) error {
	if err := o.enter(); err != nil {
		return err
	}
	defer o.leave()
	if cs := o.find(p, c); cs != nil {
		cs.mu.Lock()
		busy, err := cs.retryInTurn()
		cs.mu.Unlock()
		if busy {
			return err
		}
	}
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return err
	}
	defer func() {
		if proj != nil {
			proj.Release()
		}
	}()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if busy, err := cs.retryInTurn(); busy {
		return err
	}
	if err := o.load(ctx, cs, proj); err != nil {
		return err
	}
	if !cs.failed {
		return ErrNoRetry
	}
	cs.failed, cs.queued = false, true
	if !cs.runner {
		o.startRunner(cs, proj)
		proj = nil
	}
	o.publishStatus(cs)
	return nil
}

// retryInTurn is *Retry now* for a chat in a turn. It reports whether the
// chat is in one. cs.mu is held.
func (cs *chatState) retryInTurn() (bool, error) {
	if !cs.running && !cs.queued {
		return false, nil
	}
	if cs.retryNow == nil {
		return true, ErrNoRetry
	}
	cs.retryNow()
	return true, nil
}

// Clear removes the chat's messages, session notes and review results
// (8.6); its turns start again from 1. It fails with ErrInTurn while the
// chat is in a turn.
func (o *Orchestrator) Clear(ctx context.Context, p id.Project, c id.Chat) error {
	return o.empty(ctx, p, c, func(proj *project.Project, cs *chatState) error {
		seq, err := proj.Chats.Clear(ctx, c)
		if err != nil {
			return err
		}
		cs.bumpSeq(seq)
		return nil
	})
}

// Delete removes the chat with its messages, notes and review results.
// The Mother chat can't be deleted. It fails with ErrInTurn while the chat
// is in a turn.
func (o *Orchestrator) Delete(ctx context.Context, p id.Project, c id.Chat) error {
	err := o.empty(ctx, p, c, func(proj *project.Project, _ *chatState) error {
		return proj.Chats.DeleteChat(ctx, c)
	})
	if err != nil {
		return err
	}
	o.mu.Lock()
	delete(o.chats, key{p, c})
	o.mu.Unlock()
	return nil
}

// empty runs Clear or Delete between turns.
func (o *Orchestrator) empty(ctx context.Context, p id.Project, c id.Chat, fn func(*project.Project, *chatState) error) error {
	if err := o.enter(); err != nil {
		return err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return err
	}
	defer proj.Release()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.running || cs.queued {
		return ErrInTurn
	}
	cs.pub.Lock()
	err = fn(proj, cs)
	cs.pub.Unlock()
	if err != nil {
		return err
	}
	cs.clears++
	cs.loaded, cs.turn, cs.lastUser, cs.failed, cs.retry, cs.win = true, 0, "", false, nil, window{}
	o.publishStatus(cs)
	return nil
}

// Stop cancels the chat's running turn and a queued one (8.3). It returns
// at once; the turn closes its open tool calls and keeps the text so far.
func (o *Orchestrator) Stop(p id.Project, c id.Chat) {
	cs := o.find(p, c)
	if cs == nil {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.queued {
		cs.queued = false
		o.publishStatus(cs)
	}
	if cs.running && !cs.stopping {
		cs.stopping = true
		cs.cancel(errStopped)
	}
}

// Wrote tells the Orchestrator that the chat changed elsewhere (a title
// or a role, P1-13), so its events carry the new sequence number.
func (o *Orchestrator) Wrote(p id.Project, c id.Chat, seq uint64) {
	if cs := o.find(p, c); cs != nil {
		cs.bumpSeq(seq)
	}
}

// Live is a chat's turn as it is now, for a chat opened mid-turn: the
// stored messages come from the store, Live adds the answer still
// streaming (Status.Streaming).
type Live struct {
	Running   bool          `json:"running"`
	Turn      int           `json:"turn"`
	Seq       uint64        `json:"seq"`
	Retry     *chat.Retry   `json:"retry,omitempty"`
	Streaming id.Message    `json:"streaming,omitempty"`
	Parts     []chat.Part   `json:"parts,omitempty"` // its finished parts
	Kind      chat.PartKind `json:"kind,omitempty"`  // the part being streamed
	Text      string        `json:"text,omitempty"`  // its text so far
}

// Live returns the chat's turn as it is now.
func (o *Orchestrator) Live(p id.Project, c id.Chat) Live {
	cs := o.find(p, c)
	if cs == nil {
		return Live{}
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	l := Live{Running: cs.running || cs.queued, Turn: cs.turn, Seq: cs.seq.Load(), Retry: cs.retry}
	if r := cs.resp; r != nil {
		r.mu.Lock()
		l.Streaming, l.Parts, l.Kind, l.Text = r.id, slices.Clone(r.parts), r.kind, r.text.String()
		r.mu.Unlock()
	}
	return l
}

// ChatStatus is a chat's status for list_chats and Mother's chat list:
// "in a turn" or "" when idle. Chat IDs are unique across projects.
func (o *Orchestrator) ChatStatus(c id.Chat) string {
	o.mu.Lock()
	var cs *chatState
	for k, s := range o.chats {
		if k.c == c {
			cs = s
		}
	}
	o.mu.Unlock()
	if cs == nil {
		return ""
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.running || cs.queued {
		return "in a turn"
	}
	return ""
}

// Refuse makes Send, Retry, Clear and Delete fail from now on: shutdown
// step 1 (Q30). Running turns go on until the app context is cancelled.
func (o *Orchestrator) Refuse() {
	o.mu.Lock()
	o.refused = true
	o.mu.Unlock()
}

// Wait waits until every call and runner has ended, or ctx is done.
func (o *Orchestrator) Wait(ctx context.Context) error {
	o.mu.Lock()
	if o.busy == 0 {
		o.mu.Unlock()
		return nil
	}
	idle := o.idle
	o.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// load reads the chat's last turn and sequence number once, and whether
// the last turn stopped on an error. cs.mu is held.
func (o *Orchestrator) load(ctx context.Context, cs *chatState, p *project.Project) error {
	if cs.loaded {
		return nil
	}
	turn, err := p.Chats.LastTurn(ctx, cs.key.c)
	if err != nil {
		return err
	}
	seq, err := p.Chats.Seq(ctx, cs.key.c)
	if err != nil {
		return err
	}
	failed := false
	if turn > 0 {
		ms, _, err := p.Chats.Messages(ctx, cs.key.c, turn, turn)
		if err != nil {
			return err
		}
		if n := len(ms); n > 0 {
			for _, part := range ms[n-1].Parts {
				failed = part.Notice != nil && part.Notice.Kind == chat.NoticeTurnFailed
			}
		}
	}
	cs.turn, cs.failed, cs.loaded = turn, failed, true
	cs.bumpSeq(seq)
	return nil
}

// bumpSeq raises the known sequence number to s.
func (cs *chatState) bumpSeq(s uint64) {
	for {
		old := cs.seq.Load()
		if s <= old || cs.seq.CompareAndSwap(old, s) {
			return
		}
	}
}

// write appends a message and publishes its parts.
func (o *Orchestrator) write(ctx context.Context, cs *chatState, p *project.Project, m chat.Message) (chat.Message, error) {
	cs.pub.Lock()
	defer cs.pub.Unlock()
	m, seq, err := p.Chats.AppendMessage(ctx, m)
	if err != nil {
		return m, err
	}
	cs.bumpSeq(seq)
	for i, part := range m.Parts {
		o.pub.Part(chat.PartDone{Project: p.ID, Chat: m.Chat, Seq: seq, Message: m.ID, Index: i, Part: part})
	}
	return m, nil
}

// startRunner starts cs's runner. cs.mu is held; the runner owns proj's
// lease.
func (o *Orchestrator) startRunner(cs *chatState, proj *project.Project) {
	cs.runner = true
	cs.gen++
	o.mu.Lock()
	o.add()
	o.mu.Unlock()
	go o.run(cs, proj, cs.gen) // the chat's runner, owned by the Orchestrator (Q15, Q16)
}

// run is a runner: queued turns one after another, each followed by the
// title if the chat has none. The project's close and the app's end
// cancel it.
func (o *Orchestrator) run(cs *chatState, proj *project.Project, gen int) {
	defer o.leave()
	defer proj.Release()
	ctx, cancel := context.WithCancelCause(o.ctx)
	defer cancel(nil)
	done := make(chan struct{})
	defer proj.OnClose(func() {
		cancel(errClosed)
		// Give the turn time to close its tool calls before the
		// databases close (8.3).
		select {
		case <-done:
		case <-time.After(closeWait):
		}
	})()
	defer close(done)
	var titles sync.WaitGroup
	defer titles.Wait()
	defer proj.Report(cs)()
	defer func() {
		if r := recover(); r != nil {
			o.d.Log.Error("agent: panic in a turn", "project", proj.ID, "chat", cs.key.c, "panic", r)
		}
		cs.mu.Lock()
		if cs.gen == gen && cs.runner {
			cs.runner, cs.running, cs.queued, cs.cancel, cs.retryNow, cs.retry, cs.resp, cs.failed = false, false, false, nil, nil, nil, nil, true
			o.publishStatus(cs)
		}
		cs.mu.Unlock()
	}()
	r := &runner{o: o, cs: cs, p: proj}
	pprof.Do(ctx, pprof.Labels("project", string(proj.ID), "chat", string(cs.key.c)), func(ctx context.Context) {
		for {
			tctx, tcancel := context.WithCancelCause(ctx)
			cs.mu.Lock()
			if !cs.queued || ctx.Err() != nil {
				cs.runner = false
				if cs.queued { // the project or the app closed
					cs.queued = false
					o.publishStatus(cs)
				}
				cs.mu.Unlock()
				tcancel(nil)
				return
			}
			n := cs.turn
			cs.queued, cs.running, cs.stopping, cs.cancel = false, true, false, tcancel
			cs.started, cs.active, cs.requests, cs.retry = time.Now(), time.Now(), 0, nil
			cs.mu.Unlock() // the status is already "working"

			t, ok := r.turn(tctx, n)
			tcancel(nil)
			if ok && t.ch.Title == "" { // Mother always has one
				r.startTitle(ctx, &titles, t)
			}
		}
	})
}

// publishStatus sends the chat's status. cs.mu is held.
func (o *Orchestrator) publishStatus(cs *chatState) {
	st := chat.Status{Project: cs.key.p, Chat: cs.key.c, State: chat.StateIdle, Retry: cs.retry}
	if cs.running || cs.queued {
		st.State = chat.StateWorking
	}
	if cs.resp != nil {
		st.Streaming = cs.resp.id
	}
	cs.pub.Lock()
	defer cs.pub.Unlock()
	st.Seq = cs.seq.Load()
	o.pub.Status(st)
}

// Status is the chat's running turn for the project's activity (Q19a).
func (cs *chatState) Status() []project.Status {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.running {
		return nil
	}
	st := project.Status{ID: string(cs.key.c), Kind: "turn", State: "running", Started: cs.started, LastActivity: cs.active,
		Progress: fmt.Sprintf("turn %d, request %d", cs.turn, cs.requests)}
	if cs.retry != nil {
		st.State = "waiting"
		st.Err = fmt.Sprintf("%s is %s; the next try is at %s", cs.retry.Provider, kindText(provider.ErrorKind(cs.retry.Kind)), cs.retry.At.Local().Format(time.TimeOnly))
	}
	return []project.Status{st}
}

// safePublisher logs a panicking Publisher instead of crashing the turn
// that holds the chat's lock.
type safePublisher struct {
	p   Publisher
	log *slog.Logger
}

func (s safePublisher) Delta(d chat.Delta)    { defer s.recover("delta"); s.p.Delta(d) }
func (s safePublisher) Part(p chat.PartDone)  { defer s.recover("part"); s.p.Part(p) }
func (s safePublisher) Status(st chat.Status) { defer s.recover("status"); s.p.Status(st) }

func (s safePublisher) recover(event string) {
	if r := recover(); r != nil {
		s.log.Error("agent: the event publisher panicked", "event", event, "panic", r)
	}
}

var (
	errStopped = errors.New("stopped by the user")
	errClosed  = errors.New("the project closed")
)
