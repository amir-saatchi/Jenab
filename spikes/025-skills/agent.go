package main

// The agent loop: system prompt = guide + project card (rebuilt for every request), the 7
// tools, at most 4 repair rounds per task, one nudge if the model stops early, a request cap,
// token and time tracking, and the SPEC 3.6 history window for the chained conversation.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
)

const (
	maxRepairs     = 4  // repair rounds per task
	maxRequests    = 16 // per task
	maxOutTokens   = 8192
	historyMaxTok  = 24000 // SPEC 3.6
	historyMaxTurn = 8
	historyMinTurn = 3
	previewChars   = 6000      // ~1,500 tokens (tool_preview_tokens)
	budgetPrompt   = 3_500_000 // SPIKE-025 cap, including smoke tests and calibration
	budgetReserve  = 20_000    // stop this early so the next request (peak ~15k) cannot cross the cap
	probeMaxReq    = 6         // T-load and T-roles: requests per run
)

// ---- global prompt-token budget ----

var (
	budgetMu   sync.Mutex
	budgetUsed int // prompt tokens, including earlier invocations (read from results)
	budgetOut  int
	budgetHit  bool
)

func spend(prompt, output int) bool {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	budgetUsed += prompt
	budgetOut += output
	if budgetUsed >= budgetPrompt-budgetReserve {
		budgetHit = true
	}
	return !budgetHit
}

func overBudget() bool {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	return budgetHit
}

// ---- messages ----

type msg struct {
	Role   string // user, assistant, tool
	Text   string
	Calls  []Call
	CallID string
	Stub   bool
}

func (m msg) chars() int {
	n := len(m.Text)
	for _, c := range m.Calls {
		n += len(c.Name) + len(c.Args)
	}
	return n
}

func toOAI(m msg) openai.ChatCompletionMessageParamUnion {
	switch m.Role {
	case "user":
		return openai.UserMessage(m.Text)
	case "tool":
		return openai.ToolMessage(m.Text, m.CallID)
	}
	am := openai.ChatCompletionAssistantMessageParam{}
	if m.Text != "" || len(m.Calls) == 0 {
		am.Content.OfString = openai.String(m.Text)
	}
	for _, c := range m.Calls {
		args := c.Args
		if args == "" {
			args = "{}"
		}
		am.ToolCalls = append(am.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: c.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: c.Name, Arguments: args}}})
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &am}
}

// ---- results ----

type IssueRec struct {
	Cat, Msg string
	Warn     bool `json:",omitempty"`
}

type WriteRec struct {
	Tool, Artifact string
	OK             bool
	Batch          int
	Issues         []IssueRec `json:",omitempty"`
	Updated        []string   `json:",omitempty"`
}

type RunResult struct {
	Test, Cond, Item   string // SPIKE-025: tconfig/tload/troles, A/B/C (M for Mother), task or request id
	Rep                int
	Model, Task, Guide string
	Run                int
	Chained            bool
	Started            string
	Valid              bool // every artifact's last write succeeded and all needed artifacts saved
	FirstTryValid      bool // valid without any failed write call
	FailedBatches      int
	Rounds             int // repair rounds used until valid (valid runs only)
	Writes             []WriteRec
	Requests           int
	Prompt, Output     int
	PeakPrompt         int
	PeakHistoryChars   int // chained: previous turns in the window
	Cuts               int `json:",omitempty"`
	Seconds            float64
	Stop               string // done, repair limit, gave up, request cap, error, quota, budget
	Err                string `json:",omitempty"`
	Nudged             bool
	Checks             []Check
	Pass, Total        int
	SeedNotes          []string `json:",omitempty"`
	Transcript         string
	ReqPrompts         []int     // prompt tokens of each request
	Events             []SkEvent // skill loads and actions, in order
	Loaded             []string  // skills loaded at the end, "name:how"
	Role               *RoleRec  `json:",omitempty"` // T-roles
}

