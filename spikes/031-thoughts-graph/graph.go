package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// thought is one node. Num is its number in its graph (#1 is the problem);
// the model only ever sees these numbers.
type thought struct {
	DBID      int64
	Num       int
	Kind      string // problem, solution, step, critique, merge
	Title     string
	Content   string
	Weight    float64
	Rationale string
	State     string // open, expanded, done, dead_end
	Depth     int
	Parents   []int // the first is the parent in the index; a merge has more
	Children  []int
}

// graph is one run: one problem on one model in one mode.
type graph struct {
	ID       int64
	Problem  problem
	Model    string
	Mode     string // index or path
	MaxDepth int
	nodes    []*thought // nodes[i].Num == i+1

	Expansions  int
	NextPicked  int    // the model's next was an expandable thought
	NextInvalid int    // it named something else; the controller picked
	EarlyFinish int    // finish before any solution, ignored
	Finish      string // model, budget, no_open, broke
	Answer      string
	Used        string // the Used line of the answer
}

func (g *graph) get(n int) *thought {
	if n < 1 || n > len(g.nodes) {
		return nil
	}
	return g.nodes[n-1]
}

func (g *graph) expandable(t *thought) bool {
	return t != nil && (t.State == "open" || t.State == "expanded") && t.Depth < g.MaxDepth
}

// best is the expandable thought with the highest weight, the problem only
// when nothing else is left; 0 if none.
func (g *graph) best() int {
	var b *thought
	for _, t := range g.nodes {
		if t.Num == 1 || !g.expandable(t) {
			continue
		}
		if b == nil || t.Weight > b.Weight || t.Weight == b.Weight && t.Depth < b.Depth {
			b = t
		}
	}
	if b == nil {
		if g.expandable(g.get(1)) {
			return 1
		}
		return 0
	}
	return b.Num
}

func (g *graph) has(kind string) bool {
	return slices.ContainsFunc(g.nodes, func(t *thought) bool { return t.Kind == kind })
}

// index is the graph as titles only, one line per thought, indented under
// its first parent.
func (g *graph) index() string {
	var b strings.Builder
	var walk func(n, indent int)
	walk = func(n, indent int) {
		t := g.get(n)
		fmt.Fprintf(&b, "%s#%d [%s] %s", strings.Repeat("  ", indent), t.Num, t.Kind, t.Title)
		if t.Num != 1 {
			fmt.Fprintf(&b, " · w %.2f", t.Weight)
		}
		state := strings.ReplaceAll(t.State, "_", " ")
		if (t.State == "open" || t.State == "expanded") && t.Depth >= g.MaxDepth {
			state += ", max depth"
		}
		fmt.Fprintf(&b, " · %s", state)
		if len(t.Parents) > 1 {
			var also []string
			for _, p := range t.Parents[1:] {
				also = append(also, fmt.Sprintf("#%d", p))
			}
			fmt.Fprintf(&b, " · also from %s", strings.Join(also, ", "))
		}
		b.WriteByte('\n')
		for _, c := range t.Children {
			if g.get(c).Parents[0] == n {
				walk(c, indent+1)
			}
		}
	}
	walk(1, 0)
	return strings.TrimRight(b.String(), "\n")
}

// path is the thoughts from #1 down to n, by first parents.
func (g *graph) path(n int) []*thought {
	var out []*thought
	for t := g.get(n); t != nil; {
		out = append(out, t)
		if len(t.Parents) == 0 {
			break
		}
		t = g.get(t.Parents[0])
	}
	slices.Reverse(out)
	return out
}

func full(ts []*thought) string {
	var b strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&b, "#%d [%s] %s", t.Num, t.Kind, t.Title)
		if t.Num != 1 {
			fmt.Fprintf(&b, " (w %.2f)", t.Weight)
		}
		fmt.Fprintf(&b, "\n%s\n\n", strings.TrimSpace(t.Content))
	}
	return strings.TrimRight(b.String(), "\n")
}

// pathText is the path to n in full; a merge also brings its other parents.
func (g *graph) pathText(n int) string {
	ts := g.path(n)
	if t := g.get(n); len(t.Parents) > 1 {
		for _, p := range t.Parents[1:] {
			ts = append(ts, g.get(p))
		}
	}
	return full(ts)
}

// leaves are the thoughts with nothing under them that aren't dead ends.
func (g *graph) leaves() []*thought {
	var out []*thought
	for _, t := range g.nodes {
		if len(t.Children) == 0 && t.State != "dead_end" && t.Num != 1 {
			out = append(out, t)
		}
	}
	return out
}

