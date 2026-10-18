package sqlguard

import "strings"

// Layer 1's budget for the work of preparing a query. SQLite can't be
// interrupted while it prepares: it checks only between tokens, and
// builds the whole program after the last one. Each use of a CTE copies
// it, with the CTEs it uses, so 1 KB of text can take seconds and
// gigabytes to prepare (WITH a1 AS (… a0 … a0), a2 AS (… a1 … a1), …).
// The worker's deadline and memory cap bound any query (sqlworker); the
// budget refuses the CTE shapes early, with a message that says what to
// change. Its limits are far above what an analytic query needs, and keep
// the worst prepare within them to tens of milliseconds
// (sqlguard_test.go). It doesn't model every shape: nested subqueries
// that SQLite flattens grow the same way, and only the worker stops them.
const (
	// MaxQuery is the longest query, in bytes.
	MaxQuery = 64 << 10
	// maxKeywords bounds SELECT, WITH and VALUES in the text.
	maxKeywords = 500
	// maxCTEs bounds the CTEs a query defines; a chain of them costs the
	// square of its length to prepare.
	maxCTEs = 100
	// maxExpanded bounds the SELECTs and VALUES the query has once every
	// use of a CTE is replaced by a copy of it.
	maxExpanded = 2000
)

// cte is one CTE definition in the token list.
type cte struct {
	name        string // lower case
	nameTok     int
	open, close int // the brackets around its body
}

// budget refuses a query that would cost too much to prepare.
func (c *checked) budget() *RejectError {
	q, toks := c.q, c.toks
	keywords, with := 0, false
	for _, t := range toks {
		if t.kind != word || len(t.text) != 4 && len(t.text) != 6 {
			continue
		}
		w := strings.EqualFold(t.text, "with")
		if w || strings.EqualFold(t.text, "select") || strings.EqualFold(t.text, "values") {
			with = with || w
			if keywords++; keywords > maxKeywords {
				return reject(q, 1, t.pos, "the query has more than %d SELECT, WITH and VALUES; split it into smaller queries", maxKeywords)
			}
		}
	}
	if !with {
		return nil
	}

	match := make([]int, len(toks)) // a bracket's partner; brackets balance
	var stack []int
	for i, t := range toks {
		switch {
		case t.kind == punct && t.text == "(":
			stack = append(stack, i)
		case t.kind == punct && t.text == ")":
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			match[o], match[i] = i, o
		}
	}
	var defs []cte
	for i, t := range toks {
		if t.kind == word && strings.EqualFold(t.text, "with") {
			defs = c.parseWith(i+1, match, defs)
			if len(defs) > maxCTEs {
				d := defs[maxCTEs]
				return reject(q, 1, toks[d.nameTok].pos, "the query defines more than %d CTEs; split it into smaller queries", maxCTEs)
			}
		}
	}
	if len(defs) == 0 {
		return nil
	}
	e := expansion{toks: toks, defs: defs, byName: map[string][]int{}, bodyAt: map[int]int{},
		isName: map[int]bool{}, cost: make([]int, len(defs)), state: make([]byte, len(defs))}
	for i, d := range defs {
		e.byName[d.name] = append(e.byName[d.name], i)
		e.bodyAt[d.open] = i
		e.isName[d.nameTok] = true
	}
	if n := e.scan(0, len(toks), -1); n > maxExpanded {
		return reject(q, 1, -1, "this query expands to too many subqueries (over %d): each use of a CTE copies it and the CTEs it uses; use each CTE fewer times", maxExpanded)
	}
	return nil
}

// parseWith reads the CTE definitions after WITH at toks[i], following
// SQLite's grammar: name [(columns)] AS [NOT] [MATERIALIZED] (body), … A
// definition that doesn't fit is left to SQLite, which refuses it.
// SQLite takes a 'string' for a name here, as it does after FROM.
func (c *checked) parseWith(i int, match []int, defs []cte) []cte {
	toks := c.toks
	is := func(i int, k kind, text string) bool {
		return i < len(toks) && toks[i].kind == k && (text == "" || lower(toks[i].text) == text)
	}
	if is(i, word, "recursive") {
		i++
	}
	for {
		if i >= len(toks) || !isName(toks[i]) {
			return defs
		}
		d := cte{name: lower(toks[i].text), nameTok: i}
		i++
		if is(i, punct, "(") {
			i = match[i] + 1
		}
		if !is(i, word, "as") {
			return defs
		}
		i++
		if is(i, word, "not") {
			i++
		}
		if is(i, word, "materialized") {
			i++
		}
		if !is(i, punct, "(") {
			return defs
		}
		d.open, d.close = i, match[i]
		defs = append(defs, d)
		i = d.close + 1
		if !is(i, punct, ",") {
			return defs
		}
		i++
	}
}

// expansion counts SELECTs and VALUES with each CTE use replaced by a
// copy. It overcounts where it can't tell (a column named like a CTE, or
// a name defined twice), never undercounts. Counts stop just over
// maxExpanded, so they can't overflow.
type expansion struct {
	toks   []token
	defs   []cte
	byName map[string][]int
	bodyAt map[int]int // the opening bracket of a body → its definition
	isName map[int]bool
	cost   []int
	state  []byte // 0 not counted, 1 counting, 2 counted
}

// isName reports a token that can name a CTE: a word, a quoted name, or a
// 'string', which SQLite also takes for a name (FROM 'a').
func isName(t token) bool { return t.kind == word || t.kind == quoted || t.kind == str }

// dot reports a '.' at toks[i]; a quoted "." is a name.
func (e *expansion) dot(i int) bool {
	return i >= 0 && i < len(e.toks) && e.toks[i].kind == punct && e.toks[i].text == "."
}

// scan counts toks[from:to], the body of definition self or, for -1, the
// whole query. Bodies of other definitions inside are counted only where
// they are used.
func (e *expansion) scan(from, to, self int) int {
	n := 0
	for i := from; i < to && n <= maxExpanded; i++ {
		if d, ok := e.bodyAt[i]; ok && d != self {
			i = e.defs[d].close
			continue
		}
		t := e.toks[i]
		if e.isName[i] || !isName(t) {
			continue
		}
		name := lower(t.text)
		if t.kind == word && (name == "select" || name == "values") {
			n++
			continue
		}
		ds, ok := e.byName[name]
		if !ok || e.dot(i-1) || e.dot(i+1) {
			continue // schema.name or name.column: not a CTE
		}
		dearest := 0 // a name defined more than once costs its dearest
		for _, d := range ds {
			if d == self {
				continue // a recursive CTE reads its own rows, not a copy
			}
			if e.state[d] == 0 {
				e.state[d] = 1
				e.cost[d] = e.scan(e.defs[d].open+1, e.defs[d].close, d)
				e.state[d] = 2
			}
			c := e.cost[d]
			if e.state[d] == 1 {
				c = 1 // a cycle; SQLite refuses it
			}
			dearest = max(dearest, c)
		}
		n = min(n+dearest, maxExpanded+1)
	}
	return n
}
