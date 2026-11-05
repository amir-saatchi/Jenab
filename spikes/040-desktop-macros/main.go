package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// SPIKE-040: desktop macros and recipes, on SPIKE-039's harness. See README.md.

type Config struct {
	Scratch string
	CLI     *CLI
	Out     string
	Env     string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike040 probe|agent|replay|store|editor [flags]")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "editor" {
		runEditor(args)
		return
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	scratch := fs.String("scratch", "", "folder for agent-desktop's state and the Chrome profiles (required)")
	vendor := fs.String("vendor", "vendor/v1.0.2", "extracted agent-desktop release")
	env := fs.String("env", "../../.env", "env file with the keys")
	var af agentFlags
	if cmd == "agent" {
		af.register(fs)
	}
	var rf replayFlags
	if cmd == "replay" {
		rf.register(fs)
	}
	fs.Parse(args)
	if *scratch == "" && cmd != "store" && cmd != "regrade" {
		fmt.Fprintln(os.Stderr, "-scratch is required")
		os.Exit(2)
	}
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	cfg := &Config{
		Scratch: abs(*scratch),
		// A folder agent-desktop creates itself gets a private DACL (SPIKE-039).
		CLI: &CLI{Bin: abs(filepath.Join(*vendor, "cli", "agent-desktop.exe")), Home: filepath.Join(abs(*scratch), "adhome-spike040")},
		Out: abs("out"),
		Env: abs(*env),
	}
	os.MkdirAll(cfg.Out, 0o755)
	var err error
	switch cmd {
	case "probe":
		err = runProbe(cfg)
	case "agent":
		err = runAgent(cfg, af)
	case "replay":
		err = runReplayCmd(cfg, rf)
	case "regrade":
		err = runRegrade(fs.Arg(0))
	case "store":
		err = runStoreCmd(cfg, fs.Args())
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", redact(err.Error()))
		os.Exit(1)
	}
}