// pathWeight is the mean weight of a path, the problem left out.
func pathWeight(ts []*thought) float64 {
	sum, n := 0.0, 0
	for _, t := range ts {
		if t.Num != 1 {
			sum += t.Weight
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// strongest are the thoughts on the two best paths, with the critiques
// under them, in number order.
func (g *graph) strongest() []*thought {
	ls := g.leaves()
	slices.SortStableFunc(ls, func(a, b *thought) int {
		wa, wb := pathWeight(g.path(a.Num)), pathWeight(g.path(b.Num))
		switch {
		case wa > wb:
			return -1
		case wa < wb:
			return 1
		}
		return b.Depth - a.Depth
	})
	keep := map[int]bool{1: true}
	for _, l := range ls[:min(2, len(ls))] {
		for _, t := range g.path(l.Num) {
			keep[t.Num] = true
			for _, p := range t.Parents[min(1, len(t.Parents)):] {
				keep[p] = true
			}
		}
	}
	for _, t := range g.nodes {
		if t.Kind == "critique" && keep[t.Parents[0]] {
			keep[t.Num] = true
		}
	}
	var out []*thought
	for _, t := range g.nodes {
		if keep[t.Num] {
			out = append(out, t)
		}
	}
	return out
}

func edgeKind(kind string) string {
	switch kind {
	case "solution":
		return "branches_to"
	case "step":
		return "followed_by"
	case "critique":
		return "critiques"
	case "merge":
		return "merges"
	}
	return "leads_to"
}

// store keeps every graph, thought, edge and call in one SQLite file.
type store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS graphs (
  id INTEGER PRIMARY KEY, problem TEXT, lang TEXT, request TEXT, model TEXT, mode TEXT,
  k INTEGER, max_thoughts INTEGER, max_depth INTEGER,
  expansions INTEGER DEFAULT 0, next_picked INTEGER DEFAULT 0, next_invalid INTEGER DEFAULT 0, early_finish INTEGER DEFAULT 0,
  finish TEXT, answer TEXT, used TEXT, started_at TEXT, ended_at TEXT);
CREATE TABLE IF NOT EXISTS thoughts (
  id INTEGER PRIMARY KEY, graph_id INTEGER REFERENCES graphs(id), num INTEGER,
  kind TEXT, title TEXT, content TEXT, weight REAL, rationale TEXT, state TEXT, depth INTEGER,
  call_id INTEGER, created_at TEXT, UNIQUE (graph_id, num));
CREATE TABLE IF NOT EXISTS edges (
  from_id INTEGER REFERENCES thoughts(id), to_id INTEGER REFERENCES thoughts(id), kind TEXT,
  PRIMARY KEY (from_id, to_id, kind));
CREATE TABLE IF NOT EXISTS reweights (
  call_id INTEGER, thought_id INTEGER REFERENCES thoughts(id), old REAL, new REAL);
CREATE TABLE IF NOT EXISTS calls (
  id INTEGER PRIMARY KEY, graph_id INTEGER REFERENCES graphs(id), model TEXT, purpose TEXT, focus INTEGER,
  tries INTEGER, valid INTEGER, failures TEXT, error TEXT,
  input_tokens INTEGER, output_tokens INTEGER, thinking_chars INTEGER, ms INTEGER, index_chars INTEGER,
  prompt TEXT, reply TEXT, created_at TEXT);
-- One answer to a task: a graph's (kind index or path) or one call's
-- (plain, or thinking).
CREATE TABLE IF NOT EXISTS answers (
  id INTEGER PRIMARY KEY, problem TEXT, model TEXT, kind TEXT, graph_id INTEGER REFERENCES graphs(id),
  call_id INTEGER REFERENCES calls(id), text TEXT, used TEXT, valid INTEGER, note TEXT, created_at TEXT);
-- One judge's verdict on two answers, in the order shown: A, then B.
CREATE TABLE IF NOT EXISTS judgments (
  id INTEGER PRIMARY KEY, judge TEXT, a_id INTEGER REFERENCES answers(id), b_id INTEGER REFERENCES answers(id),
  winner TEXT, score_a INTEGER, score_b INTEGER, reason TEXT, valid INTEGER, call_id INTEGER REFERENCES calls(id),
  created_at TEXT, UNIQUE (judge, a_id, b_id));
`

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	// Added after the first round-2 run; an error means it is there.
	db.Exec(`ALTER TABLE calls ADD COLUMN thought_tokens INTEGER DEFAULT 0`)
	return &store{db: db}, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *store) newGraph(ctx context.Context, g *graph, o options) error {
	r, err := s.db.ExecContext(ctx, `INSERT INTO graphs (problem, lang, request, model, mode, k, max_thoughts, max_depth, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, g.Problem.ID, g.Problem.Lang, g.Problem.Text, g.Model, g.Mode, o.K, o.MaxThoughts, o.MaxDepth, now())
	if err != nil {
		return err
	}
	g.ID, err = r.LastInsertId()
	return err
}

func (s *store) addThought(ctx context.Context, g *graph, t *thought, call int64) error {
	r, err := s.db.ExecContext(ctx, `INSERT INTO thoughts (graph_id, num, kind, title, content, weight, rationale, state, depth, call_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, g.ID, t.Num, t.Kind, t.Title, t.Content, t.Weight, t.Rationale, t.State, t.Depth, call, now())
	if err != nil {
		return err
	}
	if t.DBID, err = r.LastInsertId(); err != nil {
		return err
	}
	for _, p := range t.Parents {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO edges (from_id, to_id, kind) VALUES (?, ?, ?)`, g.get(p).DBID, t.DBID, edgeKind(t.Kind)); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) setState(ctx context.Context, t *thought) error {
	_, err := s.db.ExecContext(ctx, `UPDATE thoughts SET state = ? WHERE id = ?`, t.State, t.DBID)
	return err
}

func (s *store) reweight(ctx context.Context, call int64, t *thought, old float64) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE thoughts SET weight = ? WHERE id = ?`, t.Weight, t.DBID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO reweights (call_id, thought_id, old, new) VALUES (?, ?, ?, ?)`, call, t.DBID, old, t.Weight)
	return err
}

