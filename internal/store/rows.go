package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"modernc.org/sqlite"
)

// Row is one row of an agent table, by column name. Values are nil,
// strings, numbers, bools, json.Number or []byte.
type Row map[string]any

// Conflict says what Insert does with a row whose primary key is taken.
type Conflict int

const (
	ConflictFail   Conflict = iota // the chunk fails with ErrRowExists
	ConflictSkip                   // the row is left as it is
	ConflictUpsert                 // the given columns are updated; others are untouched (SPEC 6.5)
)

var (
	// ErrRowExists is an insert whose primary key is taken, with ConflictFail.
	ErrRowExists = errors.New("store: a row with that key already exists")
	// ErrNotUnique is a row whose value in a UNIQUE column other than the
	// primary key is taken.
	ErrNotUnique = errors.New("store: a unique value is taken")
	// ErrNotAgentTable is a write to an internal table, a view or a virtual
	// table, or one with no explicit primary key or with triggers (SPEC 2.2).
	ErrNotAgentTable = errors.New("store: not an agent table")
	// ErrInvalid is a write that doesn't fit the table: an unknown column, a
	// missing key, a value that can't be stored or a broken constraint. All
	// the errors above match it too.
	ErrInvalid = errors.New("store: invalid write")
)

// invalid marks err as the request's own error, not the database's, so the
// writer's Stats leave it out.
type invalid struct{ error }

func (e invalid) Unwrap() error { return e.error }

func (e invalid) Is(target error) bool { return target == ErrInvalid }

// fault returns the request's own error with its message.
func fault(format string, a ...any) error { return invalid{fmt.Errorf(format, a...)} }

// checkErr maps SQLite's constraint errors to the request's own errors.
func checkErr(err error) error {
	var se *sqlite.Error
	if !errors.As(err, &se) || se.Code()&0xff != 19 { // SQLITE_CONSTRAINT and its extended codes
		return err
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, "constraint failed: "); i >= 0 {
		msg = strings.TrimSuffix(msg[i+len("constraint failed: "):], fmt.Sprintf(" (%d)", se.Code()))
	}
	switch se.Code() {
	case 1555: // SQLITE_CONSTRAINT_PRIMARYKEY
		return fault("%w (%s)", ErrRowExists, msg)
	case 2067: // SQLITE_CONSTRAINT_UNIQUE
		return fault("%w: the values of %s must be unique", ErrNotUnique, msg)
	}
	return invalid{err}
}

// maxColumns is the widest table whose rows the change log can hold:
// json_object takes at most 1,000 arguments, two per column.
const maxColumns = 500

// agentTable is a table as the live schema has it.
type agentTable struct {
	name   string            // as in the schema
	cols   []string          // in table order
	pk     []string          // in key order
	rowid  bool              // the key is an INTEGER PRIMARY KEY, so an insert may leave it out
	types  map[string]string // declared types, by column
	strict bool
	refs   []string // object_ref columns (SPEC 4.5)
	lower  map[string]string
}

// internalName reports names agent tables can't have (SPEC 2.2). SQLite
// compares names without case.
func internalName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "_jenab_") || strings.HasPrefix(n, "sqlite_")
}

