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

func count[T any](xs []T, f func(T) bool) int {
	n := 0
	for _, x := range xs {
		if f(x) {
			n++
		}
	}
	return n
}

func sum[T any](xs []T, f func(T) int) int {
	n := 0
	for _, x := range xs {
		n += f(x)
	}
	return n
}

func avg(total, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f", float64(total)/float64(n))
}

// report is results.md: every measure per model and condition.
func report(runs []runResult) string {
	by := map[cell][]scored{}
	var models []string
	conds := map[int]bool{}
	var errs []runResult
	for _, r := range runs {
		if r.Error != "" {
			errs = append(errs, r)
			continue
		}
		k := cell{r.Model, r.Cond}
		by[k] = append(by[k], scored{r, scoreRun(r)})
		if !slices.Contains(models, r.Model) {
			models = append(models, r.Model)
		}
		conds[r.Cond] = true
	}
	cs := slices.Sorted(maps.Keys(conds))
	cells := func(only func(int) bool, f func(model string, cond int, xs []scored)) {
		for _, m := range models {
			for _, c := range cs {
				if xs := by[cell{m, c}]; len(xs) > 0 && only(c) {
					f(m, c, xs)
				}
			}
		}
	}
	all := func(int) bool { return true }
	var b strings.Builder
	fmt.Fprintf(&b, "# SPIKE-045 results\n\n%d runs, %d with an error (left out below). Conditions: ", len(runs), len(errs))
	for i, c := range cs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(condNames[c])
	}
	b.WriteString(".\n\n")

	b.WriteString("## Subscribe (condition 3)\n\nThe subscriptions agents made with `subscribe`. Needed: the docs agent's `schema_changed` and the reviewer's `pipeline_run` (2 per run). " +
		"Right filter: the needed ones with the key's filter (any table; outcome = failed, any pipeline). Unneeded: neither needed nor allowed " +
		"(allowed: the price watcher's daily_btc runs, the reporter's weekly_summary runs). At creation: made in the agent's first turn.\n\n")
	b.WriteString("| Model | Runs | Made | Needed (recall) | Right filter | Unneeded | Allowed | Precision | At creation | Later | Duplicates | Unsubscribed |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	cells(func(c int) bool { return c == condAgent }, func(m string, c int, xs []scored) {
		made := sum(xs, func(x scored) int { return len(x.s.AgentSubs) })
		need := sum(xs, func(x scored) int { return x.s.NeedMade })
		unneeded := sum(xs, func(x scored) int { return x.s.Unneeded })
		allowed := sum(xs, func(x scored) int { return x.s.Allowed })
		fmt.Fprintf(&b, "| %s | %d | %d | %s | %s | %d | %d | %s | %d | %d | %d | %d |\n", m, len(xs), made,
			pct(need, needs()*len(xs)), pct(sum(xs, func(x scored) int { return x.s.NeedRight }), need), unneeded, allowed,
			pct(need+allowed, made), sum(xs, func(x scored) int { return x.s.AtCreation }), sum(xs, func(x scored) int { return x.s.Later }),
			sum(xs, func(x scored) int { return x.s.Dups }), sum(xs, func(x scored) int { return x.s.Unsubs }))
	})
	b.WriteString("\nEach subscription made, by agent (runs that made it):\n\n")
	cells(func(c int) bool { return c == condAgent }, func(m string, c int, xs []scored) {
		tally := map[string]int{}
		for _, x := range xs {
			seen := map[string]bool{}
			for _, s := range x.s.AgentSubs {
				k := agentByKey(s.Agent).Title + ": " + s.sub.String()
				if !seen[k] {
					seen[k] = true
					tally[k]++
				}
			}
		}
		fmt.Fprintf(&b, "- **%s:**", m)
		if len(tally) == 0 {
			b.WriteString(" none")
		}
		for _, k := range slices.Sorted(maps.Keys(tally)) {
			fmt.Fprintf(&b, " %s (%d);", k, tally[k])
		}
		b.WriteString("\n")
	})

	b.WriteString("\n## Delivery\n\nNotices given per run, and notices skipped because the chat caused the event.\n\n")
	b.WriteString("| Model | Condition | Runs | Notices per run | Own events skipped |\n|---|---|---|---|---|\n")
	cells(all, func(m string, c int, xs []scored) {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %d |\n", m, condNames[c], len(xs), avg(sum(xs, func(x scored) int { return x.s.Delivered }), len(xs)),
			sum(xs, func(x scored) int { return x.s.Own }))
	})

	b.WriteString("\n## Publish (conditions 4 and 5)\n\nRecall: the price watcher published in message 9's turn, when BTC fell 6.4% (the reporter subscribes to `alerts`). " +
		"Right channel: on `alerts`. Title: names BTC and the move. Noise: reports published in the 10 routine turns. " +
		"Extra: reports in turns that changed or found something no subscriber needs (the schema changes, the reviewer's turn after the failed run). " +
		"Refused: publish calls to a channel that doesn't exist.\n\n")
	b.WriteString("| Model | Condition | Runs | Published (recall) | Right channel | Title says what happened | Noise (reports) | Routine turns with a report | Extra | Refused | read_report calls |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	cells(part2, func(m string, c int, xs []scored) {
		pub := count(xs, func(x scored) bool { return x.s.Published })
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %d | %s | %d | %d | %d |\n", m, condNames[c], len(xs), pct(pub, len(xs)),
			pct(count(xs, func(x scored) bool { return x.s.RightChannel }), pub), pct(count(xs, func(x scored) bool { return x.s.TitleOK }), pub),
			sum(xs, func(x scored) int { return x.s.Noise }), pct(sum(xs, func(x scored) int { return x.s.NoiseTurns }), sum(xs, func(x scored) int { return x.s.RoutineTurns })),
			sum(xs, func(x scored) int { return x.s.Extra }), sum(xs, func(x scored) int { return x.s.Refused }), sum(xs, func(x scored) int { return x.s.ReadReport }))
	})
	b.WriteString("\nEvery report published:\n\n")
	cells(part2, func(m string, c int, xs []scored) {
		for _, x := range xs {
			for _, p := range x.r.Pubs {
				status := p.Report
				if p.Refused {
					status = "refused"
				}
				kind := ""
				if p.Msg >= 1 && p.Msg <= len(script) {
					kind = script[p.Msg-1].Pub
				}
				fmt.Fprintf(&b, "- %s c%d r%d, message %d (%s), %s on `%s` (%s): %q\n", m, c, x.r.Rep, p.Msg, kind, agentByKey(p.Agent).Title, p.Channel, status, p.Title)
			}
		}
	})

	b.WriteString("\n## Use\n\nAt the subscriber's next turn after the event. Docs (7): docs/schema.md shows `volume` after message 7 (schema change at 2). " +
		"Reviewer (8): looks at the failed run r_q7m2 (run_status, or names it or the 503). Reporter (11): the draft or reply has today's 6.4% drop " +
		"(also reachable from btc_prices with a query). Docs (13): shows `eth_prices` after message 13 (change at 10). End: the last docs have both. " +
		"In condition 1 only the project card has the schema changes, and nothing has the runs.\n\n")
	b.WriteString("| Model | Condition | Runs | Docs (7) volume | Reviewer (8) r_q7m2 | Reporter (11) drop | Docs (13) eth_prices | Docs at the end |\n|---|---|---|---|---|---|---|---|\n")
	cells(all, func(m string, c int, xs []scored) {
		n := len(xs)
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %s |\n", m, condNames[c], n,
			pct(count(xs, func(x scored) bool { return x.s.DocsVolume }), n), pct(count(xs, func(x scored) bool { return x.s.Reviewer }), n),
			pct(count(xs, func(x scored) bool { return x.s.Reporter }), n), pct(count(xs, func(x scored) bool { return x.s.DocsEth }), n),
			pct(count(xs, func(x scored) bool { return x.s.DocsEnd }), n))
	})

	b.WriteString("\n## Cost\n\nPrompt tokens per run (as the provider reports them, cached ones included), the event notices' share of them (estimated, summed over every request that carried them), " +
		"and block 5's (estimated: session notes, subscriptions and, in condition 5, the channels).\n\n")
	b.WriteString("| Model | Condition | Runs | Requests per run | Prompt tokens per run | Notices per run | Notices' share | Block 5 per run | Refused calls (groups) |\n|---|---|---|---|---|---|---|---|---|\n")
	cells(all, func(m string, c int, xs []scored) {
		n := len(xs)
		prompt := sum(xs, func(x scored) int { return x.s.Prompt })
		notice := sum(xs, func(x scored) int { return x.s.Notice })
		share := "–"
		if prompt > 0 {
			share = fmt.Sprintf("%.2f%%", 100*float64(notice)/float64(prompt))
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %s | %d |\n", m, condNames[c], n, avg(sum(xs, func(x scored) int { return x.s.Requests }), n),
			avg(prompt, n), avg(notice, n), share, avg(sum(xs, func(x scored) int { return x.s.Block5 }), n), sum(xs, func(x scored) int { return x.s.GroupRefused }))
	})

	if len(errs) > 0 {
		b.WriteString("\n## Runs with an error\n\n")
		for _, r := range errs {
			fmt.Fprintf(&b, "- %s: %s\n", r.name(), clip(r.Error, 300))
		}
	}
	return b.String()
}

