// SPIKE-005: can expr-lang/expr give us the expression rules in SPEC 6.4?
//
// Usage: go run . > results.md
package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

func main() {
	fmt.Printf("# SPIKE-005 results\n\nexpr-lang/expr v1.17.8, Go %s, %s.\n", runtime.Version(), time.Now().Format("2006-01-02"))
	fmt.Println("\n\"Sandbox\" is the candidate in `sandbox.go`. \"expr defaults\" is `expr.Compile(src, expr.Env(env))` with nothing else, to show what the sandbox adds.")
	fmt.Println("| # | Group | Expression | Engine | Expected | Got | Time | Allocated | Pass |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	s := NewSandbox()
	pass := 0
	for i, c := range cases {
		got, err, d, alloc := runCase(s, c)
		ok := check(c, got, err)
		if ok {
			pass++
		}
		gotText := show(got)
		if err != nil {
			gotText = "error: " + firstLine(err.Error())
		}
		want := show(c.want)
		if c.want == nil {
			want = "value nil"
			if c.errHas != "" {
				want = "error: " + c.errHas
			}
		}
		engine := "sandbox"
		if c.plain {
			engine = "expr defaults"
		}
		fmt.Printf("| %d | %s | `%s` | %s | %s | %s | %s | %s | %s |\n", i+1, c.group, short(c.expr), engine, cell(want), cell(gotText),
			dur(d), bytes(alloc), map[bool]string{true: "yes", false: "**NO**"}[ok])
	}
	fmt.Printf("\n**%d of %d cases pass.**\n", pass, len(cases))
	perItem(s)
}

func runCase(s *Sandbox, c tcase) (any, error, time.Duration, uint64) {
	e := env()
	var ms0, ms1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms0)
	start := clock()
	var got any
	var err error
	if c.plain {
		var p *vm.Program
		p, err = expr.Compile(c.expr, expr.Env(e))
		if err == nil {
			got, err = watchdog(p, e, s.Timeout)
		}
	} else {
		var p *vm.Program
		p, err = s.Compile(c.expr, e)
		if err == nil {
			got, err = s.Run(p, e)
		}
	}
	d := since(start)
	runtime.ReadMemStats(&ms1)
	return got, err, d, ms1.TotalAlloc - ms0.TotalAlloc
}

// watchdog runs a program with expr's defaults and the same timeout as the sandbox.
func watchdog(p *vm.Program, env map[string]any, timeout time.Duration) (any, error) {
	type result struct {
		v   any
		err error
	}
	ch := make(chan result, 1)
	start := clock()
	go func() {
		v, err := expr.Run(p, env)
		if since(start) > timeout {
			abandoned <- since(start)
		}
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout after %v", timeout)
	}
}

var abandoned = make(chan time.Duration, 10)

func check(c tcase, got any, err error) bool {
	if c.errHas != "" {
		return err != nil && strings.Contains(err.Error(), c.errHas)
	}
	if err != nil {
		return false
	}
	if c.want == "(whole env, secrets included)" {
		m, ok := got.(map[string]any)
		return ok && m["secrets"] != nil
	}
	return show(got) == show(c.want)
}

func show(v any) string {
	if m, ok := v.(map[string]any); ok && m["secrets"] != nil {
		return "(whole env, secrets included)"
	}
	return fmt.Sprint(v)
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

func short(s string) string {
	if len(s) > 70 {
		return s[:60] + fmt.Sprintf("… (%d bytes)", len(s))
	}
	return s
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	if len(s) > 90 {
		s = s[:85] + "…"
	}
	return s
}

func dur(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	}
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

func bytes(n uint64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}

// perItem measures per-item evaluation (transform.map fields and transform.filter conditions):
// compile once, run once per item with item set.
func perItem(s *Sandbox) {
	fmt.Println("\n## Per-item evaluation, 10,000 items")
	fmt.Println("Compiled once, run once per item with `item` set. Times are per item (total / 10,000).")
	fmt.Println("| Expression | Sandbox | expr defaults |")
	fmt.Println("|---|---|---|")
	e := env()
	items := e["steps"].(map[string]any)["big"].(map[string]any)["items"].([]any)
	e["item"] = items[0]
	for _, src := range []string{`item.price * 2`, `item.price > 100 and int(item.id) % 2 == 0`, `string(item.id) + "-" + string(item.price)`} {
		ps, err := s.Compile(src, e)
		must(err)
		pp, err := expr.Compile(src, expr.Env(e))
		must(err)
		timeIt := func(p *vm.Program, sandbox bool) string {
			var machine vm.VM
			start := clock()
			for _, it := range items {
				e["item"] = it
				if sandbox {
					env := withBudget(e, s.MaxText)
					_, err = machine.Run(p, env)
				} else {
					_, err = machine.Run(p, e)
				}
				must(err)
			}
			return fmt.Sprintf("%.2f µs", float64(since(start).Nanoseconds())/float64(len(items))/1000)
		}
		fmt.Printf("| `%s` | %s | %s |\n", src, timeIt(ps, true), timeIt(pp, false))
	}
	fmt.Println("\nThe sandbox adds a function call per field access (strict mode) and a fresh text budget per item.")
	select {
	case d := <-abandoned:
		fmt.Printf("\nThe nested-list case with expr defaults returned a timeout after 100 ms, but its goroutine kept running for %v in total.\n", d.Round(time.Millisecond))
	case <-time.After(2 * time.Minute):
		fmt.Println("\nThe nested-list case with expr defaults was still running 2 minutes after its timeout.")
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
