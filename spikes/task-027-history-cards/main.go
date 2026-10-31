// Command historycards is TASK-027's harness: the history-cards set, with
// today's window and with cards.
//
//	go run . -dev -work <dir>                         # the thresholds' sweep on SPIKE-028's long chats
//	go run . -cards -work <dir>                       # each follow-up's cards, offline: results/cards.md
//	go run . -rules -work <dir>                       # candidate rules for a revision: results/rules.md
//	go run . -work <dir> -reps 5                      # the live runs, on the reference models
//	go run . -work <dir> -model scripted:scenarios/history-cards/_offline.yaml -scenario C1_repeat   # no keys
//	go run . -report results/<stamp>                  # results.md again
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// refModels are TASK-027's models.
const refModels = "ollama-cloud/gemma4:31b,mistral/ministral-14b-latest,gemini/gemini-3.5-flash-lite"

func main() {
	sqlworker.IfWorker() // the runs' projects run agent queries in this executable
	var (
		dev        = flag.Bool("dev", false, "score SPIKE-028's long chats with a sweep of thresholds, and stop")
		cards      = flag.Bool("cards", false, "work out each follow-up's cards offline into results/cards.md, and stop")
		rules      = flag.Bool("rules", false, "score candidate card rules on the set and SPIKE-028's chats into results/rules.md, and stop")
		work       = flag.String("work", "", "a folder for the offline databases and the runs' projects")
		modelsFile = flag.String("models-file", "models.yaml", "the providers and models")
		envFile    = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys")
		modelList  = flag.String("model", refModels, "models, as provider/id, comma-separated; scripted:<file> plays a script")
		condList   = flag.String("conds", strings.Join(allConds, ","), "conditions: today, cards")
		only       = flag.String("scenario", "", "follow-up IDs, comma-separated; default all")
		reps       = flag.Int("reps", 5, "runs of each follow-up, model and condition")
		firstRep   = flag.Int("first-rep", 1, "the number of the first rep")
		timeout    = flag.Duration("timeout", 12*time.Minute, "the longest one run may take")
		out        = flag.String("out", "", "the results folder, gone on with if it has runs; default results/<time>")
		capGemini  = flag.Int("gemini-cap", 150, "Gemini requests a Pacific day, counted in results/gemini-budget.json")
		reportOnly = flag.String("report", "", "write results.md from these results folders (comma-separated) and stop")
		verbose    = flag.Bool("v", false, "log the app's messages")
	)
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *work == "" {
		log.Fatal("-work is needed: a folder for the offline databases and the runs' projects")
	}
	if *dev {
		if err := runDev(ctx, filepath.Join(*work, "dev"), os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *rules {
		var b strings.Builder
		if err := runRules(ctx, filepath.Join(*work, "rules-"+time.Now().Format("150405")), &b); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join("results", "rules.md"), []byte(b.String()), 0o644); err != nil {
			log.Fatal(err)
		}
		os.Stdout.WriteString(b.String())
		return
	}
	ps, err := planCards(ctx, filepath.Join(*work, "plan-"+time.Now().Format("150405")))
	if err != nil {
		log.Fatal(err)
	}
	plans := map[string]plan{}
	for _, p := range ps {
		plans[p.Scenario] = p
	}
	switch {
	case *cards:
		var b strings.Builder
		writePlan(&b, ps)
		if err := os.WriteFile(filepath.Join("results", "cards.md"), []byte(b.String()), 0o644); err != nil {
			log.Fatal(err)
		}
		os.Stdout.WriteString(b.String())
		return
	case *reportOnly != "":
		if err := writeReport(split(*reportOnly), plans); err != nil {
			log.Fatal(err)
		}
		return
	}
	conds := split(*condList)
	for _, c := range conds {
		if !slices.Contains(allConds, c) {
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
	runs := filepath.Join(*work, "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		log.Fatal(err)
	}
	b, err := openBudget(filepath.Join("results", "gemini-budget.json"), *capGemini)
	if err != nil {
		log.Fatal(err)
	}
	set, turns, err := loadSet()
	if err != nil {
		log.Fatal(err)
	}
	done, err := readDone(dir)
	if err != nil {
		log.Fatal(err)
	}
	h := &harness{dir: dir, work: runs, log: lg, timeout: *timeout, budget: b, set: set, turns: turns, plans: plans,
		only: split(*only), reps: *reps, first: *firstRep, done: done}
	if h.models, h.script, err = pickModels(split(*modelList), *modelsFile, *envFile); err != nil {
		log.Fatal(err)
	}
	log.Printf("results: %s (%d runs there); models: %s; Gemini requests left today, if run: %d",
		dir, len(done), strings.Join(h.models.Run, ", "), b.left())
	// One lane per model: each model's runs go one at a time (Ollama's
	// plan runs one request at a time), the models side by side.
	var wg sync.WaitGroup
	for _, m := range h.models.Run {
		mctx, cancel := context.WithCancel(ctx)
		b.onStop(providerKind(h.models, m), cancel)
		wg.Go(func() {
			defer cancel()
			h.lane(mctx, m, conds)
		})
	}
	wg.Wait()
	for k, why := range b.all() {
		log.Printf("%s stopped: %s", k, why)
	}
	for k, n := range b.requests() {
		log.Printf("%s: %d requests sent", k, n)
	}
	if err := writeReport([]string{dir}, plans); err != nil {
		log.Fatal(err)
	}
}