// loadTable reads name's columns, key and object references from the
// schema inside tx, so a migration between two chunks is seen.
func loadTable(tx *sql.Tx, name string) (*agentTable, error) {
	if internalName(name) {
		return nil, fault("%w: %s is internal", ErrNotAgentTable, name)
	}
	t := &agentTable{lower: map[string]string{}, types: map[string]string{}}
	var typ string
	var wr bool
	err := tx.QueryRow(`SELECT name, type, wr, strict FROM pragma_table_list WHERE schema = 'main' AND name = ? COLLATE NOCASE`, name).
		Scan(&t.name, &typ, &wr, &t.strict)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, invalid{fmt.Errorf("store: table %s: %w", name, ErrNotFound)}
	}
	if err != nil {
		return nil, err
	}
	if typ != "table" {
		return nil, fault("%w: %s is a %s", ErrNotAgentTable, t.name, typ)
	}
	var triggers int
	// A connection's temporary triggers may be on main tables too.
	if err := tx.QueryRow(`SELECT (SELECT count(*) FROM sqlite_schema WHERE type = 'trigger' AND tbl_name = ?1 COLLATE NOCASE) +
		(SELECT count(*) FROM sqlite_temp_schema WHERE type = 'trigger' AND tbl_name = ?1 COLLATE NOCASE)`, t.name).
		Scan(&triggers); err != nil {
		return nil, err
	}
	if triggers > 0 {
		return nil, fault("%w: %s has triggers, whose writes the change log can't see", ErrNotAgentTable, t.name)
	}
	type col struct {
		name, typ string
		pk        int
	}
	cols, err := queryTx(tx, `SELECT name, type, pk FROM pragma_table_info(?) ORDER BY cid`, []any{t.name}, func(r *sql.Rows) (col, error) {
		var c col
		return c, r.Scan(&c.name, &c.typ, &c.pk)
	})
	if err != nil {
		return nil, err
	}
	var keyed []col
	for _, c := range cols {
		t.cols = append(t.cols, c.name)
		t.lower[strings.ToLower(c.name)] = c.name
		t.types[c.name] = c.typ
		if c.pk > 0 {
			keyed = append(keyed, c)
		}
	}
	if len(keyed) == 0 {
		return nil, fault("%w: %s has no explicit primary key", ErrNotAgentTable, t.name)
	}
	if len(cols) > maxColumns {
		return nil, fault("store: table %s has %d columns; the change log holds rows of at most %d", t.name, len(cols), maxColumns)
	}
	slices.SortFunc(keyed, func(a, b col) int { return a.pk - b.pk })
	for _, c := range keyed {
		t.pk = append(t.pk, c.name)
	}
	// An INTEGER PRIMARY KEY is the rowid unless the table has an index for
	// its key, as for INTEGER PRIMARY KEY DESC (SQLite quirk). Any other
	// key column may hold NULL, for old SQLite's sake, so keyOf never lets
	// it be left out.
	var pkIndex bool
	if err := tx.QueryRow(`SELECT count(*) > 0 FROM pragma_index_list(?) WHERE origin = 'pk'`, t.name).Scan(&pkIndex); err != nil {
		return nil, err
	}
	t.rowid = !wr && !pkIndex && len(keyed) == 1 && strings.EqualFold(keyed[0].typ, "INTEGER")
	refs, err := queryTx(tx, `SELECT column_name FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE AND kind = 'object_ref'`,
		[]any{t.name}, func(r *sql.Rows) (string, error) {
			var c string
			return c, r.Scan(&c)
		})
	for _, c := range refs {
		if n, ok := t.lower[strings.ToLower(c)]; ok { // an annotation of a dropped column is skipped
			t.refs = append(t.refs, n)
		}
	}
	return t, err
}

func (t *agentTable) target() string { return "table:" + t.name }

// column returns the schema's name for c.
func (t *agentTable) column(c string) (string, error) {
	if n, ok := t.lower[strings.ToLower(c)]; ok {
		return n, nil
	}
	return "", fault("store: table %s has no column %q", t.name, c)
}

func (t *agentTable) isKey(c string) bool { return slices.Contains(t.pk, c) }

// quote quotes a name for SQL; names come from the schema, never from the
// caller directly.
func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = quote(n)
	}
	return out
}

func literal(s string) string { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }

// qualified quotes names as columns of the table named as in a query;
// as "" leaves them bare.
func qualified(as string, names []string) []string {
	out := quoteAll(names)
	if as != "" {
		for i := range out {
			out[i] = as + "." + out[i]
		}
	}
	return out
}

// keyExpr is the primary key as a JSON array, the row_key (SPEC 2.5).
func (t *agentTable) keyExpr() string { return t.keyExprAs("") }

func (t *agentTable) keyExprAs(as string) string {
	return "json_array(" + strings.Join(qualified(as, t.pk), ", ") + ")"
}

