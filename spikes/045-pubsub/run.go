package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Conditions (the ticket's). Part 1 (1–3): app events only. Part 2 (4–5):
// channels, with the key's subscriptions.
const (
	condNone    = 1 // no subscriptions; only the project card
	condUser    = 2 // the key's subscriptions, as if set in the panel
	condAgent   = 3 // agents subscribe themselves with subscribe
	condTool    = 4 // publish, with a description that says when to use it
	condSection = 5 // publish, plus the prompt section and the channel list in block 5
)

var condNames = map[int]string{condNone: "1 none", condUser: "2 user", condAgent: "3 agent", condTool: "4 publish tool", condSection: "5 tool + section + list"}

func part2(cond int) bool { return cond >= condTool }

// job is one run: the day on a model under a condition.
type job struct {
	Model string `json:"model"`
	Cond  int    `json:"cond"`
	Rep   int    `json:"rep"`
}

func (j job) name() string {
	return fmt.Sprintf("%s_c%d_r%d", strings.NewReplacer("/", "-", ":", "-").Replace(j.Model), j.Cond, j.Rep)
}

// call is one tool call.
type call struct {
	Msg     int    `json:"msg"`
	Agent   string `json:"agent"`
	Tool    string `json:"tool"`
	Args    string `json:"args"`
	Error   bool   `json:"error,omitempty"`
	Refused bool   `json:"refused,omitempty"` // outside the chat's groups
	Result  string `json:"result,omitempty"`
}

// turnRec is one message's turn.
type turnRec struct {
	Msg       int    `json:"msg"`
	Agent     string `json:"agent"`
	Text      string `json:"text"`
	Reply     string `json:"reply"`
	Requests  int    `json:"requests"`
	Prompt    int    `json:"prompt"`
	Output    int    `json:"output"`
	CacheRead int    `json:"cache_read"`
	// NoticeTokens are the estimated tokens of event notices in the
	// turn's requests, summed over requests; Block5 those of block 5
	// (the session notes, subscriptions and channels).
	NoticeTokens int      `json:"notice_tokens"`
	Block5       int      `json:"block5"`
	Notices      []string `json:"notices,omitempty"` // the event notices this turn got
	Errors       []string `json:"errors,omitempty"`
	// The files the use measures read, after the turn.
	Docs  string `json:"docs,omitempty"`
	Draft string `json:"draft,omitempty"`
}

// runResult is one run, as written to runs.jsonl.
type runResult struct {
	job
	Started    time.Time  `json:"started"`
	Seconds    float64    `json:"seconds"`
	Error      string     `json:"error,omitempty"`
	Turns      []turnRec  `json:"turns"`
	Calls      []call     `json:"calls"`
	Subs       []subRec   `json:"subs"`
	Events     []event    `json:"events"`
	Deliveries []delivery `json:"deliveries"`
	Pubs       []pubRec   `json:"pubs"`
	Own        int        `json:"own"`  // notices skipped: the chat caused the event
	Dups       int        `json:"dups"` // subscribe calls for a subscription the chat had
	Tables     []string   `json:"tables"`
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

type run struct {
	h   *harness
	j   job
	b   *bus
	w   *world
	pm  *project.Manager
	o   *agent.Orchestrator
	tr  *agent.Traces
	pid id.Project
	res *runResult
	now time.Time

	mu    sync.Mutex
	tools map[id.Chat][]string
	seen  map[id.Chat]int
}

func (h *harness) run(ctx context.Context, j job) (res runResult) {
	res = runResult{job: j, Started: time.Now().UTC()}
	defer func() { res.Seconds = time.Since(res.Started).Seconds() }()
	b := newBus(j.Cond)
	r := &run{h: h, j: j, b: b, w: newWorld(b), res: &res, tools: map[id.Chat][]string{}, seen: map[id.Chat]int{}, now: clock("08:30")}
	r.w.now = r.clock
	if err := r.play(ctx); err != nil {
		res.Error = h.redact(err.Error())
	}
	res.Subs, res.Events, res.Deliveries, res.Pubs, res.Own, res.Dups = b.snapshot()
	return res
}

// chatTools is the Deps.ChatTools hook: each agent's groups plus the
// condition's tools.
func (r *run) chatTools(ch chat.Chat) ([]string, func(string) string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ts, ok := r.tools[ch.ID]; ok {
		return ts, refusal
	}
	return nil, nil
}

// notes is the Deps.Notes hook: block 5 is the chat's session notes, then
// the bus's text.
func (r *run) notes(ctx context.Context, p *project.Project, ch chat.Chat) (string, error) {
	n, err := p.Chats.Notes(ctx, ch.ID)
	if err != nil {
		return "", err
	}
	s := tool.NotesBlock(n)
	r.b.mu.Lock()
	key := r.b.keys[ch.ID]
	r.b.mu.Unlock()
	if key == "" {
		return s, nil
	}
	if extra := r.b.block5(key); extra != "" {
		s = strings.TrimSpace(s + "\n\n" + extra)
	}
	return s, nil
}

// section is the Deps.Section hook: condition 5's publish rule.
func (r *run) section(ch chat.Chat) string {
	if r.j.Cond != condSection {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.Contains(r.tools[ch.ID], "publish") {
		return publishRule
	}
	return ""
}

func (r *run) registry() *tool.Registry {
	reg := tool.NewRegistry()
	for _, t := range tool.Builtin(tool.Deps{}) {
		if t.Spec().Name != "fetch_page" { // the web is faked
			reg.Add(t)
		}
	}
	for _, t := range agent.Tools() {
		if t.Spec().Name != "request_schema_change" { // the schema agent is faked
			reg.Add(t)
		}
	}
	reg.Add(r.w.tools()...)
	reg.Add(r.b.tools()...)
	return reg
}

func (r *run) settings() config.Settings {
	s := config.Defaults()
	s.LLM = r.h.llm
	return s
}

func (r *run) clock() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.now
}

func (r *run) setClock(t time.Time) {
	r.mu.Lock()
	r.now = t
	r.mu.Unlock()
}

func (r *run) play(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.h.timeout)
	defer cancel()
	root, err := os.MkdirTemp("", "spike045-")
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
	if r.pid, err = r.pm.Create(ctx, "Bitcoin"); err != nil {
		return err
	}
	if err := r.setup(ctx); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	set := r.settings()
	r.tr = agent.NewTraces()
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.o = agent.New(agent.Deps{
		Context: appCtx, Projects: r.pm, Models: r.h.models, Tools: r.registry(), Events: nopPublisher{},
		Settings: func() config.Settings { return set }, Traces: r.tr, Log: r.h.log,
		Now:       r.clock,
		Card:      r.w.card,
		ChatTools: r.chatTools,
		Notes:     r.notes,
		Notices:   r.b.notices,
		Section:   r.section,
	})
	defer func() {
		r.o.Refuse()
		stop()
		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = r.o.Wait(wctx)
	}()
	defer func() { r.res.Tables = r.tables(context.WithoutCancel(ctx)) }()

	fired := map[string]bool{}
	for i, m := range script {
		n := i + 1
		for _, rd := range runs {
			if rd.At != "" && !fired[rd.ID] && rd.At <= m.At {
				fired[rd.ID] = true
				r.setClock(clock(rd.At))
				if err := r.appRun(ctx, rd); err != nil {
					return err
				}
			}
		}
		r.b.setMsg(n)
		r.setClock(clock(m.At))
		if err := r.turn(ctx, n, m); err != nil {
			return err
		}
	}
	return nil
}

// appRun plays a pipeline run: the app publishes pipeline_run, and
// daily_btc's successful run adds today's price.
func (r *run) appRun(ctx context.Context, rd runDef) error {
	if rd.Pipeline == "daily_btc" && rd.Outcome == "success" {
		p, err := r.pm.Open(ctx, r.pid)
		if err != nil {
			return err
		}
		_, err = store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (sql.Result, error) {
			return tx.ExecContext(ctx, "INSERT INTO btc_prices(date, price, _run_id, _fetched_at) VALUES(?, ?, ?, ?)",
				today.Format(time.DateOnly), priceToday, rd.ID, today.Format(time.DateOnly)+"T"+rd.At+":04Z")
		})
		p.Release()
		if err != nil {
			return err
		}
	}
	r.b.emit(event{Kind: evRun, By: "app", Pipeline: rd.Pipeline, Outcome: rd.Outcome, Text: runNotice(rd)})
	return nil
}

