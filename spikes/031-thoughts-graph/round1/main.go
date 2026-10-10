// Command thoughtsgraph is the harness of SPIKE-031: it grows a graph of
// thoughts for how-to tasks, one model call per expansion, stores it in
// SQLite and writes a report per graph.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "thoughtsgraph:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var (
		modelsFile  = flag.String("models", "models.yaml", "the models file, in the scenario runner's format")
		envFile     = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys; its values win over the environment")
		modelList   = flag.String("model", "", "models to run, as provider/id, comma-separated")
		only        = flag.String("problem", "", "problem IDs, comma-separated; default: all")
		modes       = flag.String("mode", "siblings", "siblings (up to -k thoughts per call), one (one thought per call), or both, comma-separated")
		k           = flag.Int("k", 3, "the most thoughts per call in siblings mode")
		maxThoughts = flag.Int("max-thoughts", 16, "the most thoughts in a graph, the problem included")
		maxDepth    = flag.Int("max-depth", 4, "thoughts at this depth can't be expanded; the problem is depth 0")
		baseline    = flag.Bool("baseline", true, "also answer each task in one call, for comparison")
		reps        = flag.Int("reps", 1, "graphs per problem, model and mode")
		dbPath      = flag.String("db", filepath.Join("results", "graphs.db"), "the SQLite file")
		reportOnly  = flag.Bool("report", false, "only rewrite results/summary.md from the database")
		analyzeOnly = flag.Bool("analyze", false, "print states, weight against use, invalid next and errors from the database")
		list        = flag.Bool("list", false, "list the problems")
		verbose     = flag.Bool("v", false, "log the provider layer's messages")
	)
	flag.Parse()
	if *list {
		for _, p := range problems {
			fmt.Printf("%-12s %s  %s\n", p.ID, p.Lang, clip(p.Text))
		}
		return nil
	}
	dir := filepath.Dir(*dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	st, err := openStore(*dbPath)
	if err != nil {
		return err
	}
	defer st.db.Close()
	if *analyzeOnly {
		return analyze(ctx, st, os.Stdout)
	}
	if *reportOnly {
		path, err := writeSummary(ctx, st, dir)
		fmt.Println("summary:", path)
		return err
	}

	ps := problems
	if *only != "" {
		ps = nil
		for _, id := range split(*only) {
			p, ok := findProblem(id)
			if !ok {
				return fmt.Errorf("no problem %s (see -list)", id)
			}
			ps = append(ps, p)
		}
	}
	ms := split(*modes)
	for _, m := range ms {
		if m != "siblings" && m != "one" {
			return fmt.Errorf("-mode: %s is not siblings or one", m)
		}
	}
	names := split(*modelList)
	if len(names) == 0 {
		return errors.New("name the models with -model provider/id")
	}
	log := slog.New(slog.DiscardHandler)
	if *verbose {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	reg, err := registry(*modelsFile, *envFile, names, log)
	if err != nil {
		return err
	}
	// One goroutine per model; within a model the graphs run one by one.
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, name := range names {
		wg.Go(func() {
			c := &client{reg: reg, model: name}
			o := options{K: *k, MaxThoughts: *maxThoughts, MaxDepth: *maxDepth, Baseline: *baseline}
			for _, p := range ps {
				for _, m := range ms {
					for rep := 1; rep <= *reps; rep++ {
						o.Mode = m
						c.logf("%s · %s · r%d", p.ID, m, rep)
						start := time.Now()
						g, err := grow(ctx, st, c, p, o)
						if g != nil {
							path, werr := writeGraph(context.WithoutCancel(ctx), st, g, dir)
							err = errors.Join(err, werr)
							c.logf("%s · %s: %d thoughts, finish %s, %s → %s", p.ID, m, len(g.nodes), g.Finish, time.Since(start).Round(time.Second), path)
						}
						if err != nil {
							c.logf("error: %s", clip(err.Error()))
							mu.Lock()
							errs = append(errs, fmt.Errorf("%s %s %s: %w", name, p.ID, m, err))
							mu.Unlock()
						}
						if ctx.Err() != nil {
							return
						}
						if errors.Is(err, errQuota) {
							c.logf("stops: quota")
							return
						}
					}
				}
			}
		})
	}
	wg.Wait()
	path, err := writeSummary(context.WithoutCancel(ctx), st, dir)
	logf("summary: %s", path)
	return errors.Join(append(errs, err)...)
}

// registry builds the app's provider registry for the named models, from
// the scenario runner's models file. Keys come from the .env file or the
// environment and stay in memory.
func registry(modelsFile, envFile string, names []string, log *slog.Logger) (*provider.Registry, error) {
	mf, err := scenario.ReadModels(modelsFile)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	if envFile != "" {
		if env, err = scenario.ReadEnvFile(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	// One call at a time per provider: the second keys may share a rate
	// limit with the first, which SPIKE-030 uses.
	llm := config.LLMSettings{MaxParallelCalls: 4, ProviderMaxParallelCalls: map[string]int{}, Providers: map[string]config.ProviderSettings{}}
	kr := &keyring{m: map[string]string{}}
	for _, n := range names {
		pname, id, ok := strings.Cut(n, "/")
		var spec *scenario.ProviderSpec
		for i := range mf.Providers {
			if mf.Providers[i].Name == pname {
				spec = &mf.Providers[i]
			}
		}
		if !ok || spec == nil {
			return nil, fmt.Errorf("no provider for %s in %s", n, modelsFile)
		}
		ps := llm.Providers[pname]
		ps.Kind, ps.BaseURL = spec.Kind, spec.BaseURL
		found := false
		for _, m := range spec.Models {
			if m.ID == id {
				ps.Models, found = append(ps.Models, m), true
			}
		}
		if !found {
			return nil, fmt.Errorf("no model %s in %s", n, modelsFile)
		}
		llm.Providers[pname] = ps
		llm.ProviderMaxParallelCalls[pname] = 1
		if spec.KeyEnv != "" {
			v := env[spec.KeyEnv]
			if v == "" {
				v = os.Getenv(spec.KeyEnv)
			}
			if v == "" {
				return nil, fmt.Errorf("%s needs its key in %s", pname, spec.KeyEnv)
			}
			kr.m[provider.KeyName(pname)] = v
		}
	}
	return provider.NewRegistry(provider.Deps{
		Settings: llm, Secrets: secret.New(kr), Gate: limit.NewGate(llm.MaxParallelCalls),
		Backends: backends.All(), Catalog: provider.MustCatalog(), Log: log,
	}), nil
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

var (
	logMu  sync.Mutex
	logOut io.Writer = os.Stdout
)

func logf(format string, a ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintf(logOut, time.Now().Format("15:04:05 ")+format+"\n", a...)
}

// logf prefixes the model.
func (c *client) logf(format string, a ...any) {
	logf("%-26s "+format, append([]any{c.model}, a...)...)
}

func split(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
