package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// writeGraph writes one graph as results/<problem>/<model>-<mode>-<id>.md.
func writeGraph(ctx context.Context, st *store, g *graph, dir string) (string, error) {
	s, err := statsOf(ctx, st, g.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · %s · %s\n\n", g.Problem.ID, g.Model, g.Mode)
	fmt.Fprintf(&b, "**Task:** %s\n\n", g.Problem.Text)
	b.WriteString("| Thoughts | Calls | Tokens in / out | Time | Finish | First try valid | Next picked | Max depth | Weight spread | Duplicates |\n|---|---|---|---|---|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| %d | %d | %d / %d | %s | %s | %d/%d | %d/%d | %d | %.2f | %d |\n\n",
		s.Thoughts, s.Calls, s.In, s.Out, s.Time.Round(time.Second), g.Finish, s.FirstTry, s.Shaped, g.NextPicked, g.Expansions, s.Depth, s.Spread, s.Dups)
	if len(s.Failures) > 0 {
		fmt.Fprintf(&b, "Failures: %s\n\n", s.failureText())
	}
	b.WriteString("## Answer from the graph\n\n")
	b.WriteString(g.Answer + "\n\n")
	if g.Used != "" {
		fmt.Fprintf(&b, "*Used: %s*\n\n", g.Used)
	}
	b.WriteString("## Index\n\n```text\n" + g.index() + "\n```\n\n")
	b.WriteString("## Graph\n\n```mermaid\n" + mermaid(g) + "```\n\n")
	b.WriteString("## Thoughts\n\n")
	for _, t := range g.nodes {
		fmt.Fprintf(&b, "### #%d [%s] %s\n", t.Num, t.Kind, t.Title)
		fmt.Fprintf(&b, "w %.2f · %s · depth %d · from %s\n\n", t.Weight, t.State, t.Depth, nums(t.Parents))
		b.WriteString(strings.TrimSpace(t.Content) + "\n\n")
		if t.Rationale != "" {
			fmt.Fprintf(&b, "> %s\n\n", t.Rationale)
		}
	}
	path := filepath.Join(dir, g.Problem.ID, fmt.Sprintf("%s-%s-%d.md", slug(g.Model), g.Mode, g.ID))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func mermaid(g *graph) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, t := range g.nodes {
		label := fmt.Sprintf("#%d %s: %s", t.Num, t.Kind, t.Title)
		if t.Num != 1 {
			label += fmt.Sprintf(" (%.2f)", t.Weight)
		}
		label = strings.NewReplacer(`"`, "#quot;", "\n", " ").Replace(label)
		fmt.Fprintf(&b, "  n%d[\"%s\"]\n", t.Num, label)
	}
	for _, t := range g.nodes {
		for _, p := range t.Parents {
			fmt.Fprintf(&b, "  n%d -->|%s| n%d\n", p, edgeKind(t.Kind), t.Num)
		}
	}
	b.WriteString("  classDef dead stroke-dasharray: 5 5,opacity:0.6\n  classDef done stroke-width:3px\n")
	for _, t := range g.nodes {
		switch t.State {
		case "dead_end":
			fmt.Fprintf(&b, "  class n%d dead\n", t.Num)
		case "done":
			fmt.Fprintf(&b, "  class n%d done\n", t.Num)
		}
	}
	return b.String()
}

type stats struct {
	Thoughts, Calls, In, Out int
	Time                     time.Duration
	FirstTry, Shaped         int // define and expand calls valid on the first try, of all of them
	Retried, Broke           int
	Depth                    int
	Spread                   float64 // mean standard deviation of sibling weights
	Dups                     int     // pairs of titles with mostly the same words
	Reweights                int
	IndexChars               int // the index in the last expand call
	Kinds                    map[string]int
	States                   map[string]int
	Failures                 map[string]int
}

func (s stats) failureText() string {
	var out []string
	for _, k := range []string{"no_json", "schema", "check", "max_tokens"} {
		if n := s.Failures[k]; n > 0 {
			out = append(out, fmt.Sprintf("%s %d", k, n))
		}
	}
	return strings.Join(out, ", ")
}

func statsOf(ctx context.Context, st *store, graphID int64) (stats, error) {
	s := stats{Kinds: map[string]int{}, States: map[string]int{}, Failures: map[string]int{}}
	rows, err := st.db.QueryContext(ctx, `SELECT purpose, tries, valid, failures, input_tokens, output_tokens, ms, index_chars FROM calls WHERE graph_id = ? ORDER BY id`, graphID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var purpose, failures string
		var tries, in, out, idx int
		var valid bool
		var ms int64
		if err := rows.Scan(&purpose, &tries, &valid, &failures, &in, &out, &ms, &idx); err != nil {
			rows.Close()
			return s, err
		}
		s.Calls++
		s.In += in
		s.Out += out
		s.Time += time.Duration(ms) * time.Millisecond
		if purpose == "define" || purpose == "expand" {
			s.Shaped++
			switch {
			case valid && tries == 1:
				s.FirstTry++
			case valid:
				s.Retried++
			default:
				s.Broke++
			}
		}
		if purpose == "expand" {
			s.IndexChars = idx
		}
		for _, f := range strings.Split(failures, ",") {
			if f != "" {
				s.Failures[f]++
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, err
	}

	type row struct {
		id, parent  int64
		kind, title string
		state       string
		weight      float64
		depth       int
	}
	rows, err = st.db.QueryContext(ctx, `SELECT t.id, COALESCE((SELECT MIN(e.from_id) FROM edges e WHERE e.to_id = t.id), 0),
		t.kind, t.title, t.state, t.weight, t.depth FROM thoughts t WHERE t.graph_id = ? ORDER BY t.num`, graphID)
	if err != nil {
		return s, err
	}
	var ts []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.parent, &r.kind, &r.title, &r.state, &r.weight, &r.depth); err != nil {
			rows.Close()
			return s, err
		}
		ts = append(ts, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, err
	}
	s.Thoughts = len(ts)
	siblings := map[int64][]float64{}
	for _, t := range ts {
		s.Kinds[t.kind]++
		s.States[t.state]++
		s.Depth = max(s.Depth, t.depth)
		if t.parent != 0 {
			siblings[t.parent] = append(siblings[t.parent], t.weight)
		}
	}
	n, sum := 0, 0.0
	for _, ws := range siblings {
		if len(ws) >= 2 {
			sum += stdev(ws)
			n++
		}
	}
	if n > 0 {
		s.Spread = sum / float64(n)
	}
	for i := range ts {
		for j := i + 1; j < len(ts); j++ {
			if jaccard(ts[i].title, ts[j].title) >= 0.6 {
				s.Dups++
			}
		}
	}
	err = st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reweights r JOIN calls c ON c.id = r.call_id WHERE c.graph_id = ?`, graphID).Scan(&s.Reweights)
	return s, err
}

func stdev(xs []float64) float64 {
	m := 0.0
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	v := 0.0
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return math.Sqrt(v / float64(len(xs)))
}

func jaccard(a, b string) float64 {
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !(r == '-' || r == '‌' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127)
		}) {
			out[w] = true
		}
		return out
	}
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	both := 0
	for w := range wa {
		if wb[w] {
			both++
		}
	}
	return float64(both) / float64(len(wa)+len(wb)-both)
}

// writeSummary rewrites summary.md: one row per finished graph.
func writeSummary(ctx context.Context, st *store, dir string) (string, error) {
	rows, err := st.db.QueryContext(ctx, `SELECT id, problem, model, mode, finish, expansions, next_picked, next_invalid, early_finish
		FROM graphs WHERE ended_at IS NOT NULL ORDER BY problem, model, mode, id`)
	if err != nil {
		return "", err
	}
	type gr struct {
		id                                 int64
		problem, model, mode, finish       string
		expansions, picked, invalid, early int
	}
	var gs []gr
	for rows.Next() {
		var g gr
		if err := rows.Scan(&g.id, &g.problem, &g.model, &g.mode, &g.finish, &g.expansions, &g.picked, &g.invalid, &g.early); err != nil {
			rows.Close()
			return "", err
		}
		gs = append(gs, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# SPIKE-031 round 2: graphs\n\nOne row per graph. *Valid* counts define and expand calls: on the first try / after the retry / not at all. *Next* is how often the model's `next` named a thought that could be expanded. *Spread* is the mean standard deviation of sibling weights. *Dups* are pairs of titles with mostly the same words. *Index* is the size of the index in the last expand call, in characters.\n\n")
	b.WriteString("| Graph | Problem | Model | Mode | Thoughts | Kinds (sol/step/crit/merge) | Dead ends | Depth | Calls | Tokens in / out | Time | Valid | Failures | Next | Finish | Reweights | Spread | Dups | Index |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, g := range gs {
		s, err := statsOf(ctx, st, g.id)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d | %d/%d/%d/%d | %d | %d | %d | %d / %d | %s | %d/%d/%d | %s | %d/%d | %s | %d | %.2f | %d | %d |\n",
			g.id, g.problem, g.model, g.mode, s.Thoughts, s.Kinds["solution"], s.Kinds["step"], s.Kinds["critique"], s.Kinds["merge"],
			s.States["dead_end"], s.Depth, s.Calls, s.In, s.Out, s.Time.Round(time.Second), s.FirstTry, s.Retried, s.Broke,
			orDash(s.failureText()), g.picked, g.expansions, g.finish, s.Reweights, s.Spread, s.Dups, s.IndexChars)
	}
	path := filepath.Join(dir, "summary.md")
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func nums(ns []int) string {
	if len(ns) == 0 {
		return "–"
	}
	var out []string
	for _, n := range ns {
		out = append(out, fmt.Sprintf("#%d", n))
	}
	return strings.Join(out, ", ")
}

func slug(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == ':' || r == '\\' || r == ' ' {
			return '_'
		}
		return r
	}, s)
}
