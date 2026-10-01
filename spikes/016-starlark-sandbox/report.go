package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

var engineOrder = []string{eStar, eGoja, eLua, eRis, eTen}

// Verified on proxy.golang.org (module @latest / tag info) and the LICENSE file in the module zip.
var moduleInfo = []struct{ engine, path, date, license string }{
	{eStar, "go.starlark.net", "2026-09-08 (commit 89a6a09411d5, no tags)", "BSD-3-Clause"},
	{eGoja, "github.com/dop251/goja", "2026-09-26 (commit 39ec2650adc9, no tags)", "MIT"},
	{eLua, "github.com/yuin/gopher-lua", "2026-04-01", "MIT"},
	{eRis, "github.com/deepnoodle-ai/risor/v2", "2026-08-18", "Apache-2.0"},
	{eTen, "github.com/d5/tengo/v2", "2024-02-25", "MIT"},
}

func printVersions() {
	vers := map[string]string{}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			vers[d.Path] = d.Version
		}
	}
	fmt.Println("## Candidates")
	fmt.Println()
	fmt.Println("| Engine | Module | Version (from the build) | Date | License |")
	fmt.Println("|---|---|---|---|---|")
	for _, m := range moduleInfo {
		fmt.Printf("| %s | `%s` | `%s` | %s | %s |\n", m.engine, m.path, vers[m.path], m.date, m.license)
	}
	fmt.Printf("\nAlso: `modernc.org/sqlite` `%s`, `golang.org/x/sys` `%s`. `github.com/risor-io/risor/v2` now declares its path as `github.com/deepnoodle-ai/risor/v2`.\n\n", vers["modernc.org/sqlite"], vers["golang.org/x/sys"])
	fmt.Printf("**Limits configured** (the best each library offers):\n")
	fmt.Printf("- starlark: `SetMaxExecutionSteps(%d)`, `Thread.Cancel` on timeout, recursion off (default), `Load` nil\n", stepLimit)
	fmt.Printf("- goja: `Interrupt` on timeout, `SetMaxCallStackSize(%d)`; no step limit exists; `Date` and `Math.random` deleted\n", stackLimit)
	fmt.Printf("- gopher-lua: `SetContext` (cancel), `CallStackSize %d`, `RegistryMaxSize 256K`; only base, table, string, math opened; `dofile`, `loadfile`, `load`, `loadstring`, `require`, `math.random` removed; no step limit exists\n", stackLimit)
	fmt.Printf("- risor: `WithMaxSteps(%d)`, `WithMaxStackDepth(%d)`, context cancel; `risor.Builtins()` minus `rand`\n", stepLimit, stackLimit)
	fmt.Printf("- tengo: `SetMaxAllocs(%d)` (counts objects, not bytes), `MaxStringLen`/`MaxBytesLen` = 16 MiB, `RunContext` cancel; no imports\n", allocLimit)
	fmt.Printf("- all: cancel after %s. Hostile scripts run in a child process in a Job Object (safety cap 1 GB, hard kill after %s).\n\n", runTimeout, hardTimeout)
}

func cell(o outcome) string {
	s := o.Stopped
	if o.Stopped == "finished" && o.Result != "" {
		s = "finished → `" + o.Result + "`"
	}
	return fmt.Sprintf("%s · %s · %.0f MB", s, fmtDur(o.Time), o.PeakMB)
}

func fmtDur(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2f s", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.0f ms", float64(d)/1e6)
	}
	return fmt.Sprintf("%.0f µs", float64(d)/1e3)
}

func tableHeader(first string) {
	fmt.Printf("| %s | %s |\n", first, strings.Join(engineOrder, " | "))
	fmt.Printf("|---%s|\n", strings.Repeat("|---", len(engineOrder)))
}

