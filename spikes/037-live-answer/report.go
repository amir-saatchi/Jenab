package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// report writes the numbers per condition and model.
func report(runs []runResult) string {
	var b strings.Builder
	b.WriteString("# SPIKE-037 runs\n\n")
	var a, bb []runResult
	for _, r := range runs {
		if r.Part == "A" {
			a = append(a, r)
		} else {
			bb = append(bb, r)
		}
	}
	if len(a) > 0 {
		reportA(&b, a)
	}
	if len(bb) > 0 {
		reportB(&b, bb)
	}
	return b.String()
}

type cell struct{ test, cond, model string }

func keys(runs []runResult, withTest bool) []cell {
	seen := map[cell]bool{}
	var out []cell
	for _, r := range runs {
		c := cell{cond: r.Cond, model: r.Model}
		if withTest {
			c.test = r.Test
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	order := map[string]int{condStacked: 0, condLive: 1, condWindow: 0, condAnswers: 1, condTrace: 2}
	sort.SliceStable(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.test != y.test {
			return x.test < y.test
		}
		if x.cond != y.cond {
			return order[x.cond] < order[y.cond]
		}
		return x.model < y.model
	})
	return out
}

func pct(n, d int) string {
	if d == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(d))
}

func pctf(sum float64, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*sum/float64(n))
}

func avg(sum float64, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f", sum/float64(n))
}

func avgi(sum, n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f", float64(sum)/float64(n))
}

func short(m string) string {
	_, id, _ := strings.Cut(m, "/")
	return id
}

func ratio(bs []*bool) string {
	n, k := 0, 0
	for _, b := range bs {
		if b != nil {
			n++
			if *b {
				k++
			}
		}
	}
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d", k, n)
}

func reportA(b *strings.Builder, runs []runResult) {
	b.WriteString("## Part A: the live answer\n\n")
	b.WriteString("Per condition and model, all tests together. A finish turn is `whole` when it keeps ≥60 % of the facts that arrived before it (or, for the first, repeats ≥40 % of the first answer's longer words), `appended` when it starts with the previous answer word for word and adds a block, `rest` otherwise, `none` without text. The view is what the user reads at the end: in `live`, a finish turn replaces its request's answer while no user message came since; in `stacked`, every answer stays.\n\n")
	b.WriteString("| cond | model | runs | err | bg runs | finish turns | whole | rest | appended | none | new facts | earlier facts kept | dropped / turn | view right | last answer right | union right | slots | output / update | requests / update | cache read |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range keys(runs, false) {
		var n, errs, bg, fin, out, req, prompt, cache, dropped, slots, earlierN int
		cls := map[string]int{}
		var newCov, earlierCov, view, last, union float64
		var viewN int
		for _, r := range runs {
			if r.Cond != c.cond || r.Model != c.model {
				continue
			}
			n++
			if r.Error != "" {
				errs++
			}
			td, _ := testByID(r.Test)
			s := scoreRunA(r, td)
			if s.Started > 0 {
				bg++
			}
			for _, v := range s.Revs {
				fin++
				cls[v.Class]++
				newCov += v.NewCov
				if v.EarlierN > 0 {
					earlierCov += v.EarlierCov
					earlierN++
				}
				dropped += v.Dropped
				out += v.Output
				req += v.Requests
				prompt += v.Prompt
				cache += v.CacheRead
			}
			if s.AllFacts > 0 {
				view += s.FinalView
				last += s.FinalLast
				union += s.Union
				viewN++
				slots += s.Slots
			}
		}
		fmt.Fprintf(b, "| %s | %s | %d | %d | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			c.cond, short(c.model), n, errs, bg, fin, pct(cls["whole"], fin), pct(cls["rest"], fin), pct(cls["appended"], fin), pct(cls["none"], fin),
			pctf(newCov, fin), pctf(earlierCov, earlierN), avg(float64(dropped), fin), pctf(view, viewN), pctf(last, viewN), pctf(union, viewN),
			avg(float64(slots), viewN), avgi(out, fin), avg(float64(req), fin), pct(cache, prompt))
	}

	b.WriteString("\n### Per test\n\nFinish turns as whole/rest/appended/none, and the view's facts at the end (mean over reps).\n\n")
	b.WriteString("| test | cond | model | runs | bg | classes | view right | last answer right | slots |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range keys(runs, true) {
		var n, bg, slots, viewN int
		cls := map[string]int{}
		var view, last float64
		for _, r := range runs {
			if r.Test != c.test || r.Cond != c.cond || r.Model != c.model {
				continue
			}
			n++
			td, _ := testByID(r.Test)
			s := scoreRunA(r, td)
			if s.Started > 0 {
				bg++
			}
			for _, v := range s.Revs {
				cls[v.Class]++
			}
			if s.AllFacts > 0 {
				view += s.FinalView
				last += s.FinalLast
				slots += s.Slots
				viewN++
			}
		}
		fmt.Fprintf(b, "| %s | %s | %s | %d | %d | %d/%d/%d/%d | %s | %s | %s |\n", c.test, c.cond, short(c.model), n, bg,
			cls["whole"], cls["rest"], cls["appended"], cls["none"], pctf(view, viewN), pctf(last, viewN), avg(float64(slots), viewN))
	}

	b.WriteString("\n### Edge cases\n\n")
	b.WriteString("| cond | model | failure said | failure in view | empty result: classes | together: both in one answer | late message: Saturday | later finish: Saturday | Persian: lowest share | Persian held |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range keys(runs, false) {
		var fs, fv, tb, ls, lf, po []*bool
		emp := map[string]int{}
		var share float64
		var shareN int
		for _, r := range runs {
			if r.Cond != c.cond || r.Model != c.model {
				continue
			}
			td, _ := testByID(r.Test)
			s := scoreRunA(r, td)
			fs, fv, tb, ls, lf, po = append(fs, s.FailedSaid), append(fv, s.FailedInView), append(tb, s.TogetherBoth), append(ls, s.LateSat), append(lf, s.LateFinalSat), append(po, s.PersianOK)
			if s.EmptyClass != "" {
				emp[s.EmptyClass]++
			}
			if s.PersianOK != nil {
				share += s.Persian
				shareN++
			}
		}
		var ec []string
		for _, k := range []string{"whole", "rest", "appended", "none"} {
			if emp[k] > 0 {
				ec = append(ec, fmt.Sprintf("%s %d", k, emp[k]))
			}
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", c.cond, short(c.model), ratio(fs), ratio(fv), strings.Join(ec, ", "),
			ratio(tb), ratio(ls), ratio(lf), pctf(share, shareN), ratio(po))
	}
	var errs []string
	for _, r := range runs {
		if r.Error != "" {
			errs = append(errs, fmt.Sprintf("- %s: %s", r.name(), clip(r.Error, 200)))
		}
	}
	if len(errs) > 0 {
		b.WriteString("\n### Errors\n\n" + strings.Join(errs, "\n") + "\n")
	}
	b.WriteString("\n")
}

