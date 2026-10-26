// Command topics is SPIKE-034's harness. It plays two scripted chats on
// the app's real orchestrator, with fake project tools, under four
// conditions: one chat (1), topics the scripted user opens (2), topics
// the agent proposes (3) and SPIKE-029's subjects (4).
//
//	go run . -dry -reps 1                                                       # no model: checks the harness
//	go run . -models gemini/gemini-3.5-flash-lite -scenarios sqlite -reps 1     # a smoke run
//	go run . -models gemini/gemini-3.5-flash-lite,ollama-cloud/gemma4:31b -reps 3  # the main run
//	go run . -resume results/<stamp>                                            # runs that are missing or failed
//	go run . -report results/<stamp>/runs.jsonl                                 # results.md again
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
		modelList  = flag.String("models", "gemini/gemini-3.5-flash-lite,ollama-cloud/gemma4:31b", "models to run, as provider/id, comma-separated")
		condList   = flag.String("conds", "1,2,3,4", "conditions to run")
		scList     = flag.String("scenarios", "sqlite,jobs", "scenarios to run")
		reps       = flag.Int("reps", 3, "runs of each scenario, model and condition")
		parallel   = flag.Int("parallel", 2, "runs at a time")
		provPar    = flag.String("provider-parallel", "gemini=2,ollama-cloud=1", "calls at a time per provider, as name=n; others get 2")
		timeout    = flag.Duration("timeout", 40*time.Minute, "the longest one run may take")
		out        = flag.String("out", "results", "where results go")
		resume     = flag.String("resume", "", "a results folder to continue: runs without a line or with an error run again")
		reportOnly = flag.String("report", "", "write results.md from these runs.jsonl files (comma-separated) and stop")
		keep       = flag.Bool("keep", false, "keep the runs' projects")
		dry        = flag.Bool("dry", false, "use a rule-based fake model, dry/fake: no key, no quota")
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
	if *dry {
		models = []string{"dry/fake"}
	}
	var conds []int
	for _, c := range split(*condList) {
		n, err := strconv.Atoi(c)
		if err != nil || n < condBaseline || n > condSubjects {
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
	par := map[string]int{}
	for _, kv := range split(*provPar) {
		k, v, _ := strings.Cut(kv, "=")
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			log.Fatalf("bad -provider-parallel %q", kv)
		}
		par[k] = n
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	var reg *provider.Registry
	var keys []string
	var llm config.LLMSettings
	var err error
	if *dry {
		reg, llm = dryRegistry(logger)
	} else if reg, keys, llm, err = registry(*modelsFile, *envFile, models, par, logger); err != nil {
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
		if *dry {
			dir += "_dry"
		}
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
					status = "error: " + clip(r.Error, 160)
				}
				log.Printf("[%d/%d] %s %.0fs req %d follow %d/%d cross %d/%d new %d/%d ret %d/%d noise %d dup %d · %s",
					n, len(jobs), j.name(), r.Seconds, s.Requests, s.FollowOK, s.FollowN, s.CrossOK, s.CrossN, s.NewOK, s.NewN, s.ReturnOK, s.ReturnN, s.Noise, s.Dups, status)
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
func registry(modelsFile, envFile string, names []string, par map[string]int, log *slog.Logger) (*provider.Registry, []string, config.LLMSettings, error) {
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
		llm.ProviderMaxParallelCalls[pname] = 2
		if p, ok := par[pname]; ok {
			llm.ProviderMaxParallelCalls[pname] = p
		}
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

// dryRegistry serves dry/fake from dryModel.
func dryRegistry(log *slog.Logger) (*provider.Registry, config.LLMSettings) {
	llm := config.Defaults().LLM
	llm.Providers = map[string]config.ProviderSettings{
		"dry": {Kind: string(provider.KindCompatible), BaseURL: "https://dry.test/v1/", Models: []config.ModelSettings{{ID: "fake", Context: 131072}}},
	}
	llm.ProviderMaxParallelCalls = map[string]int{"dry": 4}
	llm.Models = map[string]string{"default": "dry/fake", "fast": "dry/fake"}
	kr := &keyring{m: map[string]string{provider.KeyName("dry"): "dry-run-not-a-key"}}
	return provider.NewRegistry(provider.Deps{
		Settings: llm, Secrets: secret.New(kr), Gate: limit.NewGate(llm.MaxParallelCalls),
		Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: func(provider.Connection) (provider.Provider, error) { return dryModel{}, nil }},
		Log:      log,
	}), llm
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
