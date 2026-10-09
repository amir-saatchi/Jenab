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

// Conditions (SPIKE-029).
const (
	condBaseline = 1 // today's loop, no subjects
	condTool     = 2 // update_subject, get_subject and the prompt rule
	condIndex    = 3 // condition 2 plus the index in the context
	condCheck    = 4 // condition 3 plus the app's check
)

var condNames = map[int]string{condBaseline: "1 baseline", condTool: "2 tool", condIndex: "3 tool+index", condCheck: "4 tool+index+check"}

// job is one run: a scenario on a model under a condition.
type job struct {
	Model    string `json:"model"`
	Scenario string `json:"scenario"`
	Cond     int    `json:"cond"`
	Rep      int    `json:"rep"`
	Rule     string `json:"rule,omitempty"` // the prompt rule; "" is round 1's, v1
}

func (j job) name() string {
	n := fmt.Sprintf("%s_%s_c%d_r%d", strings.NewReplacer("/", "-", ":", "-").Replace(j.Model), j.Scenario, j.Cond, j.Rep)
	if j.rule() != "v1" {
		n += "_" + j.rule()
	}
	return n
}

func (j job) rule() string {
	if j.Rule == "" {
		return "v1"
	}
	return j.Rule
}

// call is one tool call of a turn.
type call struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Error  bool   `json:"error,omitempty"`
	Result string `json:"result,omitempty"`
	Nudge  bool   `json:"nudge,omitempty"` // in the turn after the app's check
	Alone  bool   `json:"alone,omitempty"` // update_subject was the only call of its response
}

// msgResult is what one scripted message's turns did.
type msgResult struct {
	Index    int      `json:"index"`
	Kind     string   `json:"kind"`
	Topic    string   `json:"topic,omitempty"`
	Turns    []int    `json:"turns"`
	Calls    []call   `json:"calls"`
	Answer   string   `json:"answer"`
	Nudged   bool     `json:"nudged,omitempty"`
	Requests int      `json:"requests"`
	Prompt   int      `json:"prompt"`
	Output   int      `json:"output"`
	Errors   []string `json:"errors,omitempty"`
	Seconds  float64  `json:"seconds"`
}

// runResult is one run, as written to runs.jsonl.
type runResult struct {
	job
	Started  time.Time      `json:"started"`
	Seconds  float64        `json:"seconds"`
	Error    string         `json:"error,omitempty"`
	Messages []msgResult    `json:"messages"`
	Events   []subjectEvent `json:"events"`
	Subjects []subject      `json:"subjects"`
	Card     string         `json:"card"`
}

// harness holds what all runs share: the provider registry, so the
// per-provider call limit holds across parallel runs.
type harness struct {
	models  *provider.Registry
	llm     config.LLMSettings
	temp    string
	keep    bool
	timeout time.Duration
	log     *slog.Logger
	redact  func(string) string
}

// run plays one scenario on one model under one condition.
func (h *harness) run(ctx context.Context, j job, sc scenarioDef) (res runResult) {
	res = runResult{job: j, Started: time.Now().UTC()}
	defer func() { res.Seconds = time.Since(res.Started).Seconds() }()
	r := &run{h: h, j: j, sc: sc, w: &world{}, s: &subjects{rule: j.rule()}}
	err := r.play(ctx, &res)
	if err != nil {
		res.Error = h.redact(err.Error())
	}
	res.Events, res.Subjects, res.Card = r.s.log(), r.s.snapshot(), r.card(ctx)
	return res
}

type run struct {
	h   *harness
	j   job
	sc  scenarioDef
	w   *world
	s   *subjects
	pm  *project.Manager
	o   *agent.Orchestrator
	tr  *agent.Traces
	pid id.Project
	cid id.Chat

	seen int // the last turn read from the chat
}

// card is block 4: the project card, and in conditions 2–4 the subjects
// block. The agent asks for it at each cut, so the index is as of the
// last cut, as SPEC 3.1 has it for blocks 2–5.
func (r *run) card(context.Context) string {
	c := r.w.card()
	if r.j.Cond >= condTool {
		c += "\n\n" + r.s.block(r.j.Cond >= condIndex)
	}
	return c
}

