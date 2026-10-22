// Command jenab-scenarios runs scenario sets on real models
// and writes a report (SPEC 8.4, TASK-001):
//
//	jenab-scenarios run [flags] <dir>...
//	jenab-scenarios check <dir>...
//	jenab-scenarios list <dir>...
//
// A dir is a set (a folder with _set.yaml) or a folder of sets. Keys come
// from the environment, or a .env file given with -env; they are never
// printed or written to a report. -model scripted:<file> plays a script's
// answers instead of a model, with no keys (TASK-013).
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
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/scenario"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

const usage = `usage:
  jenab-scenarios run [flags] <dir>...   run the sets on models and write reports
  jenab-scenarios check <dir>...         load the sets and report errors; no models
  jenab-scenarios list <dir>...          list the scenarios and which run now

A dir is a set (a folder with _set.yaml) or a folder of sets.
Run "jenab-scenarios run -h" for the run flags.
`

func main() {
	sqlworker.IfWorker() // the scenarios' projects run agent queries in this executable
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "run":
		err = run(ctx, args, os.Stdout)
	case "check":
		err = check(args, os.Stdout)
	case "list":
		err = list(args, os.Stdout)
	case "-h", "-help", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jenab-scenarios:", err)
		os.Exit(1)
	}
}

// sets finds the sets in dirs.
func sets(dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		return nil, errors.New("no dir given")
	}
	var out []string
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, scenario.SetFile)); err == nil {
			out = append(out, d)
			continue
		}
		found, err := filepath.Glob(filepath.Join(d, "*", scenario.SetFile))
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("%s: no %s in it or in its folders", d, scenario.SetFile)
		}
		for _, f := range found {
			out = append(out, filepath.Dir(f))
		}
	}
	return out, nil
}

func load(dirs []string) ([]*scenario.Set, error) {
	ds, err := sets(dirs)
	if err != nil {
		return nil, err
	}
	var out []*scenario.Set
	var errs []error
	for _, d := range ds {
		s, err := scenario.Load(d)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d, err))
			continue
		}
		out = append(out, s)
	}
	return out, errors.Join(errs...)
}

func check(args []string, w io.Writer) error {
	ss, err := load(args)
	for _, s := range ss {
		fmt.Fprintf(w, "ok  %s: %d scenarios, %d run now\n", s.Dir, len(s.Scenarios), runnable(s))
	}
	return err
}

func list(args []string, w io.Writer) error {
	ss, err := load(args)
	if err != nil {
		return err
	}
	for _, s := range ss {
		fmt.Fprintf(w, "%s (%s)\n", s.Dir, s.Title)
		for _, sc := range s.Scenarios {
			state := ""
			if len(sc.Needs) > 0 {
				state = "  [skipped: needs " + strings.Join(sc.Needs, ", ") + "]"
			}
			fmt.Fprintf(w, "  %-36s %s%s\n", sc.ID, sc.Title, state)
		}
	}
	return nil
}

func runnable(s *scenario.Set) int {
	n := 0
	for _, sc := range s.Scenarios {
		if len(sc.Needs) == 0 {
			n++
		}
	}
	return n
}

