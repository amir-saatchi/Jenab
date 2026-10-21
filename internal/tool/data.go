package tool

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Data tools (SPEC 8.1): query, describe_table and insert. query runs the
// agent's SQL through the guard (2.2); insert writes through the typed
// Insert with the change log (2.5), so Undo turn reverts it.

// Limits of the data tools (SPEC 8.1).
const (
	// QueryRows and QueryTokens bound the rows a query shows; a result
	// over either is stored with a preview of its first rows (3.7).
	QueryRows   = 200
	QueryTokens = 4000
	// queryKeep is the most rows a stored result holds; the 8 MiB of
	// AgentQuery bound it too.
	queryKeep = 10_000
	// InsertRows is the most rows an insert takes.
	InsertRows = 50
	// maxFieldErrors is the most errors an insert lists.
	maxFieldErrors = 50
)

// DataTools are the data tools.
func DataTools() []Tool { return []Tool{queryTool(), describeTable(), insertTool()} }

type queryArgs struct {
	SQL    string                     `json:"sql"`
	Params map[string]json.RawMessage `json:"params"`
}

func queryTool() Tool {
	return Func(Spec{
		Name: "query",
		Description: "Run one read-only SQLite query (SELECT, WITH or VALUES) on the project's tables. " +
			"Returns up to 200 rows or 4,000 tokens and the total row count; a larger result comes as a preview " +
			"and a ref, read on with read_ref or search_ref. Jenab's internal tables can't be queried: " +
			"use describe_table for a table's columns.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"sql": {"type": "string", "minLength": 1, "description": "One SQLite query, without a trailing semicolon."},
				"params": {"type": "object", "description": "Values for the query's parameters by name, such as {\"coin\": \"BTC\"} for :coin, or by number, such as {\"1\": 5} for ?1. A bare ? takes the number after the one before it, so in SELECT :c, ? the ? is ?2."}
			},
			"required": ["sql"],
			"additionalProperties": false
		}`),
		Effects: ReadsDB,
		Areas:   Tables,
		// The query's own 10 s, and up to 30 s for a worker to start (2.2).
		Timeout: 45 * time.Second,
	}, runQuery)
}

func runQuery(ctx context.Context, env *Env, a queryArgs) (Result, error) {
	args, err := queryParams(a.SQL, a.Params)
	if err != nil {
		return Result{}, err
	}
	rows, err := env.Project.DB.AgentQuery(ctx, a.SQL, args, queryKeep)
	if err != nil {
		return Result{}, queryError(ctx, env, err)
	}
	return queryResult(rows, env.PreviewTokens), nil
}

// queryParams matches the params object to the query's own parameters
// (sqlguard.Params), so no value binds to the wrong place: ?N, and a bare
// ? that SQLite numbers N (the number after the parameter before it), take
// "N", and :name, @name and $name take "name", with or without the prefix. A parameter without a value, a key the query has no parameter
// for, and two keys for one parameter are errors. A query layer 1 refuses
// gets no arguments; AgentQuery says why it is refused.
func queryParams(q string, ps map[string]json.RawMessage) ([]any, error) {
	want, err := sqlguard.Params(q)
	if err != nil {
		return nil, nil
	}
	given := make([]string, 0, len(ps))
	for k := range ps {
		given = append(given, k)
	}
	slices.Sort(given)
	keys := map[string]string{} // the parameter a key names → the key
	for _, k := range given {
		p := paramKey(k)
		if p == "" {
			return nil, Errorf("params: %q is not a parameter's name or number", k)
		}
		if other, ok := keys[p]; ok {
			return nil, Errorf("params: %q and %q are the same parameter; give it once", other, k)
		}
		keys[p] = k
	}
	var pos, named []any
	var missing, labels []string
	used := map[string]bool{}
	for _, p := range want {
		key, label := strconv.Itoa(p.Number), "?"+strconv.Itoa(p.Number)
		if p.Name != "" {
			key, label = p.Name[1:], p.Name
		}
		labels = append(labels, label)
		k, ok := keys[key]
		if !ok {
			missing = append(missing, label)
			continue
		}
		v, err := paramValue(k, ps[k])
		if err != nil {
			return nil, err
		}
		if p.Name == "" {
			for len(pos) < p.Number {
				pos = append(pos, nil)
			}
			pos[p.Number-1] = v
		} else if !used[key] { // :a and @a are two parameters with one name
			named = append(named, sql.Named(key, v))
		}
		used[key] = true
	}
	if len(missing) > 0 {
		return nil, Errorf("params has no value for %s; give each of the query's parameters a value", strings.Join(missing, ", "))
	}
	for _, k := range given {
		if p := paramKey(k); !used[p] {
			has := "it has none"
			if len(labels) > 0 {
				has = "its parameters are " + strings.Join(labels, ", ")
			}
			return nil, Errorf("params.%s: the query has no parameter %s; %s", k, k, has)
		}
	}
	// Positional values come first, so their places are their numbers.
	return append(pos, named...), nil
}

// paramKey is the parameter a key of params names: a number without its
// ? or leading zeros, or a name without its :, @ or $; "" for neither.
func paramKey(k string) string {
	if k != "" && strings.ContainsRune(":@$?", rune(k[0])) {
		k = k[1:]
	}
	if n, err := strconv.Atoi(k); err == nil {
		if n < 1 {
			return ""
		}
		return strconv.Itoa(n)
	}
	return k
}

// paramValue is one parameter's value from its JSON.
func paramValue(k string, raw json.RawMessage) (any, error) {
	v, err := jsonValue(raw)
	if err != nil {
		return nil, Errorf("params.%s: %v", k, err)
	}
	switch x := v.(type) {
	case []any, map[string]any:
		return nil, Errorf("params.%s: a list or an object can't be a parameter; give JSON as a string", k)
	case json.Number:
		if v, err = numberValue(x); err != nil {
			return nil, Errorf("params.%s: %v", k, err)
		}
	}
	return v, nil
}

// jsonValue decodes one JSON value, keeping numbers as json.Number.
func jsonValue(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	err := d.Decode(&v)
	return v, err
}

// numberValue is n as an int64, or a float64 if it has a fraction or is
// too large.
func numberValue(n json.Number) (any, error) {
	if i, err := n.Int64(); err == nil {
		return i, nil
	}
	f, err := n.Float64()
	if err != nil || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%s is not a number SQLite can hold", n)
	}
	return f, nil
}

// queryError turns what the model can fix into an *Error: a refusal of the
// guard, an error of SQLite, a query that ran too long or used too much
// memory. Any other error is a system failure.
func queryError(ctx context.Context, env *Env, err error) error {
	var re *sqlguard.RejectError
	var me *sqlworker.MemoryError
	switch {
	case errors.As(err, &re):
		msg := "the query was refused: " + re.Error()
		if re.Schema {
			msg += ". Jenab's own tables, sqlite_schema and PRAGMA functions can't be queried; " + tableList(ctx, env) +
				" Use describe_table for a table's columns, keys and row count."
		}
		return &Error{Msg: msg, Err: err}
	case errors.As(err, &me):
		return &Error{Msg: me.Error() + "; make the query smaller, such as with fewer rows or columns, or an aggregate", Err: err}
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
		msg := strings.TrimSuffix(err.Error(), ": "+context.DeadlineExceeded.Error())
		return &Error{Msg: msg + "; make the query cheaper, such as with a WHERE on an indexed column or fewer joins", Err: err}
	case sqlworker.IsSQLite(err):
		msg := sqliteText(err.Error())
		if strings.Contains(msg, "no such table") {
			msg += "; " + tableList(ctx, env)
		}
		return &Error{Msg: "the query failed: " + msg, Err: err}
	case errors.Is(err, store.ErrClosed):
		return &Error{Msg: "the project is closing, so the query can't run", Err: err}
	}
	return err
}

// sqliteText is SQLite's message without the driver's class and code,
// "SQL logic error: no such column: x (1)" as "no such column: x".
func sqliteText(s string) string {
	if i := strings.LastIndex(s, " ("); i >= 0 && strings.HasSuffix(s, ")") {
		if _, err := strconv.Atoi(s[i+2 : len(s)-1]); err == nil {
			s = s[:i]
		}
	}
	return strings.TrimPrefix(s, "SQL logic error: ")
}

// tableList is a sentence naming the project's tables, for an error.
func tableList(ctx context.Context, env *Env) string {
	names, err := env.Project.DB.Tables(ctx)
	switch {
	case err != nil:
		return ""
	case len(names) == 0:
		return "the project has no tables yet."
	}
	return "the project's tables are: " + strings.Join(names, ", ") + "."
}

// queryResult is the rows as text: the columns, then one line a row,
// with the values split by " | ". A result within QueryRows and
// QueryTokens enters whole; over previewTokens it is also stored, under
// a header with its ref, so a cut can make it a stub (3.7). A larger
// result is stored, and its Preview shows its first rows.
func queryResult(r store.Rows, previewTokens int) Result {
	var b strings.Builder
	b.WriteString(strings.Join(r.Columns, " | "))
	b.WriteByte('\n')
	head := b.Len()                       // the columns' line
	ends := make([]int, 0, len(r.Values)) // where each row's line ends
	for _, row := range r.Values {
		for i, v := range row {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(cell(v))
		}
		b.WriteByte('\n')
		ends = append(ends, b.Len())
	}
	text := b.String()
	total := rowCount(r)
	if len(ends) <= QueryRows && tokens(text) <= QueryTokens && !r.Cut {
		whole := text + "(" + total + ")"
		if tokens(whole) <= previewTokens {
			return Result{Text: whole, Whole: true}
		}
		return Result{Text: whole, Preview: func(ref string) string {
			return fmt.Sprintf("[query result — %s — %s tokens — ref: %s]\n%s", total, count(tokens(whole)), ref, strings.TrimRight(text, "\n"))
		}}
	}
	// The preview: as many whole rows as fit, at least the start of one.
	shown := 0
	for shown < len(ends) && shown < QueryRows && tokens(text[:ends[shown]]) <= QueryTokens {
		shown++
	}
	cutAt := head
	if shown > 0 {
		cutAt = ends[shown-1]
	} else if len(ends) > 0 {
		cutAt = len(cut(text, QueryTokens*4))
	}
	stored := len(r.Values)
	full := text
	if stored < r.Total || r.Capped {
		why := ""
		if r.Full {
			why = ", as rows past 8 MiB are left out"
		}
		full += fmt.Sprintf("(the first %s of %s%s)", count(stored), total, why)
	} else {
		full += "(" + total + ")"
	}
	return Result{Text: full, Preview: func(ref string) string {
		showing := fmt.Sprintf("showing the first %s", plural(shown, "row"))
		if shown == 0 {
			showing = "showing the start of the first row"
		}
		var p strings.Builder
		fmt.Fprintf(&p, "[query result — %s — %s tokens, %s — ref: %s]\n", total, count(tokens(full)), showing, ref)
		p.WriteString(strings.TrimRight(text[:cutAt], "\n"))
		p.WriteString("\n[")
		if stored < r.Total || r.Capped {
			fmt.Fprintf(&p, "the ref holds the first %s. ", plural(stored, "row"))
		}
		p.WriteString("Narrow the query with WHERE, GROUP BY or LIMIT, or " +
			strings.TrimPrefix(strings.TrimSuffix(more(cutAt), "]"), "[") + "]")
		return p.String()
	}}
}

// rowCount is the query's rows in all, as "1,234 rows".
func rowCount(r store.Rows) string {
	if r.Capped {
		return "more than " + plural(r.Total, "row")
	}
	return plural(r.Total, "row")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return count(n) + " " + word + "s"
}

// cell is a value as a row's line shows it. Text keeps its characters,
// with \, | and line breaks escaped so each row stays on its line.
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case string:
		return cellEscaper.Replace(x)
	case []byte:
		return fmt.Sprintf("[blob, %s bytes]", count(len(x)))
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if a := math.Abs(x); a != 0 && (a < 1e-6 || a >= 1e15) {
			return strconv.FormatFloat(x, 'g', -1, 64)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

var cellEscaper = strings.NewReplacer(`\`, `\\`, "|", `\|`, "\n", `\n`, "\r", `\r`)

type describeArgs struct {
	Name string `json:"name"`
}

func describeTable() Tool {
	return Func(Spec{
		Name:        "describe_table",
		Description: "A table's columns with their types, its keys, indexes and foreign keys, column annotations such as object_ref, and its row count.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {"name": {"type": "string", "minLength": 1, "description": "The table's name."}},
			"required": ["name"],
			"additionalProperties": false
		}`),
		Effects: ReadsDB,
		Areas:   Tables,
	}, func(ctx context.Context, env *Env, a describeArgs) (Result, error) {
		t, err := env.Project.DB.DescribeTable(ctx, a.Name)
		if err != nil {
			return Result{}, tableError(ctx, env, a.Name, err)
		}
		return Result{Text: Describe(t)}, nil
	})
}

