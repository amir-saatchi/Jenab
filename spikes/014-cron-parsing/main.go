// SPIKE-014: which library parses 5-field cron and computes Next in an IANA zone correctly?
//
// Usage: go run . > results.md
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// burrowMode is the DST rule the `want` column encodes (the cronie rule, see README).
const burrowMode = modeCronie

const hangLimit = 3 * time.Second

func main() {
	// Run the timing part before part 3, so a hanging Next (if any) cannot disturb it.
	p1 := part1()
	p2 := part2()
	p5 := part5()
	p3 := part3()
	p4 := part4()

	fmt.Printf("# SPIKE-014 results\n\nGo %s, %s/%s, %s. Go's tz database: %s.\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"), tzVersion())
	fmt.Println("Every library gets `t.In(zone)` and returns the first run strictly after `t`. Times are shown in the case's zone.")
	fmt.Println("\"ref\" is the brute-force reference in `ref.go` (steps minute by minute in the zone).")
	fmt.Println()
	fmt.Print(p1, p2, p3, p4, p5)
}

func tzVersion() string {
	b, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "lib", "time", "update.bash"))
	if err != nil {
		return "unknown"
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "DATA=") {
			return "tzdata " + strings.TrimSpace(strings.TrimPrefix(l, "DATA="))
		}
	}
	return "unknown"
}

func mustLoc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func show(t time.Time) string {
	if t.IsZero() {
		return "zero time"
	}
	if t.Second() != 0 {
		return t.Format("2006-01-02 15:04:05 Z07:00")
	}
	return t.Format("2006-01-02 15:04 Z07:00")
}

type outcome struct {
	parseErr error
	nextErr  error
	next     time.Time
	panicked string
	hung     bool
	took     time.Duration
}

// runLib runs parse + Next with a panic guard and a hang limit.
func runLib(l lib, expr string, from time.Time) outcome {
	ch := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			if r := recover(); r != nil {
				o.panicked = fmt.Sprint(r)
			}
			ch <- o
		}()
		start := clock()
		nf, err := l.parse(expr)
		if err != nil {
			o.parseErr = err
			o.took = since(start)
			return
		}
		o.next, o.nextErr = nf(from)
		o.took = since(start)
	}()
	select {
	case o := <-ch:
		return o
	case <-time.After(hangLimit):
		return outcome{hung: true, took: hangLimit}
	}
}

func (o outcome) cell() string {
	switch {
	case o.hung:
		return fmt.Sprintf("hang (>%s)", hangLimit)
	case o.panicked != "":
		return "panic: " + clip(o.panicked, 50)
	case o.parseErr != nil:
		return "parse error: " + clip(o.parseErr.Error(), 60)
	case o.nextErr != nil:
		return "Next error: " + clip(o.nextErr.Error(), 40) + " (" + show(o.next) + ")"
	}
	return show(o.next)
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "|", "\\|")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func code(s string) string {
	if s == "" {
		return "(empty)"
	}
	s = strings.ReplaceAll(s, "\t", "\\t")
	if strings.TrimSpace(s) != s || strings.Contains(s, "  ") {
		return "`\"" + s + "\"`"
	}
	return "`" + s + "`"
}

func good(o outcome, want time.Time) bool {
	return !o.hung && o.panicked == "" && o.parseErr == nil && o.nextErr == nil && o.next.Equal(want)
}

// ---------- part 1 + summary ----------

