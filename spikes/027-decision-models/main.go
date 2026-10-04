// SPIKE-027: can a decision model make Jenab's small decisions? Throwaway code; see README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

var clefModels = []string{"clef-flash", "clef"}
var llmModels = []string{"gemma4:31b", "glm-4.5-flash", "gemma-4-26b", "gpt-oss-120b", "qwen3-30b-a3b"}

var (
	outMu   sync.Mutex
	outFile *os.File
	logFile *os.File
	done    = map[string]bool{}
)

func logf(format string, a ...any) {
	s := time.Now().Format("15:04:05 ") + redact(fmt.Sprintf(format, a...))
	outMu.Lock()
	defer outMu.Unlock()
	fmt.Println(s)
	if logFile != nil {
		fmt.Fprintln(logFile, s)
	}
}

func key(run, item, model string, rep int) string {
	return fmt.Sprintf("%s|%s|%s|%d", run, item, model, rep)
}

func save(r Rec) {
	r.Time = time.Now().Format(time.RFC3339)
	b, _ := json.Marshal(r)
	line := redact(string(b))
	outMu.Lock()
	defer outMu.Unlock()
	outFile.WriteString(line + "\n")
	if r.Err == "" {
		done[key(r.Run, r.Item, r.Model, r.Rep)] = true
	}
}

func loadDone(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r Rec
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Err == "" {
			done[key(r.Run, r.Item, r.Model, r.Rep)] = true
		}
	}
}

func isDone(run, item, model string, rep int) bool {
	outMu.Lock()
	defer outMu.Unlock()
	return done[key(run, item, model, rep)]
}

func ask(ctx context.Context, run, model string, it *Item, rep int) {
	if isDone(run, it.ID, model, rep) {
		return
	}
	var rec Rec
	var ans map[string]Ans
	if contains(clefModels, model) {
		rec, ans = askClef(ctx, model, clefBody{Model: model, State: it.State, Questions: it.Qs, Images: it.Images}, true)
	} else {
		rec, ans = askLLM(ctx, model, it)
	}
	rec.Run, rec.Set, rec.Item, rec.Lang, rec.Rep = run, it.Set, it.ID, it.Lang, rep
	rec.Answers = ans
	if ans != nil {
		score(it, &rec)
	}
	if rec.Err != "" {
		logf("%s %s %s: %s %s", run, model, it.ID, rec.Err, short(rec.Body, 200))
	}
	save(rec)
}

// lane runs one model's jobs with `conc` calls at a time and a pause between calls.
func lane(ctx context.Context, run, model string, items []*Item, reps []int, conc int, gap time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()
	sem := make(chan struct{}, conc)
	var inner sync.WaitGroup
	n := 0
	for _, rep := range reps {
		for _, it := range items {
			if ctx.Err() != nil {
				break
			}
			if contains(llmModels, model) && it.NoLLM || len(it.Images) > 0 && !contains(clefModels, model) {
				continue
			}
			prov := "cloudflare"
			if p, ok := llmProvs[model]; ok {
				prov = p.Name
			}
			if why := isStopped(prov); why != "" {
				logf("%s %s: provider stopped, skipping the rest", run, model)
				inner.Wait()
				return
			}
			if isDone(run, it.ID, model, rep) {
				continue
			}
			sem <- struct{}{}
			inner.Add(1)
			go func(it *Item, rep int) {
				defer inner.Done()
				defer func() { <-sem }()
				ask(ctx, run, model, it, rep)
			}(it, rep)
			n++
			if n%25 == 0 {
				logf("%s %s: %d calls", run, model, n)
			}
			if gap > 0 {
				time.Sleep(gap)
			}
		}
	}
	inner.Wait()
	logf("%s %s: lane done (%d calls)", run, model, n)
}

func filterSets(items []*Item, sets string) []*Item {
	if sets == "" {
		return items
	}
	want := map[string]bool{}
	for _, s := range strings.Split(sets, ",") {
		want[s] = true
	}
	var out []*Item
	for _, it := range items {
		if want[it.Set] {
			out = append(out, it)
		}
	}
	return out
}

