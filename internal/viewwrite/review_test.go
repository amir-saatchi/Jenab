package viewwrite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// now() is RFC 3339 in UTC, as pipelines write it (SPEC 7.2), whatever
// the project's zone.
func TestNowFormat(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	w.Now = func() time.Time { return time.Date(2026, 11, 26, 13, 30, 0, 0, tehran) }
	exec(t, w.DB, `INSERT INTO watch_keywords(id, keyword) VALUES (1, 'btc')`)
	if a, err := w.RowAction(ctx, parse(t, keywordTable), 2, []any{int64(1)}, false); err != nil || a.Rows != 1 {
		t.Fatalf("set %+v, %v", a, err)
	}
	got := one[string](t, w.DB, "SELECT seen_at FROM watch_keywords WHERE id = 1")
	if got != "2026-11-26T10:00:00Z" {
		t.Errorf("now() wrote %q, want 2026-11-26T10:00:00Z", got)
	}
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Error(err)
	}
}

const ownerTable = `version: 1
id: owner_table
title: Owners
type: table
query: "SELECT id, name FROM owners"
default_sort: { field: name, direction: asc }
rows_from: { table: owners, key: id }
columns:
  - { field: name }
row_actions:
  - { label: Delete, delete: true }
`

const dealTable = `version: 1
id: deal_table
title: Deals
type: table
query: "SELECT id, title FROM deals"
default_sort: { field: title, direction: asc }
rows_from: { table: deals, key: id }
columns:
  - { field: title }
row_actions:
  - { label: Move, set: { owner: 9 } }
`

// A delete a child row stops says other rows refer to the row, naming
// their table; a set that breaks a reference names the set column.
func TestForeignKeyMessages(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	exec(t, w.DB,
		`CREATE TABLE owners(id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE deals(id INTEGER PRIMARY KEY, owner INTEGER REFERENCES owners(id), title TEXT)`,
		`CREATE TABLE notes(id INTEGER PRIMARY KEY, owner INTEGER REFERENCES owners(id) ON DELETE CASCADE)`,
		`INSERT INTO owners VALUES (1, 'a'), (2, 'b')`,
		`INSERT INTO deals VALUES (1, 1, 'x')`,
		`INSERT INTO notes VALUES (1, 2)`)
	owners := parse(t, ownerTable)
	a, err := w.RowAction(ctx, owners, 0, []any{int64(1)}, false)
	if err != nil || a.Problem != "Not deleted: rows of deals still refer to it." || a.Change != "" {
		t.Errorf("delete with a child %+v, %v", a, err)
	}
	// The notes' reference deletes with it, so it stops nothing.
	if a, err := w.RowAction(ctx, owners, 0, []any{int64(2)}, false); err != nil || a.Rows != 1 {
		t.Errorf("delete with a cascading child %+v, %v", a, err)
	}
	a, err = w.RowAction(ctx, parse(t, dealTable), 0, []any{int64(1)}, false)
	if err != nil || a.Problem != "Not saved: owner refers to a row that isn't there." {
		t.Errorf("set %+v, %v", a, err)
	}
	if got := one[int](t, w.DB, "SELECT owner FROM deals WHERE id = 1"); got != 1 {
		t.Errorf("owner %d", got)
	}
}

