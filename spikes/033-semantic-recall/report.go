package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// writeResults writes results/queries.jsonl, one line per query and
// condition, and results/summary.md.
func writeResults(dir string, o *outcome, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "queries.jsonl"))
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, r := range o.Results {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	s, err := os.Create(filepath.Join(dir, "summary.md"))
	if err != nil {
		return err
	}
	sw := bufio.NewWriter(s)
	summary(sw, o, now)
	if err := sw.Flush(); err != nil {
		s.Close()
		return err
	}
	return s.Close()
}

// runKey names one condition: a set, a model with its prefix setting and
// form, and fts, vector or hybrid.
type runKey struct {
	Set, Model string
	Prefix     bool
	Form       form
	Condition  string
}

func (k runKey) model() string {
	if k.Model == "" {
		return "—"
	}
	if k.Prefix {
		return k.Model + " +prefix"
	}
	return k.Model
}

func (k runKey) form() string {
	if k.Form == "" {
		return "—"
	}
	return string(k.Form)
}

func keyOf(r result) runKey { return runKey{r.Set, r.Model, r.Prefix, r.Form, r.Condition} }

// runs groups the results by condition, in the order they ran.
func runs(rs []result) ([]runKey, map[runKey][]result) {
	var keys []runKey
	m := map[runKey][]result{}
	for _, r := range rs {
		k := keyOf(r)
		if _, ok := m[k]; !ok {
			keys = append(keys, k)
		}
		m[k] = append(m[k], r)
	}
	return keys, m
}

type score struct {
	n, hits int
	rr      float64
}

func (s *score) add(r result) {
	s.n++
	if r.First > 0 && r.First <= 5 {
		s.hits++
	}
	if r.First > 0 {
		s.rr += 1 / float64(r.First)
	}
}

func (s score) recall() string {
	if s.n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(s.hits)/float64(s.n))
}

func (s score) mrr() string {
	if s.n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f", s.rr/float64(s.n))
}

var answerKinds = []string{"same_words", "paraphrase", "cross_language"}

func summary(w io.Writer, o *outcome, now time.Time) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }
	keys, byRun := runs(o.Results)
	// One set's rows together; within a set, the order they ran.
	setIndex := map[string]int{}
	for i, s := range o.Sets {
		setIndex[s.Name] = i
	}
	slices.SortStableFunc(keys, func(a, b runKey) int { return setIndex[a.Set] - setIndex[b.Set] })

	p("# SPIKE-033 results\n\n")
	p("Written %s by the harness. Per-query ranks and top 5: `queries.jsonl`.\n\n", now.Format("2006-01-02 15:04"))
	p("## Corpus\n\n")
	if o.Messages > 0 {
		p("- Messages: %d, %d bytes of text.\n", o.Messages, o.MessageBytes)
	}
	for _, s := range o.Sets {
		if s.Chunking != "" {
			p("- %s: %d chunks over %.1f pages (%d characters a page).\n", s.Name, len(s.Items), s.Pages, pageRunes)
		}
		p("- %s queries: %s.\n", s.Name, countKinds(s.Queries))
	}
	p("\n## Models\n\n")
	if len(o.Models) == 0 && len(o.Stops) == 0 {
		p("None: FTS only.\n")
	}
	for _, m := range o.Models {
		p("- %s: %s `%s`, %d wide, Matryoshka %v, prefixes %v.\n", m.Spec.Name, m.Spec.Kind, m.Spec.Model, m.Dims, m.Spec.Matryoshka, m.Spec.hasPrefixes())
	}
	for _, s := range o.Stops {
		p("- **%s stopped:** %s\n", s.Model, oneLine(s.Err))
	}

	p("\n## Recall@5 and MRR\n\n")
	p("No-answer queries are left out. Recall@5 is the share of queries with a target in the top 5; MRR counts a target past rank 50 as 0.\n\n")
	p("| Set | Model | Form | Condition | n | R@5 | MRR | en | de | fa | same words | paraphrase | cross-language |\n")
	p("|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		var all score
		byLang := map[string]*score{}
		byKind := map[string]*score{}
		for _, r := range byRun[k] {
			if r.Kind == "no_answer" {
				continue
			}
			all.add(r)
			get(byLang, r.Lang).add(r)
			get(byKind, r.Kind).add(r)
		}
		p("| %s | %s | %s | %s | %d | %s | %s |", k.Set, k.model(), k.form(), k.Condition, all.n, all.recall(), all.mrr())
		for _, l := range langs {
			p(" %s |", get(byLang, l).recall())
		}
		for _, kd := range answerKinds {
			p(" %s |", get(byKind, kd).recall())
		}
		p("\n")
	}

	regressions(w, byRun, keys)
	noAnswer(w, byRun, keys)
	speeds(w, o)
	storage(w, o)
}

