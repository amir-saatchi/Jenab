// Command mcpclient is SPIKE-024's harness: the MCP client SDK checks,
// Viso's tool list and token cost, the tool-style runs and the injection
// runs. See README.md.
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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// visoExe is the installed Viso; -viso changes it.
var visoExe = filepath.Join(os.Getenv("LOCALAPPDATA"), "Viso", "Viso.exe")

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run returns errors rather than exiting, so every Viso connection is
// closed (closeOnly) on the way out.
func run() error {
	var (
		sdk      = flag.Bool("sdk", false, "run the SDK checks against the fake server")
		visoList = flag.Bool("viso-list", false, "list Viso's tools with both SDKs (Viso may be closed)")
		visoF    = flag.String("viso", visoExe, "Viso's exe")
		fake     = flag.String("fake", "", "the fake server's exe (go build -o <path> ./fake)")
		scratch  = flag.String("scratch", os.TempDir(), "where logs and temp files go")
		out      = flag.String("out", "results", "where results go; Viso's own texts go to <out>/local, which git ignores")
		inject   = flag.Bool("inject", false, "run the injection test on the fake server")
		models   = flag.String("models", "ollama-cloud/gemma4:31b,gemini/gemini-3.5-flash-lite", "models, as provider/id")
		styles   = flag.String("styles", "native,describe", "tool styles")
		conds    = flag.String("conds", "standard,always,always87", "injection conditions")
		reps     = flag.Int("reps", 1, "repetitions")
		modelsF  = flag.String("models-file", "models.yaml", "the providers and models")
		envFile  = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys")
		maxGem   = flag.Int("gemini-max", 150, "the most Gemini requests in all results files")
		timeout  = flag.Duration("timeout", 8*time.Minute, "the longest one run may take")
		calib    = flag.Bool("calib", false, "measure Viso's tool cost in prompt tokens per model (Viso may be closed)")
		probe    = flag.Bool("probe", false, "Viso: save the projects list, create the test project if needed, show get_scene")
		bench    = flag.Bool("bench", false, "run the tool-style bench on Viso (Viso open and signed in)")
		project  = flag.String("project", "", "the spike's Viso test project id")
		tasks    = flag.String("tasks", "", "bench tasks, e.g. T01,T02 (all if empty)")
		try      = flag.String("try", "", "Viso: calls on the test project, as tool {json} lines; get_scene after")
	)
	flag.Parse()
	visoExe = *visoF
	// A console close comes as SIGTERM on Windows.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	local := filepath.Join(*out, "local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		return err
	}
	switch {
	case *sdk:
		if *fake == "" {
			return errors.New("-sdk needs -fake")
		}
		c := &checker{fake: *fake, scratch: *scratch}
		c.run(ctx)
		return writeChecks(filepath.Join(*out, "sdk-checks.md"), c.out)
	case *visoList:
		return listViso(ctx, local)
	case *inject:
		e, err := newEnv(*modelsF, *envFile, split(*models), *fake, *scratch, *out, *maxGem, *timeout)
		if err != nil {
			return err
		}
		e.inject(ctx, split(*models), split(*styles), split(*conds), *reps)
	case *calib:
		e, err := newEnv(*modelsF, *envFile, split(*models), *fake, *scratch, *out, *maxGem, *timeout)
		if err != nil {
			return err
		}
		return withViso(ctx, func(v *viso) error {
			fmt.Print(describeCost(v))
			if err := os.WriteFile(filepath.Join(local, "viso-instructions.txt"), []byte(v.instr), 0o644); err != nil {
				return err
			}
			e.calib(ctx, v, split(*models))
			return nil
		})
	case *probe:
		return withViso(ctx, func(v *viso) error { return v.probe(ctx, *scratch) })
	case *try != "":
		if *project == "" {
			return errors.New("-try needs -project")
		}
		return withViso(ctx, func(v *viso) error {
			v.created[*project] = true
			return v.try(ctx, *project, *try)
		})
	case *bench:
		if *project == "" {
			return errors.New("-bench needs -project")
		}
		e, err := newEnv(*modelsF, *envFile, split(*models), *fake, *scratch, *out, *maxGem, *timeout)
		if err != nil {
			return err
		}
		return withViso(ctx, func(v *viso) error {
			e.bench(ctx, v, *project, split(*models), split(*styles), split(*tasks), *reps)
			for _, m := range v.modelMade {
				log.Printf("a model created a project: %s", m)
			}
			return nil
		})
	default:
		flag.Usage()
	}
	return nil
}