// tableError is the error for a table that isn't there or is internal.
func tableError(ctx context.Context, env *Env, name string, err error) error {
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	l := strings.ToLower(name)
	if strings.HasPrefix(l, "_jenab_") || strings.HasPrefix(l, "sqlite_") {
		return &Error{Msg: fmt.Sprintf("%s is internal to Jenab and has no description here; %s", name, tableList(ctx, env)), Err: err}
	}
	return &Error{Msg: fmt.Sprintf("there is no table %q; %s", name, tableList(ctx, env)), Err: err}
}

// Describe is a table as describe_table shows it; the schema agent gets
// the project's tables this way too (8.2).
func Describe(t store.Table) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Table %s: %s", t.Name, plural(t.Rows, "row"))
	var kind []string
	if t.Strict {
		kind = append(kind, "STRICT")
	}
	if t.WithoutRowid {
		kind = append(kind, "WITHOUT ROWID")
	}
	if len(kind) > 0 {
		b.WriteString(" (" + strings.Join(kind, ", ") + ")")
	}
	b.WriteString("\nColumns:")
	for _, c := range t.Columns {
		typ := c.Type
		if typ == "" {
			typ = "(no type)"
		}
		fmt.Fprintf(&b, "\n- %s %s", c.Name, typ)
		if c.NotNull {
			b.WriteString(" NOT NULL")
		}
		if c.Default != nil {
			b.WriteString(" DEFAULT " + *c.Default)
		}
		var notes []string
		if slices.Contains(t.Key, c.Name) {
			n := "primary key"
			if t.AutoKey {
				n += ", picked by SQLite when left out"
			}
			notes = append(notes, n)
		}
		if c.Generated {
			notes = append(notes, "generated: SQLite computes it")
		}
		switch c.Kind {
		case "":
		case "object_ref":
			notes = append(notes, "object_ref: holds bucket keys")
		default:
			notes = append(notes, c.Kind)
		}
		if len(notes) > 0 {
			b.WriteString(" — " + strings.Join(notes, "; "))
		}
	}
	if len(t.Key) > 0 {
		fmt.Fprintf(&b, "\nPrimary key: (%s)", strings.Join(t.Key, ", "))
	} else {
		b.WriteString("\nPrimary key: none, so insert can't write to it")
	}
	for _, ix := range t.Indexes {
		cols := slices.Clone(ix.Columns)
		for i, c := range cols {
			if c == "" {
				cols[i] = "(expression)"
			}
		}
		what := "Index"
		if ix.Unique {
			what = "Unique index"
		}
		fmt.Fprintf(&b, "\n%s %s: (%s)", what, ix.Name, strings.Join(cols, ", "))
		if ix.Partial {
			b.WriteString(", partial")
		}
	}
	for _, fk := range t.ForeignKeys {
		to := ""
		if len(fk.To) > 0 {
			to = " (" + strings.Join(fk.To, ", ") + ")"
		}
		fmt.Fprintf(&b, "\nForeign key: (%s) references %s%s", strings.Join(fk.Columns, ", "), fk.Table, to)
		if fk.OnDelete != "NO ACTION" && fk.OnDelete != "" {
			b.WriteString(", on delete " + strings.ToLower(fk.OnDelete))
		}
		if fk.OnUpdate != "NO ACTION" && fk.OnUpdate != "" {
			b.WriteString(", on update " + strings.ToLower(fk.OnUpdate))
		}
	}
	return b.String()
}