// SkEvent is one step that matters for skill loading: load_skill, load_with, write, search,
// read (any other tool), answer (a response without tool calls).
type SkEvent struct {
	Req  int
	Kind string
	Name string // skill or tool
	OK   bool   `json:",omitempty"`
}

// ---- agent ----

type Agent struct {
	M       *Model
	Cond    string // A: whole guide; B: skill list + load_skill; C: B + load_with
	Probe   bool   // T-load: stop after the first write attempt or the final answer
	loaded  map[string]string
	order   []string
	Guide   string // guide text
	GuideID string
	P       *Project
	history [][]msg // previous turns (chained mode)
	earlier int     // turns dropped by cuts
	cuts    int
	tr      *os.File
	trw     *redactWriter
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (a *Agent) openTranscript(name string) string {
	dir := filepath.Join("results", "transcripts")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, unsafeName.ReplaceAllString(name, "_")+".txt")
	f, err := os.Create(path)
	if err == nil {
		a.tr = f
		a.trw = &redactWriter{w: f}
	}
	return filepath.ToSlash(path)
}

func (a *Agent) logf(format string, args ...any) {
	if a.trw != nil {
		fmt.Fprintf(a.trw, format, args...)
	}
}

func (a *Agent) closeTranscript() {
	if a.trw != nil {
		a.trw.Flush()
		a.tr.Close()
		a.trw, a.tr = nil, nil
	}
}

func (a *Agent) system() string {
	var b strings.Builder
	if a.Cond == "A" {
		b.WriteString(a.Guide) // guide v2, as in SPIKE-021
	} else {
		b.WriteString(intro) // guide v2 up to section 1, then the skill list (end of block 1)
		b.WriteString("\n")
		b.WriteString(skillList(nil))
	}
	b.WriteString("\n\n")
	b.WriteString(a.P.Card())
	fmt.Fprintf(&b, "Today is %s (UTC).\n", time.Now().UTC().Format("2006-01-02"))
	if a.earlier > 0 {
		fmt.Fprintf(&b, "This chat has %d earlier turns that are not shown. Use get_config and describe_table for the current state.\n", a.earlier)
	}
	return b.String()
}

func histChars(h [][]msg) int {
	n := 0
	for _, t := range h {
		for _, m := range t {
			n += m.chars()
		}
	}
	return n
}

// cutHistory applies SPEC 3.6 before a new turn: previous tool results are previews; past
// 8 turns or 24k tokens (estimated as chars/4) the window is cut to the last 3 turns and tool
// results of previous turns become stubs.
func (a *Agent) cutHistory() {
	for _, t := range a.history {
		for i := range t {
			if t[i].Role == "tool" && len(t[i].Text) > previewChars {
				t[i].Text = t[i].Text[:previewChars] + "\n[preview; the full result is not shown]"
			}
		}
	}
	if len(a.history) <= historyMaxTurn && histChars(a.history)/4 <= historyMaxTok {
		return
	}
	a.cuts++
	if len(a.history) > historyMinTurn {
		a.earlier += len(a.history) - historyMinTurn
		a.history = a.history[len(a.history)-historyMinTurn:]
	}
	for _, t := range a.history {
		for i := range t {
			if t[i].Role == "tool" && !t[i].Stub {
				first, _, _ := strings.Cut(t[i].Text, "\n")
				t[i].Text, t[i].Stub = "[tool result stub] "+short(first, 160), true
			}
		}
	}
}

func needsMet(t *Task, p *Project, writes []WriteRec) []string {
	var miss []string
	for _, n := range t.Needs {
		if n == "migration" {
			ok := false
			for _, w := range writes {
				if w.Tool == "apply_migration" && w.OK {
					ok = true
				}
			}
			if !ok {
				miss = append(miss, "the migration (apply_migration)")
			}
			continue
		}
		ok := false
		for _, w := range writes {
			if w.Artifact == n && w.OK {
				ok = true
			}
		}
		if !ok || p.Configs[n] == nil {
			miss = append(miss, n)
		}
	}
	return miss
}

