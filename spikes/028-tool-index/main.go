// Command toolindex is SPIKE-028's harness: the scenario runner's sets, and
// long chats, with every tool native (condition 1), with the tool index
// (2), and with the index and history cards (3).
//
//	go run . -sets multi-part,rare-tools -conds 1,2 -reps 3           # scenario sets
//	go run . -long -conds 1,2,3 -reps 3                               # long chats
//	go run . -sets rare-tools -model scripted:scenarios/rare-tools/_offline.yaml   # no keys
//	go run . -report results/<stamp>                                  # results.md again
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/scenario"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// refModels are the models Main-Session chose for this spike: Gemini is
// left out to save its shared quota.
const refModels = "ollama-cloud/gemma4:31b,mistral/ministral-14b-latest"

// setDirs are the sets the spike runs, by name.
var setDirs = map[string]string{
	"multi-part":    filepath.Join("..", "..", "scenarios", "multi-part"),
	"skill-loading": filepath.Join("..", "..", "scenarios", "skill-loading"),
	"rare-tools":    filepath.Join("scenarios", "rare-tools"),
	"long-chats":    filepath.Join("scenarios", "long-chats"),
}

// longSet is the set of long chats: its earlier turns are seeded, and
// condition 3 adds history cards.
const longSet = "long-chats"

// historyMin is the app's default history_min_turns: the turns a cut keeps
// before the current one.
const historyMin = 3