func get(m map[string]*score, k string) *score {
	if m[k] == nil {
		m[k] = &score{}
	}
	return m[k]
}

func countKinds(qs []query) string {
	n := map[string]int{}
	for _, q := range qs {
		n[q.Kind]++
	}
	var parts []string
	for _, k := range queryKinds {
		parts = append(parts, fmt.Sprintf("%d %s", n[k], strings.ReplaceAll(k, "_", " ")))
	}
	return fmt.Sprintf("%d (%s)", len(qs), strings.Join(parts, ", "))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// regressions lists the same-words queries FTS finds that hybrid ranks
// lower or loses.
func regressions(w io.Writer, byRun map[runKey][]result, keys []runKey) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }
	p("\n## Same-words queries hybrid ranks lower than FTS\n\n")
	ftsFirst := map[[2]string]int{}
	for _, k := range keys {
		if k.Condition == "fts" {
			for _, r := range byRun[k] {
				ftsFirst[[2]string{r.Set, r.Query}] = r.First
			}
		}
	}
	var lines []string
	for _, k := range keys {
		if k.Condition != "hybrid" {
			continue
		}
		for _, r := range byRun[k] {
			f := ftsFirst[[2]string{r.Set, r.Query}]
			if r.Kind != "same_words" || f == 0 || (r.First != 0 && r.First <= f) {
				continue
			}
			h := "lost"
			if r.First > 0 {
				h = fmt.Sprint(r.First)
			}
			lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s | %d | %s |", r.Set, k.model(), k.form(), r.Query, f, h))
		}
	}
	if len(lines) == 0 {
		p("None.\n")
		return
	}
	p("| Set | Model | Form | Query | FTS rank | hybrid rank |\n|---|---|---|---|---|---|\n")
	for _, l := range lines {
		p("%s\n", l)
	}
}

// noAnswer compares the top cosine of no-answer queries with the cosine of
// real hits (a target ranked first by vector search), to see whether a
// cutoff could keep vector noise out.
func noAnswer(w io.Writer, byRun map[runKey][]result, keys []runKey) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }
	p("\n## No-answer queries\n\n")
	p("FTS: no-answer queries with no result at all.\n\n| Set | empty | of |\n|---|---|---|\n")
	for _, k := range keys {
		if k.Condition != "fts" {
			continue
		}
		var n, empty int
		for _, r := range byRun[k] {
			if r.Kind == "no_answer" {
				n++
				if r.Returned == 0 {
					empty++
				}
			}
		}
		p("| %s | %d | %d |\n", k.Set, empty, n)
	}
	p("\nVector search: the top cosine of no-answer queries, and the cosine of real hits (the target ranked first). *Above* is the share of real hits above the highest no-answer score, which a cutoff there would keep.\n\n")
	p("| Set | Model | Form | no-answer max | no-answer median | hits min | hits median | above |\n|---|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		if k.Condition != "vector" {
			continue
		}
		var none, hits []float64
		for _, r := range byRun[k] {
			switch {
			case r.Kind == "no_answer":
				none = append(none, r.VecTop)
			case r.First == 1:
				hits = append(hits, r.VecHit)
			}
		}
		above := "—"
		if len(none) > 0 && len(hits) > 0 {
			top := slices.Max(none)
			n := 0
			for _, h := range hits {
				if h > top {
					n++
				}
			}
			above = fmt.Sprintf("%.0f%%", 100*float64(n)/float64(len(hits)))
		}
		p("| %s | %s | %s | %s | %s | %s | %s | %s |\n", k.Set, k.model(), k.form(), stat(none, 1), stat(none, 0.5), stat(hits, 0), stat(hits, 0.5), above)
	}
}

