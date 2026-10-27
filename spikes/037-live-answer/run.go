package main

import (
	"context"
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

// Conditions.
const (
	condStacked = "stacked" // Part A: today's finish turns and prompt rule
	condLive    = "live"    // Part A: the live answer and the reversed rule item
	condWindow  = "window"  // Part B: today's window
	condAnswers = "answers" // Part B: answers only
	condTrace   = "trace"   // Part B: answers and the trace line
)

var condsOf = map[string][]string{"A": {condStacked, condLive}, "B": {condWindow, condAnswers, condTrace}}

// job is one run: a test on a model under a condition.
type job struct {
	Part  string `json:"part"`
	Test  string `json:"test"`
	Cond  string `json:"cond"`
	Model string `json:"model"`
	Rep   int    `json:"rep"`
}

func (j job) name() string {
	return fmt.Sprintf("%s_%s_%s_r%d", j.Test, j.Cond, strings.NewReplacer("/", "-", ":", "-").Replace(j.Model), j.Rep)
}

type callRec struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Error  bool   `json:"error,omitempty"`
	Result string `json:"result,omitempty"` // clipped
}

type reqRec struct {
	Prompt     int     `json:"prompt"` // input + cache read + cache write, as the provider reports them
	Output     int     `json:"output"`
	CacheRead  int     `json:"cache_read,omitempty"`
	Est        int     `json:"est"`         // the whole request, estimated at 4 bytes a token
	EstHistory int     `json:"est_history"` // its earlier turns, estimated
	EstTrace   int     `json:"est_trace,omitempty"`
	First      int     `json:"first"` // the first turn in the request: it moves at a cut
	Last       bool    `json:"last,omitempty"`
	Seconds    float64 `json:"seconds"`
	Err        string  `json:"err,omitempty"`
}

type turnRec struct {
	N        int       `json:"n"`
	Kind     string    `json:"kind"`            // user or finish
	Msg      int       `json:"msg"`             // the scripted message's index; -1 for a finish turn
	Input    string    `json:"input"`           // the user's text or the notices
	Tasks    []string  `json:"tasks,omitempty"` // a finish turn's task IDs
	Joined   []string  `json:"joined,omitempty"`
	Calls    []callRec `json:"calls,omitempty"`
	Texts    []string  `json:"texts,omitempty"` // every text part, in order
	Answer   string    `json:"answer"`          // the last text part
	Notices  []string  `json:"notices,omitempty"`
	Requests []reqRec  `json:"requests,omitempty"`
}

type taskRec struct {
	ID         string   `json:"id"`
	Args       string   `json:"args"`
	Background bool     `json:"background"`
	Rules      []string `json:"rules"`
	Turn       int      `json:"turn"`   // the turn that started it
	Notice     int      `json:"notice"` // the turn its notice started; 0 if none
	Failed     bool     `json:"failed,omitempty"`
}

type runResult struct {
	job
	Started time.Time `json:"started"`
	Seconds float64   `json:"seconds"`
	Error   string    `json:"error,omitempty"`
	Turns   []turnRec `json:"turns"`
	Tasks   []taskRec `json:"tasks"`
	Tables  []string  `json:"tables,omitempty"`
	Pages   []string  `json:"pages,omitempty"` // refs of the fake pages
	Capped  bool      `json:"capped,omitempty"`
}

type harness struct {
	models  *provider.Registry
	llm     config.LLMSettings
	temp    string
	keep    bool
	timeout time.Duration
	gap     time.Duration // before a finish notice
	log     *slog.Logger
	redact  func(string) string
}

type sentMsg struct {
	id    id.Message
	kind  string
	msg   int
	tasks []string
	text  string
}

type run struct {
	h   *harness
	j   job
	td  testDef
	pm  *project.Manager
	o   *agent.Orchestrator
	tr  *agent.Traces
	pid id.Project
	cid id.Chat
	ts  *tasks
	w   *world
	log []sentMsg
}

// maxNotices bounds a run's finish turns, for a model that keeps
// starting tasks.
const maxNotices = 8

func (h *harness) run(ctx context.Context, j job, td testDef) (res runResult) {
	res = runResult{job: j, Started: time.Now().UTC()}
	r := &run{h: h, j: j, td: td, w: &world{web: td.World}}
	r.ts = &tasks{rules: td.Tasks, polls: map[string]int{}, turnOf: func() int { return r.o.Live(r.pid, r.cid).Turn }}
	if td.World != nil {
		r.ts.web = func(text string) (string, bool) {
			res, ok := r.w.search(text)
			return "Web search results:\n" + res, ok
		}
	}
	err := r.play(ctx, &res)
	res.Seconds = time.Since(res.Started).Seconds()
	if err != nil {
		res.Error = h.redact(err.Error())
	}
	return res
}

func (r *run) settings() config.Settings {
	s := config.Defaults()
	s.LLM = r.h.llm
	if r.td.Window == "low" {
		s.Context.HistoryMinTurns = 2
		s.Context.HistoryMaxTurns = 4
	}
	return s
}

