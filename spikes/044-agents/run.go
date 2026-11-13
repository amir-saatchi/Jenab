package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
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

// Conditions (SPIKE-044). 0 is an optional control, not in the ticket.
const (
	condAll     = 0 // the key's prompt with every group
	condKey     = 1 // the key's groups and prompt, as if the user picked them in the panel
	condGroups  = 2 // the user asks Mother for an agent; she picks groups
	condSingle  = 3 // as 2, with single tools instead of groups
	condUnasked = 4 // the user describes a lasting job; Mother decides
)

var condNames = map[int]string{condAll: "0 every group", condKey: "1 key", condGroups: "2 asked, groups", condSingle: "3 asked, single tools", condUnasked: "4 unasked"}

// asked: the user asks Mother, who picks the tools.
func asked(cond int) bool { return cond >= condGroups }

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

// Phases: Mother's plain message (condition 4), the request, and the
// follow-up task. A turn a Mother call started has the phase of the
// Mother turn it came from, with ":start".
const (
	phasePlain = "plain"
	phaseAsk   = "ask"
	phaseTask  = "task"
	startOf    = ":start"
)

const mother = "mother"

// call is one tool call.
type call struct {
	Who     string `json:"who"` // "mother" or the chat's ID
	Phase   string `json:"phase"`
	Tool    string `json:"tool"`
	Args    string `json:"args"`
	Error   bool   `json:"error,omitempty"`
	Refused bool   `json:"refused,omitempty"` // outside the chat's tools
	Result  string `json:"result,omitempty"`
}

// creation is one create_chat or send_to_chat call that ran.
type creation struct {
	Phase   string   `json:"phase"`
	Sent    bool     `json:"sent,omitempty"` // send_to_chat
	Chat    string   `json:"chat"`
	Title   string   `json:"title,omitempty"`
	Role    string   `json:"role,omitempty"`
	Message string   `json:"message"`
	Picks   []string `json:"picks,omitempty"` // the tools argument as given
	Groups  []string `json:"groups,omitempty"`
	Tools   []string `json:"tools,omitempty"`
}

// turnRec is one turn.
type turnRec struct {
	Who       string `json:"who"`
	Phase     string `json:"phase"`
	Text      string `json:"text"`
	Reply     string `json:"reply"`
	Requests  int    `json:"requests"`
	Prompt    int    `json:"prompt"`
	Output    int    `json:"output"`
	CacheRead int    `json:"cache_read"`
	// ToolTokens are the estimated tokens of the first request's tool
	// definitions, System of its system blocks.
	ToolTokens int      `json:"tool_tokens"`
	System     int      `json:"system"`
	Errors     []string `json:"errors,omitempty"`
}

// state is what the checks read at the end of a run.
type state struct {
	Calls     []call             `json:"-"`
	Reply     string             `json:"reply"` // the task's last reply in the agent, else Mother's
	Files     map[string]string  `json:"files,omitempty"`
	Memory    string             `json:"memory,omitempty"`
	Pipelines map[string]string  `json:"pipelines,omitempty"` // saved in the run
	Tables    []string           `json:"tables"`
	Prices    map[string]float64 `json:"prices,omitempty"` // the five dates the entry task adds
	Keywords  []string           `json:"keywords"`
}

// runResult is one run, as written to runs.jsonl.
type runResult struct {
	job
	Started   time.Time  `json:"started"`
	Seconds   float64    `json:"seconds"`
	Error     string     `json:"error,omitempty"`
	Turns     []turnRec  `json:"turns"`
	Calls     []call     `json:"calls"`
	Creations []creation `json:"creations"`
	Agent     string     `json:"agent,omitempty"` // the chat the task went to; "" for Mother
	// AllTools is the estimated tokens of every chat tool's definition:
	// what a chat with every tool sends in each request.
	AllTools int   `json:"all_tools"`
	State    state `json:"state"`
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
	sc  scenarioDef
	w   *world
	pm  *project.Manager
	o   *agent.Orchestrator
	tr  *agent.Traces
	pid id.Project
	mom id.Chat
	res *runResult

	mu      sync.Mutex
	agents  map[id.Chat][]string // each agent's tools
	pending []creation           // Mother's calls whose message waits to run
	tasks   int
	phase   string // the Mother phase now
	seen    map[id.Chat]int
}

