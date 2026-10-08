package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// cell counts the runs of one group.
type cell struct {
	n, skipped, errors       int
	valid1, valid, right     int
	wrapped, madeUp, copied  int
	prompt, output, requests int
	seconds                  float64
	fails                    map[string]int // first-try failure kinds
}

func (c *cell) add(r Record) {
	if r.Skipped != "" {
		c.skipped++
		return
	}
	c.n++
	if r.Error != "" {
		c.errors++
	}
	if r.Valid1 {
		c.valid1++
	}
	if r.Valid {
		c.valid++
	}
	if r.Wrapped {
		c.wrapped++
	}
	if r.Score != nil {
		if r.Score.Right {
			c.right++
		}
		c.madeUp += r.Score.MadeUp
		c.copied += r.Score.Copied
	}
	c.prompt += r.Prompt
	c.output += r.Output
	c.requests += r.Requests
	c.seconds += r.Seconds
	if c.fails == nil {
		c.fails = map[string]int{}
	}
	for _, k := range r.Fail1 {
		c.fails[k]++
	}
}

func pct(a, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%d%%", (100*a+n/2)/n)
}

func avg(a, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprint((a + n/2) / n)
}

func group(recs []Record, key func(Record) string) (map[string]*cell, []string) {
	m := map[string]*cell{}
	var keys []string
	for _, r := range recs {
		k := key(r)
		if m[k] == nil {
			m[k] = &cell{}
			keys = append(keys, k)
		}
		m[k].add(r)
	}
	return m, keys
}

func methodOrder(a, b string) int {
	return slices.Index(methods, a) - slices.Index(methods, b)
}