// setup seeds the tables and creates the six agents with their groups,
// roles and, in conditions 2, 4 and 5, the key's subscriptions.
func (r *run) setup(ctx context.Context) error {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	if err := seed(ctx, p); err != nil {
		return err
	}
	for _, a := range agents {
		ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: a.Title, Role: a.Role, Model: r.j.Model})
		if err != nil {
			return err
		}
		r.tools[ch.ID] = toolsOf(a.Groups, r.j.Cond)
		r.b.addChat(a.Key, ch.ID)
		r.b.mu.Lock()
		for _, s := range keySubs(a, r.j.Cond) {
			r.b.subscribe(a.Key, s, "user")
		}
		r.b.mu.Unlock()
	}
	return nil
}

// turn sends a scripted message, waits for the turn and records it.
func (r *run) turn(ctx context.Context, n int, m msgDef) error {
	r.b.mu.Lock()
	ch := r.b.chats[m.To]
	r.b.mu.Unlock()
	tr := turnRec{Msg: n, Agent: m.To, Text: m.Text}
	if _, err := r.o.Send(ctx, r.pid, ch, agent.UserMessage{Text: m.Text}); err != nil {
		return err
	}
	err := r.idle(ctx, ch)
	if rerr := r.read(ctx, ch, &tr); err == nil {
		err = rerr
	}
	tr.Docs = r.file(ctx, "docs/schema.md")
	tr.Draft = r.file(ctx, "summaries/draft.md")
	r.res.Turns = append(r.res.Turns, tr)
	return err
}

