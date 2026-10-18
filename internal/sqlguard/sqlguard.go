// Package sqlguard checks the SQL that agents write (SPEC 2.2,
// CODE-OUTLINE 4): a text check before SQLite sees the query, an EXPLAIN
// check of its compiled program, and the settings of the reader it runs
// on. It needs no authorizer, so any SQLite driver works (SPIKE-007).
package sqlguard

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"modernc.org/sqlite"
)

// RejectError is a query the guard refused, worded so the model can fix
// it.
type RejectError struct {
	Layer  int // 1 text check, 2 EXPLAIN check
	Reason string
	Pos    int // byte offset in the query; -1 when there is no position
	Line   int // Pos as a 1-based line and column; Col counts characters
	Col    int
}

func (e *RejectError) Error() string {
	if e.Pos < 0 {
		return e.Reason
	}
	return fmt.Sprintf("line %d:%d: %s", e.Line, e.Col, e.Reason)
}

func reject(q string, layer, pos int, format string, a ...any) *RejectError {
	e := &RejectError{Layer: layer, Reason: fmt.Sprintf(format, a...), Pos: pos}
	if pos >= 0 {
		before := q[:pos]
		e.Line = strings.Count(before, "\n") + 1
		e.Col = utf8.RuneCountInString(before[strings.LastIndexByte(before, '\n')+1:]) + 1
	}
	return e
}

// allowedModules are the only virtual tables agent queries may use.
var allowedModules = map[string]bool{"json_each": true, "json_tree": true}

// deniedModules are the other virtual table modules in modernc's build
// (TestModulesCovered keeps the list complete), and modules other builds
// have that read the file or the schema. sqlite_* and pragma_* modules are
// refused by their prefix.
var deniedModules = map[string]bool{
	"dbstat": true, "fts5": true, "fts5vocab": true, "geopoly": true, "rtree": true, "rtree_i32": true,
	"fts3": true, "fts4": true, "fts3tokenize": true, "fts4aux": true, "bytecode": true, "tables_used": true,
}

// allowedFuncs are the functions agent queries may call: SQLite's core,
// date, math, JSON and window functions. Layer 2 refuses any other
// function in the program, and TestFunctionsCovered keeps this list and
// deniedFuncs complete for modernc's build.
var allowedFuncs = set(
	"->", "->>", "abs", "acos", "acosh", "asin", "asinh", "atan", "atan2", "atanh", "avg",
	"ceil", "ceiling", "changes", "char", "coalesce", "concat", "concat_ws", "cos", "cosh", "count",
	"cume_dist", "current_date", "current_time", "current_timestamp", "date", "datetime", "degrees",
	"dense_rank", "exp", "first_value", "floor", "format", "glob", "group_concat", "hex", "if", "ifnull",
	"iif", "instr", "json", "json_array", "json_array_insert", "json_array_length", "json_error_position",
	"json_extract", "json_group_array", "json_group_object", "json_insert", "json_object", "json_patch",
	"json_pretty", "json_quote", "json_remove", "json_replace", "json_set", "json_type", "json_valid",
	"jsonb", "jsonb_array", "jsonb_array_insert", "jsonb_extract", "jsonb_group_array", "jsonb_group_object",
	"jsonb_insert", "jsonb_object", "jsonb_patch", "jsonb_remove", "jsonb_replace", "jsonb_set",
	"julianday", "lag", "last_insert_rowid", "last_value", "lead", "length", "like", "likelihood", "likely",
	"ln", "log", "log10", "log2", "lower", "ltrim", "match", "max", "min", "mod", "nth_value", "ntile",
	"nullif", "octet_length", "percent_rank", "pi", "pow", "power", "printf", "quote", "radians", "random",
	"randomblob", "rank", "replace", "round", "row_number", "rtrim", "sign", "sin", "sinh", "soundex",
	"sqrt", "strftime", "string_agg", "substr", "substring", "subtype", "sum", "tan", "tanh", "time",
	"timediff", "total", "total_changes", "trim", "trunc", "typeof", "unhex", "unicode", "unistr",
	"unistr_quote", "unixepoch", "unlikely", "upper", "zeroblob",
)

// deniedFuncs are functions agent queries may never call: they load code,
// or run SQL of their own that layer 2 never sees (rtreecheck reads any
// table it is given), or belong to virtual tables agents can't use.
// sqlite_* functions are refused by their prefix.
var deniedFuncs = set(
	"load_extension", "fts3_tokenizer", "rtreecheck", "rtreenode", "rtreedepth",
	"fts5", "fts5_get_locale", "fts5_insttoken", "fts5_locale", "fts5_source_id",
	"bm25", "highlight", "snippet", "matchinfo", "offsets", "optimize",
	"geopoly_area", "geopoly_bbox", "geopoly_blob", "geopoly_ccw", "geopoly_contains_point",
	"geopoly_debug", "geopoly_group_bbox", "geopoly_json", "geopoly_overlap", "geopoly_regular",
	"geopoly_svg", "geopoly_within", "geopoly_xform",
)

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// maxParam is the largest ?NNN, set on the reader for each agent query
// (Restrict). SQLite's default, 32,766, makes binding NULL to all of them
// for EXPLAIN take seconds.
const maxParam = 999

