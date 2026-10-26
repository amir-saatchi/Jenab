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

type cell struct {
	model string
	cond  int
}

func pct(ok, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", ok, n, 100*float64(ok)/float64(n))
}

func per(x, runs int) string {
	if runs == 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f", float64(x)/float64(runs))
}

// report is results.md: per model and condition, then the checks one by
// one.
func report(runs []runResult) string {
	agg := map[cell]*score{}
	count := map[cell]int{}
	failed := map[cell]int{}
	var models []string
	var conds []int
	var all []checkResult
	for _, r := range runs {
		sc, ok := scenarioByID(r.Scenario)
		if !ok {
			continue
		}
		c := cell{r.Model, r.Cond}
		if agg[c] == nil {
			agg[c] = &score{}
		}
		s := scoreRun(r, sc)
		addScore(agg[c], s)
		count[c]++
		if s.Failed {
			failed[c]++
		}
		if !slices.Contains(models, r.Model) {
			models = append(models, r.Model)
		}
		if !slices.Contains(conds, r.Cond) {
			conds = append(conds, r.Cond)
		}
		for _, ch := range s.checks {
			ch.What = r.Model + "|" + fmt.Sprint(r.Cond) + "|" + ch.What
			all = append(all, ch)
		}
	}
	slices.Sort(conds)
	var b strings.Builder
	b.WriteString("# SPIKE-034 results\n\n")
	fmt.Fprintf(&b, "%d runs. Each cell adds up both chats and all reps.\n", len(runs))
	for _, m := range models {
		fmt.Fprintf(&b, "\n## %s\n\n", m)
		var cs []cell
		for _, c := range conds {
			if count[cell{m, c}] > 0 {
				cs = append(cs, cell{m, c})
			}
		}
		row := func(name string, f func(s *score, n int) string) {
			fmt.Fprintf(&b, "| %s |", name)
			for _, c := range cs {
				fmt.Fprintf(&b, " %s |", f(agg[c], count[c]))
			}
			b.WriteString("\n")
		}
		b.WriteString("| |")
		for _, c := range cs {
			fmt.Fprintf(&b, " %s |", condNames[c.cond])
		}
		b.WriteString("\n|---|")
		for range cs {
			b.WriteString("---|")
		}
		b.WriteString("\n")
		b.WriteString("| Runs (failed) |")
		for _, c := range cs {
			fmt.Fprintf(&b, " %d (%d) |", count[c], failed[c])
		}
		b.WriteString("\n")
		row("**Payoff**", func(*score, int) string { return "" })
		row("Follow-ups after cuts", func(s *score, _ int) string { return pct(s.FollowOK, s.FollowN) })
		row("Cross-topic questions", func(s *score, _ int) string { return pct(s.CrossOK, s.CrossN) })
		row("Calls that changed earlier work", func(s *score, _ int) string { return pct(s.CallOK, s.CallN) })
		row("Main thread: decisions made in topics", func(s *score, _ int) string { return pct(s.ReachOK, s.ReachN) })
		row("...turns of those that read history", func(s *score, _ int) string { return fmt.Sprintf("%d/%d", s.ReachRead, s.ReachN) })
		row("**Proposals (3)**", func(*score, int) string { return "" })
		row("New-topic chances (a subject without a topic) that got one", func(s *score, _ int) string { return pct(s.NewOK, s.NewN) })
		row("Returns proposed into their topic", func(s *score, _ int) string { return pct(s.ReturnOK, s.ReturnN) })
		row("Noise proposals, per run", func(s *score, n int) string { return per(s.Noise, n) })
		row("...proposals on the merge message, per run", func(s *score, n int) string { return per(s.MergeProps, n) })
		row("Duplicate topics proposed, per run", func(s *score, n int) string { return per(s.Dups, n) })
		row("Wrong topic proposed, per run", func(s *score, n int) string { return per(s.Wrong, n) })
		row("Messages answered outside their topic, per run", func(s *score, n int) string { return per(s.Misses, n) })
		row("Proposing turns: requests each", func(s *score, _ int) string { return per(s.PropReq, s.PropSends) })
		row("Proposing turns: prompt tokens each", func(s *score, _ int) string { return per(s.PropPrompt, s.PropSends) })
		row("Topics / subjects / sub-sessions at the end, per run", func(s *score, n int) string { return per(s.Topics+s.Subjects, n) })
		row("Subjects for main messages, per run (4)", func(s *score, n int) string { return per(s.SubjNoise, n) })
		row("Duplicate subjects, per run (4)", func(s *score, n int) string { return per(s.SubjDups, n) })
		row("**Sub-sessions (5–6)**", func(*score, int) string { return "" })
		row("Asked: one create_session with the task, no work in the parent", func(s *score, _ int) string { return pct(s.AskOK, s.AskN) })
		row("Finish notice passed on in the parent's answer", func(s *score, _ int) string { return pct(s.PassOK, s.PassN) })
		row("create_session calls nobody asked for, per run", func(s *score, n int) string { return per(s.Unasked, n) })
		row("Notices to the parent, per run", func(s *score, n int) string { return per(s.Ups, n) })
		row("...their tokens, per run (est.)", func(s *score, n int) string { return per(s.UpTokens, n) })
		row("**Notes and memory**", func(*score, int) string { return "" })
		row("Waiting work shows as an open item", func(s *score, _ int) string { return pct(s.WaitOK, s.WaitN) })
		row("Merge message: one topic / subject", func(s *score, _ int) string { return pct(s.MergeOK, s.MergeN) })
		row("Notes or subject writes, per run", func(s *score, n int) string { return per(s.NotesWrites, n) })
		row("...alone in their response", func(s *score, _ int) string { return fmt.Sprintf("%d/%d", s.Alone, s.NotesWrites) })
		row("Parallel steps run at the same time", func(s *score, n int) string { return fmt.Sprintf("%d/%d", s.ParallelRan, n) })
		row("Project memory has both facts", func(s *score, _ int) string { return pct(s.MemOK, s.MemN) })
		row("Memory revision conflicts (merged and saved again)", func(s *score, _ int) string { return fmt.Sprintf("%d (%d)", s.Conflicts, s.ConflictFixed) })
		row("**History and cost**", func(*score, int) string { return "" })
		row("History reads, per run", func(s *score, n int) string { return per(s.History, n) })
		row("...that reach another thread", func(s *score, n int) string { return per(s.HistoryCross, n) })
		row("Requests, per run", func(s *score, n int) string { return per(s.Requests, n) })
		row("Prompt tokens, per run (k)", func(s *score, n int) string { return per(s.Prompt/1000, n) })
		row("...of the main thread or parent (k)", func(s *score, n int) string { return per(s.ParentPrompt/1000, n) })
		row("Output tokens, per run (k)", func(s *score, n int) string { return per(s.Output/1000, n) })
		row("Cache reads, share of prompt tokens", func(s *score, _ int) string {
			if s.Prompt == 0 {
				return "–"
			}
			return fmt.Sprintf("%.0f%%", 100*float64(s.CacheRead)/float64(s.Prompt))
		})
		row("Topic's first request: prefix shared with main, of tools + system (est. tokens)", func(s *score, _ int) string {
			if s.SharedN == 0 {
				return "–"
			}
			return fmt.Sprintf("%d of %d", s.Shared/s.SharedN, s.System/s.SharedN)
		})
		row("Request errors", func(s *score, _ int) string { return fmt.Sprint(s.ReqErrors) })
	}

	b.WriteString("\n## Checks one by one\n\nPasses per check, over the reps.\n")
	type key struct {
		model, cond, sc string
		msg             int
		what            string
	}
	ok := map[key]int{}
	n := map[key]int{}
	var keys []key
	for _, c := range all {
		parts := strings.SplitN(c.What, "|", 3)
		k := key{parts[0], parts[1], c.Scenario, c.Msg, parts[2]}
		if n[k] == 0 {
			keys = append(keys, k)
		}
		n[k]++
		if c.OK {
			ok[k]++
		}
	}
	slices.SortStableFunc(keys, func(a, b key) int {
		if c := strings.Compare(a.sc, b.sc); c != 0 {
			return c
		}
		if a.msg != b.msg {
			return a.msg - b.msg
		}
		return strings.Compare(a.what, b.what)
	})
	for _, m := range models {
		fmt.Fprintf(&b, "\n### %s\n\n| Chat | # | Check | Message |", m)
		for _, c := range conds {
			fmt.Fprintf(&b, " %d |", c)
		}
		b.WriteString("\n|---|---|---|---|")
		for range conds {
			b.WriteString("---|")
		}
		b.WriteString("\n")
		done := map[string]bool{}
		for _, k := range keys {
			if k.model != m {
				continue
			}
			id := fmt.Sprintf("%s|%d|%s", k.sc, k.msg, k.what)
			if done[id] {
				continue
			}
			done[id] = true
			sc, _ := scenarioByID(k.sc)
			fmt.Fprintf(&b, "| %s | %d | %s | %s |", k.sc, k.msg+1, k.what, clip(sc.Messages[k.msg].Text, 60))
			for _, c := range conds {
				kk := key{m, fmt.Sprint(c), k.sc, k.msg, k.what}
				if n[kk] == 0 {
					b.WriteString(" – |")
				} else {
					fmt.Fprintf(&b, " %d/%d |", ok[kk], n[kk])
				}
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func addScore(s *score, o score) {
	s.NewN += o.NewN
	s.NewOK += o.NewOK
	s.ReturnN += o.ReturnN
	s.ReturnOK += o.ReturnOK
	s.Noise += o.Noise
	s.MergeProps += o.MergeProps
	s.Dups += o.Dups
	s.Wrong += o.Wrong
	s.Misses += o.Misses
	s.PropSends += o.PropSends
	s.PropReq += o.PropReq
	s.PropPrompt += o.PropPrompt
	s.Topics += o.Topics
	s.SubjNoise += o.SubjNoise
	s.SubjDups += o.SubjDups
	s.Subjects += o.Subjects
	s.AskN += o.AskN
	s.AskOK += o.AskOK
	s.PassN += o.PassN
	s.PassOK += o.PassOK
	s.Unasked += o.Unasked
	s.Ups += o.Ups
	s.UpTokens += o.UpTokens
	s.ParentPrompt += o.ParentPrompt
	s.FollowN += o.FollowN
	s.FollowOK += o.FollowOK
	s.CrossN += o.CrossN
	s.CrossOK += o.CrossOK
	s.CallN += o.CallN
	s.CallOK += o.CallOK
	s.ReachN += o.ReachN
	s.ReachOK += o.ReachOK
	s.ReachRead += o.ReachRead
	s.History += o.History
	s.HistoryCross += o.HistoryCross
	s.NotesWrites += o.NotesWrites
	s.Alone += o.Alone
	s.WaitN += o.WaitN
	s.WaitOK += o.WaitOK
	s.MergeN += o.MergeN
	s.MergeOK += o.MergeOK
	s.MemN += o.MemN
	s.MemOK += o.MemOK
	s.Conflicts += o.Conflicts
	s.ConflictFixed += o.ConflictFixed
	s.ParallelRan += o.ParallelRan
	s.Requests += o.Requests
	s.Prompt += o.Prompt
	s.Output += o.Output
	s.CacheRead += o.CacheRead
	s.SharedN += o.SharedN
	s.Shared += o.Shared
	s.System += o.System
	s.ReqErrors += o.ReqErrors
}

// transcript is one run as markdown: each message with where it went,
// the proposals, the calls and the answer; then the threads or subjects
// and project memory.
func transcript(r runResult, sc scenarioDef) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · %s · %s · rep %d\n\n", r.Model, sc.Title, condNames[r.Cond], r.Rep)
	if r.Error != "" {
		fmt.Fprintf(&b, "**Error:** %s\n\n", r.Error)
	}
	props := map[int][]proposal{}
	for _, p := range r.Proposals {
		props[p.Msg] = append(props[p.Msg], p)
	}
	ups := map[int][]upNotice{}
	for _, u := range r.Ups {
		ups[u.Delivered] = append(ups[u.Delivered], u)
	}
	for _, mr := range r.Messages {
		m := sc.Messages[mr.Index]
		fmt.Fprintf(&b, "## %d · %s · %s", mr.Index+1, m.Label, m.Kind)
		if mr.Parallel {
			b.WriteString(" · parallel")
		}
		fmt.Fprintf(&b, "\n\n**User** (in %s", mr.Sends[0].Thread)
		switch {
		case mr.Sub != "":
			fmt.Fprintf(&b, ", which started %s", mr.Sends[len(mr.Sends)-1].Thread)
		case len(mr.Sends) > 1:
			fmt.Fprintf(&b, ", moved to %s", mr.Sends[len(mr.Sends)-1].Thread)
		}
		text := m.Text
		if mr.Sends[0].Thread == mainThread && sessions(r.Cond) && len(m.Ask) > 0 {
			text = askPrefix + text
		}
		fmt.Fprintf(&b, "): %s\n\n", text)
		for _, u := range ups[mr.Index] {
			kind := "notice"
			if u.Finish {
				kind = "finish notice"
			}
			fmt.Fprintf(&b, "- %s to the parent, from message %d: %s\n", kind, u.Msg+1, clip(strings.ReplaceAll(u.Text, "\n", " "), 300))
		}
		for _, p := range props[mr.Index] {
			what := p.Title
			if p.TopicID != "" {
				what = p.TopicID
			}
			fmt.Fprintf(&b, "- proposal in %s: %q → %s", p.From, what, p.Verdict)
			if p.Accepted {
				fmt.Fprintf(&b, ", moved to %s", p.Into)
			}
			b.WriteString("\n")
		}
		for _, c := range mr.Calls {
			mark := ""
			if c.Error {
				mark = " **error**"
			}
			if c.Cross {
				mark += " (other thread)"
			}
			fmt.Fprintf(&b, "- [%s] `%s` %s%s\n", c.Thread, c.Tool, clip(c.Args, 300), mark)
			if c.Error || c.Tool == "propose_topic" {
				fmt.Fprintf(&b, "  → %s\n", clip(strings.ReplaceAll(c.Result, "\n", " "), 300))
			}
		}
		req, prompt, output, _ := mr.cost()
		answered := mr.Thread
		if mr.Sub != "" {
			answered = mainThread
		}
		fmt.Fprintf(&b, "\n**Answer** (%s; %d requests, %d prompt and %d output tokens):\n\n%s\n\n", answered, req, prompt, output, quote(mr.Answer))
		if mr.Sub != "" {
			fmt.Fprintf(&b, "**The sub-session's reply** (%s):\n\n%s\n\n", mr.Thread, quote(mr.Sub))
		}
		if mr.Memory != "" {
			fmt.Fprintf(&b, "Project memory after it:\n\n%s\n\n", quote(mr.Memory))
		}
		for _, e := range mr.Errors {
			fmt.Fprintf(&b, "Request error: %s\n\n", e)
		}
	}
	if len(r.Threads) > 0 {
		b.WriteString("## Threads at the end\n\n")
		for _, t := range r.Threads {
			fmt.Fprintf(&b, "### %s · %s", t.ID, t.Title)
			if t.MergedInto != "" {
				fmt.Fprintf(&b, " (merged into %s)", t.MergedInto)
			}
			fmt.Fprintf(&b, "\n\nLabels: %s\n\n%s\n\n", strings.Join(t.Labels, ", "), notesText(t.Summary, t.Notes, t.Open))
		}
	}
	if len(r.Subjects) > 0 {
		b.WriteString("## Subjects at the end\n\n")
		for _, x := range r.Subjects {
			fmt.Fprintf(&b, "### %s · %s", x.ID, x.Subject)
			if x.MergedInto != "" {
				fmt.Fprintf(&b, " (merged into %s)", x.MergedInto)
			}
			fmt.Fprintf(&b, "\n\nLabels: %s\n\n%s\n\n", strings.Join(x.Labels, ", "), notesText(x.Summary, x.Notes, x.Open))
		}
	}
	fmt.Fprintf(&b, "## Project memory at the end\n\n%s\n\n## Project card at the end\n\n%s\n", quote(r.Memory), quote(r.Card))
	return b.String()
}

func quote(s string) string {
	if strings.TrimSpace(s) == "" {
		return "> (empty)"
	}
	return "> " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n> ")
}