func main() {
	sqlworker.IfWorker() // the runs' projects run agent queries in this executable
	var (
		modelsFile = flag.String("models-file", "models.yaml", "the providers and models")
		envFile    = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys")
		modelList  = flag.String("model", refModels, "models, as provider/id, comma-separated; scripted:<file> plays a script")
		setList    = flag.String("sets", "", "scenario sets to run, comma-separated: multi-part, skill-loading, rare-tools")
		long       = flag.Bool("long", false, "run the long chats")
		condList   = flag.String("conds", "1,2", "conditions: 1 today, 2 index, 3 index and cards, 4 today and cards (3 and 4: long chats)")
		only       = flag.String("scenario", "", "scenario or chat IDs, comma-separated; default all")
		reps       = flag.Int("reps", 3, "runs of each scenario, model and condition")
		longReps   = flag.Int("long-reps", 5, "runs of each long chat, model and condition")
		firstRep   = flag.Int("first-rep", 1, "the number of the first rep, to go on with a later day")
		timeout    = flag.Duration("timeout", 12*time.Minute, "the longest one run may take")
		out        = flag.String("out", "", "the results folder; default results/<time>")
		capGemini  = flag.Int("gemini-cap", 290, "Gemini requests a Pacific day, counted in results/gemini-budget.json")
		reportOnly = flag.String("report", "", "write results.md from these results folders (comma-separated) and stop")
		verbose    = flag.Bool("v", false, "log the app's messages")
	)
	flag.Parse()
	switch {
	case *reportOnly != "":
		dirs := split(*reportOnly)
		if err := writeReport(dirs); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	conds := split(*condList)
	for _, c := range conds {
		if !slices.Contains([]string{"1", "2", "3", "4"}, c) {
			log.Fatalf("no condition %q", c)
		}
	}
	lg := slog.New(slog.DiscardHandler)
	if *verbose {
		lg = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	dir := *out
	if dir == "" {
		dir = filepath.Join("results", time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	b, err := openBudget(filepath.Join("results", "gemini-budget.json"), *capGemini)
	if err != nil {
		log.Fatal(err)
	}
	h := &harness{dir: dir, log: lg, timeout: *timeout, budget: b, only: split(*only), reps: *reps, longReps: *longReps, first: *firstRep, sizes: map[string]size{}}
	models, script, err := pickModels(split(*modelList), *modelsFile, *envFile)
	if err != nil {
		log.Fatal(err)
	}
	h.models, h.script = models, script
	log.Printf("results: %s; models: %s; Gemini requests left today, if run: %d", dir, strings.Join(models.Run, ", "), b.left())
	// One lane per model: each model's runs go one at a time (Ollama's
	// free plan runs one request at a time), the models side by side.
	var wg sync.WaitGroup
	for _, m := range models.Run {
		mctx, cancel := context.WithCancel(ctx)
		b.onStop(providerKind(models, m), cancel)
		wg.Go(func() {
			defer cancel()
			for _, set := range split(*setList) {
				for _, c := range conds {
					if hasCards(c) {
						continue // no cut in a set's short chats, so no cards: 3 is 2 there, 4 is 1
					}
					if mctx.Err() != nil {
						return
					}
					if err := h.runSet(mctx, set, c, m); err != nil {
						log.Printf("%s %s c%s: %v", m, set, c, err)
					}
				}
			}
			if *long {
				for _, c := range conds {
					if mctx.Err() != nil {
						return
					}
					if err := h.runSet(mctx, longSet, c, m); err != nil {
						log.Printf("%s %s c%s: %v", m, longSet, c, err)
					}
				}
			}
		})
	}
	wg.Wait()
	for k, why := range b.all() {
		log.Printf("%s stopped: %s", k, why)
	}
	if err := writeReport([]string{dir}); err != nil {
		log.Fatal(err)
	}
}

type harness struct {
	dir      string
	log      *slog.Logger
	timeout  time.Duration
	budget   *budget
	models   *scenario.Models
	script   *scenario.Script
	only     []string
	reps     int
	longReps int // runs of each long chat
	first    int
	mu       sync.Mutex // runs.jsonl and sizes
	sizes    map[string]size
}

// line is one run in runs.jsonl.
type line struct {
	Kind      string            `json:"kind"` // "set"
	Set       string            `json:"set"`
	Cond      string            `json:"cond"`
	Model     string            `json:"model"`
	Scenario  string            `json:"scenario"`
	Rep       int               `json:"rep"`
	Error     string            `json:"error,omitempty"`
	Skipped   string            `json:"skipped,omitempty"`
	Stopped   bool              `json:"stopped,omitempty"` // a quota or the budget stopped the model during the run: not counted
	Passed    bool              `json:"passed"`
	Task      bool              `json:"task"` // passed without the request and prompt-size checks
	Done      bool              `json:"done"` // task, and every tool its checks need ran without an error
	Requests  int               `json:"requests"`
	Prompt    int               `json:"prompt_tokens"`
	CacheRead int               `json:"cache_read_tokens"`
	Output    int               `json:"output_tokens"`
	Seconds   float64           `json:"seconds"`
	Asserts   []scenario.Result `json:"asserts,omitempty"`
	Calls     calls             `json:"calls"`
	Cards     []int             `json:"cards,omitempty"` // the earlier turns carded (condition 3)
	CardHit   *bool             `json:"card_hit,omitempty"`
}

// sizeChecks are checks that count requests or prompt tokens: the index
// adds a describe_tool request by design, so the task share leaves them out.
var sizeChecks = []string{"max_requests", "max_prompt_tokens"}

// done is a task passed whose tool_called checks each have a call of one
// of their tools that worked: the runner's check also counts a call that
// was refused, for its arguments or for a name the chat doesn't have.
// Results come in the order of the scenario's checks.
func done(sc *scenario.Scenario, r scenario.Run, tr string) bool {
	if !taskPassed(r) {
		return false
	}
	if sc == nil || len(sc.Asserts) != len(r.Asserts) {
		return true
	}
	worked := map[string]bool{}
	for _, c := range parseCalls(tr) {
		if c.err == "" {
			worked[c.tool] = true
		}
	}
	for i, a := range sc.Asserts {
		if a.Type != "tool_called" || a.Test == "info" || r.Asserts[i].Status != scenario.Pass {
			continue
		}
		tools := append(slices.Clone(a.Tools), a.Tool)
		if !slices.ContainsFunc(tools, func(t string) bool { return t != "" && worked[t] }) {
			return false
		}
	}
	return true
}

func taskPassed(r scenario.Run) bool {
	if r.Skipped != "" || r.Error != "" {
		return false
	}
	for _, a := range r.Asserts {
		if a.Status == scenario.Fail && a.Test != "info" && !slices.Contains(sizeChecks, a.Type) {
			return false
		}
	}
	return true
}

func (h *harness) write(l line) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(h.dir, "runs.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("runs.jsonl: %v", err)
		return
	}
	defer f.Close()
	raw, _ := json.Marshal(l)
	f.Write(append(raw, '\n'))
}

// runSet runs one set on one model in one condition.
func (h *harness) runSet(ctx context.Context, name, cond, model string) error {
	src, ok := setDirs[name]
	if !ok {
		return fmt.Errorf("no set %q", name)
	}
	s, err := scenario.Load(src)
	if err != nil {
		return err
	}
	var ids []string
	scens := map[string]*scenario.Scenario{}
	for _, sc := range s.Scenarios {
		if len(h.only) == 0 || slices.Contains(h.only, sc.ID) {
			ids = append(ids, sc.ID)
			scens[sc.ID] = sc
		}
	}
	if len(ids) == 0 {
		return nil
	}
	// Long chats: each scenario's earlier turns, seeded before its message.
	hist := map[string]history{}
	if name == longSet {
		for _, sid := range ids {
			if hist[sid], err = loadHistory(historyDir, sid); err != nil {
				return err
			}
		}
	}
	var mu sync.Mutex
	carded := map[string][]int{} // the earlier turns the app cards, by scenario (conditions 3 and 4)
	reps := h.reps
	if name == longSet {
		reps = h.longReps
	}
	ms := *h.models
	ms.Run = []string{model}
	opt := scenario.Options{
		Models: &ms, Backends: counted(h.budget), Reps: reps, Timeout: h.timeout, Parallel: 1, Only: ids, Log: h.log,
		Script: h.script,
		Progress: func(r scenario.Run) {
			r.Rep += h.first - 1
			from := 1
			if hs, ok := hist[r.Scenario]; ok {
				from = len(hs.Turns) + 1
			}
			l := setLine(name, cond, r, from)
			l.Done = done(scens[r.Scenario], r, fromTurn(r.Transcript, from))
			if h.budget.why(providerKind(h.models, model)) != "" && !l.Passed {
				l.Stopped = true
			}
			if hasCards(cond) {
				mu.Lock()
				l.Cards = carded[r.Scenario]
				mu.Unlock()
				if at := hist[r.Scenario].At; at < from-historyMin {
					hit := slices.Contains(l.Cards, at) // the needed turn is past the cut
					l.CardHit = &hit
				}
			}
			h.write(l)
			log.Printf("%-30s %-14s c%s %-28s r%d %s", model, name, cond, r.Scenario, r.Rep, status(l))
		},
	}
	if h.script != nil {
		opt.Backends = nil
		opt.Scale = 0.01
	}
	opt.Deps = func(d *agent.Deps) {
		h.measure(name, d.Tools, d.Skills)
		if hasIndex(cond) {
			all := d.Tools
			d.Tools, d.ToolIndex = newIndex(all, d.Skills)
			d.Skills = keepSkills(d.Skills, all, d.Tools)
		}
		d.NoHistoryCards = !hasCards(cond) // TASK-027: the app's own cards
	}
	if hasIndex(cond) {
		opt.Call = unwrap
	}
	if name == longSet {
		opt.Setup = func(ctx context.Context, sc *scenario.Scenario, p *project.Project, c id.Chat) error {
			if err := seed(ctx, p, c, hist[sc.ID].Turns, hasIndex(cond)); err != nil {
				return err
			}
			// The cards the app adds to the message: it finds them the
			// same way at the follow-up's first request, after the cut.
			from := len(hist[sc.ID].Turns) + 1
			scores, err := agent.CardScores(ctx, p.Chats, c, sc.Messages[0].Text, from)
			if err != nil {
				return err
			}
			mu.Lock()
			carded[sc.ID] = agent.PickCards(scores, from-historyMin)
			mu.Unlock()
			return nil
		}
	}
	rep, err := scenario.RunSet(ctx, s, opt)
	if rep != nil {
		for i := range rep.Runs {
			rep.Runs[i].Rep += h.first - 1
		}
		if _, werr := rep.Write(filepath.Join(h.dir, name, "c"+cond, slug(model)), nil); werr != nil {
			err = errors.Join(err, werr)
		}
	}
	return err
}

// setLine is a run's line; the call counts read the transcript from turn
// from on, past a long chat's seeded turns.
func setLine(set, cond string, r scenario.Run, from int) line {
	return line{
		Kind: "set", Set: set, Cond: cond, Model: r.Model, Scenario: r.Scenario, Rep: r.Rep,
		Error: r.Error, Skipped: r.Skipped, Passed: r.Passed(), Task: taskPassed(r),
		Requests: r.Requests, Prompt: r.Prompt, CacheRead: r.CacheRead, Output: r.Output, Seconds: r.Seconds,
		Asserts: r.Asserts, Calls: countCalls(fromTurn(r.Transcript, from)),
	}
}

// fromTurn is the transcript from turn n's heading on; all of it for n 1.
func fromTurn(tr string, n int) string {
	if n <= 1 {
		return tr
	}
	if i := strings.Index(tr, fmt.Sprintf("\n## Turn %d\n", n)); i >= 0 {
		return tr[i:]
	}
	return ""
}

func status(l line) string {
	switch {
	case l.Skipped != "":
		return "skip"
	case l.Stopped:
		return "stopped: " + clip(l.Error, 160)
	case l.Error != "":
		return "broke: " + clip(l.Error, 160)
	case l.Task && !l.Done:
		return fmt.Sprintf("checks pass, but a needed tool never ran without an error (%d req)", l.Requests)
	case l.Passed:
		return fmt.Sprintf("pass (%d req, %d tok)", l.Requests, l.Prompt)
	case l.Task:
		return fmt.Sprintf("task pass, size fail (%d req, %d tok)", l.Requests, l.Prompt)
	}
	var failed []string
	for _, a := range l.Asserts {
		if a.Status == scenario.Fail && a.Test != "info" {
			failed = append(failed, a.Type)
		}
	}
	return fmt.Sprintf("fail: %s (%d req)", strings.Join(failed, ", "), l.Requests)
}

// pickModels reads the models file and keys, or a script.
func pickModels(names []string, modelsFile, envFile string) (*scenario.Models, *scenario.Script, error) {
	if len(names) == 1 {
		if file, ok := strings.CutPrefix(names[0], "scripted:"); ok {
			s, err := scenario.ReadScript(file)
			if err != nil {
				return nil, nil, err
			}
			return scenario.ScriptedModels(s), s, nil
		}
	}
	mf, err := scenario.ReadModels(modelsFile)
	if err != nil {
		return nil, nil, err
	}
	env := scenario.Env{}
	if env.File, err = scenario.ReadEnvFile(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	m, err := mf.Select(names, env)
	return m, nil, err
}

// hasIndex: conditions 2 and 3 have the tool index; hasCards: 3 and 4
// have the history cards.
func hasIndex(cond string) bool { return cond == "2" || cond == "3" }
func hasCards(cond string) bool { return cond == "3" || cond == "4" }

// providerKind is the kind of the model's provider.
func providerKind(ms *scenario.Models, model string) provider.Kind {
	name, _, _ := strings.Cut(model, "/")
	for _, p := range ms.Providers {
		if p.Name == name {
			return provider.Kind(p.Kind)
		}
	}
	return ""
}

func slug(model string) string { return strings.NewReplacer("/", "_", ":", "_").Replace(model) }

func split(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