type insertArgs struct {
	Table string            `json:"table"`
	Rows  []json.RawMessage `json:"rows"`
}

func insertTool() Tool {
	return Func(Spec{
		Name: "insert",
		Description: "Add rows to a table, at most 50 a call, for small entries by hand. Each row is an object of column names and values. " +
			"Every row is checked against the column types and NOT NULL first; then all errors come back at once and nothing is written. " +
			"A key that is taken fails the call. Undo turn reverts it.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"table": {"type": "string", "minLength": 1, "description": "The table's name."},
				"rows": {"type": "array", "minItems": 1, "items": {"type": "object"}, "description": "The rows, each an object of column names and values, such as {\"date\": \"2026-10-18\", \"price\": 61240.5}."}
			},
			"required": ["table", "rows"],
			"additionalProperties": false
		}`),
		Effects: WritesDB,
		Areas:   Tables,
	}, runInsert)
}

func runInsert(ctx context.Context, env *Env, a insertArgs) (Result, error) {
	if len(a.Rows) > InsertRows {
		return Result{}, Errorf("insert takes at most %d rows a call, not %d; nothing was written. Send the rows in calls of %d or fewer",
			InsertRows, len(a.Rows), InsertRows)
	}
	t, err := env.Project.DB.DescribeTable(ctx, a.Table)
	if err != nil {
		return Result{}, tableError(ctx, env, a.Table, err)
	}
	if len(t.Key) == 0 {
		return Result{}, Errorf("%s has no primary key, so insert can't write to it: the change log knows rows by their key", t.Name)
	}
	in := make([]map[string]any, len(a.Rows))
	for i, raw := range a.Rows {
		v, err := jsonValue(raw)
		if err != nil {
			return Result{}, Errorf("rows[%d]: %v", i, err)
		}
		in[i], _ = v.(map[string]any) // the schema makes each row an object
	}
	rows, errs := t.CheckRows(in)
	if len(errs) > 0 {
		return Result{}, &Error{Msg: fieldErrors(errs)}
	}
	res, err := env.Project.DB.Insert(ctx, env.Priority, env.Source, t.Name, rows, store.ConflictFail)
	if err != nil {
		return Result{}, writeError(t.Name, len(rows), res, err)
	}
	return Result{Text: fmt.Sprintf("Inserted %s into %s.", plural(res.Inserted, "row"), t.Name)}, nil
}

// fieldErrors lists the rows' errors, by row and column.
func fieldErrors(errs []store.FieldError) string {
	var b strings.Builder
	fmt.Fprintf(&b, "nothing was written; %s:", plural(len(errs), "error"))
	for i, e := range errs {
		if i == maxFieldErrors {
			fmt.Fprintf(&b, "\n- and %s more", count(len(errs)-i))
			break
		}
		at := fmt.Sprintf("rows[%d]", e.Row)
		if e.Column != "" {
			at += "." + e.Column
		}
		fmt.Fprintf(&b, "\n- %s: %s", at, e.Msg)
	}
	return b.String()
}

// writeError is the error of a failed insert of n rows. The store writes
// a request in chunks of 500 rows, each in its own transaction, so a row
// that only the write can check, such as against a CHECK constraint or a
// partial unique index, can fail a later chunk after earlier ones were
// written (P2-02). res counts those; they stay, and the error says so.
func writeError(table string, n int, res store.WriteResult, err error) error {
	var re *store.RowError
	at, msg := "", err.Error()
	if errors.As(err, &re) {
		at, msg = fmt.Sprintf("rows[%d]: ", re.Row), re.Err.Error()
	}
	msg = strings.TrimPrefix(sqliteText(strings.ReplaceAll(msg, "store: ", "")), "constraint failed: ")
	written := res.Inserted + res.Updated
	if written == 0 && res.Change == "" {
		switch {
		case errors.Is(err, store.ErrReadOnly):
			return &Error{Msg: "the project is open read-only after damage, so nothing can be changed; tell the user", Err: err}
		case errors.Is(err, store.ErrInvalid), errors.Is(err, store.ErrNotFound):
			return &Error{Msg: "nothing was written: " + at + msg, Err: err}
		}
		return err
	}
	return &Error{Msg: fmt.Sprintf("only part of the rows were written: %s of %d went into %s before %s%s. "+
		"The written rows stay, and Undo turn reverts them; the other rows were not written.",
		plural(written, "row"), n, table, at, msg), Err: err}
}