func reportB(b *strings.Builder, runs []runResult) {
	b.WriteString("## Part B: earlier turns\n\n")
	b.WriteString("Prompt tokens as the provider reports them (input + cache read + cache write). Trace tokens are estimated (4 bytes a token) in the last request of the run.\n\n")
	b.WriteString("| chat | cond | model | runs | err | requests | prompt / request | prompt / run | largest | cache read | cuts / run | follow-ups right | reads / run | repeated / run | wrong names / run | calls / later turn | fake calls | wrong claims | trace tokens |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	type agg struct {
		n, errs, req, prompt, max, cache, cuts, ok, fol, reads, rep, wrong, lc, lt, fake, claims, trace int
	}
	kinds := []string{}
	byKind := map[cell]map[string][2]int{}
	readKind := map[cell]map[string][2]int{}
	var examples []string
	for _, c := range keys(runs, true) {
		var a agg
		byKind[c] = map[string][2]int{}
		readKind[c] = map[string][2]int{}
		for _, r := range runs {
			if r.Test != c.test || r.Cond != c.cond || r.Model != c.model {
				continue
			}
			td, _ := testByID(r.Test)
			s := scoreRunB(r, td)
			a.n++
			if r.Error != "" {
				a.errs++
			}
			a.req += s.Requests
			a.prompt += s.Prompt
			a.max = max(a.max, s.PromptMax)
			a.cache += s.CacheRead
			a.cuts += s.Cuts
			for _, f := range s.Follow {
				a.fol++
				k := byKind[c][f.Kind]
				k[1]++
				rk := readKind[c][f.Kind]
				rk[1]++
				if f.Right {
					a.ok++
					k[0]++
				}
				if f.Read {
					rk[0]++
				}
				byKind[c][f.Kind] = k
				readKind[c][f.Kind] = rk
				if !slices.Contains(kinds, f.Kind) {
					kinds = append(kinds, f.Kind)
				}
			}
			for _, v := range s.Reads {
				a.reads += v
			}
			a.rep += len(s.Repeated)
			a.wrong += len(s.WrongNames)
			a.lc += s.LateCalls
			a.lt += s.LateTurns
			a.fake += len(s.FakeCalls)
			a.claims += len(s.WrongClaims)
			a.trace += s.TraceTok
			for _, x := range s.Repeated {
				examples = append(examples, fmt.Sprintf("- %s repeated: %s", r.name(), x))
			}
			for _, x := range s.WrongNames {
				examples = append(examples, fmt.Sprintf("- %s wrong name: %s", r.name(), x))
			}
			for _, x := range s.FakeCalls {
				examples = append(examples, fmt.Sprintf("- %s fake call: `%s`", r.name(), x))
			}
			for _, x := range s.WrongClaims {
				examples = append(examples, fmt.Sprintf("- %s wrong claim: %s", r.name(), x))
			}
		}
		fmt.Fprintf(b, "| %s | %s | %s | %d | %d | %s | %s | %s | %d | %s | %s | %s | %s | %s | %s | %s | %d | %d | %s |\n",
			c.test, c.cond, short(c.model), a.n, a.errs, avg(float64(a.req), a.n), avgi(a.prompt, a.req), avgi(a.prompt, a.n), a.max, pct(a.cache, a.prompt),
			avg(float64(a.cuts), a.n), pct(a.ok, a.fol), avg(float64(a.reads), a.n), avg(float64(a.rep), a.n), avg(float64(a.wrong), a.n),
			avg(float64(a.lc), a.lt), a.fake, a.claims, avgi(a.trace, a.n))
	}
	b.WriteString("\n### Follow-ups right, per kind\n\nEach cell is right/asked, and in brackets how many of those turns read the history, a ref, a table or a page.\n\n| chat | cond | model |")
	for _, k := range kinds {
		b.WriteString(" " + k + " |")
	}
	b.WriteString("\n|---|---|---|" + strings.Repeat("---|", len(kinds)) + "\n")
	for _, c := range keys(runs, true) {
		fmt.Fprintf(b, "| %s | %s | %s |", c.test, c.cond, short(c.model))
		for _, k := range kinds {
			v, rk := byKind[c][k], readKind[c][k]
			if v[1] == 0 {
				b.WriteString(" – |")
				continue
			}
			fmt.Fprintf(b, " %d/%d (%d) |", v[0], v[1], rk[0])
		}
		b.WriteString("\n")
	}
	if len(examples) > 0 {
		b.WriteString("\n### Repeated work, wrong names, fake calls and wrong claims\n\n" + strings.Join(examples, "\n") + "\n")
	}
	var errs []string
	for _, r := range runs {
		if r.Error != "" {
			errs = append(errs, fmt.Sprintf("- %s: %s", r.name(), clip(r.Error, 200)))
		}
	}
	if len(errs) > 0 {
		b.WriteString("\n### Errors\n\n" + strings.Join(errs, "\n") + "\n")
	}
	b.WriteString("\n")
}