func main() {
	envPath := flag.String("env", "../../.env", "path to .env")
	plan := flag.String("plan", "main", "comma list: main, repeat, size, cutoff, counts, errors, burst, images, all")
	sets := flag.String("sets", "", "only these sets (main run)")
	models := flag.String("models", "", "only these models")
	report := flag.Bool("report", false, "rewrite results.md from results/runs.jsonl")
	flag.Float64Var(&budgetUSD, "budget", budgetUSD, "Workers AI LLM spending cap in USD per run (8,000 neurons)")
	flag.StringVar(&llmEffort, "effort", "", "reasoning_effort for every LLM; empty = not sent")
	flag.Parse()

	if *report {
		writeReport("results/runs.jsonl", "results.md")
		return
	}
	if err := loadEnv(*envPath); err != nil {
		fmt.Fprintln(os.Stderr, "reading .env:", err)
		os.Exit(1)
	}
	for _, k := range keyNames {
		if secrets[k] == "" {
			fmt.Fprintln(os.Stderr, "missing in .env:", k)
			os.Exit(1)
		}
	}
	secrets["BOGUS"] = "invalid-token-for-the-error-test"
	os.MkdirAll("results", 0o755)
	loadDone("results/runs.jsonl")
	var err error
	if outFile, err = os.OpenFile("results/runs.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer outFile.Close()
	logFile, _ = os.OpenFile("results/run.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	use := func(m string) bool { return *models == "" || contains(strings.Split(*models, ","), m) }
	steps := strings.Split(*plan, ",")
	if *plan == "all" {
		steps = []string{"main", "repeat", "size", "cutoff", "counts", "errors", "images", "burst"}
	}
	for _, step := range steps {
		logf("== %s", step)
		var wg sync.WaitGroup
		switch step {
		case "main":
			items := filterSets(mainItems(), *sets)
			for _, m := range clefModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "main", m, items, []int{0}, 4, 0, &wg)
				}
			}
			for _, m := range llmModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "main", m, items, []int{0}, 1, llmProvs[m].Gap, &wg)
				}
			}
		case "repeat":
			// The same request 4 more times (rep 0 is the main run), decision models only.
			var items []*Item
			cl, rt := classifyItems(), routeItems()
			items = append(items, cl[:10]...)
			items = append(items, cl[80:90]...) // Persian
			items = append(items, rt[:10]...)
			for _, m := range clefModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "main", m, items, []int{1, 2, 3, 4}, 4, 0, &wg)
				}
			}
		case "size":
			for _, m := range clefModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "size", m, sizeItems(), []int{0}, 1, 0, &wg)
				}
			}
		case "cutoff":
			for _, m := range clefModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "cutoff", m, cutoffItems(), []int{0}, 1, 0, &wg)
				}
			}
		case "counts":
			for _, m := range clefModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "counts", m, countItems(), []int{0, 1, 2}, 1, 0, &wg)
				}
			}
		case "images":
			for _, m := range clefModels {
				if use(m) {
					wg.Add(1)
					go lane(ctx, "image", m, imageItems(), []int{0, 1, 2}, 1, 0, &wg)
				}
			}
		case "errors":
			for _, c := range errCases() {
				if isDone("error", c.ID, "clef-flash", 0) {
					continue
				}
				k := "CLOUDFLARE_TOKEN"
				if c.Key != "" {
					k = c.Key
				}
				rec, ans := askClefWith(ctx, "clef-flash", k, c.Body(), false)
				rec.Run, rec.Set, rec.Item, rec.Lang = "error", "error", c.ID, "en"
				rec.Answers = ans
				rec.Err = "" // an error is the expected result here; keep the record as done
				save(rec)
				logf("error %s: HTTP %d %s", c.ID, rec.Status, short(rec.Body, 300))
			}
		case "burst":
			for _, m := range clefModels {
				if !use(m) {
					continue
				}
				runBurst(ctx, m, 1, 10)
				runBurst(ctx, m, 8, 40)
				runBurst(ctx, m, 32, 96)
			}
		default:
			fmt.Fprintln(os.Stderr, "unknown step:", step)
			os.Exit(2)
		}
		wg.Wait()
		if ctx.Err() != nil {
			break
		}
	}
	for p, why := range stopped {
		logf("provider %s stopped: %s", p, why)
	}
}
