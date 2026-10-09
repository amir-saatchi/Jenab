// Command subjects is SPIKE-029's harness. It plays long scripted chats on
// the app's real orchestrator, with fake project tools and a subject store,
// under four conditions, and scores whether the agent keeps its subjects
// and whether they help after the history window is cut.
//
//	go run . -models ollama-cloud/gemma4:31b -reps 1 -scenarios crypto -conds 3   # a smoke run
//	go run . -models ollama-cloud/gemma4:31b,ollama-cloud/gpt-oss:120b -reps 3   # the main run
//	go run . -resume results/<stamp>                                              # run what is missing or failed
//	go run . -report results/<stamp>/runs.jsonl                                   # results.md again
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
	"strconv"
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

func main() {
	var (
		modelsFile = flag.String("models-file", "models.yaml", "the providers and models")
		envFile    = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys")
		modelList  = flag.String("models", "ollama-cloud/gemma4:31b,ollama-cloud/gpt-oss:120b", "models to run, as provider/id, comma-separated")
		condList   = flag.String("conds", "1,2,3,4", "conditions to run")
		scList     = flag.String("scenarios", "crypto,jobs", "scenarios to run")
		reps       = flag.Int("reps", 3, "runs of each scenario, model and condition")
		parallel   = flag.Int("parallel", 4, "runs at a time")
		provPar    = flag.Int("provider-parallel", 3, "calls at a time per provider")
		timeout    = flag.Duration("timeout", 30*time.Minute, "the longest one run may take")
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

	models := split(*modelList)
	var conds []int
	for _, c := range split(*condList) {
		n, err := strconv.Atoi(c)
		if err != nil || n < condBaseline || n > condCheck {
			log.Fatalf("no condition %q", c)
		}
		conds = append(conds, n)
	}
	var scs []scenarioDef
	for _, id := range split(*scList) {
		sc, ok := scenarioByID(id)
		if !ok {
			log.Fatalf("no scenario %q", id)
		}
		scs = append(scs, sc)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	reg, keys, llm, err := registry(*modelsFile, *envFile, models, *provPar, logger)
	if err != nil {
		log.Fatal(err)
	}
	h := &harness{models: reg, llm: llm, keep: *keep, timeout: *timeout, log: logger, redact: func(s string) string {
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
		dir = filepath.Join(*out, time.Now().Format("2006-01-02_150405"))
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

	// Models take turns, and so do conditions, so a stopped run still has
	// a bit of every cell.
	var jobs []job
	for rep := 1; rep <= *reps; rep++ {
		for _, sc := range scs {
			for _, c := range conds {
				for _, m := range models {
					j := job{Model: m, Scenario: sc.ID, Cond: c, Rep: rep}
					if !done[j] {
						jobs = append(jobs, j)
					}
				}
			}
		}
	}
	log.Printf("%d runs to go, results in %s", len(jobs), dir)
	ch := make(chan job)
	var mu sync.Mutex
	var wg sync.WaitGroup
	n := 0
	for range *parallel {
		wg.Go(func() {
			for j := range ch {
				sc, _ := scenarioByID(j.Scenario)
				r := h.run(ctx, j, sc)
				line, _ := json.Marshal(r)
				mu.Lock()
				n++
				f.Write(append(line, '\n'))
				os.WriteFile(filepath.Join(dir, "transcripts", j.name()+".md"), []byte(transcript(r, sc)), 0o644)
				s := scoreRun(r, sc)
				status := "ok"
				if r.Error != "" {
					status = "error: " + clip(r.Error, 120)
				}
				log.Printf("[%d/%d] %s %.0fs subj %d/%d nudges %d answers %d/%d calls %d/%d status %d/%d · %s",
					n, len(jobs), j.name(), r.Seconds, s.SubjOwn, s.Work, s.Nudges, s.AnswerOK, s.AnswerN, s.CallOK, s.CallN, s.StatusOK, s.StatusN, status)
				mu.Unlock()
			}
		})
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		ch <- j
	}
	close(ch)
	wg.Wait()

	runs, err := readRuns(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "results.md"), []byte(report(runs)), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", filepath.Join(dir, "results.md"))
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