// addCall stores a call; graphID 0 is a call outside a graph.
func (s *store) addCall(ctx context.Context, graphID int64, model string, c *callRec) (int64, error) {
	r, err := s.db.ExecContext(ctx, `INSERT INTO calls (graph_id, model, purpose, focus, tries, valid, failures, error,
		input_tokens, output_tokens, thought_tokens, thinking_chars, ms, index_chars, prompt, reply, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sql.NullInt64{Int64: graphID, Valid: graphID != 0}, model, c.Purpose, c.Focus, c.Tries, c.Valid, strings.Join(c.Failures, ","), c.Err,
		c.Usage.Input+c.Usage.CacheRead+c.Usage.CacheWrite, c.Usage.Output, c.Usage.Thought, c.Thinking, c.Ms, c.IndexChars, c.Prompt, c.Reply, now())
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *store) finish(ctx context.Context, g *graph) error {
	_, err := s.db.ExecContext(ctx, `UPDATE graphs SET expansions = ?, next_picked = ?, next_invalid = ?, early_finish = ?,
		finish = ?, answer = ?, used = ?, ended_at = ? WHERE id = ?`,
		g.Expansions, g.NextPicked, g.NextInvalid, g.EarlyFinish, g.Finish, g.Answer, g.Used, now(), g.ID)
	return err
}

type answerRow struct {
	ID             int64
	Problem, Model string
	Kind           string // index, path, plain, thinking
	GraphID        int64
	CallID         int64
	Text, Used     string
	Valid          bool
	Note           string
}

func (s *store) addAnswer(ctx context.Context, a *answerRow) (int64, error) {
	r, err := s.db.ExecContext(ctx, `INSERT INTO answers (problem, model, kind, graph_id, call_id, text, used, valid, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.Problem, a.Model, a.Kind, sql.NullInt64{Int64: a.GraphID, Valid: a.GraphID != 0},
		sql.NullInt64{Int64: a.CallID, Valid: a.CallID != 0}, a.Text, a.Used, a.Valid, a.Note, now())
	if err != nil {
		return 0, err
	}
	a.ID, err = r.LastInsertId()
	return a.ID, err
}

// answers are the latest answer of each kind, by problem and model.
func (s *store) answers(ctx context.Context) (map[[2]string]map[string]*answerRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, problem, model, kind, COALESCE(graph_id, 0), COALESCE(call_id, 0), text, COALESCE(used, ''), valid, COALESCE(note, '')
		FROM answers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[[2]string]map[string]*answerRow{}
	for rows.Next() {
		a := &answerRow{}
		if err := rows.Scan(&a.ID, &a.Problem, &a.Model, &a.Kind, &a.GraphID, &a.CallID, &a.Text, &a.Used, &a.Valid, &a.Note); err != nil {
			return nil, err
		}
		key := [2]string{a.Problem, a.Model}
		if out[key] == nil {
			out[key] = map[string]*answerRow{}
		}
		out[key][a.Kind] = a
	}
	return out, rows.Err()
}
