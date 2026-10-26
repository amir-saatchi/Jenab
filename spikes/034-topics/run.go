package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Conditions (SPIKE-034).
const (
	condBaseline = 1 // one chat, today's loop
	condUser     = 2 // topics the scripted user opens
	condPropose  = 3 // topics the agent proposes
	condSubjects = 4 // one chat with SPIKE-029's proposed subjects
)

var condNames = map[int]string{condBaseline: "1 baseline", condUser: "2 topics, user opens", condPropose: "3 topics, agent proposes", condSubjects: "4 subjects"}

func topics(cond int) bool { return cond == condUser || cond == condPropose }

// job is one run: a scenario on a model under a condition.
type job struct {
	Model    string `json:"model"`
	Scenario string `json:"scenario"`
	Cond     int    `json:"cond"`
	Rep      int    `json:"rep"`
}

func (j job) name() string {
	return fmt.Sprintf("%s_%s_c%d_r%d", strings.NewReplacer("/", "-", ":", "-").Replace(j.Model), j.Scenario, j.Cond, j.Rep)
}

// call is one tool call.
type call struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Error  bool   `json:"error,omitempty"`
	Result string `json:"result,omitempty"`
	Thread string `json:"thread"`
	Alone  bool   `json:"alone,omitempty"` // a notes or subject write was the only call of its response
	Cross  bool   `json:"cross,omitempty"` // a history read that reaches another thread
}

// send is one time the message was sent to a thread: once, or twice
// when a proposal moved it.
type send struct {
	Thread    string `json:"thread"`
	Resend    bool   `json:"resend,omitempty"` // sent again in the topic a proposal moved it to
	Moved     string `json:"moved,omitempty"`  // the topic an accepted proposal moved it to
	Turns     []int  `json:"turns"`
	Requests  int    `json:"requests"`
	Prompt    int    `json:"prompt"`
	Output    int    `json:"output"`
	CacheRead int    `json:"cache_read"`
	// System is the estimated tokens of the tools and system blocks of the
	// send's first request; Shared is how many of them, from the start,
	// are the same as in the main thread's latest request (a topic's first
	// send only): an upper bound on cache reuse.
	System int `json:"system,omitempty"`
	Shared int `json:"shared,omitempty"`
}