func hostileTables() {
	fmt.Println("## 1. Hostile scripts with the library limits")
	fmt.Println()
	fmt.Println("Each cell: what stopped the script · time from start until it stopped · peak committed memory of the child process (Job Object `PeakProcessMemoryUsed`, includes the Go runtime and all five engines). \"n/a\": the language cannot express it.")
	fmt.Println()
	tableHeader("Script")
	for _, c := range hostiles {
		row := []string{c.name}
		for _, e := range engineOrder {
			if _, ok := c.src[e]; !ok {
				row = append(row, "n/a")
				continue
			}
			row = append(row, cell(runChild(e, c.id, modeLib)))
		}
		fmt.Printf("| %s |\n", strings.Join(row, " | "))
	}
	for _, mode := range []string{modeWatchdog, modeJob} {
		if mode == modeWatchdog {
			fmt.Printf("\n## 2a. Memory fallback: in-process heap watchdog\n\nA goroutine reads `%s` every %s and cancels the script above %d MB. Job safety cap 1 GB.\n\n", heapMetric, watchEvery, watchCap>>20)
		} else {
			fmt.Printf("\n## 2b. Memory fallback: child process in a Job Object with a %d MB cap\n\nLibrary limits only. When the child passes the cap, Windows refuses the allocation and posts a memory-limit message to the job's completion port; the parent then terminates the job. (Without that, a Go child that hit \"fatal error: out of memory\" sometimes hung until the hard kill.)\n\n", jobCap>>20)
		}
		tableHeader("Script")
		for _, c := range hostiles {
			if !c.memory && c.id != "trivial" {
				continue
			}
			row := []string{c.name}
			for _, e := range engineOrder {
				if _, ok := c.src[e]; !ok {
					row = append(row, "n/a")
					continue
				}
				row = append(row, cell(runChild(e, c.id, mode)))
			}
			fmt.Printf("| %s |\n", strings.Join(row, " | "))
		}
	}
	fmt.Println()
}

