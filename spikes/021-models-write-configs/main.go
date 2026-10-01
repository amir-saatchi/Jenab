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
	flagEnv     = flag.String("env", "../../.env", "path of the .env file (keys are read by this program only)")
	flagAll     = flag.Bool("all", false, "reproduce everything: reference check, calibration, 3 runs x 4 tasks, one chained run, report")
	flagRef     = flag.Bool("ref", false, "validate the SPEC 9 reference configs and assertions (no network)")
	flagCalib   = flag.Bool("calib", false, "measure guide and tool sizes in tokens per model")
	flagModels  = flag.String("models", "nemotron-3-ultra,gemma4:31b,glm-4.5-flash", "model IDs")
	flagTasks   = flag.String("tasks", "T1,T2,T3,T4", "tasks")
	flagRuns    = flag.String("runs", "", "run numbers for single tasks, e.g. 1,2,3 (empty: none)")
	flagChained = flag.String("chained", "", "run numbers for the chained conversation, e.g. 1 (empty: none)")
	flagGuide   = flag.String("guide", "v1", "guide version (guides/guide_<v>.md)")
	flagReport  = flag.Bool("report", false, "write results.md from results/*.jsonl")
	flagScan    = flag.Bool("scan", false, "secret scan of the spike folder")
)

var (
	out    *redactWriter
	resMu  sync.Mutex
	runLog = filepath.Join("results", "runs.jsonl")
	calLog = filepath.Join("results", "calib.jsonl")
)

func loadGuides() {
	ents, _ := guideFS.ReadDir("guides")
	for _, e := range ents {
		b, _ := guideFS.ReadFile("guides/" + e.Name())
		id := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "guide_"), ".md")
		guides[id] = string(b)
	}
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

// priorSpend adds the prompt tokens of earlier invocations to the budget.
func priorSpend() {
	for _, path := range []string{runLog, calLog, filepath.Join("results", "smoke.jsonl")} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var r struct {
				Prompt, Output                 int
				Baseline, WithGuide, WithTools int
			}
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				budgetUsed += r.Prompt + r.Baseline + r.WithGuide + r.WithTools
				budgetOut += r.Output
			}
		}
		f.Close()
	}
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

func main() {
	flag.Parse()
	out = &redactWriter{w: os.Stdout}
	defer out.Flush()
	stripped := stripOpenAIEnv()
	loadGuides()
	initSchemas()
	initTools()
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
	if *flagAll {
		*flagRef, *flagCalib, *flagRuns, *flagChained = true, true, "1,2,3", "1"
	}
	if *flagRef {
		if !refCheck() {
			os.Exit(1)
		}
		if !*flagAll && !*flagCalib && *flagRuns == "" && *flagChained == "" {
			return
		}
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
	if guides[*flagGuide] == "" {
		fmt.Fprintln(out, "unknown guide", *flagGuide)
		os.Exit(1)
	}
	if *flagGuide != "v1" {
		yamlHints, cardDeps = true, true
	}
	ctx := context.Background()
	// models
	var models []*Model
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
			models = append(models, m)
		}
	}
	// lanes: one per provider (Ollama free plan: one request at a time)
	var wg sync.WaitGroup
	for _, p := range providers {
		if len(p.Models) == 0 {
			continue
		}
		wg.Add(1)
		go func(p *Prov) {
			defer wg.Done()
			lane(ctx, p)
		}(p)
	}
	wg.Wait()
	fmt.Fprintf(out, "\nprompt tokens used in total: %d of %d (output %d)%s\n", budgetUsed, budgetPrompt, budgetOut,
		map[bool]string{true: " - BUDGET REACHED, stopped", false: ""}[overBudget()])
	for _, e := range rlLog {
		fmt.Fprintf(out, "rate/error log: %s %s attempt %d status %d: %s -> %s\n", e.Model, e.Label, e.Attempt, e.Status, short(e.Msg, 200), e.Action)
	}
	if *flagAll {
		writeReport()
	}
}