func (h *harness) run(ctx context.Context, j job, sc scenarioDef) (res runResult) {
	res = runResult{job: j, Started: time.Now().UTC()}
	defer func() { res.Seconds = time.Since(res.Started).Seconds() }()
	r := &run{h: h, j: j, sc: sc, w: newWorld(), res: &res, agents: map[id.Chat][]string{}, seen: map[id.Chat]int{}}
	if err := r.play(ctx); err != nil {
		res.Error = h.redact(err.Error())
	}
	return res
}

// chatTools is the Deps.ChatTools hook: Mother's tools, or an agent's.
func (r *run) chatTools(ch chat.Chat) ([]string, func(string) string) {
	if ch.Kind == chat.KindMother {
		return motherToolList(), refusal(true)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if ts, ok := r.agents[ch.ID]; ok {
		return ts, refusal(false)
	}
	return nil, nil
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
	reg.Add(r.motherTools()...)
	return reg
}

func (r *run) settings() config.Settings {
	s := config.Defaults()
	s.LLM = r.h.llm
	return s
}

func (r *run) play(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.h.timeout)
	defer cancel()
	root, err := os.MkdirTemp("", "spike044-")
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
	tools := r.registry()
	for _, t := range tools.For(chat.KindChat) {
		s := t.Spec()
		r.res.AllTools += provider.EstimateTokens(provider.Request{Tools: []provider.ToolDef{{Name: s.Name, Description: s.Description, Schema: s.Schema}}})
	}
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.o = agent.New(agent.Deps{
		Context: appCtx, Projects: r.pm, Models: r.h.models, Tools: tools, Events: nopPublisher{},
		Settings: func() config.Settings { return set }, Traces: r.tr, Log: r.h.log,
		Now:       func() time.Time { return today },
		Card:      r.w.card,
		ChatTools: r.chatTools,
	})
	defer func() {
		r.o.Refuse()
		stop()
		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = r.o.Wait(wctx)
	}()
	defer func() { r.collect(context.WithoutCancel(ctx)) }()

	if !asked(r.j.Cond) {
		groups := r.sc.KeyGroups
		if r.j.Cond == condAll {
			groups = nil
			for _, g := range groupsList() {
				groups = append(groups, g)
			}
		}
		ch, err := r.newAgent(ctx, id.SourceUser, phaseTask, r.sc.KeyTitle, r.sc.KeyRole, "", groups, false)
		if err != nil {
			return err
		}
		r.res.Agent = string(ch)
		_, err = r.turn(ctx, ch, phaseTask, r.sc.Task)
		return err
	}
	if r.j.Cond == condUnasked {
		if err := r.toMother(ctx, phasePlain, r.sc.Plain); err != nil {
			return err
		}
	}
	text := r.sc.Ask
	if r.j.Cond == condUnasked {
		text = r.sc.Unasked
	}
	if err := r.toMother(ctx, phaseAsk, text); err != nil {
		return err
	}
	if ch := r.agentFor(); ch != "" {
		r.res.Agent = string(ch)
		_, err := r.turn(ctx, ch, phaseTask, r.sc.Task)
		return err
	}
	return r.toMother(ctx, phaseTask, r.sc.Task)
}

func groupsList() []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = g.Name
	}
	return out
}

// agentFor is where the scripted user sends the task: the chat Mother
// last created in the request's turn, else the last one she sent the
// work to, else the last she created at all; "" for none.
func (r *run) agentFor() id.Chat {
	cs := r.res.Creations
	for _, want := range []func(c creation) bool{
		func(c creation) bool { return c.Phase == phaseAsk && !c.Sent },
		func(c creation) bool { return c.Phase == phaseAsk },
		func(c creation) bool { return !c.Sent },
	} {
		for i := len(cs) - 1; i >= 0; i-- {
			if want(cs[i]) {
				return id.Chat(cs[i].Chat)
			}
		}
	}
	return ""
}