// blobKeyAs is true for a row with a BLOB in its key. json_array reads a
// BLOB as JSONB and would give a wrong key, so such rows are refused.
func (t *agentTable) blobKeyAs(as string) string {
	parts := qualified(as, t.pk)
	for i, q := range parts {
		parts[i] = fmt.Sprintf("typeof(%s) = 'blob'", q)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// beforeExpr is the row as a JSON object. JSON can't hold a BLOB, so one
// is stored as {"$blob": "<hex>"}.
func (t *agentTable) beforeExpr() string { return t.beforeExprAs("") }

func (t *agentTable) beforeExprAs(as string) string {
	parts := make([]string, len(t.cols))
	for i, q := range qualified(as, t.cols) {
		parts[i] = fmt.Sprintf(`%s, CASE WHEN typeof(%s) = 'blob' THEN json_object('$blob', hex(%s)) ELSE %s END`, literal(t.cols[i]), q, q, q)
	}
	return "json_object(" + strings.Join(parts, ", ") + ")"
}

// keyMatch selects the rows whose key is in a bound JSON array of keys.
// Each key's values get the columns' affinity and collation, as in a plain
// comparison.
func (t *agentTable) keyMatch() string { return t.keyMatchAs("") }

func (t *agentTable) keyMatchAs(as string) string {
	vals := make([]string, len(t.pk))
	for i := range t.pk {
		vals[i] = fmt.Sprintf("value->>%d", i)
	}
	return fmt.Sprintf("(%s) IN (SELECT %s FROM json_each(?))", strings.Join(qualified(as, t.pk), ", "), strings.Join(vals, ", "))
}

// befores returns the current rows for keys (a JSON array of keys) by
// row_key.
func (t *agentTable) befores(tx *sql.Tx, keys string) (map[string]string, error) {
	out := map[string]string{}
	rows, err := tx.Query(fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s", t.keyExpr(), t.beforeExpr(), quote(t.name), t.keyMatch()), keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, b string
		if err := rows.Scan(&k, &b); err != nil {
			return nil, err
		}
		out[k] = b
	}
	return out, rows.Err()
}

// value checks a value for binding.
func value(v any) (any, error) {
	switch x := v.(type) {
	case string:
		// Row keys and before-images are JSON, which has no room for
		// bytes that are not UTF-8.
		if !utf8.ValidString(x) {
			return nil, fmt.Errorf("%q is not valid UTF-8 text; store bytes as []byte", x)
		}
		return x, nil
	case nil, bool, int64, float64, []byte:
		return x, nil
	case int:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint:
		if uint64(x) > math.MaxInt64 {
			return nil, fmt.Errorf("%d is too large", x)
		}
		return int64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("%d is too large", x)
		}
		return int64(x), nil
	case float32:
		return float64(x), nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		return x.Float64()
	}
	return nil, fmt.Errorf("a %T can't be stored in a column", v)
}

// keyOf returns a row's key values. With mayOmit, a row that leaves a
// rowid key out gets nil: SQLite picks its key.
func (t *agentTable) keyOf(named map[string]any, mayOmit bool) ([]any, error) {
	k := make([]any, len(t.pk))
	for i, c := range t.pk {
		v, ok := named[c]
		if !ok || v == nil {
			if mayOmit && t.rowid {
				return nil, nil
			}
			return nil, fmt.Errorf("key column %s is missing", c)
		}
		if _, ok := v.([]byte); ok {
			return nil, fmt.Errorf("key column %s can't be a blob", c)
		}
		k[i] = v
	}
	return k, nil
}

// named maps r's columns to the schema's names, with checked values.
func (t *agentTable) named(r Row) (map[string]any, error) {
	out := make(map[string]any, len(r))
	for c, v := range r {
		n, err := t.column(c)
		if err != nil {
			return nil, err
		}
		if _, dup := out[n]; dup {
			return nil, fmt.Errorf("column %s is given twice", n)
		}
		if out[n], err = value(v); err != nil {
			return nil, fmt.Errorf("column %s: %w", n, err)
		}
	}
	return out, nil
}

// checkRefs fails if a value of an object_ref column is not an existing
// object key (SPEC 4.5).
func (t *agentTable) checkRefs(tx *sql.Tx, rows []map[string]any) error {
	if len(t.refs) == 0 {
		return nil
	}
	var keys []string
	for _, r := range rows {
		for _, c := range t.refs {
			switch v := r[c].(type) {
			case nil:
			case string:
				keys = append(keys, v)
			default:
				return fault("store: column %s holds object keys, not a %T", c, v)
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	b, _ := json.Marshal(keys)
	var missing string
	err := tx.QueryRow(`SELECT value FROM json_each(?) WHERE value NOT IN (SELECT key FROM _jenab_objects) LIMIT 1`, string(b)).Scan(&missing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return invalid{fmt.Errorf("store: table %s: %w", t.name, &noObject{missing})}
}

func keysJSON(keys [][]any) (string, error) {
	b, err := json.Marshal(keys)
	return string(b), err
}

// RowError is a write that failed at one row of its request; Row counts
// the request's rows from 0.
type RowError struct {
	Row int
	Err error
}

func (e *RowError) Error() string { return fmt.Sprintf("store: row %d: %v", e.Row, e.Err) }
func (e *RowError) Unwrap() error { return e.Err }

// rowErr names the row of the request a check failed for.
func rowErr(i int, err error) error { return invalid{&RowError{Row: i, Err: err}} }

// affinity is the type a column declared decl has its affinity with,
// by SQLite's rules in their order (3.1); "" for none.
func affinity(decl string, strict bool) string {
	d := strings.ToUpper(decl)
	switch {
	case strict && d == "ANY":
		return ""
	case strings.Contains(d, "INT"):
		return "INTEGER"
	case strings.Contains(d, "CHAR"), strings.Contains(d, "CLOB"), strings.Contains(d, "TEXT"):
		return "TEXT"
	case d == "", strings.Contains(d, "BLOB"):
		return ""
	case strings.Contains(d, "REAL"), strings.Contains(d, "FLOA"), strings.Contains(d, "DOUB"):
		return "REAL"
	}
	return "NUMERIC"
}

// uniqueIndex is columns whose values no two rows of a table share: the
// primary key or a UNIQUE index, with the collation it compares each with.
type uniqueIndex struct {
	cols, colls []string
	pk          bool
}

// uniques returns t's primary key and its UNIQUE indexes on plain
// columns, declared or created. Partial indexes and indexes on
// expressions are left out: only the write itself checks them.
func (t *agentTable) uniques(tx *sql.Tx) ([]uniqueIndex, error) {
	type part struct {
		index, origin string
		partial       bool
		col           sql.NullString
		coll          string
	}
	parts, err := queryTx(tx, `SELECT l.name, l.origin, l.partial, x.name, x.coll FROM pragma_index_list(?1) l, pragma_index_xinfo(l.name) x
		WHERE l."unique" AND x.key = 1 ORDER BY l.name, x.seqno`, []any{t.name}, func(r *sql.Rows) (part, error) {
		var p part
		return p, r.Scan(&p.index, &p.origin, &p.partial, &p.col, &p.coll)
	})
	if err != nil {
		return nil, err
	}
	var out []uniqueIndex
	if t.rowid {
		out = append(out, uniqueIndex{cols: t.pk, colls: []string{"BINARY"}, pk: true})
	}
	plain := true
	for i, p := range parts {
		if i == 0 || p.index != parts[i-1].index {
			out = append(out, uniqueIndex{pk: p.origin == "pk"})
			plain = !p.partial
		}
		ix := &out[len(out)-1]
		if !p.col.Valid {
			plain = false // an expression
		}
		ix.cols = append(ix.cols, p.col.String)
		ix.colls = append(ix.colls, p.coll)
		if last := i == len(parts)-1 || parts[i+1].index != p.index; last && !plain {
			out = out[:len(out)-1]
		}
	}
	return out, nil
}

// clashes returns, in order, the rows of vals (each a row's values in ix's
// columns) whose values a row of t already has, or an earlier row of vals.
// Values are compared as ix compares them, with its collation and its
// columns' affinity, so 1 and '1' in an INTEGER column, or 'k' and 'K' in
// a NOCASE one, are one value.
func (t *agentTable) clashes(tx *sql.Tx, ix uniqueIndex, vals [][]any) ([]int, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	vj, err := keysJSON(vals)
	if err != nil {
		return nil, err
	}
	// A temporary table declared like the index holds the first row of
	// each value; agent tables can't have its name.
	n := len(ix.cols)
	cols, names, get, same := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	for i, c := range ix.cols {
		names[i] = fmt.Sprintf("k%d", i)
		cols[i] = fmt.Sprintf("%s %s COLLATE %s", names[i], affinity(t.types[c], t.strict), quote(ix.colls[i]))
		get[i] = fmt.Sprintf("value->>%d", i)
		same[i] = fmt.Sprintf("%s = _jenab_k.%s COLLATE %s", quote(c), names[i], quote(ix.colls[i]))
	}
	if _, err := tx.Exec(fmt.Sprintf("CREATE TEMP TABLE _jenab_keys (%s, i INTEGER, PRIMARY KEY (%s)) WITHOUT ROWID",
		strings.Join(cols, ", "), strings.Join(names, ", "))); err != nil {
		return nil, err
	}
	defer tx.Exec("DROP TABLE temp._jenab_keys")
	if _, err := tx.Exec(fmt.Sprintf("INSERT INTO temp._jenab_keys SELECT %s, key FROM json_each(?) WHERE true ON CONFLICT DO NOTHING",
		strings.Join(get, ", ")), vj); err != nil {
		return nil, err
	}
	return queryTx(tx, fmt.Sprintf(`SELECT i FROM temp._jenab_keys _jenab_k WHERE EXISTS (SELECT 1 FROM %s WHERE %s)
		UNION ALL
		SELECT key FROM json_each(?) WHERE key NOT IN (SELECT i FROM temp._jenab_keys) ORDER BY 1`,
		quote(t.name), strings.Join(same, " AND ")), []any{vj}, func(r *sql.Rows) (int, error) {
		var i int
		return i, r.Scan(&i)
	})
}

// precheck checks every row of a request of more than one chunk, inside
// the first chunk's transaction, so a bad row fails the request before
// anything is written. With ConflictFail, a row whose key or UNIQUE values
// a row of t already has, or a row before it in rows, fails it too; with
// ConflictSkip, the UNIQUE values of the rows that will be inserted. An
// upsert may change the values of rows that exist, so only its chunks see
// whether its UNIQUE values clash. A row that leaves out a column of an
// index, or holds NULL or a BLOB in one, is not checked against it.
func (t *agentTable) precheck(tx *sql.Tx, rows []Row, c Conflict) error {
	named := make([]map[string]any, len(rows))
	omitted, given := -1, -1
	for i, r := range rows {
		n, err := t.named(r)
		if err != nil {
			return rowErr(i, err)
		}
		if len(n) == 0 {
			return rowErr(i, errors.New("no columns"))
		}
		k, err := t.keyOf(n, true)
		if err != nil {
			return rowErr(i, err)
		}
		named[i] = n
		switch {
		case k == nil && omitted < 0:
			omitted = i
		case k != nil && given < 0:
			given = i
		}
	}
	if omitted >= 0 && given >= 0 {
		// SQLite picks a left-out key when its chunk is written, and may
		// pick one that a later chunk gives.
		return rowErr(max(omitted, given), fmt.Errorf("the rows of a request of more than %d must all give %s or all leave it out", chunkSize, t.pk[0]))
	}
	if err := t.checkRefs(tx, named); err != nil {
		return err
	}
	if c == ConflictUpsert {
		return nil
	}
	idx, err := t.uniques(tx)
	if err != nil {
		return err
	}
	slices.SortStableFunc(idx, func(a, b uniqueIndex) int { // the key first: it decides which rows ConflictSkip inserts
		if a.pk == b.pk {
			return 0
		}
		if a.pk {
			return -1
		}
		return 1
	})
	skipped := map[int]bool{}
	first, firstErr := len(rows), error(nil)
	for _, ix := range idx {
		var vals [][]any
		var at []int // the row of each of vals
	next:
		for i, n := range named {
			if skipped[i] {
				continue
			}
			v := make([]any, len(ix.cols))
			for j, col := range ix.cols {
				switch x := n[col].(type) {
				case nil, []byte:
					continue next
				default:
					v[j] = x
				}
			}
			vals = append(vals, v)
			at = append(at, i)
		}
		cl, err := t.clashes(tx, ix, vals)
		if err != nil {
			return err
		}
		switch {
		case len(cl) == 0:
		case ix.pk && c == ConflictSkip:
			for _, j := range cl {
				skipped[at[j]] = true
			}
		case at[cl[0]] < first:
			first, firstErr = at[cl[0]], ErrRowExists
			if !ix.pk {
				firstErr = fmt.Errorf("%w: the values of %s.%s must be unique", ErrNotUnique, t.name, strings.Join(ix.cols, ", "+t.name+"."))
			}
		}
	}
	if firstErr != nil {
		return rowErr(first, firstErr)
	}
	return nil
}

// ---- Insert ----

// Insert adds rows to table t, one request with one change-log entry, in
// chunks of 500 rows (SPEC 2.5, 7.4). c decides what happens to a row
// whose primary key is taken. Rows may differ in their columns. Column
// names are checked against the schema and quoted, values are bound.
//
// A request of more than one chunk checks every row first, inside the
// first chunk's transaction, and its keys and UNIQUE values as precheck
// says, so a bad row or a taken key or value fails the request before
// anything is written. What only the write itself checks, a partial UNIQUE
// index, a CHECK constraint, a STRICT column's type, a foreign key, or a
// row that another request writes between two chunks, still fails only
// its own chunk: the chunks before it stay, as after a crash (2.7), and
// the WriteResult returned with the error counts their rows, so a caller
// can report a partial write.
func (p *ProjectDB) Insert(ctx context.Context, pr limit.Priority, src id.Source, t string, rows []Row, c Conflict) (WriteResult, error) {
	op := "insert"
	if c == ConflictUpsert {
		op = "upsert"
	} else if c != ConflictFail && c != ConflictSkip {
		return WriteResult{}, fmt.Errorf("store: unknown conflict rule %d", c)
	}
	return p.chunks(ctx, pr, src, op, len(rows), func(tx *sql.Tx, from, to int, l *chunkLog) error {
		tab, err := loadTable(tx, t)
		if err != nil {
			return err
		}
		l.target = tab.target()
		if from == 0 && len(rows) > chunkSize {
			if err := tab.precheck(tx, rows, c); err != nil {
				return err
			}
		}
		named := make([]map[string]any, 0, to-from)
		var keys [][]any
		var keyed []map[string]any // the rows of keys
		for i := from; i < to; i++ {
			n, err := tab.named(rows[i])
			if err != nil {
				return rowErr(i, err)
			}
			if len(n) == 0 {
				return rowErr(i, errors.New("no columns"))
			}
			k, err := tab.keyOf(n, true)
			if err != nil {
				return rowErr(i, err)
			}
			if k != nil {
				keys = append(keys, k)
				keyed = append(keyed, n)
			}
			named = append(named, n)
		}
		if err := tab.checkRefs(tx, named); err != nil {
			return err
		}
		before := map[string]string{}
		var cas *cascade
		if c == ConflictUpsert && len(keys) > 0 {
			kj, err := keysJSON(keys)
			if err != nil {
				return err
			}
			if before, err = tab.befores(tx, kj); err != nil {
				return err
			}
			// The rows that exist get an update, which foreign keys may
			// pass on where it changes a value they reference.
			var set []string
			for _, col := range tab.cols {
				if !tab.isKey(col) && slices.ContainsFunc(named, func(n map[string]any) bool { _, ok := n[col]; return ok }) {
					set = append(set, col)
				}
			}
			if len(set) > 0 && len(before) > 0 {
				gs := make([]given, len(keys))
				for j, n := range keyed {
					gs[j] = given{Vals: make([]any, len(set)), Has: make([]bool, len(set))}
					if gs[j].Key, err = json.Marshal(keys[j]); err != nil {
						return err
					}
					for i, col := range set {
						gs[j].Vals[i], gs[j].Has[i] = n[col]
					}
				}
				roots, err := tab.changing(tx, set, gs)
				if err != nil {
					return err
				}
				if cas, err = reach(tx, tab, roots...); err != nil {
					return err
				}
			}
		}
		stmts := map[string]*sql.Stmt{}
		defer func() {
			for _, s := range stmts {
				s.Close()
			}
		}()
		for j, n := range named {
			cols := make([]string, 0, len(n))
			for _, col := range tab.cols {
				if _, ok := n[col]; ok {
					cols = append(cols, col)
				}
			}
			sig := strings.Join(cols, "\x00")
			st := stmts[sig]
			if st == nil {
				if st, err = tx.Prepare(tab.insertSQL(cols, c)); err != nil {
					return err
				}
				stmts[sig] = st
			}
			args := make([]any, len(cols))
			for i, col := range cols {
				args[i] = n[col]
			}
			var key string
			err := st.QueryRow(args...).Scan(&key)
			switch {
			case errors.Is(err, sql.ErrNoRows): // skipped, or an upsert of the key alone
				l.res.Skipped++
				continue
			case err != nil:
				return &RowError{Row: from + j, Err: checkErr(err)}
			}
			if b, ok := before[key]; ok {
				l.add(tab.name, key, rowUpdated, sql.NullString{String: b, Valid: true})
			} else {
				l.add(tab.name, key, rowInserted, sql.NullString{})
			}
		}
		return cas.log(tx, l)
	})
}

// insertSQL inserts cols and returns the row's key. An upsert sets the
// given columns that are not in the key, so the others stay as they are;
// there is no REPLACE (SPEC 6.5). OR ABORT overrides an ON CONFLICT clause
// of the schema: its REPLACE would delete rows the change log can't see,
// and its IGNORE would drop rows without a word.
func (t *agentTable) insertSQL(cols []string, c Conflict) string {
	q := fmt.Sprintf("INSERT OR ABORT INTO %s (%s) VALUES (%s)", quote(t.name), strings.Join(quoteAll(cols), ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "))
	var set []string
	for _, col := range cols {
		if !t.isKey(col) {
			set = append(set, quote(col)+" = excluded."+quote(col))
		}
	}
	switch {
	case c == ConflictSkip, c == ConflictUpsert && len(set) == 0:
		q += fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", strings.Join(quoteAll(t.pk), ", "))
	case c == ConflictUpsert:
		q += fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s", strings.Join(quoteAll(t.pk), ", "), strings.Join(set, ", "))
	}
	return q + " RETURNING " + t.keyExpr()
}

// ---- Update and Delete ----

// UpdateReq sets columns of the rows with the given primary keys.
type UpdateReq struct {
	Table string
	Set   Row   // columns to set; the key can't be set (SPEC 5.3)
	Keys  []Row // each one the full primary key of a row
}

// Update sets u.Set on the rows with u.Keys: one request with one
// change-log entry, in chunks of 500 keys. Keys with no row count as
// skipped. Rows that foreign keys change with them (ON UPDATE CASCADE, SET
// NULL, SET DEFAULT on a set column) are logged in the same entry, as are
// those of an upsert.
func (p *ProjectDB) Update(ctx context.Context, pr limit.Priority, src id.Source, u UpdateReq) (WriteResult, error) {
	if len(u.Set) == 0 {
		return WriteResult{}, fault("store: an update needs columns to set")
	}
	return p.chunks(ctx, pr, src, "update", len(u.Keys), func(tx *sql.Tx, from, to int, l *chunkLog) error {
		tab, err := loadTable(tx, u.Table)
		if err != nil {
			return err
		}
		l.target = tab.target()
		set, err := tab.named(u.Set)
		if err != nil {
			return fault("store: %w", err)
		}
		var cols, changed []string
		var args []any
		for _, c := range tab.cols {
			if v, ok := set[c]; ok {
				if tab.isKey(c) {
					return fault("store: column %s is part of the key and can't be set", c)
				}
				cols = append(cols, quote(c)+" = ?")
				changed = append(changed, c)
				args = append(args, v)
			}
		}
		if err := tab.checkRefs(tx, []map[string]any{set}); err != nil {
			return err
		}
		kj, n, err := tab.keyList(u.Keys[from:to], from)
		if err != nil {
			return err
		}
		before, err := tab.befores(tx, kj)
		if err != nil {
			return err
		}
		var ks []json.RawMessage
		if err := json.Unmarshal([]byte(kj), &ks); err != nil {
			return err
		}
		gs := make([]given, len(ks))
		vals := make([]any, len(changed))
		for i, col := range changed {
			vals[i] = set[col]
		}
		for i, k := range ks {
			gs[i] = given{Key: k, Vals: vals, Has: slices.Repeat([]bool{true}, len(changed))}
		}
		roots, err := tab.changing(tx, changed, gs)
		if err != nil {
			return err
		}
		cas, err := reach(tx, tab, roots...)
		if err != nil {
			return err
		}
		// OR ABORT, as in insertSQL.
		keys, err := queryTx(tx, fmt.Sprintf("UPDATE OR ABORT %s SET %s WHERE %s RETURNING %s", quote(tab.name), strings.Join(cols, ", "),
			tab.keyMatch(), tab.keyExpr()), append(args, kj), func(r *sql.Rows) (string, error) {
			var k string
			return k, r.Scan(&k)
		})
		if err != nil {
			return checkErr(err)
		}
		for _, key := range keys {
			b, ok := before[key]
			l.add(tab.name, key, rowUpdated, sql.NullString{String: b, Valid: ok})
		}
		l.res.Skipped = max(0, n-l.res.Updated)
		return cas.log(tx, l)
	})
}

// Delete removes the rows of table t with the given primary keys: one
// request with one change-log entry, in chunks of 500 keys. Keys with no
// row count as skipped. Rows that foreign keys delete or change with them
// (ON DELETE CASCADE, SET NULL, SET DEFAULT) are logged in the same entry.
func (p *ProjectDB) Delete(ctx context.Context, pr limit.Priority, src id.Source, t string, keys []Row) (WriteResult, error) {
	return p.chunks(ctx, pr, src, "delete", len(keys), func(tx *sql.Tx, from, to int, l *chunkLog) error {
		tab, err := loadTable(tx, t)
		if err != nil {
			return err
		}
		l.target = tab.target()
		kj, n, err := tab.keyList(keys[from:to], from)
		if err != nil {
			return err
		}
		cas, err := reach(tx, tab, root{kj, nil})
		if err != nil {
			return err
		}
		var before map[string]string
		if cas != nil {
			// An action may change a row of this table before its own
			// delete, so the before-images are taken first.
			if before, err = tab.befores(tx, kj); err != nil {
				return err
			}
		}
		gone, err := queryTx(tx, fmt.Sprintf("DELETE FROM %s WHERE %s RETURNING %s, %s", quote(tab.name), tab.keyMatch(),
			tab.keyExpr(), tab.beforeExpr()), []any{kj}, func(r *sql.Rows) ([2]string, error) {
			var kb [2]string
			return kb, r.Scan(&kb[0], &kb[1])
		})
		if err != nil {
			return checkErr(err)
		}
		for _, kb := range gone {
			if b, ok := before[kb[0]]; ok {
				kb[1] = b
			}
			l.add(tab.name, kb[0], rowDeleted, sql.NullString{String: kb[1], Valid: true})
		}
		// A row an action deleted before the statement reached it, in a
		// table that references itself, is still one the request deleted.
		for _, k := range slices.Sorted(maps.Keys(before)) {
			l.add(tab.name, k, rowDeleted, sql.NullString{String: before[k], Valid: true})
		}
		l.res.Skipped = max(0, n-l.res.Deleted)
		return cas.log(tx, l)
	})
}

// keyList checks that each of keys is exactly a primary key and returns
// them as a JSON array for keyMatch, with how many distinct keys it holds.
func (t *agentTable) keyList(keys []Row, first int) (string, int, error) {
	list := make([][]any, 0, len(keys))
	distinct := map[string]bool{}
	for i, k := range keys {
		n, err := t.named(k)
		if err != nil {
			return "", 0, rowErr(first+i, err)
		}
		if len(n) != len(t.pk) {
			return "", 0, rowErr(first+i, fmt.Errorf("a key of %s has the columns %s", t.name, strings.Join(t.pk, ", ")))
		}
		for c := range n {
			if !t.isKey(c) {
				return "", 0, rowErr(first+i, fmt.Errorf("%s is not a key column of %s", c, t.name))
			}
		}
		v, err := t.keyOf(n, false)
		if err != nil {
			return "", 0, rowErr(first+i, err)
		}
		b, _ := json.Marshal(v)
		distinct[string(b)] = true
		list = append(list, v)
	}
	kj, err := keysJSON(list)
	return kj, len(distinct), err
}
