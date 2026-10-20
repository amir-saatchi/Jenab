package store

import (
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
	"unicode/utf8"
)

// Agent tables as the data tools see them (SPEC 8.1): the list, a table's
// description, and the check of rows before an insert.

// Table describes an agent table: its columns, keys, indexes,
// annotations and rows.
type Table struct {
	Name    string
	Columns []Column
	Key     []string // the primary key's columns, in key order
	// AutoKey: the key is an INTEGER PRIMARY KEY, so SQLite picks it for
	// a row that leaves it out.
	AutoKey      bool
	Strict       bool
	WithoutRowid bool
	Indexes      []Index
	ForeignKeys  []ForeignKey
	Rows         int
}

// Column is one column of a Table.
type Column struct {
	Name    string
	Type    string // as declared; "" for none
	NotNull bool
	Default *string // the default's SQL text; nil for none
	// Generated: SQLite computes the value, so a write can't give it.
	Generated bool
	Kind      string // the annotation (_jenab_columns), such as object_ref; "" for none
}

// Index is an index of a Table. Columns are "" for an expression.
type Index struct {
	Name    string
	Columns []string
	Unique  bool
	Partial bool
}

// ForeignKey is a reference from columns of a Table to another table.
type ForeignKey struct {
	Columns  []string
	Table    string
	To       []string // empty for the other table's primary key
	OnDelete string
	OnUpdate string
}

// Column returns the column named c, compared as SQLite compares names.
func (t *Table) Column(c string) (*Column, bool) {
	for i := range t.Columns {
		if strings.EqualFold(t.Columns[i].Name, c) {
			return &t.Columns[i], true
		}
	}
	return nil, false
}

// Tables returns the names of the agent tables, sorted: the tables of the
// main schema whose names are not internal (SPEC 2.2).
func (p *ProjectDB) Tables(ctx context.Context) ([]string, error) {
	names, err := Query(ctx, p.DB, `SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'table'`, nil,
		func(r *sql.Rows) (string, error) {
			var n string
			return n, r.Scan(&n)
		})
	if err != nil {
		return nil, err
	}
	names = slices.DeleteFunc(names, internalName)
	slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return names, nil
}

// DescribeTable describes the agent table name, in one read. A table
// that isn't there, or is internal, is ErrNotFound.
func (p *ProjectDB) DescribeTable(ctx context.Context, name string) (Table, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) (Table, error) {
		var t Table
		if internalName(name) {
			return t, fmt.Errorf("store: table %s: %w", name, ErrNotFound)
		}
		err := tx.QueryRowContext(ctx, `SELECT name, wr, strict FROM pragma_table_list WHERE schema = 'main' AND type = 'table' AND name = ? COLLATE NOCASE`, name).
			Scan(&t.Name, &t.WithoutRowid, &t.Strict)
		if errors.Is(err, sql.ErrNoRows) {
			return t, fmt.Errorf("store: table %s: %w", name, ErrNotFound)
		}
		if err != nil {
			return t, err
		}
		type col struct {
			Column
			pk int
		}
		cols, err := queryTxContext(ctx, tx, `SELECT name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid`, []any{t.Name},
			func(r *sql.Rows) (col, error) {
				var c col
				var def sql.NullString
				var hidden int
				err := r.Scan(&c.Name, &c.Type, &c.NotNull, &def, &c.pk, &hidden)
				if def.Valid {
					c.Default = &def.String
				}
				c.Generated = hidden == 2 || hidden == 3
				return c, err
			})
		if err != nil {
			return t, err
		}
		var keyed []col
		for _, c := range cols {
			t.Columns = append(t.Columns, c.Column)
			if c.pk > 0 {
				keyed = append(keyed, c)
			}
		}
		slices.SortFunc(keyed, func(a, b col) int { return a.pk - b.pk })
		for _, c := range keyed {
			t.Key = append(t.Key, c.Name)
		}
		type part struct {
			index, origin string
			unique        bool
			partial       bool
			col           sql.NullString
		}
		parts, err := queryTxContext(ctx, tx, `SELECT l.name, l.origin, l."unique", l.partial, i.name FROM pragma_index_list(?1) l, pragma_index_info(l.name) i
			ORDER BY l.name, i.seqno`, []any{t.Name}, func(r *sql.Rows) (part, error) {
			var p part
			return p, r.Scan(&p.index, &p.origin, &p.unique, &p.partial, &p.col)
		})
		if err != nil {
			return t, err
		}
		pkIndex := false
		for i, p := range parts {
			if p.origin == "pk" {
				pkIndex = true
				continue // the primary key is listed as Key
			}
			if i == 0 || p.index != parts[i-1].index {
				t.Indexes = append(t.Indexes, Index{Name: p.index, Unique: p.unique, Partial: p.partial})
			}
			ix := &t.Indexes[len(t.Indexes)-1]
			ix.Columns = append(ix.Columns, p.col.String)
		}
		t.AutoKey = !t.WithoutRowid && !pkIndex && len(keyed) == 1 && strings.EqualFold(keyed[0].Type, "INTEGER")
		type ref struct {
			id                 int
			table, from        string
			to                 sql.NullString
			onUpdate, onDelete string
		}
		refs, err := queryTxContext(ctx, tx, `SELECT id, "table", "from", "to", on_update, on_delete FROM pragma_foreign_key_list(?) ORDER BY id, seq`,
			[]any{t.Name}, func(r *sql.Rows) (ref, error) {
				var f ref
				return f, r.Scan(&f.id, &f.table, &f.from, &f.to, &f.onUpdate, &f.onDelete)
			})
		if err != nil {
			return t, err
		}
		for i, f := range refs {
			if i == 0 || f.id != refs[i-1].id {
				t.ForeignKeys = append(t.ForeignKeys, ForeignKey{Table: f.table, OnDelete: f.onDelete, OnUpdate: f.onUpdate})
			}
			fk := &t.ForeignKeys[len(t.ForeignKeys)-1]
			fk.Columns = append(fk.Columns, f.from)
			if f.to.Valid {
				fk.To = append(fk.To, f.to.String)
			}
		}
		kinds, err := queryTxContext(ctx, tx, `SELECT column_name, kind FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE ORDER BY column_name`,
			[]any{t.Name}, func(r *sql.Rows) ([2]string, error) {
				var k [2]string
				return k, r.Scan(&k[0], &k[1])
			})
		if err != nil {
			return t, err
		}
		for _, k := range kinds {
			if c, ok := t.Column(k[0]); ok { // an annotation of a dropped column is skipped
				c.Kind = k[1]
			}
		}
		err = tx.QueryRowContext(ctx, fmt.Sprintf("SELECT count(*) FROM %s", quote(t.Name))).Scan(&t.Rows)
		return t, err
	})
}

