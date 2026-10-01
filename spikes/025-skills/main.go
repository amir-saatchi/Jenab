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
	"strings"
	"sync"
	"time"
)

//go:embed guides/*.md
var guideFS embed.FS

var guides = map[string]string{}

var (
	flagEnv    = flag.String("env", "../../.env", "path of the .env file (keys are read by this program only)")
	flagCheck  = flag.Bool("check", false, "offline: skill files, guide split, SPEC 9 reference check, prompt sizes")
	flagList   = flag.Bool("list", false, "print the system prompts of every condition (no network)")
	flagCalib  = flag.Bool("calib", false, "measure guide, skill, list and tool sizes in tokens per model")
	flagModels = flag.String("models", "nemotron-3-ultra,gemma4:31b,glm-4.5-flash", "model IDs")
	flagTests  = flag.String("tests", "", "tests to run: tload,tconfig,troles (empty: none)")
	flagConds  = flag.String("conds", "A,B,C", "conditions for tload/tconfig (troles always uses Mother, M)")
	flagReps   = flag.String("reps", "1", "repetitions, e.g. 1,2")
	flagItems  = flag.String("items", "", "only these items (task IDs T2..T4, request IDs or ID prefixes like L01, R3)")
	flagPlan   = flag.String("plan", "", "all: the spike's full run order (phases 1-5, see README)")
	flagSmoke  = flag.Bool("smoke", false, "write results to results/smoke.jsonl (counted in the budget, not in the tables)")
	flagReport = flag.Bool("report", false, "write the measured part of results.md from results/*.jsonl")
	flagScan   = flag.Bool("scan", false, "secret scan of the spike folder (file names only)")
)

var (
	out      *redactWriter
	resMu    sync.Mutex
	runLog   = filepath.Join("results", "runs.jsonl")
	calLog   = filepath.Join("results", "calib.jsonl")
	smokeLog = filepath.Join("results", "smoke.jsonl")
)

func loadGuides() {
	ents, _ := guideFS.ReadDir("guides")
	for _, e := range ents {
		b, _ := guideFS.ReadFile("guides/" + e.Name())
		id := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "guide_"), ".md")
		guides[id] = strings.ReplaceAll(string(b), "\r\n", "\n") // LF everywhere (the 021 file on disk has CRLF)
	}
	intro = guides["intro"]
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

// priorSpend adds the prompt tokens of earlier invocations (runs, calibration, smoke) to the budget.
func priorSpend() {
	for _, path := range []string{runLog, calLog, smokeLog} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r struct{ Prompt, Output int }
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				budgetUsed += r.Prompt
				budgetOut += r.Output
			}
		}
		f.Close()
	}
}

func runKey(test, model, cond, item string, rep int) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d", test, model, cond, item, rep)
}

// doneKeys: combinations already in runs.jsonl without an error (resume).
func doneKeys() map[string]bool {
	done := map[string]bool{}
	for _, r := range readRuns() {
		if r.Stop == "error" || r.Stop == "quota" || r.Stop == "budget" || r.Stop == "" {
			continue
		}
		done[runKey(r.Test, r.Model, r.Cond, r.Item, r.Rep)] = true
	}
	return done
}

func split(s string) []string {
	var outL []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			outL = append(outL, x)
		}
	}
	return outL
}