// transcript is a run as markdown: every turn with its calls and texts.
func transcript(r runResult, td testDef) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s · %s · %s · rep %d · %.0f s\n", r.name(), r.Test, r.Cond, r.Model, r.Rep, r.Seconds)
	if r.Error != "" {
		fmt.Fprintf(&b, "\n**Error:** %s\n", r.Error)
	}
	if r.Capped {
		b.WriteString("\n**Capped:** more finish notices than the harness sends.\n")
	}
	fmt.Fprintf(&b, "\nScore: %s\n", oneLine(r, td))
	for _, t := range r.Turns {
		head := "user"
		if t.Kind == "finish" {
			head = "finish " + strings.Join(t.Tasks, ", ")
		} else if t.Msg >= 0 {
			head = fmt.Sprintf("message %d", t.Msg)
		}
		fmt.Fprintf(&b, "\n---\n\n## Turn %d (%s)\n\n", t.N, head)
		for _, l := range strings.Split(t.Input, "\n") {
			b.WriteString("> " + l + "\n")
		}
		for _, j := range t.Joined {
			b.WriteString(">\n> (joined) " + j + "\n")
		}
		for _, n := range t.Notices {
			fmt.Fprintf(&b, "\n_notice:_ %s\n", n)
		}
		if len(t.Calls) > 0 {
			b.WriteString("\n")
		}
		for _, c := range t.Calls {
			mark := "→"
			if c.Error {
				mark = "✗"
			}
			fmt.Fprintf(&b, "- `%s %s` %s %s\n", c.Tool, clip(c.Args, 300), mark, strings.ReplaceAll(clip(c.Result, 240), "\n", " ⏎ "))
		}
		for i, x := range t.Texts {
			if i == len(t.Texts)-1 {
				b.WriteString("\n**Answer:**\n\n" + x + "\n")
			} else {
				b.WriteString("\n_text between calls:_ " + strings.ReplaceAll(clip(x, 400), "\n", " ") + "\n")
			}
		}
		if len(t.Texts) == 0 {
			b.WriteString("\n_(no text)_\n")
		}
		var reqs []string
		for _, q := range t.Requests {
			s := fmt.Sprintf("%d in (%d cached) / %d out, history ~%d", q.Prompt, q.CacheRead, q.Output, q.EstHistory)
			if q.EstTrace > 0 {
				s += fmt.Sprintf(", trace ~%d", q.EstTrace)
			}
			if q.Err != "" {
				s += " ✗ " + q.Err
			}
			reqs = append(reqs, s)
		}
		if len(reqs) > 0 {
			b.WriteString("\n<sub>requests: " + strings.Join(reqs, " · ") + "</sub>\n")
		}
	}
	if len(r.Tasks) > 0 {
		b.WriteString("\n---\n\n## Tasks\n\n")
		for _, t := range r.Tasks {
			fmt.Fprintf(&b, "- %s (%v) started in turn %d, background %v, notice turn %d: `%s`\n", t.ID, t.Rules, t.Turn, t.Background, t.Notice, clip(t.Args, 200))
		}
	}
	if len(r.Tables) > 0 {
		fmt.Fprintf(&b, "\nTables: %s\n", strings.Join(r.Tables, ", "))
	}
	return b.String()
}
