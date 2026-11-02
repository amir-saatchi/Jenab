package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SPIKE-039: desktop control with agent-desktop. See README.md.

type Config struct {
	Scratch string
	CLI     *CLI
	DLLPath string
	Out     string
	Env     string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike039 bench|sizes|hang|agent|editor [flags]")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "editor" {
		runEditor(args)
		return
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	scratch := fs.String("scratch", "", "folder for agent-desktop's state, the Edge profile and test files (required)")
	vendor := fs.String("vendor", "vendor/v1.0.2", "extracted agent-desktop release")
	env := fs.String("env", "../../.env", "env file with GEMINI_API_KEY")
	var agentArgs agentFlags
	if cmd == "agent" {
		agentArgs.register(fs)
	}
	fs.Parse(args)
	if *scratch == "" {
		fmt.Fprintln(os.Stderr, "-scratch is required")
		os.Exit(2)
	}
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	cfg := &Config{
		Scratch: abs(*scratch),
		// A folder agent-desktop creates itself: it then gets a private DACL. A folder that
		// inherits other principals' access is refused ("private file DACL grants an untrusted
		// principal").
		CLI:     &CLI{Bin: abs(filepath.Join(*vendor, "cli", "agent-desktop.exe")), Home: filepath.Join(abs(*scratch), "adhome-spike")},
		DLLPath: abs(filepath.Join(*vendor, "ffi", "agent-desktop-ffi-v1.0.2-x86_64-pc-windows-msvc", "lib", "agent_desktop_ffi.dll")),
		Out:     abs("out"),
		Env:     abs(*env),
	}
	os.MkdirAll(cfg.Out, 0o755)
	var err error
	switch cmd {
	case "bench":
		err = runBench(cfg)
	case "sizes":
		err = runSizes(cfg)
	case "hang":
		err = runHang(cfg)
	case "calcprobe":
		err = runCalcProbe(cfg)
	case "agent":
		err = runAgent(cfg, agentArgs)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", redact(err.Error()))
		os.Exit(1)
	}
}

// Stats summarizes a list of durations in milliseconds.
type Stats struct {
	N                     int
	Min, Median, P90, Max float64
}

func stats(ds []time.Duration) Stats {
	if len(ds) == 0 {
		return Stats{}
	}
	ms := make([]float64, len(ds))
	for i, d := range ds {
		ms[i] = float64(d.Microseconds()) / 1000
	}
	sort.Float64s(ms)
	return Stats{N: len(ms), Min: ms[0], Median: ms[len(ms)/2], P90: ms[(len(ms)*9+9)/10-1], Max: ms[len(ms)-1]}
}

func (s Stats) String() string {
	return fmt.Sprintf("n=%d min %.0f median %.0f p90 %.0f max %.0f ms", s.N, s.Min, s.Median, s.P90, s.Max)
}

func writeJSON(path string, v any) error {
	os.MkdirAll(filepath.Dir(path), 0o755)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
