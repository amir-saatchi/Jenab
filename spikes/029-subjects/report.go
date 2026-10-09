package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// readRuns reads runs.jsonl files. A job run twice keeps its last line,
// so a resumed run replaces a failed one.
func readRuns(paths ...string) ([]runResult, error) {
	byJob := map[job]runResult{}
	var order []job
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r runResult
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if _, ok := byJob[r.job]; !ok {
				order = append(order, r.job)
			}
			byJob[r.job] = r
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]runResult, 0, len(order))
	for _, j := range order {
		out = append(out, byJob[j])
	}
	return out, nil
}

func scenarioByID(id string) (scenarioDef, bool) {
	i := slices.IndexFunc(scenarios, func(s scenarioDef) bool { return s.ID == id })
	if i < 0 {
		return scenarioDef{}, false
	}
	return scenarios[i], true
}

// report is results.md: one table per measure group, by model and
// condition, and the checks one by one.
func report(runs []runResult) string {
	type key struct {
		model string
		cond  int
	}
	agg := map[key]*score{}
	count := map[key]int{}
	var keys []key
	type ckey struct {
		key
		scenario string
		msg      int
		what     string
	}
	checks := map[ckey][2]int{} // ok, n
	var ckeys []ckey
	for _, r := range runs {
		sc, ok := scenarioByID(r.Scenario)
		if !ok {
			continue
		}
		k := key{r.Model, r.Cond}
		if agg[k] == nil {
			agg[k] = &score{}
			keys = append(keys, k)
		}
		s := scoreRun(r, sc)
		agg[k].add(s)
		if s.Failed {
			agg[k].Failed = true
		}
		count[k]++
		for _, c := range s.checks {
			ck := ckey{k, c.Scenario, c.Msg, c.What}
			v, seen := checks[ck]
			if !seen {
				ckeys = append(ckeys, ck)
			}
			v[1]++
			if c.OK {
				v[0]++
			}
			checks[ck] = v
		}
	}
	slices.SortFunc(keys, func(a, b key) int {
		if c := strings.Compare(a.model, b.model); c != 0 {
			return c
		}
		return a.cond - b.cond
	})
	failed := map[key]int{}
	for _, r := range runs {
		if r.Error != "" {
			failed[key{r.Model, r.Cond}]++
		}
	}
	pct := func(a, b int) string {
		if b == 0 {
			return "–"
		}
		return fmt.Sprintf("%d/%d (%.0f%%)", a, b, 100*float64(a)/float64(b))
	}
	per := func(a, n int) string {
		if n == 0 {
			return "–"
		}
		return fmt.Sprintf("%.1f", float64(a)/float64(n))
	}
	var b strings.Builder
	b.WriteString("# SPIKE-029 results\n\n")
	var rules []string
	for _, r := range runs {
		if !slices.Contains(rules, r.rule()) {
			rules = append(rules, r.rule())
		}
	}
	fmt.Fprintf(&b, "%d runs, prompt rule %s. Conditions: 1 baseline, 2 tool, 3 tool + index, 4 tool + index + the app's check.\n\n", len(runs), strings.Join(rules, ", "))

	b.WriteString("## Keeping the subjects\n\n")
	b.WriteString("| model | condition | runs | work turns with a subject, on their own | with the app's check | nudges | subjects at the end, per run | made for questions | changes that updated the existing subject | duplicates | update_subject alone in its response |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		if k.cond < condTool {
			continue
		}
		s := agg[k]
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %d | %s | %d | %s | %d | %s |\n", k.model, condNames[k.cond], count[k],
			pct(s.SubjOwn, s.Work), pct(s.SubjCheck, s.Work), s.Nudges, per(s.Subjects, count[k]), s.Noise, pct(s.ChangeUpd, s.ChangeN), s.Duplicates, pct(s.Alone, s.Updates))
	}

	b.WriteString("\n## Correctness and payoff\n\n")
	b.WriteString("| model | condition | subject status right | follow-up answers right | calls that needed earlier turns right | history reads per run | get_subject per run |\n|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		s := agg[k]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", k.model, condNames[k.cond],
			pct(s.StatusOK, s.StatusN), pct(s.AnswerOK, s.AnswerN), pct(s.CallOK, s.CallN), per(s.History, count[k]), per(s.GetSubject, count[k]))
	}

	b.WriteString("\n## Cost\n\n")
	b.WriteString("| model | condition | requests per run | prompt tokens per run | output tokens per run | request errors | runs that ended early |\n|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		s := agg[k]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %d |\n", k.model, condNames[k.cond],
			per(s.Requests, count[k]), per(s.Prompt, count[k]), per(s.Output, count[k]), s.ReqErrors, failed[k])
	}

	b.WriteString("\n## Checks one by one\n\nPasses out of runs, per condition.\n\n")
	models := []string{}
	for _, k := range keys {
		if !slices.Contains(models, k.model) {
			models = append(models, k.model)
		}
	}
	for _, m := range models {
		fmt.Fprintf(&b, "### %s\n\n| scenario | # | check | message | 1 | 2 | 3 | 4 |\n|---|---|---|---|---|---|---|---|\n", m)
		type row struct {
			scenario string
			msg      int
			what     string
		}
		var rows []row
		for _, ck := range ckeys {
			if ck.model == m {
				r := row{ck.scenario, ck.msg, ck.what}
				if !slices.Contains(rows, r) {
					rows = append(rows, r)
				}
			}
		}
		slices.SortFunc(rows, func(a, b row) int {
			if c := strings.Compare(a.scenario, b.scenario); c != 0 {
				return c
			}
			if a.msg != b.msg {
				return a.msg - b.msg
			}
			return strings.Compare(a.what, b.what)
		})
		for _, r := range rows {
			sc, _ := scenarioByID(r.scenario)
			fmt.Fprintf(&b, "| %s | %d | %s | %s |", r.scenario, r.msg+1, r.what, clip(sc.Messages[r.msg].Text, 60))
			for c := condBaseline; c <= condCheck; c++ {
				v, ok := checks[ckey{key{m, c}, r.scenario, r.msg, r.what}]
				if !ok {
					b.WriteString(" – |")
					continue
				}
				fmt.Fprintf(&b, " %d/%d |", v[0], v[1])
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// transcript is one run as Markdown.
func transcript(r runResult, sc scenarioDef) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · %s · condition %s · rep %d\n\n", r.Model, r.Scenario, condNames[r.Cond], r.Rep)
	if r.Error != "" {
		fmt.Fprintf(&b, "**Run error:** %s\n\n", r.Error)
	}
	for _, mr := range r.Messages {
		m := sc.Messages[mr.Index]
		fmt.Fprintf(&b, "## %d · %s %s · turns %v · %d requests\n\n**User:** %s\n\n", mr.Index+1, m.Kind, m.Topic, mr.Turns, mr.Requests, m.Text)
		for _, c := range mr.Calls {
			mark := ""
			if c.Nudge {
				mark = " (after the check)"
			}
			if c.Error {
				mark += " (error)"
			}
			fmt.Fprintf(&b, "> %s%s `%s`\n> → %s\n\n", c.Tool, mark, clip(c.Args, 500), strings.ReplaceAll(clip(c.Result, 300), "\n", " "))
		}
		fmt.Fprintf(&b, "**Agent:** %s\n\n", strings.TrimSpace(mr.Answer))
		for _, e := range mr.Errors {
			fmt.Fprintf(&b, "*request error: %s*\n\n", e)
		}
	}
	b.WriteString("## Subjects at the end\n\n")
	for _, s := range r.Subjects {
		fmt.Fprintf(&b, "- **%s · %s · %s**: %s", s.ID, s.Subject, s.Status, s.Outcome)
		if len(s.Open) > 0 {
			fmt.Fprintf(&b, " Open: %s", strings.Join(s.Open, "; "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n## Card at the end\n\n```\n%s\n```\n", r.Card)
	return b.String()
}