func probeTable() {
	fmt.Println("## 3. Reaching outside the sandbox")
	fmt.Println()
	fmt.Println("Each probe runs in-process twice: with the library's default setup (\"default\") and with the Burrow setup above (\"sandbox\"). **reached** means the script got a value; values are not shown. Otherwise the cell shows the start of the error.")
	fmt.Println()
	fmt.Printf("| Probe | Setup | %s |\n", strings.Join(engineOrder, " | "))
	fmt.Printf("|---|---%s|\n", strings.Repeat("|---", len(engineOrder)))
	for _, p := range probes {
		for _, def := range []bool{true, false} {
			setup := "sandbox"
			if def {
				setup = "default"
			}
			row := []string{p.name, setup}
			for _, e := range engineOrder {
				ctx, cancel := context.WithTimeoutCause(context.Background(), runTimeout, errTimeout)
				_, err := engineByName(e).Run(ctx, p.src[e], RunOpts{Default: def})
				cancel()
				if err == nil {
					row = append(row, "**reached**")
				} else {
					row = append(row, "blocked: "+clip(firstLine(err.Error()), 48))
				}
			}
			fmt.Printf("| %s |\n", strings.Join(row, " | "))
		}
	}
	fmt.Println()
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

var dbChecks = []struct{ name, src string }{
	{"read 10,000 rows", `result = len(db.query("SELECT * FROM prices"))`},
	{"DELETE", `result = db.query("DELETE FROM prices")`},
	{"PRAGMA query_only = 0", `result = db.query("PRAGMA query_only = 0")`},
	{"WITH … DELETE", `result = db.query("WITH x AS (SELECT 1) DELETE FROM prices")`},
	{"two statements", `result = db.query("SELECT 1; DELETE FROM prices")`},
	{"ATTACH", `result = db.query("ATTACH 'x.db' AS x")`},
	{"100 MB blob", `result = db.query("SELECT randomblob(100000000)")`},
	{"10,000,000 rows", `result = len(db.query("WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c LIMIT 10000000) SELECT x FROM c"))`},
	{"endless query", `result = db.query("WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c) SELECT count(*) FROM c")`},
}

func dbTable() {
	db, err := openDB()
	if err != nil {
		fmt.Println("openDB:", err)
		return
	}
	fmt.Println("## 4. Read-only `db.query` (Starlark, in-memory modernc database)")
	fmt.Println()
	fmt.Printf("The reader connection has `query_only`, `SQLITE_LIMIT_ATTACHED = 0` and `SQLITE_LIMIT_LENGTH = 16 MiB`. `db.query` accepts one `SELECT`/`WITH` statement (a simplified SPIKE-007 text check), resets `query_only` before each call, returns at most %d rows, and uses the script's context (cancel after %s).\n\n", maxRows, runTimeout)
	fmt.Println("| Call | Result | Time |")
	fmt.Println("|---|---|---|")
	for _, c := range dbChecks {
		ctx, cancel := context.WithTimeoutCause(context.Background(), runTimeout, errTimeout)
		start := clock()
		v, err := starlarkEngine{}.Run(ctx, c.src, RunOpts{DB: db})
		d := since(start)
		cancel()
		res := fmt.Sprintf("ok: %v", short(v))
		if err != nil {
			res = "error: " + clip(firstLine(err.Error()), 80)
		}
		fmt.Printf("| `%s` | %s | %s |\n", c.name, res, fmtDur(d))
	}
	var n int
	db.writer.QueryRow("SELECT count(*) FROM prices").Scan(&n)
	fmt.Printf("\nRows in `prices` afterwards: %d.\n\n", n)
}

func transformTable() {
	db, err := openDB()
	if err != nil {
		fmt.Println("openDB:", err)
		return
	}
	fmt.Println("## 5. Typical transform over 10,000 rows")
	fmt.Println()
	fmt.Println("`db.query` returns 10,000 rows (10 symbols × 1,000 days); the script groups by symbol, averages the price with `round`, and builds a label with `add_days` and `format_date`. Time is for the whole run: new runtime, compile, query, conversion into script values, transform, conversion of the result. 20 runs each.")
	fmt.Println()
	const runs = 20
	var goQuery []time.Duration
	for i := 0; i < runs; i++ {
		s := clock()
		db.Query(context.Background(), transformSQL, []any{transformSince})
		goQuery = append(goQuery, since(s))
	}
	fmt.Println("| Engine | Median | Min | Same output as starlark | First item |")
	fmt.Println("|---|---|---|---|---|")
	var ref string
	for _, e := range engineOrder {
		var ds []time.Duration
		var out any
		var runErr error
		for i := 0; i < runs; i++ {
			ctx, cancel := context.WithTimeoutCause(context.Background(), 10*time.Second, errTimeout)
			s := clock()
			v, err := engineByName(e).Run(ctx, transformSrc[e], RunOpts{DB: db})
			ds = append(ds, since(s))
			cancel()
			out, runErr = v, err
		}
		if runErr != nil {
			fmt.Printf("| %s | error: %s | | | |\n", e, firstLine(runErr.Error()))
			continue
		}
		js, _ := json.Marshal(out)
		if e == eStar {
			ref = string(js)
		}
		same := "yes"
		if string(js) != ref {
			same = "**no**"
		}
		first := ""
		if l, ok := out.([]any); ok && len(l) > 0 {
			b, _ := json.Marshal(l[0])
			first = strings.ReplaceAll(string(b), "|", "/")
		}
		fmt.Printf("| %s | %s | %s | %s | `%s` |\n", e, fmtDur(median(ds)), fmtDur(slices.Min(ds)), same, first)
	}
	fmt.Printf("\nThe query alone from Go (scan into Go values): median %s.\n", fmtDur(median(goQuery)))
	fmt.Printf("Starlark steps for one transform run: %d (step limit %d).\n\n", lastStarlarkSteps, stepLimit)
}

func median(ds []time.Duration) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[len(s)/2]
}

func determinismTable() {
	fmt.Println("## 6. Determinism")
	fmt.Println()
	fmt.Println("A map gets 1,000 keys in a scrambled order, then the script lists its keys. 20 runs, each in a fresh runtime. The transform from section 5 also runs 20 times.")
	fmt.Println()
	db, _ := openDB()
	fmt.Println("| Engine | Distinct key orders | Key order starts with | Distinct transform outputs |")
	fmt.Println("|---|---|---|---|")
	for _, e := range engineOrder {
		seen, tseen := map[string]bool{}, map[string]bool{}
		first := ""
		for i := 0; i < 20; i++ {
			ctx, cancel := context.WithTimeoutCause(context.Background(), 10*time.Second, errTimeout)
			v, err := engineByName(e).Run(ctx, detSrc[e], RunOpts{})
			s := fmt.Sprint(v)
			if err != nil {
				s = "error: " + err.Error()
			}
			seen[s] = true
			if i == 0 {
				first = clip(s, 24)
			}
			tv, _ := engineByName(e).Run(ctx, transformSrc[e], RunOpts{DB: db})
			js, _ := json.Marshal(tv)
			tseen[string(js)] = true
			cancel()
		}
		fmt.Printf("| %s | %d | `%s` | %d |\n", e, len(seen), first, len(tseen))
	}
	fmt.Println()
}

