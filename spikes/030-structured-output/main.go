// Command structured is SPIKE-030's harness. It asks models for JSON that
// matches a schema in four ways, through the app's own provider registry
// and backends, and scores the answers against fixed right answers.
//
//	go run . -probe                     # what each API accepts
//	go run . -reps 3                    # the main run
//	go run . -thinking -methods tool    # one method with thinking on
//	go run . -report results/*.jsonl    # results.md
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// providers are the development providers (SPIKE-018). Spikes may name
// them; the app never does.
var providers = map[string]struct {
	kind      provider.Kind
	base, env string
	fields    map[string]string // base-URL placeholders and their .env names
}{
	"ollama-cloud": {provider.KindOllama, "https://ollama.com/", "OLLAMA_API_KEY", nil},
	"zai":          {provider.KindCompatible, "https://api.z.ai/api/paas/v4/", "Z_API_KEY", nil},
	"gemini":       {provider.KindGemini, "", "GEMINI_API_KEY", nil},
	"groq":         {provider.KindCompatible, "https://api.groq.com/openai/v1/", "GROQ_API_KEY", nil},
}

const defaultModels = "ollama-cloud/gemma4:31b,ollama-cloud/nemotron-3-ultra,zai/glm-4.5-flash,gemini/gemini-3.5-flash-lite"

