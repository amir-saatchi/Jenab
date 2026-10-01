package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

var options = []Option{optSDK, optGenkit, optEino, optAnyLLM, optGoAI}

func main() {
	withOllama := flag.Bool("ollama", true, "run the real round trip against Ollama if it is up")
	withWeight := flag.Bool("weight", true, "measure dependency weight (builds the programs in weight/)")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) // genkit and others log to slog
	os.Unsetenv("GENKIT_ENV")
	start := time.Now()

	type row struct {
		test  string
		cells []Cell
	}
	var rows []row
	add := func(name string, f func(Option) Cell) {
		r := row{test: name}
		for _, o := range options {
			fmt.Fprintf(os.Stderr, "%-28s %s\n", name, o.Name)
			r.cells = append(r.cells, f(o))
		}
		rows = append(rows, r)
	}
	var errRows []ErrRow
	var truncRows []TruncRow

	for _, k := range []Kind{Anthropic, OpenAI} {
		k := k
		tag := map[Kind]string{Anthropic: "A", OpenAI: "O"}[k]
		add("T1 stream + 2 parallel tools ("+tag+")", func(o Option) Cell { return testT1(o, k) })
		add("T2 image in tool result ("+tag+")", func(o Option) Cell { return testT2(o, k) })
		if k == Anthropic {
			add("T3 cache points + usage (A)", func(o Option) Cell { return testT3(o, k) })
			add("T4 thinking + signature (A)", func(o Option) Cell { return testT4(o, k) })
		} else {
			add("T3 cached_tokens usage (O)", func(o Option) Cell { return testT3(o, k) })
		}
		add("T5 429/529/500 ("+tag+")", func(o Option) Cell {
			er, c := testT5(o, k)
			errRows = append(errRows, er...)
			return c
		})
		add("T6 cancel mid-stream ("+tag+")", func(o Option) Cell { return testT6(o, k) })
		add("T7 truncated/malformed ("+tag+")", func(o Option) Cell {
			tr, c := testT7(o, k)
			truncRows = append(truncRows, tr...)
			return c
		})
	}
	ollama := "skipped (-ollama=false)"
	if *withOllama {
		if ollamaUp() {
			ollama = "ran"
			add("T8 Ollama "+ollamaModel+" round trip", testOllama)
		} else {
			ollama = "skipped (Ollama or " + ollamaModel + " not available)"
		}
	}
	ranByLib := libraryRanTool.Load()
	gkDefault := genkitDefault()
	var weights []WeightRow
	if *withWeight {
		fmt.Fprintln(os.Stderr, "weight ...")
		weights = measureWeight()
	}

	// ---- output ----
	fmt.Println("# SPIKE-012 results")
	fmt.Println()
	fmt.Printf("Go %s %s/%s, CGO_ENABLED=%s. A = Anthropic Messages mock, O = OpenAI Chat Completions mock. Ollama: %s. Total run time %.0f s.\n\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, cgoEnv(), ollama, time.Since(start).Seconds())
	fmt.Println("## Options")
	fmt.Println()
	fmt.Println("Versions and release dates from `proxy.golang.org/<module>/@latest` on 2026-09-28; licenses from the LICENSE file in the module zip.")
	fmt.Println()
	fmt.Println("| Option | Modules | Version | Released | License |")
	fmt.Println("|---|---|---|---|---|")
	for _, o := range options {
		fmt.Printf("| %s | %s | %s | %s | %s |\n", o.Name, o.Module, o.Version, o.Released, o.License)
	}
	fmt.Println()
	fmt.Println("Screened out, not run: `github.com/tmc/langchaingo` v0.1.14, released 2025-10-20 (no release for 11 months), MIT.")
	fmt.Println()
	fmt.Println("## Summary")
	fmt.Println()
	head := "| Test |"
	sep := "|---|"
	for _, o := range options {
		head += " " + o.Name + " |"
		sep += "---|"
	}
	fmt.Println(head)
	fmt.Println(sep)
	for _, r := range rows {
		line := "| " + r.test + " |"
		for _, c := range r.cells {
			line += " " + c.Status + " |"
		}
		fmt.Println(line)
	}
	if len(weights) > 0 {
		line := "| T9 modules linked / binary MB |"
		for _, o := range options {
			for _, w := range weights {
				if w.Option == o.Name {
					line += fmt.Sprintf(" %d / %.1f |", w.Linked, w.MB)
				}
			}
		}
		fmt.Println(line)
	}
	fmt.Println()
	fmt.Printf("Key criterion: in all runs above, a library ran a tool on its own %d times. (T1 also checks that no tool ran before the assistant message was written.)\n\n%s.\n\n", ranByLib, gkDefault)
	fmt.Println("## Notes per test")
	for _, r := range rows {
		fmt.Printf("\n### %s\n\n| Option | Result | Note |\n|---|---|---|\n", r.test)
		for i, c := range r.cells {
			fmt.Printf("| %s | %s | %s |\n", options[i].Name, c.Status, esc(c.Note))
		}
	}
	fmt.Println()
	fmt.Println("## T5 errors in detail")
	fmt.Println()
	fmt.Println("Mock always returns the status. \"default\" = library defaults; \"retries=0\" = after asking the library for no retries.")
	fmt.Println()
	fmt.Println("| Option | API | Status | Attempts (default) | Time (default) | Attempts (retries=0) | What our code can read | Error text |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	for _, e := range errRows {
		fmt.Printf("| %s | %s | %s | %d | %.2f s | %d | %s | %s |\n", e.Option, e.Kind, e.Case, e.DefAttempts, e.DefTime.Seconds(), e.Attempts0, esc(e.Visible), shortErr(fmt.Errorf("%s", e.Err)))
	}
	fmt.Println()
	fmt.Println("## T7 truncated and malformed streams in detail")
	fmt.Println()
	fmt.Println("| Option | API | Variant | Result |")
	fmt.Println("|---|---|---|---|")
	for _, t := range truncRows {
		fmt.Printf("| %s | %s | %s | %s |\n", t.Option, t.Kind, t.Variant, esc(t.Result))
	}
	if len(weights) > 0 {
		fmt.Println()
		fmt.Println("## T9 dependency weight")
		fmt.Println()
		fmt.Println("Each program in `weight/` streams one request through the Anthropic and the OpenAI-compatible provider of that option. `go build` with default flags, CGO_ENABLED=0.")
		fmt.Println()
		fmt.Println("| Option | Modules in `go list -m all` | Edges in `go mod graph` | Modules linked (`go version -m`) | Binary (MB) | Builds with CGO_ENABLED=0 | Official SDKs in its own graph |")
		fmt.Println("|---|---|---|---|---|---|---|")
		for _, w := range weights {
			fmt.Printf("| %s | %d | %d | %d | %.1f | %s | %s |\n", w.Option, w.Modules, w.Edges, w.Linked, w.MB, w.Build, w.SDKs)
		}
	}
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	return strings.ReplaceAll(s, "\n", " ")
}