func writeReport(files []string, tasks []*Task, out string) error {
	if len(files) == 0 {
		return fmt.Errorf("give the .jsonl files")
	}
	recs, err := readRecords(files)
	if err != nil {
		return err
	}
	rescore(recs, tasks)
	var b strings.Builder
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	fmt.Fprintf(&b, "# SPIKE-030 results\n\nGenerated from %s: %d records.\n", strings.Join(names, ", "), len(recs))
	b.WriteString("\n*Valid* is schema-valid plus the index checks; *right* is valid and equal to the fixed answer. Percentages are of the runs that weren't skipped. Tokens and time are per run, the retry included.\n")

	for _, thinking := range []bool{false, true} {
		part := slices.DeleteFunc(slices.Clone(recs), func(r Record) bool { return r.Thinking != thinking })
		if len(part) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## Thinking %v\n", map[bool]string{false: "off", true: "on"}[thinking])

		b.WriteString("\n### By model and method\n\n| Model | Method | Runs | Valid first | Valid after retry | Right | Made-up values | Copied values | Text around JSON | Prompt | Output | Requests | Time | Errors | Skipped |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
		cells, keys := group(part, func(r Record) string { return r.Model + "\x00" + r.Method })
		sort.SliceStable(keys, func(i, j int) bool {
			mi, ai, _ := strings.Cut(keys[i], "\x00")
			mj, aj, _ := strings.Cut(keys[j], "\x00")
			if mi != mj {
				return mi < mj
			}
			return methodOrder(ai, aj) < 0
		})
		for _, k := range keys {
			c := cells[k]
			model, method, _ := strings.Cut(k, "\x00")
			if c.n == 0 {
				fmt.Fprintf(&b, "| %s | %s | 0 | | | | | | | | | | | | %d |\n", model, method, c.skipped)
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %d | %d | %s | %s | %s | %.1f | %.1f s | %d | %d |\n", model, method, c.n,
				pct(c.valid1, c.n), pct(c.valid, c.n), pct(c.right, c.n), c.madeUp, c.copied, pct(c.wrapped, c.valid),
				avg(c.prompt, c.n), avg(c.output, c.n), float64(c.requests)/float64(c.n), c.seconds/float64(c.n), c.errors, c.skipped)
		}

		b.WriteString("\n### By method, all models\n\n| Method | Runs | Valid first | Valid after retry | Right | Made-up values | Copied values | Output | Time |\n|---|---|---|---|---|---|---|---|---|\n")
		cells, keys = group(part, func(r Record) string { return r.Method })
		slices.SortFunc(keys, methodOrder)
		for _, k := range keys {
			c := cells[k]
			if c.n == 0 {
				continue
			}
			fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %d | %d | %s | %.1f s |\n", k, c.n, pct(c.valid1, c.n), pct(c.valid, c.n), pct(c.right, c.n), c.madeUp, c.copied, avg(c.output, c.n), c.seconds/float64(c.n))
		}

		b.WriteString("\n### Right answers by task and method, all models\n\nRight of runs.\n\n| Task |")
		ms := present(part, func(r Record) string { return r.Method })
		slices.SortFunc(ms, methodOrder)
		for _, m := range ms {
			b.WriteString(" " + m + " |")
		}
		b.WriteString("\n|---|" + strings.Repeat("---|", len(ms)) + "\n")
		cells, _ = group(part, func(r Record) string { return r.Task + "\x00" + r.Method })
		for _, t := range taskIDs(tasks) {
			if len(present(part, func(r Record) string {
				if r.Task == t {
					return t
				}
				return ""
			})) == 0 {
				continue
			}
			b.WriteString("| " + t + " |")
			for _, m := range ms {
				c := cells[t+"\x00"+m]
				if c == nil || c.n == 0 {
					b.WriteString(" – |")
					continue
				}
				fmt.Fprintf(&b, " %d/%d |", c.right, c.n)
			}
			b.WriteString("\n")
		}

		b.WriteString("\n### First-try failures by method\n\n| Method | Failures |\n|---|---|\n")
		cells, keys = group(part, func(r Record) string { return r.Method })
		slices.SortFunc(keys, methodOrder)
		for _, k := range keys {
			var fs []string
			for f, n := range cells[k].fails {
				fs = append(fs, fmt.Sprintf("%s %d", f, n))
			}
			slices.Sort(fs)
			fmt.Fprintf(&b, "| %s | %s |\n", k, strings.Join(fs, ", "))
		}
	}

	if pb, err := os.ReadFile(filepath.Join(filepath.Dir(files[0]), "probe.json")); err == nil {
		var known map[string]Support
		if json.Unmarshal(pb, &known) == nil {
			b.WriteString("\n## What each API accepts (probe)\n\n| Model | tool_choice named | tool_choice required | JSON schema mode | JSON object mode | Notes |\n|---|---|---|---|---|---|\n")
			var keys []string
			for k := range known {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			yes := map[bool]string{true: "yes", false: "no"}
			for _, k := range keys {
				s := known[k]
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", k, yes[s.ForcedNamed], yes[s.ForcedRequired], yes[s.JSONSchema], yes[s.JSONObject], strings.ReplaceAll(s.Notes, "|", "/"))
			}
		}
	}

	var errs []string
	for _, r := range recs {
		if r.Error != "" {
			errs = append(errs, fmt.Sprintf("- %s, %s, %s r%d: %s %s", r.Model, r.Method, r.Task, r.Rep, r.ErrKind, clipLine(r.Error, 200)))
		}
	}
	if len(errs) > 0 {
		b.WriteString("\n## Provider errors\n\n" + strings.Join(errs, "\n") + "\n")
	}
	return os.WriteFile(out, []byte(b.String()), 0o644)
}

func present(recs []Record, key func(Record) string) []string {
	var out []string
	for _, r := range recs {
		if k := key(r); k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// rescore scores each valid answer again with the current scoring, so the
// scoring can change without asking the models again.
func rescore(recs []Record, tasks []*Task) {
	byID := map[string]*Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	for i, r := range recs {
		t := byID[r.Task]
		if !r.Valid || t == nil {
			continue
		}
		ans := r.Answer1
		if !r.Valid1 {
			ans = r.Answer2
		}
		var v any
		var err error
		if r.Method == mTool || r.Method == mForced {
			v, err = parseArgs(json.RawMessage(ans))
		} else {
			v, _, err = parseText(ans)
		}
		if err != nil {
			continue // an answer clipped by an older run
		}
		sch, err := compileSchema(t.ID, t.Schema)
		if err != nil || !check(t, sch, v).valid() {
			continue
		}
		sc := scoreAnswer(t, v)
		recs[i].Score = &sc
	}
}