// turn runs one task as one user turn.
func (a *Agent) turn(ctx context.Context, t *Task, res *RunResult) {
	t0 := time.Now()
	defer func() { res.Seconds = time.Since(t0).Seconds() }()
	a.cutHistory()
	res.Cuts = a.cuts
	res.PeakHistoryChars = histChars(a.history)
	cur := []msg{{Role: "user", Text: t.Prompt}}
	a.logf("===== %s %s guide %s run %d chained %v\n----- user\n%s\n", res.Model, t.ID, a.GuideID, res.Run, res.Chained, t.Prompt)
	failed := 0
	batch := 0
	defer func() { a.history = append(a.history, cur) }()
	capReq := maxRequests
	if a.Probe {
		capReq = probeMaxReq
	}
	for req := 0; ; req++ {
		if req >= capReq {
			res.Stop = "request cap"
			break
		}
		if overBudget() {
			res.Stop = "budget"
			break
		}
		msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(a.system())}
		for _, h := range a.history {
			for _, m := range h {
				msgs = append(msgs, toOAI(m))
			}
		}
		for _, m := range cur {
			msgs = append(msgs, toOAI(m))
		}
		r := stream(ctx, a.M, Req{Label: fmt.Sprintf("%s/r%d/q%d", t.ID, res.Run, req+1), Messages: msgs, Tools: a.tools(), MaxTokens: maxOutTokens})
		res.Requests++
		res.ReqPrompts = append(res.ReqPrompts, r.Prompt)
		res.Prompt += r.Prompt
		res.Output += r.Output
		if r.Prompt > res.PeakPrompt {
			res.PeakPrompt = r.Prompt
		}
		spend(r.Prompt, r.Output)
		if r.Err != nil {
			res.Err = short(r.ErrText, 600)
			res.Stop = "error"
			if a.M.Stopped() != "" {
				res.Stop = "quota"
			}
			a.logf("----- error\n%s\n", res.Err)
			break
		}
		a.logf("----- assistant (finish %s, prompt %d, output %d, %.1fs)\n", r.Finish, r.Prompt, r.Output, r.Total.Seconds())
		if r.Reasoning != "" {
			a.logf("[reasoning %d chars]\n", len(r.Reasoning))
		}
		if r.Text != "" {
			a.logf("%s\n", r.Text)
		}
		for _, c := range r.Calls {
			a.logf("> %s %s\n", c.Name, c.Args)
		}
		cur = append(cur, msg{Role: "assistant", Text: r.Text, Calls: r.Calls})
		if len(r.Calls) == 0 {
			res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "answer"})
			if a.Probe {
				res.Stop = "answer"
				break
			}
			miss := needsMet(t, a.P, res.Writes)
			if len(miss) == 0 {
				res.Stop = "done"
				break
			}
			if res.Nudged {
				res.Stop = "gave up"
				break
			}
			res.Nudged = true
			nudge := "Not finished: " + strings.Join(miss, ", ") + " not saved yet. Use the tools now and save it; do not ask for confirmation (destructive steps are approved)."
			if r.Finish == "length" {
				nudge = "Your answer was cut off (output limit). " + nudge + " Send one tool call at a time if the configs are long."
			}
			cur = append(cur, msg{Role: "user", Text: nudge})
			a.logf("----- user (nudge)\n%s\n", nudge)
			continue
		}
		batch++
		batchFailed := false
		wrote := false
		for _, c := range r.Calls {
			var out string
			var wc *WriteCall
			switch c.Name {
			case "load_skill":
				out = a.loadSkill(c.Args, req+1, res)
			case "web_search":
				out = fakeSearch(c.Args)
				res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "search", Name: c.Name})
			default:
				out, wc = a.P.callTool(c.Name, c.Args)
				if wc != nil {
					res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "write", Name: c.Name, OK: wc.OK})
					wrote = true
				} else {
					res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "read", Name: c.Name})
				}
			}
			if a.Cond == "C" { // load_with: the first call of the tool also returns its skills
				for _, sk := range loadWith[c.Name] {
					if a.loaded[sk] == "" {
						a.markLoaded(sk, "load_with")
						res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "load_with", Name: sk})
						out += "\n\n" + skillText(sk)
					}
				}
			}
			a.logf("< %s\n%s\n", c.Name, out)
			cur = append(cur, msg{Role: "tool", Text: out, CallID: c.ID})
			if wc == nil {
				continue
			}
			w := WriteRec{Tool: wc.Tool, Artifact: wc.Artifact, OK: wc.OK, Batch: batch, Updated: wc.Updated}
			for _, i := range wc.Issues {
				w.Issues = append(w.Issues, IssueRec{Cat: i.Cat, Msg: i.String(), Warn: i.Warn})
			}
			res.Writes = append(res.Writes, w)
			if !wc.OK {
				batchFailed = true
			}
		}
		if a.Probe && wrote {
			res.Stop = "write"
			break
		}
		if batchFailed {
			failed++
			if failed > maxRepairs {
				res.Stop = "repair limit"
				break
			}
		}
	}
	res.FailedBatches = failed
	for _, n := range a.order {
		res.Loaded = append(res.Loaded, n+":"+a.loaded[n])
	}
	if t.Check == nil { // T-load probe: no task assertions
		a.logf("----- result: stop %s, loaded %v, events %d\n", res.Stop, res.Loaded, len(res.Events))
		return
	}
	// A failed save of an artifact that was later saved under another tool (save_view vs
	// save_form) or fixed counts as fixed: look at the artifact id only.
	byArt := map[string]bool{}
	for _, w := range res.Writes {
		byArt[w.Artifact] = w.OK
		if w.OK {
			for _, u := range w.Updated {
				byArt[depID(u)] = true
			}
		}
	}
	valid := len(res.Writes) > 0 && len(needsMet(t, a.P, res.Writes)) == 0
	for art, ok := range byArt {
		if !ok && art != "?" {
			valid = false
		}
	}
	// a config without a readable id counts as fixed when a later write succeeded
	if n := len(res.Writes); n > 0 && res.Writes[n-1].Artifact == "?" && !res.Writes[n-1].OK {
		valid = false
	}
	res.Valid = valid
	res.FirstTryValid = valid && failed == 0
	if valid {
		res.Rounds = failed
	}
	res.Checks = t.Check(a.P)
	res.Total = len(res.Checks)
	for _, c := range res.Checks {
		if c.OK {
			res.Pass++
		}
	}
	a.logf("----- result: stop %s, valid %v, first-try %v, failed batches %d, checks %d/%d\n", res.Stop, res.Valid, res.FirstTryValid, failed, res.Pass, res.Total)
	for _, c := range res.Checks {
		if !c.OK {
			a.logf("  FAIL %s: %s\n", c.Name, c.Why)
		}
	}
}