func part1() string {
	var b strings.Builder
	b.WriteString("## 1. Expected next runs\n\n")
	b.WriteString("`want` uses Vixie/cronie day matching and, for DST cases, the cronie rule: a fixed-time job (minute and hour without `*`) whose time is skipped runs once at the first minute after the gap, and a repeated time runs only at its first occurrence; a job with `*` in minute or hour follows the real clock. \"ok\" means the library returned exactly `want`.\n\n")
	b.WriteString("| # | Group | Expression | Zone | From | Want | ref |")
	for _, l := range libs {
		b.WriteString(" " + l.short + " |")
	}
	b.WriteString(" Note |\n|---|---|---|---|---|---|---|")
	for range libs {
		b.WriteString("---|")
	}
	b.WriteString("---|\n")

	pass := map[string]map[string]int{}
	total := map[string]int{}
	var refBad int
	for i, c := range cases {
		loc := mustLoc(c.zone)
		from := mustTime(c.from).In(loc)
		want := mustTime(c.want).In(loc)
		spec, err := refParse(c.expr)
		refCell := ""
		if err != nil {
			refCell = "parse error: " + err.Error()
			refBad++
		} else if got, ok := refNext(spec, from, loc, burrowMode); !ok || !got.Equal(want) {
			refCell = "**" + show(got) + "**"
			refBad++
		} else {
			refCell = "ok"
		}
		total[c.group]++
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s |", i+1, c.group, code(c.expr), c.zone, show(from), show(want), refCell)
		for _, l := range libs {
			o := runLib(l, c.expr, from)
			if pass[l.short] == nil {
				pass[l.short] = map[string]int{}
			}
			if good(o, want) {
				pass[l.short][c.group]++
				b.WriteString(" ok |")
			} else {
				b.WriteString(" **" + o.cell() + "** |")
			}
		}
		fmt.Fprintf(&b, " %s |\n", c.note)
	}
	fmt.Fprintf(&b, "\nReference disagrees with the hand-written `want` in %d of %d cases.\n\n", refBad, len(cases))

	// Same cases with the candidate Burrow DST layer (dst.go) on top of each library.
	fails := map[string][]string{}
	for i, c := range cases {
		loc := mustLoc(c.zone)
		from := mustTime(c.from).In(loc)
		want := mustTime(c.want).In(loc)
		for _, l := range libs {
			w := withDSTRule(l)
			if pass[w.short] == nil {
				pass[w.short] = map[string]int{}
			}
			if o := runLib(w, c.expr, from); good(o, want) {
				pass[w.short][c.group]++
			} else {
				fails[w.short] = append(fails[w.short], fmt.Sprintf("#%d %s", i+1, o.cell()))
			}
		}
	}

	b.WriteString("### Summary (passed / total)\n\n| Library | ")
	for _, g := range groups {
		b.WriteString(g + " | ")
	}
	b.WriteString("all | Failures (with DST layer) |\n|---|")
	for range groups {
		b.WriteString("---|")
	}
	b.WriteString("---|---|\n")
	var rows []lib
	for _, l := range libs {
		rows = append(rows, l)
	}
	for _, l := range libs {
		rows = append(rows, withDSTRule(l))
	}
	for _, l := range rows {
		b.WriteString("| " + l.name + " | ")
		all, allT := 0, 0
		for _, g := range groups {
			fmt.Fprintf(&b, "%d/%d | ", pass[l.short][g], total[g])
			all += pass[l.short][g]
			allT += total[g]
		}
		f := strings.Join(fails[l.short], "; ")
		if !strings.HasSuffix(l.short, "+layer") {
			f = "see table above"
		}
		fmt.Fprintf(&b, "%d/%d | %s |\n", all, allT, f)
	}
	b.WriteString("\n\"+ DST layer\" rows run the same cases through `dst.go`: the library computes only in UTC on the naive wall clock, and Burrow maps the result back to the zone with the cronie rule.\n\n")
	return b.String()
}

// ---------- part 2: DST behaviour ----------

