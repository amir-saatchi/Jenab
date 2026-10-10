package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// analyze prints what the summary doesn't show: the states, whether the
// weights predict what the answer uses, what an invalid next named, and
// the answers' lengths against the baseline.
func analyze(ctx context.Context, st *store, w io.Writer) error {
	db := st.db

	fmt.Fprintln(w, "## States by mode")
	rows, err := db.QueryContext(ctx, `SELECT g.mode, t.state, COUNT(*) FROM thoughts t JOIN graphs g ON g.id = t.graph_id
		WHERE t.num > 1 GROUP BY g.mode, t.state ORDER BY g.mode, t.state`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var mode, state string
		var n int
		rows.Scan(&mode, &state, &n)
		fmt.Fprintf(w, "%-9s %-9s %d\n", mode, state, n)
	}
	rows.Close()

	// Weight against use: the share of thoughts in each weight band that the
	// answer's Used line names.
	fmt.Fprintln(w, "\n## Weight against Used (thoughts after #1)")
	used := map[int64]map[int]bool{}
	rows, err = db.QueryContext(ctx, `SELECT id, COALESCE(used, '') FROM graphs`)
	if err != nil {
		return err
	}
	num := regexp.MustCompile(`#(\d+)`)
	for rows.Next() {
		var id int64
		var u string
		rows.Scan(&id, &u)
		used[id] = map[int]bool{}
		for _, m := range num.FindAllStringSubmatch(u, -1) {
			n, _ := strconv.Atoi(m[1])
			used[id][n] = true
		}
	}
	rows.Close()
	type band struct{ n, used int }
	bands := map[string]*band{}
	kinds := map[string]*band{}
	rows, err = db.QueryContext(ctx, `SELECT graph_id, num, weight, kind, state FROM thoughts WHERE num > 1`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var g int64
		var n int
		var wt float64
		var kind, state string
		rows.Scan(&g, &n, &wt, &kind, &state)
		key := "<0.70"
		switch {
		case wt >= 0.9:
			key = ">=0.90"
		case wt >= 0.8:
			key = "0.80-0.89"
		case wt >= 0.7:
			key = "0.70-0.79"
		}
		if bands[key] == nil {
			bands[key] = &band{}
		}
		if kinds[kind] == nil {
			kinds[kind] = &band{}
		}
		bands[key].n++
		kinds[kind].n++
		if used[g][n] {
			bands[key].used++
			kinds[kind].used++
		}
	}
	rows.Close()
	for _, k := range []string{"<0.70", "0.70-0.79", "0.80-0.89", ">=0.90"} {
		if b := bands[k]; b != nil {
			fmt.Fprintf(w, "w %-10s %3d thoughts, %3d used (%.0f%%)\n", k, b.n, b.used, 100*float64(b.used)/float64(b.n))
		}
	}
	for _, k := range []string{"solution", "step", "critique", "merge"} {
		if b := kinds[k]; b != nil {
			fmt.Fprintf(w, "%-10s   %3d thoughts, %3d used (%.0f%%)\n", k, b.n, b.used, 100*float64(b.used)/float64(b.n))
		}
	}

	// What an invalid next named.
	fmt.Fprintln(w, "\n## Invalid next")
	rows, err = db.QueryContext(ctx, `SELECT c.graph_id, g.mode, c.focus, c.reply FROM calls c JOIN graphs g ON g.id = c.graph_id
		WHERE c.purpose = 'expand' AND c.valid = 1 AND g.mode = 'index' ORDER BY c.id`)
	if err != nil {
		return err
	}
	type call struct {
		g          int64
		mode, next string
		focus      int
	}
	var calls []call
	for rows.Next() {
		var c call
		var reply string
		rows.Scan(&c.g, &c.mode, &c.focus, &reply)
		var a args
		if json.Unmarshal([]byte(reply), &a) == nil {
			c.next = a.Next
		} else if j := jsonIn(reply); j != nil && json.Unmarshal(j, &a) == nil {
			c.next = a.Next
		}
		calls = append(calls, c)
	}
	rows.Close()
	why := map[string]int{}
	for _, c := range calls {
		n, finish := parseNext(c.next)
		if finish {
			continue
		}
		var state string
		var depth, maxDepth int
		err := db.QueryRowContext(ctx, `SELECT t.state, t.depth, g.max_depth FROM thoughts t JOIN graphs g ON g.id = t.graph_id
			WHERE t.graph_id = ? AND t.num = ?`, c.g, n).Scan(&state, &depth, &maxDepth)
		switch {
		case err != nil:
			why[c.mode+": names no thought ("+c.next+")"]++
		case state == "done" || state == "dead_end":
			why[c.mode+": names a "+state+" thought"]++
		case depth >= maxDepth:
			why[c.mode+": names a thought at max depth"]++
		}
	}
	for k, n := range why {
		fmt.Fprintf(w, "%-50s %d\n", k, n)
	}

	fmt.Fprintln(w, "\n## Answers: length in characters")
	rows, err = db.QueryContext(ctx, `SELECT problem, model, kind, LENGTH(text), valid, COALESCE(note, '') FROM answers ORDER BY problem, model, kind`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p, m, kind, note string
		var n int
		var valid bool
		rows.Scan(&p, &m, &kind, &n, &valid, &note)
		if !valid {
			note = "not valid: " + note
		}
		fmt.Fprintf(w, "%-12s %-34s %-9s %6d %s\n", p, m, kind, n, note)
	}
	rows.Close()

	// A graph's cost is all its calls; a one-call answer's is its call.
	fmt.Fprintln(w, "\n## Cost per answer, mean")
	rows, err = db.QueryContext(ctx, `SELECT a.model, a.kind, COUNT(*),
		AVG((SELECT SUM(input_tokens) FROM calls c WHERE c.graph_id = a.graph_id OR c.id = a.call_id)),
		AVG((SELECT SUM(output_tokens) FROM calls c WHERE c.graph_id = a.graph_id OR c.id = a.call_id)),
		AVG((SELECT SUM(thought_tokens) FROM calls c WHERE c.graph_id = a.graph_id OR c.id = a.call_id)),
		AVG((SELECT SUM(thinking_chars) FROM calls c WHERE c.graph_id = a.graph_id OR c.id = a.call_id)),
		AVG((SELECT SUM(ms) FROM calls c WHERE c.graph_id = a.graph_id OR c.id = a.call_id))
		FROM answers a WHERE a.valid = 1 GROUP BY a.model, a.kind ORDER BY a.model, a.kind`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m, kind string
		var n int
		var in, out, thought, think, ms float64
		rows.Scan(&m, &kind, &n, &in, &out, &thought, &think, &ms)
		fmt.Fprintf(w, "%-34s %-9s %2d answers: %7.0f in %6.0f out (%5.0f thought), %6.0f thinking chars, %5.0fs\n", m, kind, n, in, out, thought, think, ms/1000)
	}
	rows.Close()

	fmt.Fprintln(w, "\n## Judge calls")
	rows, err = db.QueryContext(ctx, `SELECT model, COUNT(*), SUM(valid), SUM(tries > 1), AVG(output_tokens), AVG(thinking_chars), AVG(ms)
		FROM calls WHERE purpose = 'judge' GROUP BY model`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m string
		var n, valid, retried int
		var out, think, ms float64
		rows.Scan(&m, &n, &valid, &retried, &out, &think, &ms)
		fmt.Fprintf(w, "%-34s %3d calls, %3d valid, %3d retried, %5.0f out, %6.0f thinking chars, %4.0fs\n", m, n, valid, retried, out, think, ms/1000)
	}
	rows.Close()

	fmt.Fprintln(w, "\n## Calls with errors")
	rows, err = db.QueryContext(ctx, `SELECT COALESCE(graph_id, 0), purpose, focus, tries, failures, LENGTH(prompt), output_tokens, error FROM calls WHERE error != '' ORDER BY id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var g int64
		var purpose, failures, e string
		var focus, tries, plen, out int
		rows.Scan(&g, &purpose, &focus, &tries, &failures, &plen, &out, &e)
		fmt.Fprintf(w, "graph %d %s #%d tries %d [%s] prompt %d chars, out %d tokens: %s\n", g, purpose, focus, tries, failures, plen, out, clip(strings.ReplaceAll(e, "\n", " ")))
	}
	rows.Close()
	return rows.Err()
}