func (r *run) tools() *tool.Registry {
	reg := tool.NewRegistry()
	keep := []string{"search_history", "read_messages"}
	if r.td.Part == "B" {
		keep = append(keep, "read_ref", "search_ref")
	}
	for _, t := range tool.Builtin(tool.Deps{}) {
		if slices.Contains(keep, t.Spec().Name) {
			reg.Add(t)
		}
	}
	reg.Add(r.ts.tools()...)
	if r.td.Part == "B" {
		reg.Add(r.w.tools()...)
	}
	return reg
}

func (r *run) play(ctx context.Context, res *runResult) error {
	ctx, cancel := context.WithTimeout(ctx, r.h.timeout)
	defer cancel()
	root, err := os.MkdirTemp(r.h.temp, "spike037-")
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
	if r.pid, err = r.pm.Create(ctx, "Spike 037"); err != nil {
		return err
	}
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: r.td.ID, Model: r.j.Model})
	p.Release()
	if err != nil {
		return err
	}
	r.cid = ch.ID
	set := r.settings()
	r.tr = agent.NewTraces()
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	d := agent.Deps{
		Context: appCtx, Projects: r.pm, Models: r.h.models, Tools: r.tools(), Events: nopPublisher{},
		Settings: func() config.Settings { return set }, Traces: r.tr, Log: r.h.log,
		LiveAnswer: r.j.Cond == condLive,
	}
	switch r.j.Cond {
	case condAnswers:
		d.Earlier = earlierTurns{}.earlier
	case condTrace:
		d.Earlier = earlierTurns{trace: true, line: func(c chat.ToolCall, res *chat.ToolResult) (string, string) {
			text := ""
			if res != nil {
				text = res.Text
			}
			return traceLine(c.Name, c.Args, text)
		}}.earlier
	}
	if r.td.Part == "B" {
		d.Card = func(context.Context, *project.Project) (string, error) { return r.w.card(), nil }
	}
	r.o = agent.New(d)
	defer func() {
		r.o.Refuse()
		stop()
		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = r.o.Wait(wctx)
	}()
	err = r.script(ctx, res)
	if cerr := r.collect(context.WithoutCancel(ctx), res); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// script sends the messages and the finish notices. A Part A message with
// After waits for that many notice turns; the others go once no notice is
// due. A notice goes out when the chat is idle, the tasks of one order
// together (8.3).
func (r *run) script(ctx context.Context, res *runResult) error {
	msgs := r.td.Messages
	sent, delivered := 0, 0
	next := func() error {
		err := r.send(ctx, msgs[sent].Text, "user", sent, nil)
		sent++
		return err
	}
	if err := next(); err != nil {
		return err
	}
	for {
		if err := r.idle(ctx); err != nil {
			return err
		}
		if sent < len(msgs) && msgs[sent].After > 0 && delivered >= msgs[sent].After {
			if err := next(); err != nil {
				return err
			}
			continue
		}
		if g := r.ts.due(sent); len(g) > 0 {
			if delivered >= maxNotices {
				res.Capped = true
				r.ts.notified(g)
				continue
			}
			select {
			case <-time.After(r.h.gap):
			case <-ctx.Done():
				return ctx.Err()
			}
			var texts, ids []string
			for _, t := range g {
				texts, ids = append(texts, t.notice()), append(ids, t.ID)
			}
			r.ts.notified(g)
			delivered++
			if err := r.send(ctx, strings.Join(texts, "\n"), "finish", -1, ids); err != nil {
				return err
			}
			continue
		}
		if sent < len(msgs) {
			if err := next(); err != nil {
				return err
			}
			continue
		}
		return nil
	}
}

func (r *run) send(ctx context.Context, text, kind string, msg int, tasks []string) error {
	mid, err := r.o.Send(ctx, r.pid, r.cid, agent.UserMessage{Text: text})
	if err != nil {
		return fmt.Errorf("message %d: %w", msg+1, err)
	}
	r.log = append(r.log, sentMsg{id: mid, kind: kind, msg: msg, tasks: tasks, text: text})
	return nil
}

// idle waits until the chat's turn has ended. A card or form is answered
// as a user without an opinion would.
func (r *run) idle(ctx context.Context) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		l := r.o.Live(r.pid, r.cid)
		if !l.Running {
			return nil
		}
		if w := l.Waiting; w != nil {
			a := agent.Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce}
			if w.Kind != chat.PartApproval {
				a = agent.Answer{Message: w.Message, Index: w.Index, Note: "no opinion"}
			}
			if err := r.o.Answer(r.pid, r.cid, a); err != nil && !errors.Is(err, agent.ErrNotWaiting) {
				return err
			}
		}
		select {
		case <-ctx.Done():
			r.o.Stop(r.pid, r.cid)
			return fmt.Errorf("the run took longer than %s", r.h.timeout)
		case <-t.C:
		}
	}
}