func part2() string {
	var b strings.Builder
	b.WriteString("## 2. DST behaviour\n\n")
	b.WriteString("Reference in three modes: **wall** = match the wall clock literally (a skipped time never runs, a repeated time runs twice); **once** = rule A; **cronie** = rule A for fixed-time jobs, wall for jobs whose minute or hour field starts with `*` (what cronie does). Each library cell lists the modes its answer agrees with.\n\n")
	b.WriteString("| # | Expression | Zone | From | wall | once | cronie |")
	for _, l := range libs {
		b.WriteString(" " + l.short + " |")
	}
	b.WriteString("\n|---|---|---|---|---|---|---|")
	for range libs {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	agree := map[string]map[dstMode]int{}
	n := 0
	for i, c := range cases {
		if c.group != gDST {
			continue
		}
		n++
		loc := mustLoc(c.zone)
		from := mustTime(c.from).In(loc)
		spec, _ := refParse(c.expr)
		ref := map[dstMode]time.Time{}
		for _, m := range []dstMode{modeWall, modeOnce, modeCronie} {
			ref[m], _ = refNext(spec, from, loc, m)
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s |", i+1, code(c.expr), c.zone, show(from), show(ref[modeWall]), show(ref[modeOnce]), show(ref[modeCronie]))
		for _, l := range libs {
			o := runLib(l, c.expr, from)
			if agree[l.short] == nil {
				agree[l.short] = map[dstMode]int{}
			}
			var ms []string
			for _, m := range []dstMode{modeWall, modeOnce, modeCronie} {
				if good(o, ref[m]) {
					ms = append(ms, modeNames[m])
					agree[l.short][m]++
				}
			}
			if len(ms) == 3 {
				b.WriteString(" all |")
			} else if len(ms) > 0 {
				b.WriteString(" " + strings.Join(ms, ", ") + " |")
			} else {
				b.WriteString(" **" + o.cell() + "** |")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n| Library | agrees with wall | agrees with once | agrees with cronie |\n|---|---|---|---|\n")
	for _, l := range libs {
		fmt.Fprintf(&b, "| %s | %d/%d | %d/%d | %d/%d |\n", l.short, agree[l.short][modeWall], n, agree[l.short][modeOnce], n, agree[l.short][modeCronie], n)
	}
	b.WriteString("\n")
	return b.String()
}

// ---------- part 3: invalid and never-matching ----------

func part3() string {
	var b strings.Builder
	b.WriteString("## 3. Invalid, non-standard and never-matching expressions\n\n")
	b.WriteString("Parse + Next from 2026-09-28 10:00 UTC, with a 3 s hang limit. \"Burrow\" is what the trigger check should do. \"ref\" is the strict parser in `ref.go` (a never-matching expression shows as \"no run in 9 years\"). Time is Parse + Next.\n\n")
	b.WriteString("| Expression | Note | Burrow | ref |")
	for _, l := range libs {
		b.WriteString(" " + l.short + " |")
	}
	b.WriteString("\n|---|---|---|---|")
	for range libs {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	from := mustTime("2026-09-28T10:00:00Z")
	agree := map[string]int{}
	refAgree := 0
	for _, c := range invalid {
		burrow := "reject"
		if c.accept {
			burrow = "accept"
		}
		refCell := ""
		refAccepts := false
		if s, err := refParse(c.expr); err != nil {
			refCell = "error: " + clip(err.Error(), 40)
		} else if nx, ok := refNext(s, from, time.UTC, burrowMode); !ok {
			refCell = "no run in 9 years"
		} else {
			refCell = show(nx)
			refAccepts = true
		}
		if refAccepts == c.accept {
			refAgree++
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |", code(c.expr), c.note, burrow, refCell)
		for _, l := range libs {
			o := runLib(l, c.expr, from)
			accepted := !o.hung && o.panicked == "" && o.parseErr == nil && o.nextErr == nil && !o.next.IsZero()
			if accepted == c.accept {
				agree[l.short]++
			}
			fmt.Fprintf(&b, " %s, %s |", o.cell(), dur(o.took))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n\"Agrees\" = the library returns a usable next run exactly when Burrow should accept (a parse error, Next error, zero time, hang or panic all count as rejecting).\n\n| Library | agrees with Burrow |\n|---|---|\n| ref | %d/%d |\n", refAgree, len(invalid))
	for _, l := range libs {
		fmt.Fprintf(&b, "| %s | %d/%d |\n", l.short, agree[l.short], len(invalid))
	}
	b.WriteString("\n")
	return b.String()
}

func dur(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2f s", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.2f ms", float64(d)/1e6)
	default:
		return fmt.Sprintf("%.1f µs", float64(d)/1e3)
	}
}

// ---------- part 5: speed ----------

var speedExprs = []string{"*/15 * * * *", "30 2 * * 1-5", "0 9 1,15 * MON", "0 0 29 2 *"}

func part5() string {
	var b strings.Builder
	const n = 3000
	loc := mustLoc("Europe/Berlin")
	from := mustTime("2026-09-28T10:00:00Z").In(loc)
	b.WriteString("## 5. Speed\n\n")
	fmt.Fprintf(&b, "Parse + Next, %d runs per cell after 200 warm-up runs, Europe/Berlin, from 2026-09-28 12:00. p50 (p99). Timed with the Windows performance counter.\n\n| Library |", n)
	for _, e := range speedExprs {
		b.WriteString(" " + code(e) + " |")
	}
	b.WriteString(" Next only, `30 2 * * 1-5` |\n|---|")
	for range speedExprs {
		b.WriteString("---|")
	}
	b.WriteString("---|\n")
	for _, l := range append(append([]lib{}, libs...), withDSTRule(libs[2])) { // plus gronx through the DST layer
		b.WriteString("| " + l.short + " |")
		for _, e := range speedExprs {
			ds := make([]time.Duration, 0, n)
			for i := 0; i < n+200; i++ {
				s := clock()
				nf, err := l.parse(e)
				if err == nil {
					nf(from)
				}
				d := since(s)
				if i >= 200 {
					ds = append(ds, d)
				}
			}
			b.WriteString(" " + pct(ds) + " |")
		}
		nf, _ := l.parse("30 2 * * 1-5")
		ds := make([]time.Duration, 0, n)
		for i := 0; i < n+200; i++ {
			s := clock()
			nf(from)
			if d := since(s); i >= 200 {
				ds = append(ds, d)
			}
		}
		b.WriteString(" " + pct(ds) + " |\n")
	}
	b.WriteString("\n")
	return b.String()
}

func pct(ds []time.Duration) string {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return fmt.Sprintf("%s (%s)", dur(ds[len(ds)/2]), dur(ds[len(ds)*99/100]))
}