func itemWanted(id string) bool {
	its := split(*flagItems)
	if len(its) == 0 {
		return true
	}
	for _, x := range its {
		if id == x || strings.HasPrefix(id, x+"_") || strings.HasPrefix(id, x) {
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
	loadGuides()
	loadSkills()
	initSchemas()
	initTools()
	yamlHints, cardDeps = true, true // guide v2 harness settings from SPIKE-021, all conditions
	if *flagScan {
		if err := loadEnv(*flagEnv); err != nil {
			fmt.Fprintln(out, "cannot read the .env file:", err)
			os.Exit(1)
		}
		hits := secretScan(".")
		if len(hits) == 0 {
			fmt.Fprintln(out, "secret scan: clean")
		} else {
			fmt.Fprintln(out, "secret scan: key values found in:", strings.Join(hits, ", "))
		}
		return
	}
	if *flagReport {
		writeReport()
		return
	}
	if *flagCheck || *flagList {
		ok := offlineCheck(*flagList)
		if !ok {
			os.Exit(1)
		}
		return
	}
	if err := loadEnv(*flagEnv); err != nil {
		fmt.Fprintln(out, "cannot read the .env file:", err)
		os.Exit(1)
	}
	if len(stripped) > 0 {
		fmt.Fprintf(out, "removed %d OPENAI_* variables from the environment\n", len(stripped))
	}
	priorSpend()
	fmt.Fprintf(out, "prompt tokens used before this invocation: %d of %d\n", budgetUsed, budgetPrompt)
	if budgetUsed >= budgetPrompt-budgetReserve {
		fmt.Fprintln(out, "budget reached; nothing to do")
		return
	}
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
	done := doneKeys()
	var wg sync.WaitGroup
	for _, p := range providers {
		if len(p.Models) == 0 {
			continue
		}
		wg.Add(1)
		go func(p *Prov) {
			defer wg.Done()
			lane(ctx, p, done)
		}(p)
	}
	wg.Wait()
	fmt.Fprintf(out, "\nprompt tokens used in total: %d of %d (output %d)%s\n", budgetUsed, budgetPrompt, budgetOut,
		map[bool]string{true: " - BUDGET REACHED, stopped", false: ""}[overBudget()])
	for _, e := range rlLog {
		fmt.Fprintf(out, "rate/error log: %s %s attempt %d status %d: %s -> %s\n", e.Model, e.Label, e.Attempt, e.Status, short(e.Msg, 200), e.Action)
	}
}

type job struct {
	test, cond, item string
	rep              int
	m                *Model
}

// lane runs one provider's jobs one after another (Ollama: one request at a time), ordered by
// rep, test, item, condition and model, skipping combinations already done.
func lane(ctx context.Context, p *Prov, done map[string]bool) {
	if *flagCalib {
		for _, m := range p.Models {
			if overBudget() {
				return
			}
			c := calibrate(ctx, m)
			appendJSON(calLog, c)
			fmt.Fprintf(out, "calib %s: baseline %d, %v %s\n", m.ID, c.Baseline, c.Tokens, c.Err)
		}
	}
	var jobs []job
	for _, ph := range phases() {
		jobs = append(jobs, phaseJobs(p, ph)...)
	}
	for _, j := range jobs {
		if overBudget() || j.m.Stopped() != "" {
			continue
		}
		if !*flagSmoke && done[runKey(j.test, j.m.ID, j.cond, j.item, j.rep)] {
			continue
		}
		var r *RunResult
		switch j.test {
		case "tload":
			r = runProbe(ctx, j.m, j.cond, loadReqByID(j.item), j.rep)
		case "tconfig":
			r = runSingle(ctx, j.m, j.cond, taskByID(j.item), j.rep)
		case "troles":
			r = runRole(ctx, j.m, roleReqByID(j.item), j.rep)
		}
		if *flagSmoke {
			appendJSON(smokeLog, r)
		} else {
			appendJSON(runLog, r)
		}
		progress(r)
	}
}

type phase struct{ tests, conds, reps string }

// phases: -plan all is the spike's run order (every lane goes through it on its own, so the
// Z.ai lane does not wait for the slower Ollama lane); otherwise one phase from the flags.
func phases() []phase {
	if *flagPlan == "all" {
		return []phase{
			{"tload,troles", "B,C", "1"},
			{"tconfig", "A,B,C", "1"},
			{"tload", "A", "1"},
			{"tconfig", "A,B,C", "2"},
			{"tload,troles", "B,C", "2"},
		}
	}
	return []phase{{*flagTests, *flagConds, *flagReps}}
}

func phaseJobs(p *Prov, ph phase) []job {
	var jobs []job
	for _, rs := range split(ph.reps) {
		rep := atoiDef(rs)
		for _, test := range split(ph.tests) {
			var items []string
			conds := split(ph.conds)
			switch test {
			case "tload":
				for _, r := range loadReqs {
					items = append(items, r.ID)
				}
			case "tconfig":
				items = []string{"T2", "T3", "T4"}
			case "troles":
				for _, r := range roleReqs {
					items = append(items, r.ID)
				}
				conds = []string{"M"}
			default:
				fmt.Fprintln(out, "unknown test", test)
				continue
			}
			for _, it := range items {
				if !itemWanted(it) {
					continue
				}
				for _, c := range conds {
					for _, m := range p.Models {
						jobs = append(jobs, job{test, c, it, rep, m})
					}
				}
			}
		}
	}
	return jobs
}

func atoiDef(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func progress(r *RunResult) {
	resMu.Lock()
	defer resMu.Unlock()
	extra := ""
	switch r.Test {
	case "tconfig":
		extra = fmt.Sprintf("valid=%v first=%v failed=%d checks=%d/%d", r.Valid, r.FirstTryValid, r.FailedBatches, r.Pass, r.Total)
	case "troles":
		if r.Role != nil {
			extra = fmt.Sprintf("route=%s skills=%v score=%s", r.Role.Route, r.Role.Skills, r.Role.Score)
		}
	case "tload":
		if lr := loadReqByID(r.Item); lr != nil {
			s := scoreLoad(*r, lr)
			extra = fmt.Sprintf("gate=%s rightBefore=%v unneeded=%v", s.Gate, s.RightBefore, s.Unneeded)
		}
	}
	fmt.Fprintf(out, "%s %-16s %-7s %-4s %-20s r%d: stop=%s loaded=%v %s req=%d prompt=%d peak=%d %.0fs budget=%d %s\n",
		time.Now().Format("15:04:05"), r.Model, r.Test, r.Cond, r.Item, r.Rep, r.Stop, r.Loaded, extra,
		r.Requests, r.Prompt, r.PeakPrompt, r.Seconds, budgetUsed, short(r.Err, 160))
	out.Flush()
}

// offlineCheck: skill files parse and stay within the limits, the split gives guide v2 back
// byte for byte, the SPEC 9 reference passes, and the system prompts are printed with sizes.
func offlineCheck(list bool) bool {
	ok := true
	say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }
	for _, n := range skillOrder {
		s := skills[n]
		say("skill %-16s desc %3d chars, body %5d chars (~%d tokens), load_with %v", n, len([]rune(s.Description)), len(s.Body), estTokens(s.Body), s.LoadWith)
	}
	if g := rebuildGuide(); g != guides["v2"] {
		ok = false
		say("SPLIT MISMATCH: intro + skills (%d chars) != guide v2 (%d chars)", len(g), len(guides["v2"]))
	} else {
		say("split check: intro + migrations(+database-design) + pipelines + config-guide + sql-queries == guide v2 (%d bytes)", len(g))
	}
	say("skill list: %d chars", len(skillList(nil)))
	p, err := buildState(filepath.Join(os.TempDir(), fmt.Sprintf("spike025-check-%d", time.Now().UnixNano())), 4)
	if err != nil {
		say("state: %v", err)
		return false
	}
	defer func() { p.Close(); os.RemoveAll(p.Dir) }()
	for _, c := range []string{"A", "B"} {
		a := &Agent{Cond: c, Guide: guides["v2"], P: p}
		s := a.system()
		say("system prompt %s (state after T3): %d chars", c, len(s))
		if list {
			say("===== %s\n%s", c, s)
		}
	}
	ms := motherSystem()
	say("system prompt Mother: %d chars", len(ms))
	if list {
		say("===== Mother\n%s", ms)
	}
	if !refCheck() {
		ok = false
	}
	return ok
}
