package main

// The report: runs.jsonl of one or more results folders, summed per
// model, condition and kind of follow-up, into results.md in the last
// folder. A run that appears twice counts once, from the later line. A
// follow-up with no card has no cards run: its cards row counts its
// today runs, marked.

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var kinds = []string{"repeat", "paraphrase", "control"}

type cell struct {
	runs, done, passed, broke, stopped     int
	requests, prompt, history, withHistory int
	borrowed                               int // today's runs counted for a follow-up with no card
}

func (c *cell) add(l line) {
	if l.Stopped || l.Skipped != "" {
		c.stopped++
		return
	}
	c.runs++
	if l.Done {
		c.done++
	}
	if l.Passed {
		c.passed++
	}
	if l.Error != "" {
		c.broke++
	}
	c.requests += l.Requests
	c.prompt += l.Prompt
	c.history += l.History
	if l.History > 0 {
		c.withHistory++
	}
}

func readAll(dirs []string) ([]line, error) {
	seen := map[string]int{}
	var out []line
	for _, d := range dirs {
		ls, err := readLines(d)
		if err != nil {
			return nil, err
		}
		for _, l := range ls {
			k := runKey(l.Model, l.Cond, l.Scenario, l.Rep)
			if i, ok := seen[k]; ok {
				out[i] = l
				continue
			}
			seen[k] = len(out)
			out = append(out, l)
		}
	}
	return out, nil
}

func writeReport(dirs []string, plans map[string]plan) error {
	runs, err := readAll(dirs)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return errors.New("no runs")
	}
	var models []string
	sent := map[string]int{}
	for _, l := range runs {
		if !slices.Contains(models, l.Model) {
			models = append(models, l.Model)
		}
		sent[l.Model] += l.Provider
	}
	slices.Sort(models)
	type key struct{ model, cond, kind string }
	cells := map[key]*cell{}
	get := func(k key) *cell {
		if cells[k] == nil {
			cells[k] = &cell{}
		}
		return cells[k]
	}
	byScen := map[key]*cell{} // kind is the scenario here
	for _, l := range runs {
		get(key{l.Model, l.Cond, l.Kind}).add(l)
		if byScen[key{l.Model, l.Cond, l.Scenario}] == nil {
			byScen[key{l.Model, l.Cond, l.Scenario}] = &cell{}
		}
		byScen[key{l.Model, l.Cond, l.Scenario}].add(l)
		if p := plans[l.Scenario]; l.Cond == condToday && len(p.Cards) == 0 {
			c := get(key{l.Model, condCards, l.Kind})
			c.add(l)
			c.borrowed++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# TASK-027: history cards, live\n\nFrom %s. A run is done when every check but info passed and each tool the checks need ran without an error. Stopped runs (a quota, a rate limit or the harness budget) are left out. History calls are search_history and read_messages in the follow-up.\n\n", strings.Join(dirs, ", "))
	for _, m := range models {
		fmt.Fprintf(&b, "## %s\n\nRequests the provider got (all runs, stopped ones too, titles and memory too): %d.\n\n", m, sent[m])
		fmt.Fprintf(&b, "| follow-up | condition | done | runs with history calls | history calls/run | requests/run | prompt tokens/run | broke | stopped |\n|---|---|---|---|---|---|---|---|---|\n")
		for _, k := range kinds {
			for _, c := range allConds {
				x := cells[key{m, c, k}]
				if x == nil {
					continue
				}
				mark := ""
				if x.borrowed > 0 {
					mark = fmt.Sprintf(" (%d of them today's: no card)", x.borrowed)
				}
				fmt.Fprintf(&b, "| %s | %s | %s%s | %s | %s | %s | %s | %d | %d |\n", k, c, share(x.done, x.runs), mark,
					share(x.withHistory, x.runs), per(x.history, x.runs), per(x.requests, x.runs), per(x.prompt, x.runs), x.broke, x.stopped)
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## By follow-up\n\nDone of the counted runs, today / cards; \"=\" is a follow-up with no card, which has no cards run.\n\n| follow-up | test | cards |")
	for _, m := range models {
		fmt.Fprintf(&b, " %s |", shortModel(m))
	}
	b.WriteString("\n|---|---|---|" + strings.Repeat("---|", len(models)) + "\n")
	ids := make([]string, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Or(cmp.Compare(chatOf(a), chatOf(b)), cmp.Compare(kindRank(plans[a].Kind), kindRank(plans[b].Kind)))
	})
	for _, id := range ids {
		p := plans[id]
		cards := "none"
		if len(p.Cards) > 0 {
			cards = fmt.Sprint(p.Cards)
		}
		fmt.Fprintf(&b, "| %s | %s | %s |", id, p.Kind, cards)
		for _, m := range models {
			t, c := byScen[key{m, condToday, id}], byScen[key{m, condCards, id}]
			ts, cs := "-", "-"
			if t != nil {
				ts = fmt.Sprintf("%d/%d", t.done, t.runs)
			}
			switch {
			case len(p.Cards) == 0:
				cs = "="
			case c != nil:
				cs = fmt.Sprintf("%d/%d", c.done, c.runs)
			}
			fmt.Fprintf(&b, " %s / %s |", ts, cs)
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(dirs[len(dirs)-1], "results.md"), []byte(b.String()), 0o644)
}

func kindRank(k string) int { return slices.Index(kinds, k) }

func shortModel(m string) string {
	_, id, _ := strings.Cut(m, "/")
	return id
}

func share(k, n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", k, n, 100*float64(k)/float64(n))
}

func per(k, n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", float64(k)/float64(n))
}