func run(ctx context.Context, args []string, w io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: jenab-scenarios run [flags] <dir>...\n\n")
		fs.PrintDefaults()
	}
	var (
		modelsFile = fs.String("models", filepath.Join("scenarios", "models.yaml"), "the models file (see scenarios/models.example.yaml)")
		modelList  = fs.String("model", "", "models to run, as provider/id, comma-separated; default: every model of a provider that isn't paid. scripted:<file> plays the file's answers instead, with no keys")
		envFile    = fs.String("env", "", "a .env file with the keys; its values win over the environment")
		only       = fs.String("scenario", "", "scenario IDs to run, comma-separated; default: all")
		reps       = fs.Int("reps", 1, "runs of each scenario on each model")
		scale      = fs.Float64("scale", 1, "multiplies tool delays and message times")
		timeout    = fs.Duration("timeout", scenario.DefaultTimeout, "the longest one run may take")
		parallel   = fs.Int("parallel", 1, "runs at a time")
		out        = fs.String("out", "", "where reports go; default: <set>/results, and <set>/results/scripted for a script")
		baseline   = fs.String("baseline", "", "a report's JSON file to compare with; default: the newest report where this one goes")
		keep       = fs.Bool("keep", false, "keep each run's project folder")
		verbose    = fs.Bool("v", false, "log the app's messages to stderr")
		grounding  = fs.String("grounding", "", "SPIKE-032's number check over the sets': mark, same_turn, next_turn, or off")
		groundK    = fs.Int("k", 0, "with -grounding: the unsourced numbers at which the check fires; 0 is the default")
		tellK      = fs.Int("tell-k", 0, "with -grounding: tell the model only at this many unsourced numbers; 0 is k")
		tellSaved  = fs.Bool("tell-saved", false, "with -grounding: also tell the model of an unsourced number in what it saved")
		compute    = fs.Bool("compute", false, "with -grounding: block 1 asks the model to compute numbers with query")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var ground *scenario.GroundingSpec
	if *grounding != "" {
		var err error
		if ground, err = scenario.ParseGrounding(*grounding, *groundK); err != nil {
			return err
		}
		ground.TellK, ground.TellSaved, ground.Compute = *tellK, *tellSaved, *compute
	}
	ss, err := load(fs.Args())
	if err != nil {
		return err
	}
	var base *scenario.Report
	if *baseline != "" {
		if len(ss) > 1 {
			return errors.New("-baseline compares one set; give one dir")
		}
		if base, err = scenario.ReadReport(*baseline); err != nil {
			return err
		}
	}
	models, script, err := pick(split(*modelList), *modelsFile, *envFile)
	if err != nil {
		return err
	}
	log := slog.New(slog.DiscardHandler)
	if *verbose {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	want := split(*only)
	for _, id := range want {
		if !slices.ContainsFunc(ss, func(s *scenario.Set) bool { return slices.Contains(mine(s, want), id) }) {
			return fmt.Errorf("no scenario %s in the sets", id)
		}
	}
	fmt.Fprintf(w, "models: %s\n", strings.Join(models.Run, ", "))
	for _, s := range ss {
		var ids []string
		if len(want) > 0 {
			if ids = mine(s, want); len(ids) == 0 {
				continue // none of them is in this set
			}
		}
		fmt.Fprintf(w, "\n%s (%s)\n", s.Dir, s.Title)
		rep, runErr := scenario.RunSet(ctx, s, scenario.Options{
			Models: models, Backends: backends.All(), Reps: *reps, Scale: *scale, Timeout: *timeout,
			Parallel: *parallel, Only: ids, Keep: *keep, Log: log, Script: script, Grounding: ground,
			Progress: func(r scenario.Run) { fmt.Fprintln(w, " ", line(r)) },
		})
		if rep == nil {
			return runErr
		}
		dir := *out
		switch {
		case dir == "" && script != nil:
			dir = filepath.Join(s.Dir, "results", "scripted") // apart, so real runs don't compare with it
		case dir == "":
			dir = filepath.Join(s.Dir, "results")
		case len(ss) > 1:
			dir = filepath.Join(dir, filepath.Base(s.Dir))
		}
		prev := base
		if prev == nil {
			if prev, err = scenario.Previous(dir); err != nil {
				fmt.Fprintf(w, "  the previous report can't be read, so no changes are shown: %v\n", err)
			}
		}
		md, err := rep.Write(dir, prev)
		if err != nil {
			return err
		}
		for _, sum := range rep.Summary {
			fmt.Fprintf(w, "  %s: %d/%d runs passed, %d broke, %d skipped\n", sum.Model, sum.Passed, sum.Runs, sum.Errors, sum.Skipped)
		}
		fmt.Fprintf(w, "  report: %s\n", md)
		if runErr != nil {
			return runErr // stopped; the report has the runs so far
		}
	}
	return nil
}

// pick reads the models to run: from the models file and the keys, or a
// script for scripted:<file>, which needs neither and runs alone.
func pick(names []string, modelsFile, envFile string) (*scenario.Models, *scenario.Script, error) {
	for _, n := range names {
		file, ok := strings.CutPrefix(n, "scripted:")
		if !ok {
			continue
		}
		if len(names) > 1 {
			return nil, nil, errors.New("scripted:<file> runs alone; give no other models")
		}
		s, err := scenario.ReadScript(file)
		if err != nil {
			return nil, nil, err
		}
		return scenario.ScriptedModels(s), s, nil
	}
	mf, err := scenario.ReadModels(modelsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("no models file at %s: copy scenarios/models.example.yaml there and list your providers, or give one with -models", modelsFile)
	} else if err != nil {
		return nil, nil, err
	}
	env := scenario.Env{}
	if envFile != "" {
		if env.File, err = scenario.ReadEnvFile(envFile); err != nil {
			return nil, nil, err
		}
	}
	models, err := mf.Select(names, env)
	return models, nil, err
}

// mine are the IDs that are scenarios of s.
func mine(s *scenario.Set, ids []string) []string {
	var out []string
	for _, sc := range s.Scenarios {
		for _, id := range ids {
			if sc.ID == id {
				out = append(out, id)
			}
		}
	}
	return out
}

// line is a run's progress line.
func line(r scenario.Run) string {
	head := fmt.Sprintf("%-28s %-36s r%d", r.Model, r.Scenario, r.Rep)
	switch {
	case r.Skipped != "":
		return head + "  skip (" + r.Skipped + ")"
	case r.Error != "":
		return fmt.Sprintf("%s  broke after %s: %s", head, secs(r.Seconds), clip(r.Error))
	}
	var failed []string
	for _, a := range r.Asserts {
		if a.Status == scenario.Fail && a.Test != "info" {
			failed = append(failed, a.Type)
		}
	}
	if len(failed) > 0 {
		return fmt.Sprintf("%s  fail in %s%s: %s", head, secs(r.Seconds), reqs(r), strings.Join(failed, ", "))
	}
	return fmt.Sprintf("%s  pass in %s%s", head, secs(r.Seconds), reqs(r))
}

// reqs is a run's requests, with the subagents' after a "+".
func reqs(r scenario.Run) string {
	switch {
	case r.Requests == 0 && r.SubRequests == 0:
		return ""
	case r.SubRequests > 0:
		return fmt.Sprintf(", %d+%d requests", r.Requests, r.SubRequests)
	}
	return fmt.Sprintf(", %d requests", r.Requests)
}

func secs(s float64) string { return (time.Duration(s*10) * time.Second / 10).String() }

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
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