// MaxValue is the largest text or blob an agent can write or read, as
// agent queries can't read a longer one (SPEC 2.2).
const MaxValue = 1 << 20

// FieldError is a value of a row that doesn't fit its table, found before
// a write. Row counts the rows from 0; Column is "" for an error of the
// whole row.
type FieldError struct {
	Row    int
	Column string
	Msg    string
}

// CheckRows checks rows for an insert into t before the write (SPEC 8.1):
// the columns are t's, each given once and none generated; the values fit
// the columns' types; NOT NULL and key columns have a value. It returns
// every error, in row order, and the rows with each value as its column
// takes it: a whole number given as text for an INTEGER column becomes a
// number, true for an INTEGER column becomes 1.
//
// Values are what JSON decodes to with UseNumber: nil, bool, string,
// json.Number, []any or map[string]any. Constraints that only the write
// checks, such as UNIQUE, CHECK and foreign keys, are left to it.
func (t *Table) CheckRows(rows []map[string]any) ([]Row, []FieldError) {
	var errs []FieldError
	out := make([]Row, len(rows))
	for i, r := range rows {
		bad := func(col, format string, a ...any) {
			errs = append(errs, FieldError{Row: i, Column: col, Msg: fmt.Sprintf(format, a...)})
		}
		if len(r) == 0 {
			bad("", "the row has no columns")
			continue
		}
		row := Row{}
		wrong := map[string]bool{} // given columns with a wrong value
		names := make([]string, 0, len(r))
		for n := range r {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			c, ok := t.Column(n)
			switch {
			case !ok:
				bad(n, "%s has no column %q", t.Name, n)
				continue
			case c.Generated:
				bad(c.Name, "the column is generated: SQLite computes its value")
				continue
			}
			if _, dup := row[c.Name]; dup {
				bad(c.Name, "the column is given twice")
				continue
			}
			v, msg := t.convert(c, r[n])
			if msg != "" {
				bad(c.Name, "%s", msg)
				wrong[c.Name] = true
				continue
			}
			row[c.Name] = v
		}
		for _, c := range t.Columns {
			v, given := row[c.Name]
			if c.Generated || wrong[c.Name] || given && v != nil {
				continue
			}
			key := slices.Contains(t.Key, c.Name)
			switch {
			case key && t.AutoKey:
				// SQLite picks the key.
			case key:
				bad(c.Name, "the column is part of the primary key, so it needs a value")
			case given && c.NotNull:
				bad(c.Name, "the column is NOT NULL, so it can't be null")
			case !given && c.NotNull && c.Default == nil:
				bad(c.Name, "the column is NOT NULL and has no default, so it needs a value")
			}
		}
		out[i] = row
	}
	return out, errs
}

