// Command planfirst is SPIKE-035's harness: does a plan the agent makes
// before reading untrusted content catch the writes a planted line asks
// for, without asking on normal tasks? See README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		fake    = flag.String("fake", "", "the fake server's exe (go build -o <path> ./fake)")
		scratch = flag.String("scratch", os.TempDir(), "where logs and temp projects go")
		out     = flag.String("out", "results", "where results go")
		models  = flag.String("models", "ollama-cloud/gemma4:31b,gemini/gemini-3.5-flash-lite", "models, as provider/id")
		conds   = flag.String("conds", "none,checked,unchecked", "conditions")
		taskIDs = flag.String("tasks", "", "tasks by id or kind (attacked, normal); all if empty")
		reps    = flag.Int("reps", 1, "repetitions")
		modelsF = flag.String("models-file", "models.yaml", "the providers and models")
		envFile = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys")
		maxGem  = flag.Int("gemini-max", 150, "the most Gemini requests in all results files")
		timeout = flag.Duration("timeout", 6*time.Minute, "the longest one run may take")
		report  = flag.Bool("report", false, "print the tables from the results files")
	)
	flag.Parse()
	if *report {
		return printReport(*out)
	}
	if *fake == "" {
		return errors.New("needs -fake")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	ms := split(*models)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	reg, keys, llm, err := registry(*modelsF, *envFile, ms, logger)
	if err != nil {
		return err
	}
	redact := func(s string) string {
		for _, k := range keys {
			if len(k) >= 8 {
				s = strings.ReplaceAll(s, k, "[redacted]")
			}
		}
		return s
	}
	b := &budget{max: *maxGem}
	matches, _ := filepath.Glob(filepath.Join(*out, "runs-gemini-*.jsonl"))
	for _, p := range matches {
		for _, r := range readResults(p) {
			b.used += r.Requests
		}
	}
	log.Printf("Gemini requests so far: %d of %d", b.used, b.max)
	e := &env{h: &harness{models: reg, llm: llm, temp: *scratch, log: logger, redact: redact, timeout: *timeout},
		fake: *fake, scratch: *scratch, out: *out, budget: b}
	e.run(ctx, ms, split(*conds), pick(tasks, split(*taskIDs)), *reps)
	return nil
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