func newEnv(modelsFile, envFile string, models []string, fake, scratch, out string, maxGem int, timeout time.Duration) (*env, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	reg, keys, llm, err := registry(modelsFile, envFile, models, logger)
	if err != nil {
		return nil, err
	}
	redact := func(s string) string {
		for _, k := range keys {
			if len(k) >= 8 {
				s = strings.ReplaceAll(s, k, "[redacted]")
			}
		}
		return s
	}
	b := &budget{max: maxGem}
	b.countGemini(filepath.Join(out, "inject.jsonl"), filepath.Join(out, "bench.jsonl"), filepath.Join(out, "calib.jsonl"))
	log.Printf("Gemini requests so far: %d of %d", b.used, b.max)
	return &env{h: &harness{models: reg, llm: llm, temp: scratch, log: logger, redact: redact, timeout: timeout},
		fake: fake, scratch: scratch, out: out, budget: b}, nil
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

// visoClient starts Viso.exe --mcp in its own job with the given SDK.
func visoClient(ctx context.Context, sdk string, j *job) (mcpClient, error) {
	if sdk == "go-sdk" {
		return goStdio(ctx, exec.Command(visoExe, "--mcp"), j)
	}
	return m3Stdio(ctx, visoExe, []string{"--mcp"}, j)
}

// visoGUI lists the Viso processes the spike didn't start.
func visoProcs() []proc {
	ps, _ := processes()
	var out []proc
	for _, p := range ps {
		if strings.EqualFold(p.Exe, "Viso.exe") {
			out = append(out, p)
		}
	}
	return out
}

// listViso lists Viso's tools with both SDKs. The tool definitions are
// Viso's own text, so they go to the git-ignored local folder.
func listViso(ctx context.Context, local string) error {
	fmt.Printf("Viso processes before: %s\n", procList(visoProcs()))
	for _, sdk := range []string{"go-sdk", "mcp-go"} {
		if err := listOnce(ctx, sdk, local); err != nil {
			return err
		}
	}
	fmt.Printf("Viso processes after: %s\n", procList(visoProcs()))
	return nil
}

func listOnce(ctx context.Context, sdk, local string) error {
	j, err := newJob()
	if err != nil {
		return err
	}
	start := time.Now()
	cl, err := visoClient(ctx, sdk, j)
	if err != nil {
		j.release()
		fmt.Printf("%s: connect failed: %v\n", sdk, err)
		return nil
	}
	stopWatch := j.watch(cl.PID())
	defer func() {
		stopWatch()
		before := tree(cl.PID())
		jobPIDs, qerr := j.pids()
		cerr := cl.Close()
		time.Sleep(300 * time.Millisecond)
		afterClose := still(before)
		others, jerr := j.closeOnly(cl.PID())
		time.Sleep(300 * time.Millisecond)
		fmt.Printf("%s: job %v (err %v); tree %s; after Close (err=%v): %s; left running on purpose: %v (err %v); after the job: %s\n",
			sdk, jobPIDs, qerr, procList(before), short(cerr), procList(afterClose), others, jerr, procList(still(before)))
	}()
	jobPIDs, _ := j.pids()
	fmt.Printf("%s: protocol %s in %dms, shim pid %d, job %v\n", sdk, cl.Version(), time.Since(start).Milliseconds(), cl.PID(), jobPIDs)
	tools, err := cl.Tools(ctx)
	fmt.Printf("%s: %d tools, err=%v\n", sdk, len(tools), err)
	if sdk == "go-sdk" && err == nil {
		b, _ := json.MarshalIndent(tools, "", "  ")
		if err := os.WriteFile(filepath.Join(local, "viso-tools.json"), b, 0o644); err != nil {
			return err
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	r, err := cl.Call(cctx, "get_scene", json.RawMessage(`{}`), nil)
	cancel()
	fmt.Printf("%s: get_scene: isError=%v text=%q err=%v\n", sdk, r.IsError, clip(r.Text, 300), err)
	return nil
}

// clip shortens s to n bytes at a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}