// idle waits until the chat's turn has ended. A form is answered as a
// user without an opinion would; an approval is granted once.
func (r *run) idle(ctx context.Context, ch id.Chat) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		l := r.o.Live(r.pid, ch)
		if !l.Running {
			return nil
		}
		if w := l.Waiting; w != nil {
			a := agent.Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce}
			if w.Kind != chat.PartApproval {
				a = agent.Answer{Message: w.Message, Index: w.Index, Answers: r.formAnswers(ctx, ch, *w)}
			}
			if err := r.o.Answer(r.pid, ch, a); err != nil && !errors.Is(err, agent.ErrNotWaiting) {
				return err
			}
		}
		select {
		case <-ctx.Done():
			r.o.Stop(r.pid, ch)
			return fmt.Errorf("the run took longer than %s", r.h.timeout)
		case <-t.C:
		}
	}
}

func (r *run) formAnswers(ctx context.Context, ch id.Chat, w chat.Waiting) map[string][]string {
	out := map[string][]string{}
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return out
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, ch, r.seen[ch]+1, 0)
	if err != nil {
		return out
	}
	for _, m := range ms {
		if m.ID != w.Message || w.Index >= len(m.Parts) || m.Parts[w.Index].Question == nil {
			continue
		}
		for _, it := range m.Parts[w.Index].Question.Questions {
			out[it.Header] = []string{"No preference; decide yourself."}
		}
	}
	return out
}

// refusedMark is in every refusal's text.
const refusedMark = "isn't one of this chat's tools"

// read adds the chat's turn since the last read to tr and the run's calls.
func (r *run) read(ctx context.Context, ch id.Chat, tr *turnRec) error {
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
	last := r.seen[ch]
	var final, anyText, failed string
	for _, m := range ms {
		last = max(last, m.Turn)
		for _, pt := range m.Parts {
			if n := pt.Notice; n != nil {
				switch n.Kind {
				case chat.NoticeTurnFailed:
					failed = n.Text // a provider error stopped the turn: the run failed, and -resume runs it again
				case noticeEvent:
					tr.Notices = append(tr.Notices, n.Text)
				}
			}
		}
		if m.Role != chat.RoleAssistant {
			continue
		}
		var text strings.Builder
		calls := 0
		for _, pt := range m.Parts {
			switch {
			case pt.ToolCall != nil:
				calls++
				c := call{Msg: tr.Msg, Agent: tr.Agent, Tool: pt.ToolCall.Name, Args: string(pt.ToolCall.Args)}
				if res := results[pt.ToolCall.ID]; res != nil {
					c.Error, c.Result = res.IsError, clip(res.Text, 600)
					c.Refused = res.IsError && strings.Contains(res.Text, refusedMark)
				}
				r.res.Calls = append(r.res.Calls, c)
			case pt.Text != nil:
				text.WriteString(pt.Text.Text)
			}
		}
		if t := strings.TrimSpace(text.String()); t != "" {
			anyText = t
			if calls == 0 {
				final = t
			}
		}
	}
	if final == "" {
		final = anyText
	}
	tr.Reply = final
	for _, t := range r.tr.List() {
		if t.Chat != ch || t.Turn <= r.seen[ch] || t.Turn > last {
			continue
		}
		if full, ok := r.tr.Turn(agent.TraceKey{Project: t.Project, Chat: t.Chat, Turn: t.Turn}); ok {
			t = full
		}
		for _, q := range t.Requests {
			tr.Requests++
			tr.Prompt += q.Usage.Input + q.Usage.CacheRead + q.Usage.CacheWrite
			tr.CacheRead += q.Usage.CacheRead
			tr.Output += q.Usage.Output
			if q.Err != "" {
				tr.Errors = append(tr.Errors, r.h.redact(clip(q.Err, 200)))
			}
			if q.Dropped {
				continue
			}
			for _, b := range q.Req.System {
				if b.Name == "Session notes" {
					tr.Block5 += provider.EstimateTokens(provider.Request{System: []provider.Block{{Text: b.Text}}})
				}
			}
			var notices strings.Builder
			for _, m := range q.Req.Messages {
				for _, pt := range m.Parts {
					if pt.Notice != nil && pt.Notice.Kind == noticeEvent {
						notices.WriteString(pt.Notice.Text + "\n")
					}
				}
			}
			if notices.Len() > 0 {
				tr.NoticeTokens += provider.EstimateTokens(provider.Request{System: []provider.Block{{Text: notices.String()}}})
			}
		}
	}
	r.seen[ch] = last
	if failed != "" {
		return fmt.Errorf("the turn failed: %s", r.h.redact(clip(failed, 300)))
	}
	return nil
}

// file is a bucket file's newest content, or "".
func (r *run) file(ctx context.Context, key string) string {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return ""
	}
	defer p.Release()
	_, rc, err := p.DB.OpenObject(ctx, key)
	if err != nil {
		return ""
	}
	defer rc.Close()
	b, _ := io.ReadAll(io.LimitReader(rc, 64<<10))
	return string(b)
}

func (r *run) tables(ctx context.Context) []string {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return nil
	}
	defer p.Release()
	ts, _ := p.DB.Tables(ctx)
	return ts
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