// setup seeds the tables and gives Mother the run's model.
func (r *run) setup(ctx context.Context) error {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	if err := seed(ctx, p); err != nil {
		return err
	}
	cs, err := p.Chats.Chats(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(cs, func(c chat.Chat) bool { return c.Kind == chat.KindMother })
	if i < 0 {
		return errors.New("the project has no Mother chat")
	}
	r.mom = cs[i].ID
	_, err = p.Chats.SetModel(ctx, r.mom, r.j.Model)
	return err
}

// toMother sends a message to Mother, then runs the turns her
// create_chat and send_to_chat calls started, in order.
func (r *run) toMother(ctx context.Context, phase, text string) error {
	r.mu.Lock()
	r.phase = phase
	r.mu.Unlock()
	if _, err := r.turn(ctx, r.mom, phase, text); err != nil {
		return err
	}
	r.mu.Lock()
	todo := r.pending
	r.pending = nil
	r.mu.Unlock()
	for _, c := range todo {
		if _, err := r.turn(ctx, id.Chat(c.Chat), phase+startOf, "[Message from Mother]\n"+c.Message); err != nil {
			return err
		}
	}
	return nil
}

// turn sends text to a chat, waits for the turn and records it.
func (r *run) turn(ctx context.Context, ch id.Chat, phase, text string) (turnRec, error) {
	who := string(ch)
	if ch == r.mom {
		who = mother
	}
	tr := turnRec{Who: who, Phase: phase, Text: text}
	if _, err := r.o.Send(ctx, r.pid, ch, agent.UserMessage{Text: text}); err != nil {
		return tr, err
	}
	err := r.idle(ctx, ch)
	if rerr := r.read(ctx, ch, &tr); err == nil {
		err = rerr
	}
	r.res.Turns = append(r.res.Turns, tr)
	return tr, err
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

// formAnswers answers each of a form's questions as a user without an
// opinion would, in its Other text.
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

// read adds the chat's turns since the last read to tr and the run's calls.
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
			if n := pt.Notice; n != nil && n.Kind == chat.NoticeTurnFailed {
				failed = n.Text // a provider error stopped the turn: the run failed, and -resume runs it again
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
				c := call{Who: tr.Who, Phase: tr.Phase, Tool: pt.ToolCall.Name, Args: string(pt.ToolCall.Args)}
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
	first := true
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
			if first && !q.Dropped {
				first = false
				tr.ToolTokens = provider.EstimateTokens(provider.Request{Tools: q.Req.Tools})
				tr.System = provider.EstimateTokens(provider.Request{System: q.Req.System})
			}
		}
	}
	r.seen[ch] = last
	if failed != "" {
		return fmt.Errorf("the turn failed: %s", r.h.redact(clip(failed, 300)))
	}
	return nil
}

// collect reads the end state for the checks.
func (r *run) collect(ctx context.Context) {
	s := &r.res.State
	for i := len(r.res.Turns) - 1; i >= 0; i-- {
		t := r.res.Turns[i]
		if strings.HasPrefix(t.Phase, phaseTask) && t.Who != mother {
			s.Reply = t.Reply
			break
		}
	}
	if s.Reply == "" {
		for _, t := range r.res.Turns {
			if t.Phase == phaseTask {
				s.Reply = t.Reply
			}
		}
	}
	r.w.mu.Lock()
	s.Pipelines = maps.Clone(r.w.saved)
	r.w.mu.Unlock()
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return
	}
	defer p.Release()
	s.Tables, _ = p.DB.Tables(ctx)
	s.Files = map[string]string{}
	for _, c := range r.res.Calls {
		if c.Tool != "bucket_put" || c.Error {
			continue
		}
		var a struct {
			Key string `json:"key"`
		}
		if json_(c.Args, &a) != nil || a.Key == "" {
			continue
		}
		if _, rc, err := p.DB.OpenObject(ctx, a.Key); err == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, 64<<10))
			rc.Close()
			s.Files[a.Key] = string(b)
		}
	}
	if ss, err := p.DB.Memory(ctx); err == nil {
		var b strings.Builder
		for _, m := range ss {
			if m.Content != "" {
				fmt.Fprintf(&b, "[%s]\n%s\n", m.Section, m.Content)
			}
		}
		s.Memory = strings.TrimSpace(b.String())
	}
	s.Prices = map[string]float64{}
	_, _ = store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		rows, err := tx.QueryContext(ctx, "SELECT date, price FROM btc_prices WHERE date LIKE '2026-09-%'")
		if err != nil {
			return struct{}{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var d string
			var v sql.NullFloat64
			if rows.Scan(&d, &v) == nil {
				s.Prices[d] = v.Float64
			}
		}
		rows2, err := tx.QueryContext(ctx, "SELECT keyword FROM watch_keywords ORDER BY id")
		if err != nil {
			return struct{}{}, err
		}
		defer rows2.Close()
		for rows2.Next() {
			var k string
			if rows2.Scan(&k) == nil {
				s.Keywords = append(s.Keywords, k)
			}
		}
		return struct{}{}, nil
	})
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