func main() {
	modelsFlag := flag.String("models", defaultModels, "models as provider/id, comma-separated")
	methodsFlag := flag.String("methods", strings.Join(methods, ","), "methods, comma-separated")
	tasksFlag := flag.String("tasks", "", "task IDs, comma-separated; default: all")
	reps := flag.Int("reps", 3, "runs of each task, method and model")
	thinking := flag.Bool("thinking", false, "ask for the model's thinking")
	parallel := flag.Int("parallel", 2, "requests at a time per model")
	maxTokens := flag.Int("max-tokens", 0, "output cap; default 4096, 16384 with thinking")
	envFile := flag.String("env", "../../.env", "the .env file with the keys")
	out := flag.String("out", "results", "where results go")
	probeOnly := flag.Bool("probe", false, "only probe what each API accepts")
	resume := flag.String("resume", "", "a .jsonl to append to; finished runs in it are skipped")
	report := flag.Bool("report", false, "write results.md from the .jsonl files given as arguments")
	raw := flag.String("raw", "", "send the tool requests of -tasks to this provider/model over plain HTTP and print the replies")
	flag.Parse()
	log.SetFlags(0)

	tasks, err := loadTasks("data")
	if err != nil {
		log.Fatal(err)
	}
	if *report {
		if err := writeReport(flag.Args(), tasks, filepath.Join(*out, "..", "results.md")); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *tasksFlag != "" {
		want := strings.Split(*tasksFlag, ",")
		tasks = slices.DeleteFunc(tasks, func(t *Task) bool { return !slices.Contains(want, t.ID) })
	}
	ms := strings.Split(*methodsFlag, ",")
	for _, m := range ms {
		if !slices.Contains(methods, m) {
			log.Fatalf("unknown method %q", m)
		}
	}
	if *maxTokens == 0 {
		*maxTokens = 4096
		if *thinking {
			*maxTokens = 16384
		}
	}

	keys, err := readEnv(*envFile)
	if err != nil {
		log.Fatal(err)
	}
	if *raw != "" {
		name, id, _ := strings.Cut(*raw, "/")
		p, ok := providers[name]
		if !ok || p.kind != provider.KindCompatible || p.fields != nil {
			log.Fatalf("-raw takes an OpenAI-compatible provider without placeholders")
		}
		for _, t := range tasks {
			fmt.Println(t.ID)
			rawCheck(p.base, keys[p.env], id, t, *maxTokens)
		}
		return
	}
	kr := &memKeyring{m: map[string]string{}}
	sec := secret.New(kr)
	settings := config.LLMSettings{MaxParallelCalls: 8, Providers: map[string]config.ProviderSettings{}}
	var models []*Model
	for _, ref := range strings.Split(*modelsFlag, ",") {
		name, id, ok := strings.Cut(ref, "/")
		p, known := providers[name]
		if !ok || !known {
			log.Fatalf("unknown model %q", ref)
		}
		if keys[p.env] == "" {
			log.Fatalf("%s needs %s in %s", ref, p.env, *envFile)
		}
		kr.m[provider.KeyName(name)] = keys[p.env]
		for field, env := range p.fields {
			if keys[env] == "" {
				log.Fatalf("%s needs %s in %s", ref, env, *envFile)
			}
			kr.m[provider.FieldName(name, field)] = keys[env]
		}
		ps := settings.Providers[name]
		ps.Kind, ps.BaseURL = string(p.kind), p.base
		ps.Models = append(ps.Models, config.ModelSettings{ID: id, Context: 64000})
		settings.Providers[name] = ps
		models = append(models, &Model{Ref: ref, Kind: p.kind})
	}
	wrapped := map[provider.Kind]provider.Factory{}
	for k, f := range backends.All() {
		wrapped[k] = func(c provider.Connection) (provider.Provider, error) {
			c.HTTP = &http.Client{Transport: inject{http.DefaultTransport}}
			return f(c)
		}
	}
	reg := provider.NewRegistry(provider.Deps{Settings: settings, Secrets: sec, Gate: limit.NewGate(8), Backends: wrapped})
	a := &asker{reg: reg, redact: sec.Redact, maxTokens: *maxTokens}
	ctx := context.Background()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	stamp := time.Now().Format("20060102-150405")
	probeFile := filepath.Join(*out, "probe.json")
	if err := probeAll(ctx, a, models, probeFile, *probeOnly); err != nil {
		log.Fatal(err)
	}
	if *probeOnly {
		return
	}

	path := filepath.Join(*out, stamp+".jsonl")
	done := map[string]bool{}
	if *resume != "" {
		path = *resume
		recs, err := readRecords([]string{path})
		if err != nil {
			log.Fatal(err)
		}
		for _, r := range recs {
			if r.Error == "" && r.Skipped == "" {
				done[runKey(r.Model, r.Method, r.Task, r.Rep, r.Thinking)] = true
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var mu sync.Mutex
	write := func(r Record) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
		status := "skip: " + r.Skipped
		switch {
		case r.Skipped != "":
		case r.Error != "":
			status = "error " + r.ErrKind + ": " + clipLine(r.Error, 160)
		case !r.Valid:
			status = fmt.Sprintf("invalid %v → %v", r.Fail1, r.Fail2)
		default:
			status = fmt.Sprintf("valid (first %v), right %v %s", r.Valid1, r.Score.Right, clipLine(r.Score.Detail, 100))
		}
		log.Printf("%-32s %-11s %-11s r%d %5.1fs  %s", r.Model, r.Method, r.Task, r.Rep, r.Seconds, status)
	}

	log.Printf("%d models, methods %v, %d tasks, %d reps, thinking %v → %s", len(models), ms, len(tasks), *reps, *thinking, path)
	var wg sync.WaitGroup
	for _, m := range models {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runModel(ctx, a, m, tasks, ms, *reps, *thinking, *parallel, done, write)
		}()
	}
	wg.Wait()
	log.Printf("done: %s", path)
}

type job struct {
	task   *Task
	method string
	rep    int
}

func runModel(ctx context.Context, a *asker, m *Model, tasks []*Task, ms []string, reps int, thinking bool, parallel int, done map[string]bool, write func(Record)) {
	jobs := make(chan job)
	var stopped atomic.Value // why the model stopped, e.g. a quota error
	var wg sync.WaitGroup
	for range max(parallel, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if why, _ := stopped.Load().(string); why != "" {
					write(Record{Model: m.Ref, Method: j.method, Task: j.task.ID, Kind: j.task.Kind, Lang: j.task.Lang, Rep: j.rep, Thinking: thinking, Skipped: why})
					continue
				}
				var r Record
				for attempt := 0; attempt < 3; attempt++ {
					cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
					r = a.ask(cctx, m, j.task, j.method, thinking)
					cancel()
					// A provider error is retried as a step's retry would; a
					// wrong answer never is.
					if !slices.Contains([]string{"rate_limited", "overloaded", "transport"}, r.ErrKind) {
						break
					}
					time.Sleep(time.Duration(10*(attempt+1)) * time.Second)
				}
				r.Rep = j.rep
				if r.ErrKind == "quota" {
					stopped.Store("stopped after a quota error")
				}
				write(r)
			}
		}()
	}
	for rep := 1; rep <= reps; rep++ {
		for _, t := range tasks {
			for _, meth := range ms {
				if done[runKey(m.Ref, meth, t.ID, rep, thinking)] {
					continue
				}
				jobs <- job{task: t, method: meth, rep: rep}
			}
		}
	}
	close(jobs)
	wg.Wait()
}

func runKey(model, method, task string, rep int, thinking bool) string {
	return fmt.Sprintf("%s|%s|%s|%d|%v", model, method, task, rep, thinking)
}

func clipLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// readEnv reads KEY=value lines. Values are only kept in memory and only
// go to the keyring the registry reads; they are never printed.
func readEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out, sc.Err()
}

type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *memKeyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}

func readRecords(files []string) ([]Record, error) {
	var out []Record
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var r Record
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			out = append(out, r)
		}
		f.Close()
	}
	return out, nil
}