// collect reads the chat and the traces into res.
func (r *run) collect(ctx context.Context, res *runResult) error {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, r.cid, 1, 0)
	if err != nil {
		return err
	}
	sentBy := map[id.Message]sentMsg{}
	for _, s := range r.log {
		sentBy[s.id] = s
	}
	results := map[string]*chat.ToolResult{}
	msgTurn := map[string]int{}
	for _, m := range ms {
		msgTurn[string(m.ID)] = m.Turn
		for _, pt := range m.Parts {
			if pt.ToolResult != nil {
				results[pt.ToolResult.CallID] = pt.ToolResult
			}
		}
	}
	byTurn := map[int]*turnRec{}
	var order []int
	for _, m := range ms {
		t := byTurn[m.Turn]
		if t == nil {
			t = &turnRec{N: m.Turn, Kind: "user", Msg: -2}
			byTurn[m.Turn] = t
			order = append(order, m.Turn)
		}
		switch m.Role {
		case chat.RoleUser:
			if s, ok := sentBy[m.ID]; ok {
				if t.Msg == -2 {
					t.Kind, t.Msg, t.Input, t.Tasks = s.kind, s.msg, s.text, s.tasks
				} else {
					t.Joined = append(t.Joined, s.text)
				}
				continue
			}
			for _, pt := range m.Parts {
				if pt.Notice != nil {
					t.Notices = append(t.Notices, pt.Notice.Text)
				}
			}
		case chat.RoleAssistant:
			for _, pt := range m.Parts {
				switch {
				case pt.ToolCall != nil:
					c := callRec{Tool: pt.ToolCall.Name, Args: string(pt.ToolCall.Args)}
					if res := results[pt.ToolCall.ID]; res != nil {
						c.Error, c.Result = res.IsError, clip(res.Text, 600)
					}
					t.Calls = append(t.Calls, c)
				case pt.Text != nil && strings.TrimSpace(pt.Text.Text) != "":
					t.Texts = append(t.Texts, pt.Text.Text)
					t.Answer = pt.Text.Text
				}
			}
		}
	}
	for _, n := range order {
		// List drops the requests' texts; Turn keeps them.
		tt, ok := r.tr.Turn(agent.TraceKey{Project: r.pid, Chat: r.cid, Turn: n})
		if !ok {
			continue
		}
		for _, q := range tt.Requests {
			rr := reqRec{Prompt: q.Usage.Input + q.Usage.CacheRead + q.Usage.CacheWrite, Output: q.Usage.Output, CacheRead: q.Usage.CacheRead,
				Last: q.Last, Seconds: q.Took.Seconds(), Err: r.h.redact(clip(q.Err, 300))}
			for _, b := range q.Blocks {
				rr.Est += b.Tokens
				if b.Name == "History window" {
					rr.EstHistory = b.Tokens
				}
			}
			rr.First = firstTurn(q.Req.Messages)
			if r.j.Cond == condTrace {
				rr.EstTrace = traceTokens(q.Req.Messages, tt.Turn)
			}
			byTurn[tt.Turn].Requests = append(byTurn[tt.Turn].Requests, rr)
		}
	}
	for _, n := range order {
		res.Turns = append(res.Turns, *byTurn[n])
	}
	r.ts.mu.Lock()
	for _, t := range r.ts.list {
		tr := taskRec{ID: t.ID, Args: t.Args, Background: t.Background, Turn: msgTurn[t.Message], Failed: t.failed()}
		for _, rl := range t.Rules {
			tr.Rules = append(tr.Rules, rl.Name)
		}
		for _, tu := range res.Turns {
			if slices.Contains(tu.Tasks, t.ID) {
				tr.Notice = tu.N
			}
		}
		res.Tasks = append(res.Tasks, tr)
	}
	r.ts.mu.Unlock()
	r.w.mu.Lock()
	for _, t := range r.w.tables {
		res.Tables = append(res.Tables, t.Name)
	}
	r.w.mu.Unlock()
	if r.w.web != nil {
		for _, pg := range r.w.web.Pages {
			res.Pages = append(res.Pages, pageKey(pg.URL))
		}
	}
	return nil
}

func firstTurn(ms []chat.Message) int {
	f := 0
	for _, m := range ms {
		if f == 0 || m.Turn < f {
			f = m.Turn
		}
	}
	return f
}

// traceTokens estimates the trace lines in a request's earlier turns: the
// lines in square brackets that start an assistant message.
func traceTokens(ms []chat.Message, cur int) int {
	n := 0
	for _, m := range ms {
		if m.Turn >= cur || m.Role != chat.RoleAssistant {
			continue
		}
		for _, p := range m.Parts {
			if p.Text == nil {
				continue
			}
			for _, l := range strings.Split(p.Text.Text, "\n") {
				if !strings.HasPrefix(l, "[") {
					break
				}
				n += (len(l) + 1 + 3) / 4
			}
		}
	}
	return n
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
