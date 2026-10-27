package main

// The report: runs.jsonl of one or more results folders, summed per set,
// model and condition, into results.md in the last folder. A run that
// appears twice (the same set, condition, model, scenario and rep) counts
// once, from the later folder; so do a set's sizes.

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/scenario"
)

var allConds = []string{"1", "2", "3", "4"}

var setOrder = []string{"multi-part", "skill-loading", "rare-tools", longSet}

type cellKey struct{ set, model, cond string }

// cell sums the counted runs of a set, model and condition.
type cell struct {
	runs, passed, task, done, broke, stopped int
	requests, prompt, cacheRead              int
	calls                                    calls
	cardRuns, cardHits, carded               int
	failedChecks                             map[string]int // failed checks by type, info left out
}

func readRuns(dirs []string) ([]line, error) {
	seen := map[string]int{}
	var out []line
	for _, d := range dirs {
		f, err := os.Open(filepath.Join(d, "runs.jsonl"))
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var l line
			if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", d, err)
			}
			k := fmt.Sprintf("%s|%s|%s|%s|%d", l.Set, l.Cond, l.Model, l.Scenario, l.Rep)
			if i, ok := seen[k]; ok {
				out[i] = l
				continue
			}
			seen[k] = len(out)
			out = append(out, l)
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func writeReport(dirs []string) error {
	runs, err := readRuns(dirs)
	if err != nil {
		return err
	}
	cells := map[cellKey]*cell{}
	var models []string
	for _, l := range runs {
		if l.Skipped != "" {
			continue
		}
		k := cellKey{l.Set, l.Model, l.Cond}
		c := cells[k]
		if c == nil {
			c = &cell{failedChecks: map[string]int{}}
			cells[k] = c
		}
		if !slices.Contains(models, l.Model) {
			models = append(models, l.Model)
		}
		if l.Stopped {
			c.stopped++
			continue
		}
		c.runs++
		if l.Error != "" {
			c.broke++
		}
		if l.Passed {
			c.passed++
		}
		if l.Task {
			c.task++
		}
		if l.Done {
			c.done++
		}
		c.requests += l.Requests
		c.prompt += l.Prompt
		c.cacheRead += l.CacheRead
		addCalls(&c.calls, l.Calls)
		if hasCards(l.Cond) {
			c.carded += len(l.Cards)
		}
		if l.CardHit != nil {
			c.cardRuns++
			if *l.CardHit {
				c.cardHits++
			}
		}
		for _, a := range l.Asserts {
			if a.Status == scenario.Fail && a.Test != "info" {
				c.failedChecks[a.Type]++
			}
		}
	}
	slices.SortFunc(models, func(a, b string) int {
		return cmp.Or(cmp.Compare(modelRank(a), modelRank(b)), strings.Compare(a, b))
	})
	var b strings.Builder
	fmt.Fprintf(&b, "# SPIKE-028 runs\n\nFrom %s. Conditions: 1 every tool native (today), 2 the tool index, 3 the index and history cards, 4 today's tools and history cards (3 and 4: long chats only).\n", strings.Join(dirs, ", "))
	b.WriteString("Task: passed without the request and prompt-size checks (the index adds a describe_tool request by design). Done: a task whose needed tools each ran once without an error (the runner's tool_called also counts refused calls). Stopped runs (a quota or the spike's budget) are left out.\n")
	for _, set := range setOrder {
		if !slices.ContainsFunc(models, func(m string) bool { return hasSet(cells, set, m) }) {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", set)
		b.WriteString("| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |")
		if set == longSet {
			b.WriteString(" card had the needed turn | cards added |")
		}
		b.WriteString("\n|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|")
		if set == longSet {
			b.WriteString("--:|--:|")
		}
		b.WriteString("\n")
		for _, m := range models {
			for _, cond := range allConds {
				c := cells[cellKey{set, m, cond}]
				if c == nil {
					continue
				}
				n := max(c.runs, 1)
				fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %d | %d | %.1f | %d | %s | %d | %d | %d | %d | %s | %d | %d | %d | %d → %d | %d |",
					m, cond, c.runs, share(c.passed, c.runs), share(c.task, c.runs), share(c.done, c.runs), c.broke, c.stopped,
					float64(c.requests)/float64(n), c.prompt/n, pct(c.cacheRead, c.prompt),
					c.calls.Rare, c.calls.Indexed, c.calls.Describes, c.calls.Blind,
					share(c.calls.ArgErrors, c.calls.Rare), c.calls.Unknown, c.calls.Native, c.calls.Nested, c.calls.Failed, c.calls.Recovered, c.calls.History)
				if set == longSet {
					if hasCards(cond) {
						fmt.Fprintf(&b, " %s | %d |", share(c.cardHits, c.cardRuns), c.carded)
					} else {
						b.WriteString(" – | – |")
					}
				}
				b.WriteString("\n")
			}
		}
		writeFailed(&b, cells, set, models)
		writeScenarios(&b, runs, set, models)
	}
	writeSizes(&b, dirs)
	return os.WriteFile(filepath.Join(dirs[len(dirs)-1], "results.md"), []byte(b.String()), 0o644)
}

func modelRank(m string) int {
	if i := slices.Index(split(refModels), m); i >= 0 {
		return i
	}
	return 99
}

func hasSet(cells map[cellKey]*cell, set, model string) bool {
	for _, c := range allConds {
		if cells[cellKey{set, model, c}] != nil {
			return true
		}
	}
	return false
}

func addCalls(to *calls, c calls) {
	to.Rare += c.Rare
	to.Indexed += c.Indexed
	to.Describes += c.Describes
	to.ArgErrors += c.ArgErrors
	to.Unknown += c.Unknown
	to.Native += c.Native
	to.Nested += c.Nested
	to.Blind += c.Blind
	to.Failed += c.Failed
	to.Recovered += c.Recovered
	to.History += c.History
}

// writeFailed lists the failed checks by type: a tool or skill missed shows
// as tool_called or skill_loaded.
func writeFailed(b *strings.Builder, cells map[cellKey]*cell, set string, models []string) {
	var lines []string
	for _, m := range models {
		for _, cond := range allConds {
			c := cells[cellKey{set, m, cond}]
			if c == nil || len(c.failedChecks) == 0 {
				continue
			}
			var parts []string
			for _, t := range slices.Sorted(maps.Keys(c.failedChecks)) {
				parts = append(parts, fmt.Sprintf("%s %d", t, c.failedChecks[t]))
			}
			lines = append(lines, fmt.Sprintf("- %s, condition %s: %s", m, cond, strings.Join(parts, ", ")))
		}
	}
	if len(lines) > 0 {
		b.WriteString("\nFailed checks by type:\n\n" + strings.Join(lines, "\n") + "\n")
	}
}

// writeScenarios is each scenario's done runs, k of n, by model and
// condition.
func writeScenarios(b *strings.Builder, runs []line, set string, models []string) {
	type sk struct{ scenario, model, cond string }
	pass, total := map[sk]int{}, map[sk]int{}
	var scens []string
	var cols []sk
	for _, l := range runs {
		if l.Set != set || l.Skipped != "" || l.Stopped {
			continue
		}
		k := sk{l.Scenario, l.Model, l.Cond}
		total[k]++
		if l.Done {
			pass[k]++
		}
		if !slices.Contains(scens, l.Scenario) {
			scens = append(scens, l.Scenario)
		}
		if c := (sk{"", l.Model, l.Cond}); !slices.Contains(cols, c) {
			cols = append(cols, c)
		}
	}
	if len(scens) == 0 {
		return
	}
	slices.Sort(scens)
	slices.SortFunc(cols, func(a, b sk) int {
		return cmp.Or(cmp.Compare(modelRank(a.model), modelRank(b.model)), strings.Compare(a.model, b.model), strings.Compare(a.cond, b.cond))
	})
	b.WriteString("\nDone by scenario:\n\n| scenario |")
	for _, c := range cols {
		fmt.Fprintf(b, " %s c%s |", shortModel(c.model), c.cond)
	}
	b.WriteString("\n|---|" + strings.Repeat("--:|", len(cols)) + "\n")
	for _, s := range scens {
		fmt.Fprintf(b, "| %s |", s)
		for _, c := range cols {
			k := sk{s, c.model, c.cond}
			if total[k] == 0 {
				b.WriteString(" – |")
				continue
			}
			fmt.Fprintf(b, " %d/%d |", pass[k], total[k])
		}
		b.WriteString("\n")
	}
}

func shortModel(m string) string {
	if _, id, ok := strings.Cut(m, "/"); ok {
		return id
	}
	return m
}

func share(k, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d", k, n)
}

func pct(k, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(k)/float64(n))
}
