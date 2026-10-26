// Command liveanswer is SPIKE-037's harness. It plays scripted chats on
// the app's real orchestrator. Part A: requests that start background
// subagents, with today's stacked finish turns or the live answer. Part B:
// two long chats with today's window, answers only, or answers and trace.
//
//	go run . -part A -tests a_table -reps 1                    # a smoke run
//	go run . -part A -reps 3                                   # Part A
//	go run . -part B -reps 3                                   # Part B
//	go run . -resume results/<stamp>                           # run what is missing or failed
//	go run . -report results/<stamp>/runs.jsonl                # results.md again
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

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/scenario"
	"github.com/amir-saatchi/jenab/internal/secret"
)

const allModels = "ollama-cloud/gemma4:31b,ollama-cloud/gpt-oss:120b,mistral/ministral-14b-latest"

func main() {
	var (
		modelsFile = flag.String("models-file", "models.yaml", "the providers and models")
		envFile    = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys")
		modelList  = flag.String("models", allModels, "models to run, as provider/id, comma-separated")
		part       = flag.String("part", "A", "A or B")
		testList   = flag.String("tests", "", "tests to run, comma-separated; all of the part's by default")
		condList   = flag.String("conds", "", "conditions to run; all of the part's by default")
		reps       = flag.Int("reps", 3, "runs of each test, model and condition")
		lanes      = flag.String("lanes", "ollama-cloud=3,mistral=2", "runs at a time per provider")
		provPar    = flag.Int("provider-parallel", 3, "calls at a time per provider")
		timeout    = flag.Duration("timeout", 25*time.Minute, "the longest one run may take")
		gap        = flag.Duration("gap", 2*time.Second, "the wait before a finish notice")
		out        = flag.String("out", "results", "where results go")
		resume     = flag.String("resume", "", "a results folder to continue: runs without a line or with an error run again")
		reportOnly = flag.String("report", "", "write results.md from these runs.jsonl files (comma-separated) and stop")
		keep       = flag.Bool("keep", false, "keep the runs' projects")
	)
	flag.Parse()
	if *reportOnly != "" {
		paths := strings.Split(*reportOnly, ",")
		runs, err := readRuns(paths...)
		if err != nil {
			log.Fatal(err)
		}
		dst := filepath.Join(filepath.Dir(paths[0]), "results.md")
		if err := os.WriteFile(dst, []byte(report(runs)), 0o644); err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s", dst)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	*part = strings.ToUpper(*part)
	conds := split(*condList)
	if len(conds) == 0 {
		conds = condsOf[*part]
	}
	for _, c := range conds {
		if !slices.Contains(condsOf[*part], c) {
			log.Fatalf("no condition %q in part %s", c, *part)
		}
	}
	var tds []testDef
	if *testList == "" {
		tds = partA
		if *part == "B" {
			tds = partB
		}
	}
	for _, id := range split(*testList) {
		td, ok := testByID(id)
		if !ok || td.Part != *part {
			log.Fatalf("no test %q in part %s", id, *part)
		}
		tds = append(tds, td)
	}
	models := split(*modelList)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	reg, keys, llm, err := registry(*modelsFile, *envFile, models, *provPar, logger)
	if err != nil {
		log.Fatal(err)
	}
	h := &harness{models: reg, llm: llm, keep: *keep, timeout: *timeout, gap: *gap, log: logger, redact: func(s string) string {
		for _, k := range keys {
			if len(k) >= 8 {
				s = strings.ReplaceAll(s, k, "[redacted]")
			}
		}
		return s
	}}

	dir := *resume
	done := map[job]bool{}
	if dir == "" {
		dir = filepath.Join(*out, time.Now().Format("2006-01-02_150405")+"_"+*part)
	} else if prev, err := readRuns(filepath.Join(dir, "runs.jsonl")); err == nil {
		for _, r := range prev {
			if r.Error == "" {
				done[r.job] = true
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "transcripts"), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "runs.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// One queue per provider, so a slow or limited provider doesn't hold
	// up the others. Reps come last, so a stopped run still has a bit of
	// every cell.
	queues := map[string][]job{}
	total := 0
	for rep := 1; rep <= *reps; rep++ {
		for _, td := range tds {
			for _, c := range conds {
				for _, m := range models {
					j := job{Part: *part, Test: td.ID, Cond: c, Model: m, Rep: rep}
					if !done[j] {
						p, _, _ := strings.Cut(m, "/")
						queues[p] = append(queues[p], j)
						total++
					}
				}
			}
		}
	}
	laneOf := map[string]int{}
	for _, l := range split(*lanes) {
		var p string
		var n int
		if k, v, ok := strings.Cut(l, "="); ok {
			if _, err := fmt.Sscan(v, &n); err == nil {
				p = k
			}
		}
		if p == "" || n < 1 {
			log.Fatalf("bad lane %q", l)
		}
		laneOf[p] = n
	}
	log.Printf("%d runs to go, results in %s", total, dir)

	var mu sync.Mutex
	var wg sync.WaitGroup
	n := 0
	stopped := map[string]string{} // model: why
	for p, q := range queues {
		ch := make(chan job)
		for range max(1, laneOf[p]) {
			wg.Go(func() {
				for j := range ch {
					mu.Lock()
					why := stopped[j.Model]
					mu.Unlock()
					if why != "" {
						continue
					}
					td, _ := testByID(j.Test)
					r := h.run(ctx, j, td)
					line, _ := json.Marshal(r)
					mu.Lock()
					n++
					f.Write(append(line, '\n'))
					os.WriteFile(filepath.Join(dir, "transcripts", j.name()+".md"), []byte(transcript(r, td)), 0o644)
					status := "ok"
					if r.Error != "" {
						status = "error: " + clip(r.Error, 160)
					}
					if q := quotaError(r); q != "" {
						stopped[j.Model] = q
						log.Printf("stopping %s: %s", j.Model, q)
					}
					log.Printf("[%d/%d] %s %.0fs turns %d %s · %s", n, total, j.name(), r.Seconds, len(r.Turns), oneLine(r, td), status)
					mu.Unlock()
				}
			})
		}
		go func() {
			defer close(ch)
			for _, j := range q {
				if ctx.Err() != nil {
					return
				}
				ch <- j
			}
		}()
	}
	wg.Wait()
	for m, why := range stopped {
		log.Printf("stopped %s: %s", m, why)
	}

	runs, err := readRuns(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "results.md"), []byte(report(runs)), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", filepath.Join(dir, "results.md"))
}

// quotaError tells a free tier that ran out (a daily or monthly quota,
// or payment needed) from a passing rate limit, which the provider
// layer already retries.
func quotaError(r runResult) string {
	var errs []string
	if r.Error != "" {
		errs = append(errs, r.Error)
	}
	for _, t := range r.Turns {
		for _, q := range t.Requests {
			if q.Err != "" {
				errs = append(errs, q.Err)
			}
		}
	}
	for _, e := range errs {
		l := strings.ToLower(e)
		for _, k := range []string{"quota", "402", "payment required", "insufficient", "weekly usage limit", "daily limit", "usage limit"} {
			if strings.Contains(l, k) {
				return clip(e, 200)
			}
		}
	}
	return ""
}

func readRuns(paths ...string) ([]runResult, error) {
	var out []runResult
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			var r runResult
			if err := json.Unmarshal([]byte(l), &r); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, r)
		}
	}
	// A run done again on resume replaces its earlier line.
	last := map[job]int{}
	for i, r := range out {
		last[r.job] = i
	}
	var dedup []runResult
	for i, r := range out {
		if last[r.job] == i {
			dedup = append(dedup, r)
		}
	}
	return dedup, nil
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// registry builds the app's provider registry for the named models. Keys
// come from the .env file or the environment and stay in memory; they are
// returned only so errors can be redacted.
func registry(modelsFile, envFile string, names []string, provPar int, log *slog.Logger) (*provider.Registry, []string, config.LLMSettings, error) {
	mf, err := scenario.ReadModels(modelsFile)
	if err != nil {
		return nil, nil, config.LLMSettings{}, err
	}
	env := map[string]string{}
	if envFile != "" {
		if env, err = scenario.ReadEnvFile(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, config.LLMSettings{}, err
		}
	}
	llm := config.Defaults().LLM
	llm.Providers = map[string]config.ProviderSettings{}
	llm.ProviderMaxParallelCalls = map[string]int{}
	llm.Models = map[string]string{"default": names[0], "fast": names[0]}
	kr := &keyring{m: map[string]string{}}
	var keys []string
	for _, n := range names {
		pname, mid, ok := strings.Cut(n, "/")
		i := slices.IndexFunc(mf.Providers, func(p scenario.ProviderSpec) bool { return p.Name == pname })
		if !ok || i < 0 {
			return nil, nil, llm, fmt.Errorf("no provider for %s in %s", n, modelsFile)
		}
		spec := mf.Providers[i]
		j := slices.IndexFunc(spec.Models, func(m config.ModelSettings) bool { return m.ID == mid })
		if j < 0 {
			return nil, nil, llm, fmt.Errorf("no model %s in %s", n, modelsFile)
		}
		ps := llm.Providers[pname]
		ps.Kind, ps.BaseURL = spec.Kind, spec.BaseURL
		ps.Models = append(ps.Models, spec.Models[j])
		llm.Providers[pname] = ps
		llm.ProviderMaxParallelCalls[pname] = provPar
		if spec.KeyEnv != "" {
			if _, have := kr.m[provider.KeyName(pname)]; have {
				continue
			}
			v := env[spec.KeyEnv]
			if v == "" {
				v = os.Getenv(spec.KeyEnv)
			}
			if v == "" {
				return nil, nil, llm, fmt.Errorf("%s needs its key in %s", pname, spec.KeyEnv)
			}
			kr.m[provider.KeyName(pname)] = v
			keys = append(keys, v)
		}
	}
	return provider.NewRegistry(provider.Deps{
		Settings: llm, Secrets: secret.New(kr), Gate: limit.NewGate(llm.MaxParallelCalls),
		Backends: backends.All(), Catalog: provider.MustCatalog(), Log: log,
	}), keys, llm, nil
}

type keyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *keyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *keyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *keyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}