func costTables() {
	fmt.Println("## 7. Cost")
	fmt.Println()
	fmt.Println("**Startup per run** (in-process, Burrow setup, new runtime + compile + run `1 + 1` + result; 200 runs):")
	fmt.Println()
	fmt.Println("| Engine | Median | Min |")
	fmt.Println("|---|---|---|")
	triv := hostileByID("trivial")
	for _, e := range engineOrder {
		var ds []time.Duration
		for i := 0; i < 200; i++ {
			s := clock()
			engineByName(e).Run(context.Background(), triv.src[e], RunOpts{})
			ds = append(ds, since(s))
		}
		fmt.Printf("| %s | %s | %s |\n", e, fmtDur(median(ds)), fmtDur(slices.Min(ds)))
	}

	fmt.Println()
	fmt.Println("**Memory limit overhead** (Starlark):")
	fmt.Println()
	fmt.Println("| Setup | Median | Min |")
	fmt.Println("|---|---|---|")
	var child []time.Duration
	for i := 0; i < 20; i++ {
		child = append(child, runChildWall(eStar, "trivial"))
	}
	fmt.Printf("| trivial script in a child process + Job Object (start to exit) | %s | %s |\n", fmtDur(median(child)), fmtDur(slices.Min(child)))
	db, _ := openDB()
	// Interleave runs with and without the watchdog so drift hits both equally.
	ds := map[bool][]time.Duration{}
	for i := 0; i < 40; i++ {
		for _, wd := range []bool{false, true} {
			runtime.GC()
			ctx, cancel := context.WithCancelCause(context.Background())
			var w *watchdog
			if wd {
				w = startWatchdog(cancel, watchCap, watchEvery)
			}
			s := clock()
			starlarkEngine{}.Run(ctx, transformSrc[eStar], RunOpts{DB: db})
			ds[wd] = append(ds[wd], since(s))
			if w != nil {
				w.stop()
			}
			cancel(nil)
		}
	}
	for _, wd := range []bool{false, true} {
		name := "transform (section 5), no watchdog"
		if wd {
			name = fmt.Sprintf("transform (section 5), watchdog every %s", watchEvery)
		}
		fmt.Printf("| %s | %s | %s |\n", name, fmtDur(median(ds[wd])), fmtDur(slices.Min(ds[wd])))
	}
	var hs []time.Duration
	s := metricsSample()
	for i := 0; i < 1000; i++ {
		st := clock()
		heapBytes(s)
		hs = append(hs, since(st))
	}
	fmt.Printf("| one heap sample (`runtime/metrics.Read`) | %s | %s |\n", fmtDur(median(hs)), fmtDur(slices.Min(hs)))

	fmt.Println()
	fmt.Println("**Binary size added** (`go build -trimpath -ldflags=\"-s -w\"` of a program that runs `1 + 1`, minus the same program without an engine):")
	fmt.Println()
	fmt.Println("| Engine | Size | Added |")
	fmt.Println("|---|---|---|")
	dir, err := os.MkdirTemp("", "spike016-size")
	if err != nil {
		fmt.Println("tempdir:", err)
		return
	}
	defer os.RemoveAll(dir)
	size := func(pkg string) int64 {
		out := filepath.Join(dir, pkg+".exe")
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", out, "./sizes/"+pkg)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "build", pkg, err, string(b))
			return 0
		}
		st, err := os.Stat(out)
		if err != nil {
			return 0
		}
		return st.Size()
	}
	base := size("base")
	fmt.Printf("| (none) | %.1f MB | |\n", float64(base)/1e6)
	for _, e := range engineOrder {
		n := size(sizePkg[e])
		fmt.Printf("| %s | %.1f MB | %.1f MB |\n", e, float64(n)/1e6, float64(n-base)/1e6)
	}
	fmt.Println()
}

var sizePkg = map[string]string{eStar: "starlark", eGoja: "goja", eLua: "lua", eRis: "risor", eTen: "tengo"}

// runChildWall measures a whole child run: start, job, script, exit.
func runChildWall(engine, caseID string) time.Duration {
	s := clock()
	runChild(engine, caseID, modeJob)
	return since(s)
}