// stat is the q-quantile of xs, nearest rank.
func stat(xs []float64, q float64) string {
	if len(xs) == 0 {
		return "—"
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	return fmt.Sprintf("%.3f", s[quantileIndex(len(s), q)])
}

func quantileIndex(n int, q float64) int {
	return min(n-1, max(0, int(math.Ceil(q*float64(n)))-1))
}

func speeds(w io.Writer, o *outcome) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }
	p("\n## Embedding time\n\n")
	if len(o.Speeds) == 0 {
		p("No model ran.\n")
		return
	}
	p("From each vector's share of its batch's time, cached vectors too (*fresh* were embedded in this run). Queries are embedded one at a time.\n\n")
	p("| Model | Set | items | fresh | per 1,000 messages | per 100 pages | query p50 | query p95 |\n|---|---|---|---|---|---|---|---|\n")
	for _, s := range o.Speeds {
		k := runKey{Model: s.Model, Prefix: s.Prefix}
		perK, perPages := "—", "—"
		if s.Pages == 0 && s.Items > 0 {
			perK = dur(float64(s.NS) / float64(s.Items) * 1000)
		}
		if s.Pages > 0 {
			perPages = dur(float64(s.NS) / s.Pages * 100)
		}
		q := slices.Clone(s.Query)
		slices.Sort(q)
		qp50, qp95 := "—", "—"
		if len(q) > 0 {
			qp50, qp95 = dur(float64(q[quantileIndex(len(q), 0.5)])), dur(float64(q[quantileIndex(len(q), 0.95)]))
		}
		p("| %s | %s | %d | %d | %s | %s | %s | %s |\n", k.model(), s.Set, s.Items, s.Fresh, perK, perPages, qp50, qp95)
	}
}

func dur(ns float64) string {
	d := time.Duration(ns)
	switch {
	case d >= time.Second:
		return d.Round(10 * time.Millisecond).String()
	case d >= time.Millisecond:
		return d.Round(100 * time.Microsecond).String()
	}
	return d.Round(time.Microsecond).String()
}

func storage(w io.Writer, o *outcome) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) }
	p("\n## Storage\n\n")
	p("Bytes per vector, per message (one vector each) and per page (the chunks a page has). FTS is 0.4 × the text size (SPIKE-011).\n\n")
	type row struct {
		set   string
		per   string
		units float64 // vectors per unit
		text  float64 // text bytes per unit
	}
	var rows []row
	for _, s := range o.Sets {
		switch {
		case s.Chunking == "" && len(s.Items) > 0:
			rows = append(rows, row{s.Name, "message", 1, float64(s.TextBytes) / float64(len(s.Items))})
		case s.Pages > 0:
			rows = append(rows, row{s.Name, "page", float64(len(s.Items)) / s.Pages, float64(s.TextBytes) / s.Pages})
		}
	}
	p("| Model | Form | bytes/vector |")
	for _, r := range rows {
		p(" %s per %s |", r.set, r.per)
	}
	p("\n|---|---|---|%s\n", strings.Repeat("---|", len(rows)))
	for _, m := range o.Models {
		if m.Dims == 0 {
			continue
		}
		for _, f := range formsFor(m.Spec, m.Dims) {
			b := bytesPerVector(f, m.Dims)
			p("| %s | %s | %d |", m.Spec.Name, f, b)
			for _, r := range rows {
				p(" %.0f |", r.units*float64(b))
			}
			p("\n")
		}
	}
	p("| FTS | — | — |")
	for _, r := range rows {
		p(" %.0f |", 0.4*r.text)
	}
	p("\n")
}