func (a *Agent) tools() []openai.ChatCompletionToolUnionParam {
	if a.Cond == "A" {
		return oaiTools
	}
	return oaiToolsBC
}

func (a *Agent) markLoaded(name, how string) {
	if a.loaded == nil {
		a.loaded = map[string]string{}
	}
	a.loaded[name] = how
	a.order = append(a.order, name)
}

// loadSkill is the load_skill tool (SPEC 8.9): the body as the result; at most 6 skills or
// about 10,000 tokens per chat.
func (a *Agent) loadSkill(args string, req int, res *RunResult) string {
	var in struct {
		Name string `json:"name"`
	}
	json.Unmarshal([]byte(args), &in)
	name := strings.TrimSpace(in.Name)
	res.Events = append(res.Events, SkEvent{Req: req, Kind: "load_skill", Name: name})
	if a.Cond == "A" {
		return `error: unknown tool "load_skill"`
	}
	if skills[name] == nil {
		return fmt.Sprintf("error: no skill %q. Skills: %s", name, strings.Join(skillOrder, ", "))
	}
	if a.loaded[name] != "" {
		return fmt.Sprintf("skill %s is already loaded (see the earlier result).", name)
	}
	tok := 0
	for n := range a.loaded {
		tok += estTokens(skills[n].Body)
	}
	if len(a.loaded) >= chatSkillMax || tok+estTokens(skills[name].Body) > chatSkillTokens {
		return fmt.Sprintf("error: skill limit reached (6 skills or 10,000 tokens per chat). Loaded: %s", strings.Join(a.order, ", "))
	}
	a.markLoaded(name, "load_skill")
	return skillText(name)
}