// msgResult is what one scripted message did.
type msgResult struct {
	Index    int      `json:"index"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Sends    []send   `json:"sends"`
	Calls    []call   `json:"calls"`
	Answer   string   `json:"answer"`
	Thread   string   `json:"thread"`             // where it was answered
	Had      string   `json:"had,omitempty"`      // the topic its label had when it was sent
	Parallel bool     `json:"parallel,omitempty"` // sent at the same time as its partner
	Open     int      `json:"open"`               // open items of its thread, or of its label's subjects, after it
	Joined   *bool    `json:"joined,omitempty"`   // after a merge message: one topic or subject holds the labels
	Memory   string   `json:"memory,omitempty"`   // project memory after it, for a message that checks memory
	Errors   []string `json:"errors,omitempty"`
	Seconds  float64  `json:"seconds"`
}

func (mr *msgResult) cost() (req, prompt, output, cache int) {
	for _, s := range mr.Sends {
		req += s.Requests
		prompt += s.Prompt
		output += s.Output
		cache += s.CacheRead
	}
	return
}

// runResult is one run, as written to runs.jsonl.
type runResult struct {
	job
	Started   time.Time      `json:"started"`
	Seconds   float64        `json:"seconds"`
	Error     string         `json:"error,omitempty"`
	Messages  []msgResult    `json:"messages"`
	Threads   []thread       `json:"threads,omitempty"`
	Proposals []proposal     `json:"proposals,omitempty"`
	Subjects  []subject      `json:"subjects,omitempty"`
	Events    []subjectEvent `json:"events,omitempty"`
	Memory    string         `json:"memory"`
	Card      string         `json:"card"`
}

// harness holds what all runs share: the provider registry, so the
// per-provider call limit holds across parallel runs.
type harness struct {
	models  *provider.Registry
	llm     config.LLMSettings
	keep    bool
	timeout time.Duration
	log     *slog.Logger
	redact  func(string) string
}

func (h *harness) run(ctx context.Context, j job, sc scenarioDef) (res runResult) {
	res = runResult{job: j, Started: time.Now().UTC()}
	defer func() { res.Seconds = time.Since(res.Started).Seconds() }()
	r := &run{h: h, j: j, sc: sc, w: &world{}, seen: map[id.Chat]int{}, sys: map[id.Chat]string{}}
	if j.Cond == condSubjects {
		r.sb = &subjects{}
	}
	if err := r.play(ctx, &res); err != nil {
		res.Error = h.redact(err.Error())
	}
	if r.th != nil {
		res.Threads, res.Proposals = r.th.snapshot()
	}
	if r.sb != nil {
		res.Subjects, res.Events = r.sb.snapshot()
	}
	res.Card = r.w.card()
	return res
}

type run struct {
	h    *harness
	j    job
	sc   scenarioDef
	w    *world
	th   *threads  // conditions 2–3
	sb   *subjects // condition 4
	pm   *project.Manager
	o    *agent.Orchestrator
	tr   *agent.Traces
	pid  id.Project
	main id.Chat
	at   id.Chat // where the scripted user is (condition 3)

	seen map[id.Chat]int    // the last turn read, per chat
	sys  map[id.Chat]string // the tools and system blocks of the chat's latest request
}

// block5 is the Notes hook: block 5, chosen per chat.
func (r *run) block5(_ context.Context, _ *project.Project, ch chat.Chat) (string, error) {
	switch {
	case r.th != nil:
		return r.th.block(ch.ID), nil
	case r.sb != nil:
		return r.sb.block(), nil
	}
	return "", nil
}

func (r *run) tools() *tool.Registry {
	reg := tool.NewRegistry()
	for _, t := range tool.Builtin(tool.Deps{}) {
		if n := t.Spec().Name; n == "search_history" || n == "read_messages" || n == "update_memory" {
			reg.Add(t)
		}
	}
	reg.Add(r.w.tools()...)
	switch {
	case topics(r.j.Cond):
		reg.Add(r.th.tools()...)
	case r.sb != nil:
		reg.Add(r.sb.tools()...)
	}
	return reg
}

func (r *run) settings() config.Settings {
	s := config.Defaults()
	s.LLM = r.h.llm
	s.Context.HistoryMinTurns = 2
	s.Context.HistoryMaxTurns = 4
	return s
}

func (r *run) play(ctx context.Context, res *runResult) error {
	ctx, cancel := context.WithTimeout(ctx, r.h.timeout)
	defer cancel()
	root, err := os.MkdirTemp("", "spike034-")
	if err != nil {
		return err
	}
	if !r.h.keep {
		defer os.RemoveAll(root)
	}
	paths := config.Paths{Root: root, Registry: filepath.Join(root, "registry.db")}.WithDataFolder(filepath.Join(root, "data"))
	reg, err := store.OpenRegistry(ctx, paths.Registry)
	if err != nil {
		return err
	}
	defer reg.Close()
	r.pm = project.NewManager(project.Deps{Paths: paths, Registry: reg, Log: r.h.log})
	defer r.pm.CloseAll(context.WithoutCancel(ctx))
	if r.pid, err = r.pm.Create(ctx, r.sc.Title); err != nil {
		return err
	}
	if r.main, err = r.newChat(ctx, r.sc.Title); err != nil {
		return err
	}
	r.at = r.main
	if topics(r.j.Cond) {
		r.th = newThreads(r.sc, r.j.Model, r.main, r.j.Cond == condPropose)
	}
	set := r.settings()
	r.tr = agent.NewTraces()
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.o = agent.New(agent.Deps{
		Context: appCtx, Projects: r.pm, Models: r.h.models, Tools: r.tools(), Events: nopPublisher{},
		Settings: func() config.Settings { return set }, Traces: r.tr, Log: r.h.log,
		Card:  func(context.Context, *project.Project) (string, error) { return r.w.card(), nil },
		Notes: r.block5,
	})
	defer func() {
		r.o.Refuse()
		stop()
		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = r.o.Wait(wctx)
	}()
	defer func() { res.Memory, _ = r.memory(context.WithoutCancel(ctx)) }()

	ms := r.sc.Messages
	for i := 0; i < len(ms); i++ {
		if i+1 < len(ms) && ms[i+1].Parallel {
			mrs, err := r.parallel(ctx, i)
			res.Messages = append(res.Messages, mrs...)
			if err != nil {
				return fmt.Errorf("messages %d and %d: %w", i+1, i+2, err)
			}
			i++
			continue
		}
		mr, err := r.message(ctx, i)
		res.Messages = append(res.Messages, mr)
		if err != nil {
			return fmt.Errorf("message %d: %w", i+1, err)
		}
	}
	return nil
}

// newChat makes a chat in the run's project with the scenario's role.
func (r *run) newChat(ctx context.Context, title string) (id.Chat, error) {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return "", err
	}
	defer p.Release()
	ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: title, Role: r.sc.Role, Model: r.j.Model})
	return ch.ID, err
}

// userChat is where the scripted user writes message i in condition 2:
// the main thread for main messages, else the label's topic, which they
// open at its first message.
func (r *run) userChat(ctx context.Context, i int) (id.Chat, error) {
	m := r.sc.Messages[i]
	if m.Label == mainLabel {
		return r.main, nil
	}
	if t := r.th.TopicFor(m.Label); t != nil {
		return t.Chat, nil
	}
	title := r.sc.Titles[m.Label]
	ch, err := r.newChat(ctx, topicChatTitle(r.th.NextID(), title))
	if err != nil {
		return "", err
	}
	return r.th.Add(ch, title, m.Label, i).Chat, nil
}

// message plays one message as the condition's scripted user does.
func (r *run) message(ctx context.Context, i int) (msgResult, error) {
	m := r.sc.Messages[i]
	start := time.Now()
	mr := msgResult{Index: i, Label: m.Label, Kind: m.Kind, Had: r.had(m.Label)}
	defer func() { mr.Seconds = time.Since(start).Seconds() }()
	ch := r.main
	var err error
	switch r.j.Cond {
	case condUser:
		if ch, err = r.userChat(ctx, i); err != nil {
			return mr, err
		}
	case condPropose:
		if m.Label == mainLabel {
			r.at = r.main
		}
		ch = r.at
	}
	if err := r.turns(ctx, &mr, i, ch); err != nil {
		return mr, err
	}
	if r.j.Cond == condPropose {
		if t := r.th.takeMove(ch); t != nil {
			mr.Sends[len(mr.Sends)-1].Moved = t.ID
			r.at, ch = t.Chat, t.Chat
			if err := r.turns(ctx, &mr, i, ch); err != nil {
				return mr, err
			}
			mr.Sends[len(mr.Sends)-1].Resend = true
		}
	}
	return mr, r.after(ctx, &mr, ch)
}

// turns sends message i to one chat and reads what its turn did.
func (r *run) turns(ctx context.Context, mr *msgResult, i int, ch id.Chat) error {
	r.mark(ch, i)
	if _, err := r.o.Send(ctx, r.pid, ch, agent.UserMessage{Text: r.sc.Messages[i].Text}); err != nil {
		return err
	}
	if err := r.idle(ctx, ch); err != nil {
		return err
	}
	return r.read(ctx, ch, mr)
}

// had is the topic a label has now, in conditions 2–3.
func (r *run) had(label string) string {
	if r.th == nil || label == mainLabel {
		return ""
	}
	if t := r.th.TopicFor(label); t != nil {
		return t.ID
	}
	return ""
}

func (r *run) mark(ch id.Chat, i int) {
	if r.th != nil {
		r.th.at(ch, i)
	}
	if r.sb != nil {
		r.sb.at(i, r.sc.Messages[i].Label)
	}
}

// parallel plays the parallel step, messages i and i+1: at the same time
// in their topics when both labels have one, else one after the other as
// any message.
func (r *run) parallel(ctx context.Context, i int) ([]msgResult, error) {
	a, b := r.sc.Messages[i], r.sc.Messages[i+1]
	var ca, cb id.Chat
	switch r.j.Cond {
	case condUser:
		var err error
		if ca, err = r.userChat(ctx, i); err != nil {
			return nil, err
		}
		if cb, err = r.userChat(ctx, i+1); err != nil {
			return nil, err
		}
	case condPropose:
		ta, tb := r.th.TopicFor(a.Label), r.th.TopicFor(b.Label)
		if ta != nil && tb != nil {
			ca, cb = ta.Chat, tb.Chat
		}
	}
	if ca == "" || cb == "" || ca == cb {
		mra, err := r.message(ctx, i)
		if err != nil {
			return []msgResult{mra}, err
		}
		mrb, err := r.message(ctx, i+1)
		return []msgResult{mra, mrb}, err
	}
	start := time.Now()
	mra := msgResult{Index: i, Label: a.Label, Kind: a.Kind, Parallel: true, Had: r.had(a.Label)}
	mrb := msgResult{Index: i + 1, Label: b.Label, Kind: b.Kind, Parallel: true, Had: r.had(b.Label)}
	r.mark(ca, i)
	r.mark(cb, i+1)
	if _, err := r.o.Send(ctx, r.pid, ca, agent.UserMessage{Text: a.Text}); err != nil {
		return nil, err
	}
	if _, err := r.o.Send(ctx, r.pid, cb, agent.UserMessage{Text: b.Text}); err != nil {
		return nil, err
	}
	if err := r.idle(ctx, ca, cb); err != nil {
		return nil, err
	}
	if err := r.read(ctx, ca, &mra); err != nil {
		return nil, err
	}
	if err := r.read(ctx, cb, &mrb); err != nil {
		return nil, err
	}
	r.at = cb
	mra.Seconds = time.Since(start).Seconds()
	mrb.Seconds = mra.Seconds
	if err := r.after(ctx, &mra, ca); err != nil {
		return nil, err
	}
	return []msgResult{mra, mrb}, r.after(ctx, &mrb, cb)
}

// after records the state a message's checks look at: where it was
// answered, the open items, the merge and project memory.
func (r *run) after(ctx context.Context, mr *msgResult, ch id.Chat) error {
	m := r.sc.Messages[mr.Index]
	mr.Thread = mainThread
	switch {
	case r.th != nil:
		if t := r.th.Thread(ch); t != nil {
			mr.Thread = t.ID
		}
		mr.Open = r.th.openOf(ch)
		if len(m.Merge) > 0 {
			ta, tb := r.th.TopicFor(m.Merge[0]), r.th.TopicFor(m.Merge[1])
			ok := ta != nil && ta == tb
			mr.Joined = &ok
		}
	case r.sb != nil:
		mr.Open = r.sb.openFor(m.Label)
		if len(m.Merge) > 0 {
			ok := r.sb.joined(m.Merge)
			mr.Joined = &ok
		}
	}
	if len(m.Memory) > 0 {
		mem, err := r.memory(ctx)
		if err != nil {
			return err
		}
		mr.Memory = mem
	}
	return nil
}

// memory is project memory as text.
func (r *run) memory(ctx context.Context) (string, error) {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return "", err
	}
	defer p.Release()
	ss, err := p.DB.Memory(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range ss {
		if s.Content != "" {
			fmt.Fprintf(&b, "[%s, revision %d]\n%s\n", s.Section, s.Revision, s.Content)
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// idle waits until the chats' turns have ended. A card or form is
// answered as a user without an opinion would.
func (r *run) idle(ctx context.Context, chats ...id.Chat) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		busy := false
		for _, ch := range chats {
			l := r.o.Live(r.pid, ch)
			if !l.Running {
				continue
			}
			busy = true
			if w := l.Waiting; w != nil {
				a := agent.Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce}
				if w.Kind != chat.PartApproval {
					a = agent.Answer{Message: w.Message, Index: w.Index, Note: "no opinion"}
				}
				if err := r.o.Answer(r.pid, ch, a); err != nil && !errors.Is(err, agent.ErrNotWaiting) {
					return err
				}
			}
		}
		if !busy {
			return nil
		}
		select {
		case <-ctx.Done():
			for _, ch := range chats {
				r.o.Stop(r.pid, ch)
			}
			return fmt.Errorf("the run took longer than %s", r.h.timeout)
		case <-t.C:
		}
	}
}

// read adds the chat's turns since the last read to mr as one send.
func (r *run) read(ctx context.Context, ch id.Chat, mr *msgResult) error {
	thread := mainThread
	if r.th != nil {
		if t := r.th.Thread(ch); t != nil {
			thread = t.ID
		}
	}
	first := r.seen[ch] == 0
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, ch, r.seen[ch]+1, 0)
	if err != nil {
		return err
	}
	results := map[string]*chat.ToolResult{}
	for _, m := range ms {
		for _, pt := range m.Parts {
			if pt.ToolResult != nil {
				results[pt.ToolResult.CallID] = pt.ToolResult
			}
		}
	}
	s := send{Thread: thread}
	last := r.seen[ch]
	var final, anyText string
	for _, m := range ms {
		if m.Turn <= r.seen[ch] {
			continue
		}
		last = max(last, m.Turn)
		if !slices.Contains(s.Turns, m.Turn) {
			s.Turns = append(s.Turns, m.Turn)
		}
		if m.Role != chat.RoleAssistant {
			continue
		}
		var calls []call
		var text strings.Builder
		for _, pt := range m.Parts {
			switch {
			case pt.ToolCall != nil:
				c := call{Tool: pt.ToolCall.Name, Args: string(pt.ToolCall.Args), Thread: thread}
				if res := results[pt.ToolCall.ID]; res != nil {
					c.Error, c.Result = res.IsError, clip(res.Text, 400)
				}
				c.Cross = crossRead(c, ch)
				calls = append(calls, c)
			case pt.Text != nil:
				text.WriteString(pt.Text.Text)
			}
		}
		if len(calls) == 1 && notesWrite(calls[0].Tool) {
			calls[0].Alone = true
		}
		mr.Calls = append(mr.Calls, calls...)
		if t := strings.TrimSpace(text.String()); t != "" {
			anyText = t
			if len(calls) == 0 {
				final = t
			}
		}
	}
	if final == "" {
		final = anyText
	}
	mr.Answer = final
	firstReq := true
	for _, t := range r.tr.List() {
		if t.Chat != ch || t.Turn <= r.seen[ch] || t.Turn > last {
			continue
		}
		// List leaves out the requests' texts; Turn has them.
		if full, ok := r.tr.Turn(agent.TraceKey{Project: t.Project, Chat: t.Chat, Turn: t.Turn}); ok {
			t = full
		}
		for _, q := range t.Requests {
			s.Requests++
			s.Prompt += q.Usage.Input + q.Usage.CacheRead + q.Usage.CacheWrite
			s.CacheRead += q.Usage.CacheRead
			s.Output += q.Usage.Output
			if q.Err != "" {
				mr.Errors = append(mr.Errors, r.h.redact(clip(q.Err, 200)))
			}
			if q.Dropped {
				continue
			}
			sys := systemText(q.Req)
			if firstReq {
				firstReq = false
				s.System = len(sys) / 4
				if first && ch != r.main && r.sys[r.main] != "" {
					s.Shared = commonPrefix(sys, r.sys[r.main]) / 4
				}
			}
			r.sys[ch] = sys
		}
	}
	mr.Sends = append(mr.Sends, s)
	r.seen[ch] = last
	return nil
}

// systemText is a request's tools and system blocks, in the order
// providers cache them.
func systemText(q provider.Request) string {
	var b strings.Builder
	for _, t := range q.Tools {
		b.WriteString(t.Name + "\n" + t.Description + "\n" + string(t.Schema) + "\n")
	}
	for _, s := range q.System {
		b.WriteString(s.Text + "\n")
	}
	return b.String()
}

func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// crossRead says whether a history read reaches another thread: a
// project-wide search, or read_messages of another chat.
func crossRead(c call, own id.Chat) bool {
	switch c.Tool {
	case "search_history":
		return strings.Contains(c.Args, `"project"`)
	case "read_messages":
		var a struct {
			Chat string `json:"chat_id"`
		}
		_ = json.Unmarshal([]byte(c.Args), &a)
		return a.Chat != "" && id.Chat(a.Chat) != own
	}
	return false
}

func notesWrite(name string) bool {
	switch name {
	case "update_notes", "merge_topics", "update_subject", "merge_subjects":
		return true
	}
	return false
}

// nopPublisher drops the chat events; the harness reads the store.
type nopPublisher struct{}

func (nopPublisher) Delta(chat.Delta)   {}
func (nopPublisher) Part(chat.PartDone) {}
func (nopPublisher) Status(chat.Status) {}

// clip shortens s to n bytes at a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}
