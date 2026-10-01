package main

// The agent loop: system prompt = guide + project card (rebuilt for every request), the 7
// tools, at most 4 repair rounds per task, one nudge if the model stops early, a request cap,
// token and time tracking, and the SPEC 3.6 history window for the chained conversation.

import (
	"context"
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
	previewChars   = 6000 // ~1,500 tokens (tool_preview_tokens)
	budgetPrompt   = 2_000_000
	budgetReserve  = 20_000 // stop this early so the next request (peak ~15k) cannot cross the cap
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
}

// ---- agent ----

type Agent struct {
	M       *Model
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
	b.WriteString(a.Guide)
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
	for req := 0; ; req++ {
		if req >= maxRequests {
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
		r := stream(ctx, a.M, Req{Label: fmt.Sprintf("%s/r%d/q%d", t.ID, res.Run, req+1), Messages: msgs, Tools: oaiTools, MaxTokens: maxOutTokens})
		res.Requests++
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
		for _, c := range r.Calls {
			out, wc := a.P.callTool(c.Name, c.Args)
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
		if batchFailed {
			failed++
			if failed > maxRepairs {
				res.Stop = "repair limit"
				break
			}
		}
	}
	res.FailedBatches = failed
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

func newResult(m *Model, task, guide string, run int, chained bool) *RunResult {
	return &RunResult{Model: m.ID, Task: task, Guide: guide, Run: run, Chained: chained, Started: time.Now().UTC().Format(time.RFC3339)}
}

// runSingle runs one task from the reference state of the previous task.
func runSingle(ctx context.Context, m *Model, guideID string, t *Task, run int) *RunResult {
	res := newResult(m, t.ID, guideID, run, false)
	dir, err := os.MkdirTemp("", "spike021-")
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
	a := &Agent{M: m, Guide: guides[guideID], GuideID: guideID, P: p}
	res.Transcript = a.openTranscript(fmt.Sprintf("%s_%s_%s_run%d", guideID, m.ID, t.ID, run))
	defer a.closeTranscript()
	a.turn(ctx, t, res)
	return res
}

// runChained runs T1..T4 as one conversation from an empty project. After T1 the seeds are
// added (standing in for data collected since), best effort.
func runChained(ctx context.Context, m *Model, guideID string, run int) []*RunResult {
	var out []*RunResult
	dir, err := os.MkdirTemp("", "spike021-")
	if err != nil {
		return out
	}
	p, err := newProject(filepath.Join(dir, "p"))
	if err != nil {
		os.RemoveAll(dir)
		return out
	}
	defer func() { p.Close(); os.RemoveAll(dir) }()
	a := &Agent{M: m, Guide: guides[guideID], GuideID: guideID, P: p}
	tp := a.openTranscript(fmt.Sprintf("%s_%s_chained_run%d", guideID, m.ID, run))
	defer a.closeTranscript()
	for i := range tasks {
		t := &tasks[i]
		res := newResult(m, t.ID, guideID, run, true)
		res.Transcript = tp
		if overBudget() {
			res.Stop = "budget"
			out = append(out, res)
			break
		}
		a.turn(ctx, t, res)
		if t.ID == "T1" {
			res.SeedNotes = seed(p)
			a.logf("----- seeds added (%d problems)\n", len(res.SeedNotes))
		}
		out = append(out, res)
		if res.Stop == "quota" || res.Stop == "budget" {
			break
		}
	}
	return out
}

// ---- guide size calibration ----

type Calib struct {
	Model, Guide                     string
	Baseline, WithGuide, WithTools   int
	GuideTokens, ToolTokens          int
	GuideChars, ToolChars, CardChars int
	Err                              string `json:",omitempty"`
}

func calibrate(ctx context.Context, m *Model, guideID string) Calib {
	c := Calib{Model: m.ID, Guide: guideID, GuideChars: len(guides[guideID]), ToolChars: toolBytes}
	ask := func(sys string, tools bool) int {
		msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(sys), openai.UserMessage("Reply with the word ok.")}
		rq := Req{Label: "calibrate", Messages: msgs, MaxTokens: 64}
		if tools {
			rq.Tools = oaiTools
		}
		r := stream(ctx, m, rq)
		spend(r.Prompt, r.Output)
		if r.Err != nil && c.Err == "" {
			c.Err = short(r.ErrText, 300)
		}
		return r.Prompt
	}
	c.Baseline = ask("You are a test.", false)
	c.WithGuide = ask("You are a test.\n\n"+guides[guideID], false)
	c.WithTools = ask("You are a test.", true)
	c.GuideTokens = c.WithGuide - c.Baseline
	c.ToolTokens = c.WithTools - c.Baseline
	return c
}

// depID takes the id from "pipeline daily_btc" / "view btc_price_table".
func depID(u string) string {
	f := strings.Fields(u)
	if len(f) == 0 {
		return u
	}
	return f[len(f)-1]
}