// A key is read for its column's type: past 2^53 a float may be another
// row's key rounded, so it is refused; the int64 or its digits find the
// row.
func TestBigKeys(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	const big = int64(1<<53 + 1)
	exec(t, w.DB, `INSERT INTO watch_keywords(id, keyword) VALUES (9007199254740992, 'two53'), (9007199254740993, 'two53+1'), (7, 'seven')`)
	v := parse(t, keywordTable)
	for _, k := range []any{float64(big), float64(1 << 53), 7.5, "seven", true} {
		if _, err := w.RowAction(ctx, v, 4, []any{k}, false); !errors.Is(err, ErrRequest) {
			t.Errorf("key %#v: %v", k, err)
		}
	}
	if n := one[int](t, w.DB, "SELECT count(*) FROM watch_keywords"); n != 3 {
		t.Fatalf("%d rows after the refused keys", n)
	}
	if a, err := w.RowAction(ctx, v, 4, []any{big}, false); err != nil || a.Rows != 1 {
		t.Fatalf("int64 key %+v, %v", a, err)
	}
	if got := one[string](t, w.DB, "SELECT group_concat(keyword) FROM (SELECT keyword FROM watch_keywords ORDER BY id)"); got != "seven,two53" {
		t.Errorf("left %s", got)
	}
	if a, err := w.RowAction(ctx, v, 4, []any{"9007199254740992"}, false); err != nil || a.Rows != 1 {
		t.Errorf("digits %+v, %v", a, err)
	}
	if a, err := w.RowAction(ctx, v, 4, []any{7.0}, false); err != nil || a.Rows != 1 {
		t.Errorf("a whole float below 2^53 %+v, %v", a, err)
	}
	// An update form's key is read the same way.
	edit := parse(t, editKeyword)
	if _, err := w.Open(ctx, edit, float64(big)); !errors.Is(err, ErrRequest) {
		t.Errorf("open: %v", err)
	}
	if _, err := w.Submit(ctx, edit, float64(big), every(map[string]Value{"keyword": {Text: "x"}})); !errors.Is(err, ErrRequest) {
		t.Errorf("submit: %v", err)
	}
}

// A date is the day the user picked in its own zone, never moved through
// UTC: local midnight in +03:30 is still that day.
func TestDateZone(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	midnight := time.Date(2026, 11, 21, 0, 0, 0, 0, tehran) // 2026-11-20 20:30 UTC
	sub, err := w.Submit(ctx, parse(t, addKeyword), nil, map[string]Value{"keyword": {Text: "a"}, "added": {Date: &midnight}})
	if err != nil || !sub.OK() {
		t.Fatalf("submit %+v, %v", sub, err)
	}
	if got := one[string](t, w.DB, "SELECT added FROM watch_keywords"); got != "2026-11-21" {
		t.Errorf("added %s, want 2026-11-21", got)
	}
}

const addRange = `version: 1
id: add_range
title: Add a range
type: form
fields:
  - { field: lo, control: number }
  - { field: hi, control: number }
  - { field: n, control: number }
submit: { action: insert, table: ranges }
`

// A CHECK goes on a field only when its text names exactly one of the
// form's columns; one on two columns, or reported by its name, is the
// form's problem.
func TestCheckPlacement(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	exec(t, w.DB, `CREATE TABLE ranges(id INTEGER PRIMARY KEY, lo INTEGER, hi INTEGER CHECK (hi < 100), n INTEGER,
		CHECK (lo <= hi), CONSTRAINT positive CHECK (n IS NULL OR n > 0))`)
	v := parse(t, addRange)
	i := func(n int64) Value { return Value{Int: &n} }
	for _, c := range []struct {
		name          string
		values        map[string]Value
		field, errMsg string
		problem       string
	}{
		{"one column", map[string]Value{"lo": i(1), "hi": i(200)}, "hi", "doesn't pass the table's check: hi < 100", ""},
		{"two columns", map[string]Value{"lo": i(5), "hi": i(1)}, "", "", "Not saved: the row doesn't pass the table's check lo <= hi."},
		{"by its name", map[string]Value{"lo": i(1), "hi": i(2), "n": i(-1)}, "", "", "Not saved: the row doesn't pass the table's check positive."},
	} {
		sub, err := w.Submit(ctx, v, nil, c.values)
		if err != nil || sub.Change != "" || sub.Problem != c.problem || (c.field != "" && sub.Errors[c.field] != c.errMsg) || (c.field == "" && len(sub.Errors) != 0) {
			t.Errorf("%s: %+v, %v", c.name, sub, err)
		}
	}
}

// A key chosen twice is one row: the confirm counts rows, and 500 rows
// with repeats fit in one bulk.
func TestDuplicateKeys(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	exec(t, w.DB, `WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 600)
		INSERT INTO watch_keywords(id, keyword) SELECT i, 'k' || i FROM c`)
	v := parse(t, keywordTable)
	keys := []any{int64(1), int64(1), 2.0, "2", []byte("1")}
	a, err := w.RowAction(ctx, v, 1, keys, false)
	if err != nil || a.Confirm != "Delete 2 rows?" {
		t.Fatalf("confirm %+v, %v", a, err)
	}
	a, err = w.RowAction(ctx, v, 1, keys, true)
	if err != nil || a.Rows != 2 || one[int](t, w.DB, "SELECT count(*) FROM watch_keywords") != 598 {
		t.Fatalf("delete %+v, %v", a, err)
	}
	var many []any
	for i := 101; i <= 600; i++ {
		many = append(many, int64(i), int64(i))
	}
	a, err = w.RowAction(ctx, v, 2, many, false)
	if err != nil || a.Rows != 500 || a.Problem != "" {
		t.Errorf("500 rows chosen twice %+v, %v", a, err)
	}
	a, err = w.RowAction(ctx, v, 2, append(many, int64(3)), false)
	if err != nil || a.Problem != "At most 500 rows at once; 501 are chosen." {
		t.Errorf("501 rows %+v, %v", a, err)
	}
}

