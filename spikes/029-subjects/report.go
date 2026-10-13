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

// scenarioByID returns a scenario as it was played: chats before round 3
// (chats 0) have no merge messages.
func scenarioByID(id string, chats int) (scenarioDef, bool) {
	i := slices.IndexFunc(scenarios, func(s scenarioDef) bool { return s.ID == id })
	if i < 0 {
		return scenarioDef{}, false
	}
	sc := scenarios[i]
	if chats < 3 {
		sc.Messages = slices.DeleteFunc(slices.Clone(sc.Messages), func(m message) bool { return m.Kind == kindMerge })
	}
	return sc, true
}

// variant is a table row: a model under a condition and a prompt rule.
type variant struct {
	model string
	cond  int
	rule  string
}

func (v variant) label() string {
	if v.cond < condTool {
		return condNames[v.cond]
	}
	return condNames[v.cond] + " · " + v.rule
}

// report is results.md: one table per measure group, by model, condition
// and rule, the merges, and the checks one by one.
func report(runs []runResult) string {
	agg := map[variant]*score{}
	count := map[variant]int{}
	var keys []variant
	type ckey struct {
		variant
		scenario string
		chats    int
		msg      int
		what     string
	}
	checks := map[ckey][2]int{} // ok, n
	var ckeys []ckey
	failed := map[variant]int{}
	var merges []string
	for _, r := range runs {
		sc, ok := scenarioByID(r.Scenario, r.Chats)
		if !ok {
			continue
		}
		k := variant{r.Model, r.Cond, r.rule()}
		if agg[k] == nil {
			agg[k] = &score{}
			keys = append(keys, k)
		}
		s := scoreRun(r, sc)
		agg[k].add(s)
		count[k]++
		if r.Error != "" {
			failed[k]++
		}
		for _, c := range s.checks {
			ck := ckey{k, c.Scenario, r.Chats, c.Msg, c.What}
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
		for _, mr := range r.Messages {
			for _, c := range mr.Calls {
				if c.Tool == "merge_subjects" && !c.Error {
					merges = append(merges, fmt.Sprintf("| %s | %s | %s | %d | %d · %s | %s |", r.Model, k.label(), r.Scenario, r.Rep,
						mr.Index+1, sc.Messages[mr.Index].Kind, strings.ReplaceAll(c.Result, "|", "/")))
				}
			}
		}
	}
	slices.SortFunc(keys, func(a, b variant) int {
		if c := strings.Compare(a.model, b.model); c != 0 {
			return c
		}
		if a.cond != b.cond {
			return a.cond - b.cond
		}
		return strings.Compare(a.rule, b.rule)
	})
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
	for _, k := range keys {
		if !slices.Contains(rules, k.rule) {
			rules = append(rules, k.rule)
		}
	}
	fmt.Fprintf(&b, "%d runs, prompt rules %s. Conditions: 1 baseline, 2 tool, 3 tool + index, 4 tool + index + the app's check. "+
		"v2 is round 2's rule; v3 makes a subject one goal, with notes and merge_subjects; v4 is v3 with open items instead of a status.\n\n", len(runs), strings.Join(rules, ", "))

	b.WriteString("## Keeping the subjects\n\n")
	b.WriteString("| model | condition | runs | work turns with a subject, on their own | with the app's check | nudges | subjects at the end, per run | made for questions | changes that updated the existing subject | duplicates | work in one subject after the merge message | merges made | subject calls alone in their response |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		if k.cond < condTool {
			continue
		}
		s := agg[k]
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %d | %s | %d | %s | %d | %s | %d | %s |\n", k.model, k.label(), count[k],
			pct(s.SubjOwn, s.Work), pct(s.SubjCheck, s.Work), s.Nudges, per(s.Subjects, count[k]), s.Noise, pct(s.ChangeUpd, s.ChangeN), s.Duplicates,
			pct(s.MergeOK, s.MergeN), s.Merges, pct(s.Alone, s.Updates))
	}

	b.WriteString("\n## Correctness and payoff\n\n")
	b.WriteString("| model | condition | subject status right | waiting work shown | follow-up answers right | calls that needed earlier turns right | history reads per run | get_subject per run |\n|---|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		s := agg[k]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", k.model, k.label(),
			pct(s.StatusOK, s.StatusN), pct(s.WaitOK, s.WaitN), pct(s.AnswerOK, s.AnswerN), pct(s.CallOK, s.CallN), per(s.History, count[k]), per(s.GetSubject, count[k]))
	}

	b.WriteString("\n## Cost\n\n")
	b.WriteString("| model | condition | requests per run | prompt tokens per run | output tokens per run | request errors | runs that ended early |\n|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		s := agg[k]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %d |\n", k.model, k.label(),
			per(s.Requests, count[k]), per(s.Prompt, count[k]), per(s.Output, count[k]), s.ReqErrors, failed[k])
	}

	if len(merges) > 0 {
		b.WriteString("\n## Merges\n\n| model | condition | scenario | rep | message | result |\n|---|---|---|---|---|---|\n")
		for _, m := range merges {
			b.WriteString(m + "\n")
		}
	}

	b.WriteString("\n## Checks one by one\n\nPasses out of runs, per condition.\n\n")
	var models []string
	for _, k := range keys {
		if !slices.Contains(models, k.model) {
			models = append(models, k.model)
		}
	}
	for _, m := range models {
		var cols []variant
		for _, k := range keys {
			if k.model == m {
				cols = append(cols, k)
			}
		}
		fmt.Fprintf(&b, "### %s\n\n| scenario | # | check | message |", m)
		for _, c := range cols {
			fmt.Fprintf(&b, " %s |", c.label())
		}
		b.WriteString("\n|---|---|---|---|" + strings.Repeat("---|", len(cols)) + "\n")
		type row struct {
			scenario string
			chats    int
			msg      int
			what     string
		}
		var rows []row
		for _, ck := range ckeys {
			if ck.model == m {
				r := row{ck.scenario, ck.chats, ck.msg, ck.what}
				if !slices.Contains(rows, r) {
					rows = append(rows, r)
				}
			}
		}
		slices.SortFunc(rows, func(a, b row) int {
			if c := strings.Compare(a.scenario, b.scenario); c != 0 {
				return c
			}
			if a.chats != b.chats {
				return a.chats - b.chats
			}
			if a.msg != b.msg {
				return a.msg - b.msg
			}
			return strings.Compare(a.what, b.what)
		})
		for _, r := range rows {
			sc, _ := scenarioByID(r.scenario, r.chats)
			fmt.Fprintf(&b, "| %s | %d | %s | %s |", r.scenario, r.msg+1, r.what, clip(sc.Messages[r.msg].Text, 60))
			for _, c := range cols {
				v, ok := checks[ckey{c, r.scenario, r.chats, r.msg, r.what}]
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
	fmt.Fprintf(&b, "# %s · %s · condition %s · rule %s · rep %d\n\n", r.Model, r.Scenario, condNames[r.Cond], r.rule(), r.Rep)
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
		state := s.Status
		if state == "" {
			state = fmt.Sprintf("%d open", len(s.Open))
		}
		fmt.Fprintf(&b, "- **%s · %s · %s**: %s%s", s.ID, s.Subject, state, s.Outcome, s.Summary)
		if s.MergedInto != "" {
			fmt.Fprintf(&b, " (merged into %s)", s.MergedInto)
		}
		if len(s.Open) > 0 {
			fmt.Fprintf(&b, " Open: %s", strings.Join(s.Open, "; "))
		}
		b.WriteString("\n")
		for _, n := range s.Notes {
			fmt.Fprintf(&b, "  - %s\n", n.Text)
		}
	}
	fmt.Fprintf(&b, "\n## Card at the end\n\n```\n%s\n```\n", r.Card)
	return b.String()
}