// checked is what layer 1 learned about a query.
type checked struct {
	q      string
	toks   []token // without a final ';'
	end    int     // where the statement ends: before a final ';'
	vtab   bool    // calls json_each( or json_tree(
	params []int   // for each param token, its number as SQLite assigns it
	nVar   int     // the largest parameter number
}

// Check is layer 1, the text check (2.2). It must run before SQLite
// prepares any of the text: SQLite runs some PRAGMAs while preparing them,
// so even EXPLAIN PRAGMA query_only = 0 turns read-only off. It refuses
// more than one statement, anything that doesn't start with SELECT, WITH
// or VALUES, _jenab_, sqlite_ and pragma_ names, virtual tables other than
// json_each and json_tree, denied functions, unbalanced brackets, any
// character it doesn't know outside strings, quoted names and comments
// (lex.go), and a query that would cost too much to prepare (budget.go).
// The error is a *RejectError.
func Check(q string) error {
	if _, err := check(q); err != nil {
		return err
	}
	return nil
}

func check(q string) (*checked, *RejectError) {
	if len(q) > MaxQuery {
		return nil, reject(q, 1, -1, "the query is %d bytes; at most %d are allowed", len(q), MaxQuery)
	}
	toks, err := lex(q)
	if err != nil {
		return nil, err
	}
	c := &checked{q: q, toks: toks, end: len(q)}
	if n := len(toks); n > 0 && toks[n-1].kind == semi {
		c.toks, c.end = toks[:n-1], toks[n-1].pos
	}
	if len(c.toks) == 0 {
		return nil, reject(q, 1, -1, "the query is empty")
	}
	first := c.toks[0]
	if kw := lower(first.text); first.kind != word || kw != "select" && kw != "with" && kw != "values" {
		return nil, reject(q, 1, first.pos, "a query must start with SELECT, WITH or VALUES, not `%s`", first.text)
	}
	names := map[string]int{}
	var open []int // where each open '(' is
	for i, t := range c.toks {
		switch t.kind {
		case semi:
			return nil, reject(q, 1, t.pos, "only one statement is allowed")
		case word, quoted:
			call := i+1 < len(c.toks) && c.toks[i+1].text == "(" && c.toks[i+1].kind == punct
			if e := checkName(q, t, call); e != nil {
				return nil, e
			}
			if call && allowedModules[lower(t.text)] {
				c.vtab = true
			}
		case param:
			n, e := c.number(t, names)
			if e != nil {
				return nil, e
			}
			c.params = append(c.params, n)
		case punct:
			// Balanced, so that q can't close the brackets Wrap puts around
			// it.
			switch t.text {
			case "(":
				open = append(open, t.pos)
			case ")":
				if len(open) == 0 {
					return nil, reject(q, 1, t.pos, "this `)` has no matching `(`")
				}
				open = open[:len(open)-1]
			}
		}
	}
	if len(open) > 0 {
		return nil, reject(q, 1, open[len(open)-1], "this `(` is never closed")
	}
	if e := c.budget(); e != nil {
		return nil, e
	}
	return c, nil
}

// checkName checks a name; call says it is followed by '(', so denied
// functions don't make a column of the same name (snippet) unusable.
func checkName(q string, t token, call bool) *RejectError {
	switch n := lower(t.text); {
	case strings.HasPrefix(n, "_jenab_"):
		return reject(q, 1, t.pos, "`%s` is internal", t.text)
	case strings.HasPrefix(n, "sqlite_"):
		return reject(q, 1, t.pos, "`%s` is reserved", t.text)
	case strings.HasPrefix(n, "pragma_"):
		return reject(q, 1, t.pos, "`%s` is not allowed: queries can't use PRAGMA functions", t.text)
	case call && deniedFuncs[n]:
		return reject(q, 1, t.pos, "`%s` is not allowed", t.text)
	case deniedModules[n]:
		return reject(q, 1, t.pos, "`%s` is not allowed: json_each and json_tree are the only virtual tables", t.text)
	}
	return nil
}

// number gives a parameter the number SQLite gives it: ? takes the next
// one, ?NNN takes NNN, and a named parameter keeps the number of its first
// use (sqlite3ExprAssignVarNumber).
func (c *checked) number(t token, names map[string]int) (int, *RejectError) {
	switch {
	case t.text == "?":
		c.nVar++
		return c.nVar, nil
	case t.text[0] == '?':
		n, err := strconv.Atoi(t.text[1:])
		if err != nil || n < 1 || n > maxParam {
			return 0, reject(c.q, 1, t.pos, "parameter %s must be between ?1 and ?%d", t.text, maxParam)
		}
		c.nVar = max(c.nVar, n)
		return n, nil
	}
	if n, ok := names[t.text]; ok {
		return n, nil
	}
	c.nVar++
	names[t.text] = c.nVar
	return c.nVar, nil
}