func lane(ctx context.Context, p *Prov) {
	g := *flagGuide
	if *flagCalib {
		for _, m := range p.Models {
			c := calibrate(ctx, m, g)
			c.CardChars = 0
			appendJSON(calLog, c)
			fmt.Fprintf(out, "calib %s %s: guide %d tokens (%d chars), tools %d tokens (%d chars) %s\n", m.ID, g, c.GuideTokens, c.GuideChars, c.ToolTokens, c.ToolChars, c.Err)
		}
	}
	for _, run := range split(*flagRuns) {
		n := atoiDef(run)
		for _, tid := range split(*flagTasks) {
			t := taskByID(tid)
			for _, m := range p.Models {
				if overBudget() || m.Stopped() != "" {
					continue
				}
				r := runSingle(ctx, m, g, t, n)
				appendJSON(runLog, r)
				progress(r)
			}
		}
	}
	for _, run := range split(*flagChained) {
		n := atoiDef(run)
		for _, m := range p.Models {
			if overBudget() || m.Stopped() != "" {
				continue
			}
			for _, r := range runChained(ctx, m, g, n) {
				appendJSON(runLog, r)
				progress(r)
			}
		}
	}
}

func atoiDef(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func progress(r *RunResult) {
	resMu.Lock()
	defer resMu.Unlock()
	ch := ""
	if r.Chained {
		ch = " chained"
	}
	fmt.Fprintf(out, "%s %-16s %s run %d%s: stop=%s valid=%v first=%v failedBatches=%d checks=%d/%d req=%d prompt=%d peak=%d %.0fs budget=%d %s\n",
		time.Now().Format("15:04:05"), r.Model, r.Task, r.Run, ch, r.Stop, r.Valid, r.FirstTryValid, r.FailedBatches, r.Pass, r.Total,
		r.Requests, r.Prompt, r.PeakPrompt, r.Seconds, budgetUsed, short(r.Err, 160))
	out.Flush()
}

// refCheck validates the SPEC 9 example through the tools and runs the assertions on the
// reference states; all must pass before any model run.
func refCheck() bool {
	ok := true
	say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }
	tmp, _ := os.MkdirTemp("", "spike021-ref-")
	defer os.RemoveAll(tmp)
	report := func(name string, cs []Check) {
		for _, c := range cs {
			if !c.OK {
				ok = false
				say("  FAIL %s: %s", c.Name, c.Why)
			}
		}
		say("%s: %d checks", name, len(cs))
	}
	// T1
	p, err := newProject(filepath.Join(tmp, "t1"))
	if err != nil {
		say("project: %v", err)
		return false
	}
	res, wc := p.callTool("apply_migration", refT1)
	say("T1 migration (SPEC 9.1):\n%s", res)
	if wc == nil || !wc.OK {
		ok = false
	}
	report("T1 assertions on the reference", checkT1(p))
	p.Close()
	for i := 2; i <= 4; i++ {
		p, err := buildState(filepath.Join(tmp, fmt.Sprintf("s%d", i)), i)
		if err != nil {
			say("state before T%d: %v", i, err)
			return false
		}
		say("state before T%d built (reference configs of the earlier tasks validated)", i)
		switch i {
		case 2:
			r, _ := p.callTool("save_pipeline", yamlJSON(refPipeline))
			say("T2 pipeline (SPEC 9.2):\n%s", r)
			report("T2 assertions on the reference", checkT2(p))
		case 3:
			for _, v := range refViewsT3 {
				tool := "save_view"
				if strings.Contains(v, "type: form") {
					tool = "save_form"
				}
				r, _ := p.callTool(tool, yamlJSON(v))
				say("T3 %s:\n%s", tool, r)
			}
			report("T3 assertions on the reference", checkT3(p))
		case 4:
			r, wc := p.callTool("apply_migration", refT4Args())
			say("T4 migration + dependents (SPEC 9.5):\n%s", r)
			if wc == nil || !wc.OK {
				ok = false
			}
			report("T4 assertions on the reference", checkT4(p))
		}
		p.Close()
	}
	say("reference check: %v", map[bool]string{true: "all passed", false: "FAILED"}[ok])
	return ok
}