func (r *run) tools() *tool.Registry {
	reg := tool.NewRegistry()
	for _, t := range tool.Builtin(tool.Deps{}) {
		if n := t.Spec().Name; n == "search_history" || n == "read_messages" {
			reg.Add(t)
		}
	}
	reg.Add(r.w.tools()...)
	if r.j.Cond >= condTool {
		reg.Add(r.s.tools()...)
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
	root, err := os.MkdirTemp(r.h.temp, "spike029-")
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
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: r.sc.Title, Model: r.j.Model})
	p.Release()
	if err != nil {
		return err
	}
	r.cid = ch.ID
	set := r.settings()
	r.tr = agent.NewTraces()
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.o = agent.New(agent.Deps{
		Context: appCtx, Projects: r.pm, Models: r.h.models, Tools: r.tools(), Events: nopPublisher{},
		Settings: func() config.Settings { return set }, Traces: r.tr, Log: r.h.log,
		Card: func(ctx context.Context, _ id.Project) (string, error) { return r.card(ctx), nil },
	})
	defer func() {
		r.o.Refuse()
		stop()
		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = r.o.Wait(wctx)
	}()
	for i, m := range r.sc.Messages {
		start := time.Now()
		mr := msgResult{Index: i, Kind: m.Kind, Topic: m.Topic}
		r.s.at(i, false)
		err := r.turn(ctx, m.Text, false, &mr)
		if err == nil && r.j.Cond == condCheck && wrote(mr.Calls) && !subjectUpdated(mr.Calls) {
			mr.Nudged = true
			r.s.at(i, true)
			err = r.turn(ctx, "[app check: this turn changed the project but updated no subject. Call update_subject for this work now.]", true, &mr)
		}
		mr.Seconds = time.Since(start).Seconds()
		res.Messages = append(res.Messages, mr)
		if err != nil {
			return fmt.Errorf("message %d: %w", i+1, err)
		}
	}
	return nil
}

// turn sends one message, waits for its turn to end and adds what the
// turn did to mr.
func (r *run) turn(ctx context.Context, text string, nudge bool, mr *msgResult) error {
	if _, err := r.o.Send(ctx, r.pid, r.cid, agent.UserMessage{Text: text}); err != nil {
		return err
	}
	if err := r.idle(ctx); err != nil {
		return err
	}
	return r.read(ctx, nudge, mr)
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

// read adds the turns since the last read to mr: calls, the final answer,
// requests and tokens.
func (r *run) read(ctx context.Context, nudge bool, mr *msgResult) error {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, r.cid, r.seen+1, 0)
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
	last := r.seen
	var final, anyText string // the last text without calls, and the last text at all
	for _, m := range ms {
		if m.Turn <= r.seen {
			continue
		}
		last = max(last, m.Turn)
		if !slices.Contains(mr.Turns, m.Turn) {
			mr.Turns = append(mr.Turns, m.Turn)
		}
		if m.Role != chat.RoleAssistant {
			continue
		}
		var calls []call
		var text strings.Builder
		for _, pt := range m.Parts {
			switch {
			case pt.ToolCall != nil:
				c := call{Tool: pt.ToolCall.Name, Args: string(pt.ToolCall.Args), Nudge: nudge}
				if res := results[pt.ToolCall.ID]; res != nil {
					c.Error, c.Result = res.IsError, clip(res.Text, 300)
				}
				calls = append(calls, c)
			case pt.Text != nil:
				text.WriteString(pt.Text.Text)
			}
		}
		if len(calls) == 1 && calls[0].Tool == "update_subject" {
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
	answer := final
	if answer == "" {
		answer = anyText
	}
	switch {
	case !nudge:
		mr.Answer = answer
	case answer != "":
		mr.Answer += "\n\n[after the app's check] " + answer
	}
	for _, t := range r.tr.List() {
		if t.Chat != r.cid || t.Turn <= r.seen || t.Turn > last {
			continue
		}
		for _, q := range t.Requests {
			mr.Requests++
			mr.Prompt += q.Usage.Input + q.Usage.CacheRead + q.Usage.CacheWrite
			mr.Output += q.Usage.Output
			if q.Err != "" {
				mr.Errors = append(mr.Errors, r.h.redact(clip(q.Err, 200)))
			}
		}
	}
	r.seen = last
	return nil
}

// nopPublisher drops the chat events; the harness reads the store.
type nopPublisher struct{}

func (nopPublisher) Delta(chat.Delta)   {}
func (nopPublisher) Part(chat.PartDone) {}
func (nopPublisher) Status(chat.Status) {}

func wrote(cs []call) bool {
	return slices.ContainsFunc(cs, func(c call) bool { return !c.Error && slices.Contains(writeTools, c.Tool) })
}

func subjectUpdated(cs []call) bool {
	return slices.ContainsFunc(cs, func(c call) bool { return !c.Error && c.Tool == "update_subject" })
}

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
