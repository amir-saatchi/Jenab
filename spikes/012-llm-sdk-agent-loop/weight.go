package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// T9: each option has a separate module in weight/<dir> so its dependency graph is its own.

type WeightRow struct {
	Option         string
	Modules, Edges int
	MB             float64
	Build          string
	SDKs           string // official SDK versions in the graph
	Linked         int    // modules compiled into the binary (go version -m)
}

var weightDirs = []struct{ option, dir string }{
	{"(baseline: net/http only)", "baseline"},
	{optSDK.Name, "sdk"},
	{"genkit", "genkit"},
	{"eino", "eino"},
	{"any-llm-go", "anyllm"},
	{"goai", "goai"},
}

func goCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func measureWeight() []WeightRow {
	var rows []WeightRow
	for _, w := range weightDirs {
		dir := filepath.Join("weight", w.dir)
		r := WeightRow{Option: w.option}
		if out, err := goCmd(dir, "list", "-m", "all"); err == nil {
			r.Modules = countLines(out) - 1 // minus the main module
			for _, l := range strings.Split(out, "\n") {
				f := strings.Fields(l)
				if len(f) == 2 && (strings.HasPrefix(f[0], "github.com/anthropics/anthropic-sdk-go") || strings.HasPrefix(f[0], "github.com/openai/openai-go")) {
					if r.SDKs != "" {
						r.SDKs += ", "
					}
					r.SDKs += strings.TrimPrefix(f[0], "github.com/") + " " + f[1]
				}
			}
		}
		if out, err := goCmd(dir, "mod", "graph"); err == nil {
			r.Edges = countLines(out)
		}
		exe := filepath.Join(os.TempDir(), "burrow-spike012-"+w.dir+".exe")
		if out, err := goCmd(dir, "build", "-o", exe, "."); err != nil {
			r.Build = "no: " + firstLine(out)
		} else {
			r.Build = "yes"
			if st, err := os.Stat(exe); err == nil {
				r.MB = float64(st.Size()) / (1 << 20)
			}
			if out, err := goCmd(dir, "version", "-m", exe); err == nil {
				for _, l := range strings.Split(out, "\n") {
					if f := strings.Fields(l); len(f) > 0 && f[0] == "dep" {
						r.Linked++
					}
				}
			}
			os.Remove(exe)
		}
		rows = append(rows, r)
	}
	return rows
}

func countLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// cgoEnv reports `go env CGO_ENABLED` (0 here: no C compiler on this machine).
func cgoEnv() string {
	out, err := exec.Command("go", "env", "CGO_ENABLED").Output()
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(out))
}