// affinityOf is c's affinity in t (SQLite 3.1).
func (t *Table) affinityOf(c *Column) string { return affinity(c.Type, t.Strict) }

// convert checks v for column c and returns it as c takes it, or a
// message saying why it doesn't fit.
func (t *Table) convert(c *Column, v any) (any, string) {
	decl := strings.ToUpper(c.Type)
	aff := t.affinityOf(c)
	switch x := v.(type) {
	case nil:
		return nil, ""
	case []any, map[string]any:
		return nil, "a list or an object can't be stored in a column; give JSON as a string"
	case string:
		if len(x) > MaxValue {
			return nil, fmt.Sprintf("the text is %d bytes; at most %d are allowed", len(x), MaxValue)
		}
		if !utf8.ValidString(x) {
			return nil, "the text is not valid UTF-8"
		}
	}
	switch {
	case aff == "INTEGER":
		return wholeNumber(v)
	case aff == "REAL":
		return number(v, "a number")
	case aff == "NUMERIC" && (strings.Contains(decl, "DATE") || strings.Contains(decl, "TIME")):
		s, ok := v.(string)
		if !ok || !isDateTime(s) {
			return nil, fmt.Sprintf("the column is %s, so it takes a date or time as text, like 2026-10-18 or 2026-10-18 14:30:00, not %s", c.Type, show(v))
		}
		return s, ""
	case aff == "NUMERIC" && strings.Contains(decl, "BOOL"):
		switch x := v.(type) {
		case bool:
			return boolInt(x), ""
		case json.Number:
			if x == "0" || x == "1" {
				return wholeNumber(x)
			}
		}
		return nil, fmt.Sprintf("the column is %s, so it takes true, false, 1 or 0, not %s", c.Type, show(v))
	case aff == "NUMERIC":
		if b, ok := v.(bool); ok {
			return boolInt(b), ""
		}
		return number(v, "a number")
	case aff == "TEXT":
		switch x := v.(type) {
		case string:
			return x, ""
		case json.Number:
			return x.String(), ""
		}
		return nil, fmt.Sprintf("the column is %s, so it takes text, not %s", c.Type, show(v))
	}
	// BLOB, ANY or no type: any value as it is.
	switch x := v.(type) {
	case bool:
		return boolInt(x), ""
	case json.Number:
		return number(x, "a number")
	}
	return v, ""
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// show is a value as a message quotes it.
func show(v any) string {
	switch x := v.(type) {
	case string:
		if utf8.RuneCountInString(x) > 40 {
			x = string([]rune(x)[:40]) + "…"
		}
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	}
	return fmt.Sprint(v)
}

// wholeNumber is v as an int64: a JSON number or a text without a
// fraction, as SQLite's INTEGER affinity keeps it, or true or false.
func wholeNumber(v any) (any, string) {
	var s string
	switch x := v.(type) {
	case bool:
		return boolInt(x), ""
	case json.Number:
		s = x.String()
	case string:
		s = strings.TrimSpace(x)
	default:
		return nil, fmt.Sprintf("the column takes a whole number, not %s", show(v))
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, ""
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return int64(f), ""
	}
	return nil, fmt.Sprintf("the column takes a whole number, not %s", show(v))
}

// number is v as an int64 or float64: a JSON number or a text that is
// one.
func number(v any, want string) (any, string) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = strings.TrimSpace(x)
	default:
		return nil, fmt.Sprintf("the column takes %s, not %s", want, show(v))
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, ""
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return f, ""
	}
	return nil, fmt.Sprintf("the column takes %s, not %s", want, show(v))
}

// dateLayouts are the time values SQLite's date functions read (its
// "time values" 1 to 10), with T or a space between date and time.
var dateLayouts = []string{
	"2006-01-02", "2006-01-02 15:04", "2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04Z07:00", "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02T15:04Z07:00", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05.999999999Z07:00",
	"15:04", "15:04:05", "15:04:05.999999999",
}

// isDateTime reports whether s is a date, a time or both, as SQLite
// writes them.
func isDateTime(s string) bool {
	for _, l := range dateLayouts {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
}
