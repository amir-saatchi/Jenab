package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed prompts/*.md
var promptFS embed.FS

var prompts = map[string]string{}

var chatListText string

var (
	flagEnv     = flag.String("env", "../../.env", "path of the .env file (keys are read by this program only)")
	flagModels  = flag.String("models", "nemotron-3-ultra,gemma4:31b,glm-4.5-flash", "model IDs")
	flagScen    = flag.String("scen", "", "scenario IDs or ID prefixes, comma-separated (empty: all)")
	flagConds   = flag.String("conds", "rule,base", "conditions: rule (SPEC 8.3 rule, and for Mother the delegation guidance) and base")
	flagReps    = flag.String("reps", "1,2,3", "repetition numbers")
	flagBudget  = flag.Int("budget", 2_000_000, "prompt-token cap, including earlier runs in results/runs.jsonl")
	flagOut     = flag.String("out", filepath.Join("results", "runs.jsonl"), "where runs are appended")
	flagReport  = flag.Bool("report", false, "rewrite the measured part of results.md from results/runs.jsonl")
	flagScan    = flag.Bool("scan", false, "secret scan of this folder")
	flagList    = flag.Bool("list", false, "list the scenarios and the system prompts, no network")
	flagSkip    = flag.Bool("skip-done", true, "skip model/scenario/cond/rep combinations already in the output file")
	flagRescore = flag.Bool("rescore", false, "recompute the assertions of the stored runs (no network)")
)

var (
	out   *redactWriter
	resMu sync.Mutex
)

func loadPrompts() {
	ents, _ := promptFS.ReadDir("prompts")
	for _, e := range ents {
		b, _ := promptFS.ReadFile("prompts/" + e.Name())
		prompts[strings.TrimSuffix(e.Name(), ".md")] = strings.TrimSpace(string(b))
	}
	chatListText = prompts["chats"]
}

// systemPrompt builds block 1 (system prompt) and block 4 (project card, chat list) of SPEC 3.1.
// There is no user or project memory and no session notes in this spike.
func systemPrompt(ctxName, cond string) string {
	parts := []string{prompts["base"]}
	if ctxName == "mother" {
		parts = append(parts, prompts["mother"])
	}
	if cond == "rule" || cond == "rule2" {
		parts = append(parts, prompts["rule"])
		if ctxName == "mother" {
			if cond == "rule2" {
				parts = append(parts, prompts["delegation_v2"]) // follow-up: role check first
			} else {
				parts = append(parts, prompts["delegation"])
			}
		}
	}
	parts = append(parts, prompts["card"])
	if ctxName == "mother" {
		parts = append(parts, prompts["chats"])
	}
	return strings.Join(parts, "\n\n")
}

// ---- budget ----

var (
	budgetMu   sync.Mutex
	budgetUsed int
	budgetOut  int
	budgetHit  bool
)

func spend(prompt, output int) {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	budgetUsed += prompt
	budgetOut += output
	if budgetUsed >= *flagBudget-15_000 {
		budgetHit = true
	}
}

func overBudget() bool {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	return budgetHit
}

// ---- run records ----

type MsgRec struct {
	Idx  int
	Text string
	At   float64
	How  string
}

type RunRec struct {
	Model, Scenario, Cond string
	Rep                   int
	Tests                 []string
	Context               string
	Started               string
	SystemPromptChars     int
	Msgs                  []MsgRec
	Turns                 []*Turn
	Tasks                 []*BgTask
	Asserts               []AssertRes
	TestStatus            map[string]string
	Prompt, Output        int
	Requests              int
	PeakPrompt            int
	FirstPrompt           int
	Polls                 int
	Seconds               float64
	VirtualEnd            float64
	Err                   string `json:",omitempty"`
}

func appendJSON(path string, v any) {
	resMu.Lock()
	defer resMu.Unlock()
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(out, "cannot write", path, err)
		return
	}
	defer f.Close()
	b, _ := json.Marshal(v)
	f.Write([]byte(redact(string(b)) + "\n"))
}

func readRuns(path string) []*RunRec {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var runs []*RunRec
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var r RunRec
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			runs = append(runs, &r)
		}
	}
	return runs
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func runOne(ctx context.Context, m *Model, llm LLM, sc *Scenario, cond string, rep int) *RunRec {
	rec := &RunRec{Model: m.ID, Scenario: sc.ID, Cond: cond, Rep: rep, Tests: sc.Tests, Context: sc.Context, Started: time.Now().UTC().Format(time.RFC3339)}
	db, err := openFixtureDB()
	if err != nil {
		rec.Err = "fixture: " + err.Error()
		return rec
	}
	defer db.Close()
	sys := systemPrompt(sc.Context, cond)
	rec.SystemPromptChars = len(sys)
	o := newOrch(sc, llm, sys, db)
	o.label = fmt.Sprintf("%s/%s/%s/r%d", m.ID, sc.ID, cond, rep)
	o.onCall = func(r Result) { spend(r.Prompt, r.Output) }
	t0 := time.Now()
	o.Run(ctx)
	rec.Seconds = time.Since(t0).Seconds()
	finishRec(rec, o, sc)
	writeTranscript(rec, sys)
	return rec
}

func finishRec(rec *RunRec, o *Orch, sc *Scenario) {
	rec.Turns, rec.Tasks, rec.Polls, rec.VirtualEnd, rec.Err = o.turns, o.tasks, o.polls, o.now, o.err
	for _, m := range o.msgs {
		rec.Msgs = append(rec.Msgs, MsgRec{m.idx, m.text, m.at, m.how})
	}
	for _, t := range o.turns {
		for _, r := range t.Requests {
			rec.Requests++
			rec.Prompt += r.Prompt
			rec.Output += r.Output
			if r.Prompt > rec.PeakPrompt {
				rec.PeakPrompt = r.Prompt
			}
			if rec.FirstPrompt == 0 {
				rec.FirstPrompt = r.Prompt
			}
		}
	}
	as := sc.Asserts
	hasMax := false
	for _, a := range as {
		if a.Type == "max_prompt_tokens" {
			hasMax = true
		}
	}
	if !hasMax {
		as = append(append([]Assert{}, as...), Assert{Type: "max_prompt_tokens", Test: "5", N: 8000})
	}
	if rec.Err != "" {
		rec.Asserts = []AssertRes{{Type: "run", Test: "info", Status: "skip", Why: "run error: " + short(rec.Err, 200)}}
		rec.TestStatus = map[string]string{}
		return
	}
	rec.Asserts = checkAll(o, as, rec.PeakPrompt)
	rec.TestStatus = testStatus(rec.Asserts)
}

func writeTranscript(rec *RunRec, sys string) {
	dir := filepath.Join("results", "transcripts")
	os.MkdirAll(dir, 0o755)
	name := unsafeName.ReplaceAllString(fmt.Sprintf("%s_%s_%s_r%d", rec.Scenario, rec.Cond, rec.Model, rec.Rep), "_") + ".txt"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return
	}
	defer f.Close()
	w := &redactWriter{w: f}
	defer w.Flush()
	fmt.Fprintf(w, "===== %s | %s | cond %s | rep %d | tests %v\n", rec.Model, rec.Scenario, rec.Cond, rec.Rep, rec.Tests)
	fmt.Fprintf(w, "system prompt: %d chars (see prompts/)\n", len(sys))
	for _, t := range rec.Turns {
		fmt.Fprintf(w, "\n----- turn %d (%s) %s-%s, stop %s\n", t.N, t.Kind, clock(t.Start), clock(t.End), t.Stop)
		ri := 0
		lastResp := -1
		for _, p := range t.Parts {
			if (p.Kind == "text" || p.Kind == "call") && p.Resp != lastResp {
				lastResp = p.Resp
				if ri < len(t.Requests) {
					r := t.Requests[ri]
					fmt.Fprintf(w, "  (request: prompt %d, output %d, %.1fs, finish %s, reasoning %d chars)\n", r.Prompt, r.Output, r.Seconds, r.Finish, r.ReasoningChars)
				}
				ri++
			}
			switch p.Kind {
			case "user", "mid_user":
				fmt.Fprintf(w, "[%s %s] %s\n", clock(p.T), p.Kind, p.Text)
			case "notice":
				fmt.Fprintf(w, "[%s notice] %s\n", clock(p.T), p.Text)
			case "text":
				fmt.Fprintf(w, "[%s assistant] %s\n", clock(p.T), p.Text)
			case "call":
				fmt.Fprintf(w, "[%s call] %s %s\n", clock(p.T), p.Tool, p.Args)
			case "result":
				fmt.Fprintf(w, "[%s result %s] %s\n", clock(p.T), p.Tool, short(p.Text, 300))
			}
		}
		for ; ri < len(t.Requests); ri++ {
			r := t.Requests[ri]
			fmt.Fprintf(w, "  (request: prompt %d, output %d, %.1fs, finish %s, err %s)\n", r.Prompt, r.Output, r.Seconds, r.Finish, r.Err)
		}
	}
	fmt.Fprintf(w, "\n----- background tasks\n")
	for _, t := range rec.Tasks {
		fmt.Fprintf(w, "%s %s %s start %s finish %s delivered %s\n", t.ID, t.Tool, t.Target, clock(t.Start), clock(t.Finish), t.Delivered)
	}
	fmt.Fprintf(w, "\n----- assertions\n")
	for _, a := range rec.Asserts {
		fmt.Fprintf(w, "%-5s test %-4s %-34s %s\n", a.Status, a.Test, a.Type, a.Why)
	}
	fmt.Fprintf(w, "tests: %v | prompt %d output %d requests %d peak %d polls %d | %.0fs real, %.0fs virtual\n",
		rec.TestStatus, rec.Prompt, rec.Output, rec.Requests, rec.PeakPrompt, rec.Polls, rec.Seconds, rec.VirtualEnd)
	if rec.Err != "" {
		fmt.Fprintf(w, "error: %s\n", rec.Err)
	}
}

func split(s string) []string {
	var o []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			o = append(o, x)
		}
	}
	return o
}

func wantScen(id string) bool {
	if *flagScen == "" {
		return true
	}
	for _, p := range split(*flagScen) {
		if id == p || strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

func main() {
	flag.Parse()
	out = &redactWriter{w: os.Stdout}
	defer out.Flush()
	stripped := stripOpenAIEnv()
	loadPrompts()
	initTools()
	scens, err := loadScenarios("scenarios")
	if err != nil {
		fmt.Fprintln(out, "scenarios:", err)
		os.Exit(1)
	}
	if *flagRescore {
		rescore(*flagOut, scens)
		return
	}
	if *flagReport {
		writeReport(readRuns(*flagOut), scens)
		return
	}
	if *flagList {
		for _, s := range scens {
			fmt.Fprintf(out, "%-28s tests %-6s %-6s %s\n", s.ID, strings.Join(s.Tests, ","), s.Context, s.Title)
		}
		for _, c := range []string{"main", "mother"} {
			for _, cond := range []string{"base", "rule"} {
				fmt.Fprintf(out, "\n===== system prompt %s / %s (%d chars)\n%s\n", c, cond, len(systemPrompt(c, cond)), systemPrompt(c, cond))
			}
		}
		return
	}
	if err := loadEnv(*flagEnv); err != nil {
		fmt.Fprintln(out, "cannot read the .env file:", err)
		os.Exit(1)
	}
	if *flagScan {
		hits := secretScan(".")
		if len(hits) == 0 {
			fmt.Fprintln(out, "secret scan: clean")
		} else {
			fmt.Fprintln(out, "secret scan: key values found in:", strings.Join(hits, ", "))
		}
		return
	}
	if len(stripped) > 0 {
		fmt.Fprintf(out, "removed %d OPENAI_* variables from the environment\n", len(stripped))
	}
	prior := readRuns(*flagOut)
	done := map[string]bool{}
	for _, r := range prior {
		budgetUsed += r.Prompt
		budgetOut += r.Output
		if r.Err == "" {
			done[fmt.Sprintf("%s|%s|%s|%d", r.Model, r.Scenario, r.Cond, r.Rep)] = true
		}
	}
	// smoke runs count toward the budget too
	for _, r := range readRuns(filepath.Join("results", "superseded.jsonl")) {
		budgetUsed += r.Prompt
		budgetOut += r.Output
	}
	smoke := filepath.Join("results", "smoke.jsonl")
	for _, r := range readRuns(smoke) {
		if filepath.Clean(*flagOut) == smoke {
			break
		}
		budgetUsed += r.Prompt
		budgetOut += r.Output
	}
	fmt.Fprintf(out, "prompt tokens used before this invocation: %d of %d\n", budgetUsed, *flagBudget)
	ctx := context.Background()
	for _, p := range providers {
		for _, id := range modelIDs[p.Name] {
			for _, want := range split(*flagModels) {
				if want == id {
					p.Models = append(p.Models, &Model{P: p, ID: id})
				}
			}
		}
		if len(p.Models) == 0 {
			continue
		}
		if err := p.setup(ctx); err != nil {
			fmt.Fprintf(out, "%s: %s\n", p.Name, err)
			if p.client == nil {
				p.Models = nil
				continue
			}
		}
		for _, m := range p.Models {
			if len(p.Listed) > 0 && !p.Listed[m.ID] {
				fmt.Fprintf(out, "note: %s is not in %s /models\n", m.ID, p.Name)
			}
		}
	}
	var sel []*Scenario
	for _, s := range scens {
		if wantScen(s.ID) {
			sel = append(sel, s)
		}
	}
	var wg sync.WaitGroup
	for _, p := range providers {
		if len(p.Models) == 0 {
			continue
		}
		wg.Add(1)
		go func(p *Prov) {
			defer wg.Done()
			for _, rs := range split(*flagReps) {
				rep := 0
				fmt.Sscanf(rs, "%d", &rep)
				for _, sc := range sel {
					for _, cond := range split(*flagConds) {
						for _, m := range p.Models {
							if overBudget() || m.Stopped() != "" {
								continue
							}
							key := fmt.Sprintf("%s|%s|%s|%d", m.ID, sc.ID, cond, rep)
							if *flagSkip && done[key] {
								continue
							}
							r := runOne(ctx, m, modelLLM{m}, sc, cond, rep)
							appendJSON(*flagOut, r)
							progress(r)
						}
					}
				}
			}
		}(p)
	}
	wg.Wait()
	fmt.Fprintf(out, "\nprompt tokens used in total: %d of %d (output %d)%s\n", budgetUsed, *flagBudget, budgetOut,
		map[bool]string{true: " - BUDGET REACHED, stopped", false: ""}[overBudget()])
	for _, e := range rlLog {
		fmt.Fprintf(out, "rate/error log: %s %s attempt %d status %d: %s -> %s\n", e.Model, e.Label, e.Attempt, e.Status, short(e.Msg, 200), e.Action)
	}
	for _, p := range providers {
		for _, m := range p.Models {
			if s := m.Stopped(); s != "" {
				fmt.Fprintf(out, "model %s stopped: %s\n", m.ID, s)
			}
		}
	}
}

func progress(r *RunRec) {
	resMu.Lock()
	defer resMu.Unlock()
	var ks []string
	for k := range r.TestStatus {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var ts []string
	for _, k := range ks {
		ts = append(ts, "T"+k+"="+r.TestStatus[k])
	}
	var fails []string
	for _, a := range r.Asserts {
		if a.Status == "fail" {
			fails = append(fails, a.Type)
		}
	}
	fmt.Fprintf(out, "%s %-16s %-26s %-4s r%d: %s | turns %d req %d prompt %d peak %d polls %d %.0fs | budget %d | fails %s %s\n",
		time.Now().Format("15:04:05"), r.Model, r.Scenario, r.Cond, r.Rep, strings.Join(ts, " "), len(r.Turns), r.Requests, r.Prompt, r.PeakPrompt, r.Polls, r.Seconds,
		budgetUsed, strings.Join(fails, ","), short(r.Err, 160))
	out.Flush()
}

// rescore recomputes the assertions of every stored run from its recorded turns, tasks and
// messages (used after an assertion fix; the model output is not touched).
func rescore(path string, scens []*Scenario) {
	byID := map[string]*Scenario{}
	for _, s := range scens {
		byID[s.ID] = s
	}
	runs := readRuns(path)
	var b strings.Builder
	n := 0
	for _, r := range runs {
		if sc := byID[r.Scenario]; sc != nil && r.Err == "" {
			o := &Orch{sc: sc, turns: r.Turns, tasks: r.Tasks, polls: r.Polls, now: r.VirtualEnd}
			for _, m := range r.Msgs {
				o.msgs = append(o.msgs, &pendingMsg{idx: m.Idx, text: m.Text, at: m.At, done: m.How != "", how: m.How})
			}
			r.Msgs = nil
			r.Asserts, r.TestStatus = nil, nil
			r.Requests, r.Prompt, r.Output, r.PeakPrompt, r.FirstPrompt = 0, 0, 0, 0, 0
			finishRec(r, o, sc)
			n++
		}
		j, _ := json.Marshal(r)
		b.WriteString(redact(string(j)) + "\n")
	}
	if err := os.WriteFile(path+".tmp", []byte(b.String()), 0o644); err == nil {
		os.Rename(path+".tmp", path)
	}
	fmt.Fprintf(out, "rescored %d of %d runs in %s\n", n, len(runs), path)
}
