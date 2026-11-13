package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

func readRuns(paths ...string) ([]runResult, error) {
	var out []runResult
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
		for sc.Scan() {
			var r runResult
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, r)
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	// A run done again after an error replaces it.
	last := map[job]int{}
	for i, r := range out {
		if j, ok := last[r.job]; !ok || out[j].Error != "" || r.Error == "" {
			last[r.job] = i
		}
	}
	var kept []runResult
	for i, r := range out {
		if last[r.job] == i {
			kept = append(kept, r)
		}
	}
	return kept, nil
}

type cell struct {
	model string
	cond  int
}

type scored struct {
	r runResult
	s score
}

func pct(a, b int) string {
	if b == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", a, b, 100*float64(a)/float64(b))
}

func mean(xs []float64) string {
	if len(xs) == 0 {
		return "–"
	}
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return fmt.Sprintf("%.2f", t/float64(len(xs)))
}

func count[T any](xs []T, f func(T) bool) int {
	n := 0
	for _, x := range xs {
		if f(x) {
			n++
		}
	}
	return n
}

// report is results.md: every measure per model and condition.
func report(runs []runResult) string {
	by := map[cell][]scored{}
	var models []string
	conds := map[int]bool{}
	errs := 0
	for _, r := range runs {
		if r.Error != "" {
			errs++
			continue
		}
		sc, ok := scenarioByID(r.Scenario)
		if !ok {
			continue
		}
		k := cell{r.Model, r.Cond}
		by[k] = append(by[k], scored{r, scoreRun(r, sc)})
		if !slices.Contains(models, r.Model) {
			models = append(models, r.Model)
		}
		conds[r.Cond] = true
	}
	cs := slices.Sorted(maps.Keys(conds))
	cells := func(f func(model string, cond int, xs []scored)) {
		for _, m := range models {
			for _, c := range cs {
				if xs := by[cell{m, c}]; len(xs) > 0 {
					f(m, c, xs)
				}
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# SPIKE-044 results\n\n%d runs, %d with an error (left out below). Conditions: ", len(runs), errs)
	for i, c := range cs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(condNames[c])
	}
	b.WriteString(".\n\n")

	b.WriteString("## Groups\n\nOf the agent the task went to. Recall: needed groups present; precision: picked groups that are needed; forbidden: runs with a forbidden group. Condition 1 is the key, so it is right by design.\n\n")
	b.WriteString("| Model | Condition | Runs | With an agent | Recall | Precision | Forbidden group | Every group |\n|---|---|---|---|---|---|---|---|\n")
	cells(func(m string, c int, xs []scored) {
		var rec, prec []float64
		ag := 0
		for _, x := range xs {
			if x.s.HasAgent {
				ag++
				rec = append(rec, x.s.Recall)
				prec = append(prec, x.s.Precision)
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %s |\n", m, condNames[c], len(xs), pct(ag, len(xs)), mean(rec), mean(prec),
			pct(count(xs, func(x scored) bool { return x.s.HasAgent && x.s.Forbidden > 0 }), ag),
			pct(count(xs, func(x scored) bool { return x.s.HasAgent && x.s.AllGroups }), ag))
	})

	b.WriteString("\n## Prompt\n\nThe agent's custom prompt, by keyword checks (see `score.go`): states the job, says what to report, repeats a core rule, at most 500 tokens (the app's estimate), in the user's language. For the Persian request, also whether the prompt is in Persian or at least names Persian as the language to answer in.\n\n")
	b.WriteString("| Model | Condition | Prompts | Job | Report | Repeats a core rule | ≤ 500 tokens | Mean tokens | Language | Persian request: Persian prompt | Persian request: Persian or names it |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	cells(func(m string, c int, xs []scored) {
		var ag []scored
		var toks []float64
		for _, x := range xs {
			if x.s.HasAgent {
				ag = append(ag, x)
				toks = append(toks, float64(x.s.RoleTokens))
			}
		}
		fa := count(ag, func(x scored) bool { return x.r.Scenario == "persian" })
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %.0f | %s | %s | %s |\n", m, condNames[c], len(ag),
			pct(count(ag, func(x scored) bool { return x.s.JobOK }), len(ag)),
			pct(count(ag, func(x scored) bool { return x.s.ReportOK }), len(ag)),
			pct(count(ag, func(x scored) bool { return x.s.RepeatsCore }), len(ag)),
			pct(count(ag, func(x scored) bool { return x.s.RoleTokens <= maxRoleTokens }), len(ag)),
			meanf(toks),
			pct(count(ag, func(x scored) bool { return x.s.LangOK }), len(ag)),
			pct(count(ag, func(x scored) bool { return x.r.Scenario == "persian" && x.s.LangOK }), fa),
			pct(count(ag, func(x scored) bool { return x.r.Scenario == "persian" && (x.s.LangOK || x.s.NamesLang) }), fa))
	})

	b.WriteString("\n## Task\n\nThe follow-up task's check, per scenario (passes / runs).\n\n| Model | Scenario |")
	for _, c := range cs {
		b.WriteString(" " + condNames[c] + " |")
	}
	b.WriteString("\n|---|---|" + strings.Repeat("---|", len(cs)) + "\n")
	for _, m := range models {
		for _, sc := range scenarios {
			fmt.Fprintf(&b, "| %s | %s |", m, sc.ID)
			for _, c := range cs {
				var xs []scored
				for _, x := range by[cell{m, c}] {
					if x.r.Scenario == sc.ID {
						xs = append(xs, x)
					}
				}
				fmt.Fprintf(&b, " %s |", frac(count(xs, func(x scored) bool { return x.s.TaskOK }), len(xs)))
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "| %s | **all** |", m)
		for _, c := range cs {
			xs := by[cell{m, c}]
			fmt.Fprintf(&b, " **%s** |", pct(count(xs, func(x scored) bool { return x.s.TaskOK }), len(xs)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## Refused calls\n\nCalls to a tool outside the chat's tools. After: what the agent did after its first refusal in a run: told the user it can't, asked, called the tool again, or went on another way without saying so.\n\n")
	b.WriteString("| Model | Condition | Agent runs with a refusal | Agent refusals | Told | Asked | Retried | Worked around | Mother's refusals | Tools refused |\n|---|---|---|---|---|---|---|---|---|---|\n")
	cells(func(m string, c int, xs []scored) {
		after := map[string]int{}
		var tools []string
		agentN, motherN, runsWith := 0, 0, 0
		for _, x := range xs {
			agentN += x.s.Refused
			motherN += x.s.MotherRefused
			if x.s.Refused > 0 {
				runsWith++
				after[x.s.After]++
			}
			tools = append(tools, x.s.RefusedTools...)
			for _, t := range x.s.MotherRefusedTools {
				tools = append(tools, "mother:"+t)
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d | %d | %d | %d | %s |\n", m, condNames[c], pct(runsWith, len(xs)), agentN,
			after["told"], after["asked"], after["retried"], after["worked around"], motherN, tally(tools))
	})

	b.WriteString("\n## Creating\n\nAsked requests are scenarios 1–6 in conditions 2–3: one `create_chat` is right. In condition 4, a creation on the plain message is one nobody asked for; on the job message Mother decides. Asked first: no creation on the request, and Mother's reply asks a question (often \"Should I create it?\"). Created at the task: no agent came of the request, and Mother created one when the task came; its groups and prompt are scored above.\n\n")
	b.WriteString("| Model | Condition | Requests | Exactly one create_chat | No create_chat | More than one | send_to_chat | Asked first | Created at the task | Plain messages with a creation |\n|---|---|---|---|---|---|---|---|---|---|\n")
	cells(func(m string, c int, xs []scored) {
		if !asked(c) {
			return
		}
		var req []scored
		for _, x := range xs {
			if sc, _ := scenarioByID(x.r.Scenario); !sc.Write {
				req = append(req, x)
			}
		}
		plain := "–"
		if c == condUnasked {
			plain = pct(count(xs, func(x scored) bool { return x.s.PlainCreated > 0 }), len(xs))
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %d | %s | %s | %s |\n", m, condNames[c], len(req),
			pct(count(req, func(x scored) bool { return x.s.Created == 1 }), len(req)),
			pct(count(req, func(x scored) bool { return x.s.Created == 0 }), len(req)),
			pct(count(req, func(x scored) bool { return x.s.Created > 1 }), len(req)),
			sumf(req, func(x scored) int { return x.s.Sent }),
			pct(count(req, func(x scored) bool { return x.s.AskedFirst }), len(req)),
			pct(count(req, func(x scored) bool { return x.s.AtTask }), len(req)), plain)
	})
	b.WriteString("\nThe write request with no agent yet (scenario 7): Mother delegated it (create_chat or send_to_chat), the two keywords landed, or she neither delegated nor said she can't (pretended).\n\n")
	b.WriteString("| Model | Condition | Runs | Delegated | Landed | Pretended | Mother tried a write |\n|---|---|---|---|---|---|---|\n")
	cells(func(m string, c int, xs []scored) {
		if !asked(c) {
			return
		}
		var w []scored
		for _, x := range xs {
			if x.r.Scenario == "write" {
				w = append(w, x)
			}
		}
		if len(w) == 0 {
			return
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s |\n", m, condNames[c], len(w),
			pct(count(w, func(x scored) bool { return x.s.Delegated }), len(w)),
			pct(count(w, func(x scored) bool { return x.s.Landed }), len(w)),
			pct(count(w, func(x scored) bool { return x.s.Pretended }), len(w)),
			pct(count(w, func(x scored) bool { return x.s.MotherRefused > 0 }), len(w)))
	})

	b.WriteString("\n## Tokens\n\nPrompt tokens per request in the agent's task turns (as the provider reports them), and the estimated tokens of the agent's tool definitions against every chat tool's: what each request saves.\n\n")
	b.WriteString("| Model | Condition | Prompt tokens per request | Agent's tools | Every tool | Saved per request | Requests per run |\n|---|---|---|---|---|---|---|\n")
	cells(func(m string, c int, xs []scored) {
		var ppr, tt, all, reqs []float64
		for _, x := range xs {
			reqs = append(reqs, float64(x.s.Requests))
			if x.s.HasAgent && x.s.PromptPerReq > 0 {
				ppr = append(ppr, x.s.PromptPerReq)
				tt = append(tt, float64(x.s.ToolTokens))
				all = append(all, float64(x.s.AllTools))
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %.0f | %.0f | %.0f | %.0f | %.1f |\n", m, condNames[c], meanf(ppr), meanf(tt), meanf(all), meanf(all)-meanf(tt), meanf(reqs))
	})

	b.WriteString("\n## Groups picked\n\nPer scenario, the groups each run's agent got (asked conditions), as `read_data+files`; `none` when no agent got the task.\n\n")
	for _, m := range models {
		for _, c := range cs {
			if !asked(c) || len(by[cell{m, c}]) == 0 {
				continue
			}
			fmt.Fprintf(&b, "**%s, %s**\n\n", m, condNames[c])
			for _, sc := range scenarios {
				var picks []string
				for _, x := range by[cell{m, c}] {
					if x.r.Scenario != sc.ID {
						continue
					}
					p := "none"
					if x.s.HasAgent {
						p = strings.Join(x.s.Picks, "+")
					}
					picks = append(picks, "`"+p+"`")
				}
				if len(picks) > 0 {
					fmt.Fprintf(&b, "- %s (needs %s; never %s): %s\n", sc.ID, strings.Join(sc.Need, "+"), orNone(sc.Forbid), strings.Join(picks, ", "))
				}
			}
			b.WriteString("\n")
		}
	}
	if errs > 0 {
		b.WriteString("## Errors\n\n")
		for _, r := range runs {
			if r.Error != "" {
				fmt.Fprintf(&b, "- %s: %s\n", r.name(), clip(r.Error, 300))
			}
		}
	}
	return b.String()
}

func orNone(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ", ")
}

func frac(a, b int) string {
	if b == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d", a, b)
}

func meanf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return t / float64(len(xs))
}

func sumf(xs []scored, f func(scored) int) int {
	n := 0
	for _, x := range xs {
		n += f(x)
	}
	return n
}

// tally is "insert ×3, web_search" for a list of names.
func tally(xs []string) string {
	if len(xs) == 0 {
		return "–"
	}
	n := map[string]int{}
	for _, x := range xs {
		n[x]++
	}
	ks := slices.Sorted(maps.Keys(n))
	slices.SortStableFunc(ks, func(a, b string) int { return n[b] - n[a] })
	var out []string
	for _, k := range ks {
		if n[k] > 1 {
			out = append(out, fmt.Sprintf("%s ×%d", k, n[k]))
		} else {
			out = append(out, k)
		}
	}
	return strings.Join(out, ", ")
}

// transcript is one run as Markdown.
func transcript(r runResult, sc scenarioDef) string {
	s := scoreRun(r, sc)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s, %s, rep %d, %.0f s.\n\n", r.name(), sc.Name, condNames[r.Cond], r.Rep, r.Seconds)
	if r.Error != "" {
		fmt.Fprintf(&b, "**Error:** %s\n\n", r.Error)
	}
	fmt.Fprintf(&b, "**Task:** %v (%s). Groups %v, recall %.2f, precision %.2f, forbidden %d. Refused %d (after: %s), Mother refused %d. Created %d, sent %d, on the plain message %d.\n\n",
		s.TaskOK, s.TaskWhy, s.Picks, s.Recall, s.Precision, s.Forbidden, s.Refused, s.After, s.MotherRefused, s.Created, s.Sent, s.PlainCreated)
	for _, c := range r.Creations {
		what := "create_chat"
		if c.Sent {
			what = "send_to_chat"
		}
		fmt.Fprintf(&b, "**%s** (%s) → %s %q, picks %v → groups %v, %d tokens\n\n", what, c.Phase, c.Chat, c.Title, c.Picks, c.Groups, roleTokens(c.Role))
		if c.Role != "" {
			fmt.Fprintf(&b, "> Role: %s\n\n", strings.ReplaceAll(c.Role, "\n", "\n> "))
		}
		fmt.Fprintf(&b, "> Message: %s\n\n", strings.ReplaceAll(c.Message, "\n", "\n> "))
	}
	ci := 0
	for _, t := range r.Turns {
		fmt.Fprintf(&b, "## %s · %s\n\n%d requests, %d prompt tokens, tools %d tokens, system %d tokens.\n\n", t.Who, t.Phase, t.Requests, t.Prompt, t.ToolTokens, t.System)
		fmt.Fprintf(&b, "**User:** %s\n\n", strings.ReplaceAll(t.Text, "\n", "  \n"))
		for ; ci < len(r.Calls) && r.Calls[ci].Who == t.Who && r.Calls[ci].Phase == t.Phase; ci++ {
			c := r.Calls[ci]
			mark := ""
			if c.Refused {
				mark = " **refused**"
			} else if c.Error {
				mark = " (error)"
			}
			fmt.Fprintf(&b, "- `%s` %s%s\n  → %s\n", c.Tool, clip(c.Args, 500), mark, strings.ReplaceAll(clip(c.Result, 300), "\n", " "))
		}
		fmt.Fprintf(&b, "\n**Reply:** %s\n\n", t.Reply)
		for _, e := range t.Errors {
			fmt.Fprintf(&b, "Provider error: %s\n\n", e)
		}
	}
	fmt.Fprintf(&b, "## End state\n\nTables %v; keywords %v; pipelines saved %v; files %v.\n", r.State.Tables, r.State.Keywords, keys(r.State.Pipelines), keys(r.State.Files))
	if r.State.Memory != "" {
		fmt.Fprintf(&b, "\nMemory:\n\n```\n%s\n```\n", r.State.Memory)
	}
	return b.String()
}