// nullArgs binds NULL to every parameter, so EXPLAIN compiles the query
// whatever the caller's arguments are (2.2). modernc binds ?, ?NNN and
// $NNN by position and other names by name.
func (c *checked) nullArgs() []any {
	n := c.nVar
	var named []any
	seen := map[string]bool{}
	for _, t := range c.toks {
		if t.kind != param || t.text[0] == '?' || seen[t.text] {
			continue
		}
		seen[t.text] = true
		name := t.text[1:]
		if p, err := strconv.Atoi(name); err == nil && t.text[0] == '$' {
			n = max(n, min(p, maxParam))
			continue
		}
		// database/sql refuses a name that doesn't start with a letter, as
		// it would refuse the caller's own argument; SQLite then says
		// which one is missing.
		if r, _ := utf8.DecodeRuneInString(name); unicode.IsLetter(r) {
			named = append(named, sql.Named(name, nil))
		}
	}
	return append(make([]any, n), named...)
}

// Wrap returns q, which must pass Check, as one statement that does all of
// q's work in its first step and also reports how many rows q has. It is
// part of layer 3: modernc interrupts a statement at its deadline only
// while the first row is computed, and a later row can take forever
// (WHERE i = 1 OR i = 1e12 over an endless CTE).
//
// The statement stores at most rows rows of q in a materialized CTE and
// adds a last column with the total: the stored rows, or, when q filled
// them, a second count of q that stops after countTo+1 rows. CASE runs
// that count only when it is needed. The CTE keeps q's row order, and
// duplicate column names get SQLite's ":1" suffix. A bare ? becomes ?NNN
// so that both copies of q share their parameters, and a final ';' is
// dropped.
//
// Both copies of q sit where no outer table is in scope, so a name in q
// means what it means in q alone: SQLite lets a subquery in FROM see the
// columns of the query around it, and a "double-quoted" string that isn't
// a column of q could otherwise turn into one of _jenab_rows.
func Wrap(q string, rows, countTo int) (string, error) {
	c, err := check(q)
	if err != nil {
		return "", err
	}
	body := c.body()
	m, n := strconv.Itoa(rows), strconv.Itoa(countTo+1)
	return "WITH _jenab_rows AS MATERIALIZED (SELECT * FROM (" + body + "\n) LIMIT " + m + "),\n" +
		"_jenab_total AS MATERIALIZED (SELECT CASE WHEN (SELECT count(*) FROM _jenab_rows) < " + m +
		" THEN (SELECT count(*) FROM _jenab_rows)\n" +
		"ELSE (SELECT count(*) FROM (SELECT 1 FROM (" + body + "\n) LIMIT " + n + ")) END AS n)\n" +
		"SELECT _jenab_rows.*, _jenab_total.n FROM _jenab_rows, _jenab_total", nil
}

// body is the statement without a final ';', with each bare ? numbered.
func (c *checked) body() string {
	var b strings.Builder
	last, p := 0, 0
	for _, t := range c.toks {
		if t.kind != param {
			continue
		}
		if t.text == "?" {
			b.WriteString(c.q[last:t.pos])
			b.WriteString("?" + strconv.Itoa(c.params[p]))
			last = t.pos + 1
		}
		p++
	}
	b.WriteString(c.q[last:c.end])
	return b.String()
}

// SQLite limit IDs (sqlite3.h); modernc doesn't export names for them.
const (
	limitLength   = 0 // SQLITE_LIMIT_LENGTH
	limitAttached = 7 // SQLITE_LIMIT_ATTACHED
	limitVariable = 9 // SQLITE_LIMIT_VARIABLE_NUMBER
)

// MaxLength is the largest string or blob a reader builds (2.2).
const MaxLength = 16 << 20

// AgentLength is the largest string or blob an agent query builds or
// reads. The worker's memory cap bounds the query as a whole (sqlworker);
// this keeps one value from taking a large part of it.
const AgentLength = 1 << 20

// defaultVariables is SQLITE_LIMIT_VARIABLE_NUMBER in modernc's build.
const defaultVariables = 32766

// Prepare is layer 3's connection part (2.2, 7.2): query_only, no ATTACH
// (a read-only connection can attach and read any other file), the 16 MiB
// length limit, and the default parameter limit, undoing Restrict. Run it
// each time a reader is taken from the pool, so a connection that
// something changed can't be used without them.
func Prepare(ctx context.Context, c *sql.Conn) error {
	if _, err := c.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return err
	}
	return limits(c, [][2]int{{limitAttached, 0}, {limitLength, MaxLength}, {limitVariable, defaultVariables}})
}

// Restrict sets the tighter limits of an agent query on a prepared
// reader: values of at most AgentLength and parameters up to ?999. The
// next Prepare undoes them.
func Restrict(c *sql.Conn) error {
	return limits(c, [][2]int{{limitLength, AgentLength}, {limitVariable, maxParam}})
}

func limits(c *sql.Conn, ls [][2]int) error {
	for _, l := range ls {
		if _, err := sqlite.Limit(c, l[0], l[1]); err != nil {
			return err
		}
	}
	return nil
}