// transcript is one run as Markdown: each message, the notices its turn
// got, the calls and the reply.
func transcript(r runResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nCondition %s, %.0fs.", r.name(), condNames[r.Cond], r.Seconds)
	if r.Error != "" {
		fmt.Fprintf(&b, " **Error:** %s", r.Error)
	}
	b.WriteString("\n\n## Subscriptions\n\n")
	for _, s := range r.Subs {
		fmt.Fprintf(&b, "- %s %s: %s (by %s, message %d", s.ID, agentByKey(s.Agent).Title, s.sub, s.By, s.Msg)
		if s.Ended {
			fmt.Fprintf(&b, ", ended at %d", s.EndedAt)
		}
		b.WriteString(")\n")
	}
	b.WriteString("\n## Events\n\n")
	for _, e := range r.Events {
		var to []string
		for _, d := range r.Deliveries {
			if d.Event == e.N {
				to = append(to, fmt.Sprintf("%s at %d", agentByKey(d.Agent).Title, d.Msg))
			}
		}
		fmt.Fprintf(&b, "- %d (message %d) %s → %s\n", e.N, e.Msg, e.Text, strings.Join(to, ", "))
	}
	for _, t := range r.Turns {
		m := script[t.Msg-1]
		fmt.Fprintf(&b, "\n## %d · %s · %s (%s)\n\n> %s\n\n", t.Msg, m.At, agentByKey(t.Agent).Title, m.Pub, t.Text)
		for _, n := range t.Notices {
			fmt.Fprintf(&b, "Notice: `%s`\n\n", n)
		}
		for _, c := range r.Calls {
			if c.Msg != t.Msg {
				continue
			}
			mark := ""
			if c.Refused {
				mark = " **refused**"
			} else if c.Error {
				mark = " (error)"
			}
			fmt.Fprintf(&b, "- `%s` %s%s\n  → %s\n", c.Tool, clip(c.Args, 400), mark, strings.ReplaceAll(clip(c.Result, 300), "\n", " "))
		}
		fmt.Fprintf(&b, "\n%s\n\n*%d requests, %d prompt tokens, notices %d, block 5 %d.*\n", t.Reply, t.Requests, t.Prompt, t.NoticeTokens, t.Block5)
		for _, e := range t.Errors {
			fmt.Fprintf(&b, "\nError: %s\n", e)
		}
	}
	if n := len(r.Turns); n > 0 {
		fmt.Fprintf(&b, "\n## docs/schema.md at the end\n\n```\n%s\n```\n\n## summaries/draft.md at the end\n\n```\n%s\n```\n", r.Turns[n-1].Docs, r.Turns[n-1].Draft)
	}
	return b.String()
}