const editFile = `version: 1
id: edit_file
title: Edit a file
type: form
load: "SELECT id, keyword, file FROM watch_keywords WHERE id = :key"
fields:
  - { field: id, control: number }
  - { field: keyword, control: text }
  - { field: file, control: file, prefix: receipts/ }
submit: { action: update, table: watch_keywords, key: id }
`

// A file field takes only a key Upload handed out for that form and field,
// or on an update the row's own; any other file, even under the prefix,
// is refused on the field.
func TestFileKeys(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	for _, k := range []string{"receipts/mine.pdf", "receipts/theirs.pdf", "receipts/new.pdf"} {
		if _, err := w.DB.PutObject(ctx, limit.Interactive, Source("x"), k, strings.NewReader("%PDF-1.4"), store.PutOptions{Source: "upload"}); err != nil {
			t.Fatal(err)
		}
	}
	exec(t, w.DB, `INSERT INTO watch_keywords(id, keyword, file) VALUES (1, 'a', 'receipts/mine.pdf'), (2, 'b', 'receipts/theirs.pdf')`)
	w.Issued = func(form, field, key string) bool {
		return form == "edit_file" && field == "file" && key == "receipts/new.pdf"
	}

	// Insert: not handed out, so refused, though it is there.
	sub, err := w.Submit(ctx, parse(t, addKeyword), nil, map[string]Value{"keyword": {Text: "c"}, "file": {Text: "receipts/theirs.pdf"}})
	if err != nil || sub.Errors["file"] != "is not a file uploaded to this field" || sub.Change != "" {
		t.Errorf("insert %+v, %v", sub, err)
	}
	// Handed out for another form: refused too.
	sub, _ = w.Submit(ctx, parse(t, addKeyword), nil, map[string]Value{"keyword": {Text: "c"}, "file": {Text: "receipts/new.pdf"}})
	if sub.Errors["file"] != "is not a file uploaded to this field" {
		t.Errorf("another form's upload %+v", sub)
	}

	v := parse(t, editFile)
	// The row's own file stays.
	sub, err = w.Submit(ctx, v, int64(1), map[string]Value{"id": {Text: "1"}, "keyword": {Text: "A"}, "file": {Text: "receipts/mine.pdf"}})
	if err != nil || !sub.OK() {
		t.Errorf("kept file %+v, %v", sub, err)
	}
	// Another row's file isn't this row's.
	sub, err = w.Submit(ctx, v, int64(1), map[string]Value{"id": {Text: "1"}, "keyword": {Text: "A"}, "file": {Text: "receipts/theirs.pdf"}})
	if err != nil || sub.Errors["file"] != "is not a file uploaded to this field" || sub.Change != "" {
		t.Errorf("another row's file %+v, %v", sub, err)
	}
	// A new upload for this field.
	sub, err = w.Submit(ctx, v, int64(1), map[string]Value{"id": {Text: "1"}, "keyword": {Text: "A"}, "file": {Text: "receipts/new.pdf"}})
	if err != nil || !sub.OK() {
		t.Errorf("new file %+v, %v", sub, err)
	}
	if got := one[string](t, w.DB, "SELECT file FROM watch_keywords WHERE id = 1"); got != "receipts/new.pdf" {
		t.Errorf("file %s", got)
	}
	// A kept key on a row that is gone: Missing.
	sub, err = w.Submit(ctx, v, int64(9), map[string]Value{"id": {Text: "9"}, "keyword": {Text: "A"}, "file": {Text: "receipts/mine.pdf"}})
	if err != nil || !sub.Missing {
		t.Errorf("gone row %+v, %v", sub, err)
	}
}