func newResult(m *Model, task, guide string, run int, chained bool) *RunResult {
	return &RunResult{Model: m.ID, Task: task, Guide: guide, Run: run, Chained: chained, Started: time.Now().UTC().Format(time.RFC3339)}
}

// runSingle runs one SPIKE-021 task from the reference state of the previous task (T-config).
func runSingle(ctx context.Context, m *Model, cond string, t *Task, rep int) *RunResult {
	res := newResult(m, t.ID, "v2", rep, false)
	res.Test, res.Cond, res.Item, res.Rep = "tconfig", cond, t.ID, rep
	dir, err := os.MkdirTemp("", "spike025-")
	if err != nil {
		res.Stop, res.Err = "error", err.Error()
		return res
	}
	idx := map[string]int{"T1": 1, "T2": 2, "T3": 3, "T4": 4}[t.ID]
	p, err := buildState(filepath.Join(dir, "p"), idx)
	if err != nil {
		os.RemoveAll(dir)
		res.Stop, res.Err = "error", err.Error()
		return res
	}
	defer func() { p.Close(); os.RemoveAll(dir) }()
	a := &Agent{M: m, Cond: cond, Guide: guides["v2"], GuideID: "v2", P: p}
	res.Transcript = a.openTranscript(fmt.Sprintf("tconfig_%s_%s_%s_r%d", cond, m.ID, t.ID, rep))
	defer a.closeTranscript()
	a.turn(ctx, t, res)
	return res
}

// runProbe runs one T-load request on the state after T3 (the full SPEC 9 Bitcoin project),
// stopping at the first write attempt or the final answer.
func runProbe(ctx context.Context, m *Model, cond string, lr *LoadReq, rep int) *RunResult {
	res := newResult(m, lr.ID, "v2", rep, false)
	res.Test, res.Cond, res.Item, res.Rep = "tload", cond, lr.ID, rep
	dir, err := os.MkdirTemp("", "spike025-")
	if err != nil {
		res.Stop, res.Err = "error", err.Error()
		return res
	}
	p, err := buildState(filepath.Join(dir, "p"), 4)
	if err != nil {
		os.RemoveAll(dir)
		res.Stop, res.Err = "error", err.Error()
		return res
	}
	defer func() { p.Close(); os.RemoveAll(dir) }()
	a := &Agent{M: m, Cond: cond, Probe: true, Guide: guides["v2"], GuideID: "v2", P: p}
	res.Transcript = a.openTranscript(fmt.Sprintf("tload_%s_%s_%s_r%d", cond, m.ID, lr.ID, rep))
	defer a.closeTranscript()
	a.turn(ctx, &Task{ID: lr.ID, Prompt: lr.Prompt}, res)
	return res
}

// depID takes the id from "pipeline daily_btc" / "view btc_price_table".
func depID(u string) string {
	f := strings.Fields(u)
	if len(f) == 0 {
		return u
	}
	return f[len(f)-1]
}
