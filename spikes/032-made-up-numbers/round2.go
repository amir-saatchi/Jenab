package main

import (
	"fmt"
	"sort"
	"strings"
)

// Round 2: every answer of the grounding-honest set is honest, so each
// firing is a false alarm. printRound2 prints, for each condition (the
// mode and round 2's switches of a run), the unsourced counts per case
// and model, the false alarms of each way to cut them, and what the
// notice did where the model was told.

// condition names a run's mode and switches.
func condition(r run) string {
	c := r.Mode
	if r.Variant != "" {
		c += " (" + r.Variant + ")"
	}
	return c
}

type answer struct {
	scenario, model string
	n, saved        int // unsourced, and of them in what was saved
	pct             int
}

func printRound2(runs []run) {
	byCond := map[string][]run{}
	for _, r := range runs {
		if r.Skipped != "" {
			continue
		}
		byCond[condition(r)] = append(byCond[condition(r)], r)
	}
	var conds []string
	for c := range byCond {
		conds = append(conds, c)
	}
	sort.Strings(conds)
	for _, c := range conds {
		rs := byCond[c]
		var as []answer
		broke := 0
		for _, r := range rs {
			first := false
			for _, g := range r.Grounding {
				if g.After {
					continue
				}
				first = true
				as = append(as, answer{r.Scenario, short(r.Model), unsourced(g), g.Saved, g.Pct})
			}
			if !first {
				broke++
			}
		}
		fmt.Printf("\n### %s: %d runs, %d answers checked, %d runs without a checked answer\n\n", c, len(rs), len(as), broke)
		models := sortedKeys(as, func(a answer) string { return a.model })
		scens := sortedKeys(as, func(a answer) string { return a.scenario })
		fmt.Println("Unsourced numbers per answer (saved in brackets), and marked at K=3:")
		fmt.Println()
		fmt.Println("| Case | " + strings.Join(models, " | ") + " |")
		fmt.Println("|---|" + strings.Repeat("---|", len(models)))
		for _, s := range scens {
			var cells []string
			for _, m := range models {
				var ns []string
				marked, tot := 0, 0
				for _, a := range as {
					if a.scenario != s || a.model != m {
						continue
					}
					v := fmt.Sprint(a.n)
					if a.saved > 0 {
						v += fmt.Sprintf(" [%d]", a.saved)
					}
					ns = append(ns, v)
					tot++
					if a.n >= 3 {
						marked++
					}
				}
				cells = append(cells, fmt.Sprintf("%s: %d/%d", strings.Join(ns, ", "), marked, tot))
			}
			fmt.Printf("| %s | %s |\n", s, strings.Join(cells, " | "))
		}
		fmt.Println()
		fmt.Println("False alarms by way of telling (the mark at K=3 in every row):")
		fmt.Println()
		fmt.Println("| Model | Answers | Marked, K=3 | Told at 3 | Told at 5 | Told at 8 | Told at 10 | Told at 8 or saved | Pct matches |")
		fmt.Println("|---|---|---|---|---|---|---|---|---|")
		for _, m := range append(models, "all") {
			var n, pct int
			told := map[string]int{}
			for _, a := range as {
				if m != "all" && a.model != m {
					continue
				}
				n++
				pct += a.pct
				for _, k := range []int{3, 5, 8, 10} {
					if a.n >= k {
						told[fmt.Sprint(k)]++
					}
				}
				if a.n >= 8 || a.saved > 0 {
					told["8s"]++
				}
			}
			fmt.Printf("| %s | %d | %d | %d | %d | %d | %d | %d | %d |\n", m, n, told["3"], told["3"], told["5"], told["8"], told["10"], told["8s"], pct)
		}
		printTold(rs)
	}
}

// printTold lists the runs where the model was told, and what followed.
func printTold(rs []run) {
	var lines []string
	for _, r := range rs {
		var first, after *ground
		for i := range r.Grounding {
			g := &r.Grounding[i]
			switch {
			case g.Told && first == nil:
				first = g
			case g.After && first != nil:
				after = g
			}
		}
		if first == nil {
			continue
		}
		a := "no check after"
		if after != nil {
			a = fmt.Sprintf("%d unsourced", unsourced(*after))
			if after.SavedLeft > 0 {
				a += fmt.Sprintf(", %d left in the saved file", after.SavedLeft)
			}
			if after.Quoted > 0 {
				a += fmt.Sprintf(", %d quoted", after.Quoted)
			}
		}
		x := ""
		if r.Extra != nil {
			x = fmt.Sprintf("%d req, %d/%d tok, %.1f s", r.Extra.Requests, r.Extra.Prompt, r.Extra.Output, r.Extra.Seconds)
		}
		lines = append(lines, fmt.Sprintf("| %s | %s r%d | %d (saved %d) | %s | %s |", short(r.Model), r.Scenario, r.Rep, unsourced(*first), first.Saved, a, x))
	}
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)
	fmt.Println()
	fmt.Println("Told:")
	fmt.Println()
	fmt.Println("| Model | Run | First check | After the notice | Extra |")
	fmt.Println("|---|---|---|---|---|")
	for _, l := range lines {
		fmt.Println(l)
	}
}

func short(model string) string {
	if i := strings.LastIndex(model, "/"); i >= 0 {
		return model[i+1:]
	}
	return model
}

func sortedKeys(as []answer, f func(answer) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range as {
		if k := f(a); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
