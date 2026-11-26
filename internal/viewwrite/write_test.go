package viewwrite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// TestMain lets the test binary be its own query worker, for the loads
// and options that take the views' read-only path.
func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(m.Run())
}

var tehran = time.FixedZone("Tehran", 3*3600+1800)

// now is 2026-11-20 22:00 UTC, already the 21st in Tehran, the project's
// zone: today() is the 21st, now() the 20th at 22:00 UTC.
func now() time.Time { return time.Date(2026, 11, 20, 22, 0, 0, 0, time.UTC).In(tehran) }

var schema = []string{
	`CREATE TABLE watch_keywords(id INTEGER PRIMARY KEY, keyword TEXT NOT NULL UNIQUE, weight INTEGER,
		score REAL, added TEXT, active INTEGER NOT NULL DEFAULT 1, kind TEXT, file TEXT, seen_at TEXT,
		CHECK (weight IS NULL OR weight >= 0))`,
	`INSERT INTO _jenab_columns VALUES ('watch_keywords', 'file', 'object_ref')`,
	`CREATE TABLE kinds(name TEXT PRIMARY KEY)`,
	`INSERT INTO kinds VALUES ('news'), ('price')`,
}

// project opens a new project with the schema and returns a writer on it.
func project(t *testing.T) *Writer {
	t.Helper()
	ctx := context.Background()
	p, err := store.OpenProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(ctx) })
	exec(t, p, schema...)
	return &Writer{DB: p, Project: "01JPROJECT", Now: now}
}

func exec(t *testing.T, p *store.ProjectDB, stmts ...string) {
	t.Helper()
	_, err := store.Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return struct{}{}, fmt.Errorf("%s: %w", s, err)
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func one[T any](t *testing.T, p *store.ProjectDB, q string, args ...any) T {
	t.Helper()
	rows, err := store.Query(context.Background(), p.DB, q, args, func(r *sql.Rows) (T, error) {
		var v T
		return v, r.Scan(&v)
	})
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if len(rows) != 1 {
		t.Fatalf("%s: %d rows", q, len(rows))
	}
	return rows[0]
}

func parse(t *testing.T, y string) *view.View {
	t.Helper()
	v, err := view.ParseView(view.Default, []byte(y))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, y)
	}
	return v
}

const addKeyword = `version: 1
id: add_keyword
title: Add a keyword
type: form
fields:
  - { field: keyword, label: Keyword, control: text, required: true }
  - { field: weight, control: number }
  - { field: score, control: number }
  - { field: added, control: date, default: today }
  - { field: active, control: checkbox, default: true }
  - { field: kind, control: select, options: { query: "SELECT name FROM kinds ORDER BY name" } }
  - { field: file, control: file, prefix: receipts/ }
submit: { action: insert, table: watch_keywords }
`

const editKeyword = `version: 1
id: edit_keyword
title: Edit a keyword
type: form
load: "SELECT id, keyword, weight, added, active, kind FROM watch_keywords WHERE id = :key"
fields:
  - { field: id, control: number }
  - { field: keyword, control: text, required: true }
  - { field: weight, control: number }
  - { field: added, control: date }
  - { field: active, control: checkbox }
  - { field: kind, control: select, options: [news, price] }
submit: { action: update, table: watch_keywords, key: id }
`

func num(f float64) *float64 { return &f }

// changes counts the change-log entries from src.
func changes(t *testing.T, w *Writer, src id.Source) int {
	return one[int](t, w.DB, "SELECT count(*) FROM _jenab_changes WHERE source = ?", string(src))
}

// An insert form: its options and defaults, a write with typed values and
// one change-log entry as form:<view_id>, type errors on their fields with
// nothing written, and a unique constraint on its field.
func TestInsert(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	v := parse(t, addKeyword)

	f, err := w.Open(ctx, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Fields) != 7 || f.Fields[0].Label != "Keyword" || f.Fields[1].Label != "weight" || f.Action != "insert" {
		t.Fatalf("form %+v", f)
	}
	if o := f.Fields[5].Options; len(o) != 2 || o[0].Text != "news" || o[1].Value != "price" {
		t.Errorf("options %+v", o)
	}
	if d := f.Values["added"].Date; d == nil || d.Format(time.DateOnly) != "2026-11-21" {
		t.Errorf("added %+v, want today in Tehran", f.Values["added"])
	}
	if !f.Values["active"].Checked || f.Fields[6].Prefix != "receipts/" {
		t.Errorf("active %+v, file %+v", f.Values["active"], f.Fields[6])
	}

	added := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	sub, err := w.Submit(ctx, v, nil, map[string]Value{
		"keyword": {Text: "bitcoin"}, "weight": {Text: "۱۲"}, "score": {Number: num(0.5)},
		"added": {Date: &added}, "active": {Checked: true}, "kind": {Text: "news"},
	})
	if err != nil || !sub.OK() || sub.Change == "" || sub.Table != "watch_keywords" || sub.Notice != "Added a row to watch_keywords." {
		t.Fatalf("submit %+v, %v", sub, err)
	}
	got := one[string](t, w.DB, `SELECT keyword || '|' || typeof(weight) || weight || '|' || score || '|' || added || '|' ||
		active || '|' || kind || '|' || typeof(file) FROM watch_keywords`)
	if got != "bitcoin|integer12|0.5|2026-11-21|1|news|null" {
		t.Errorf("row %s", got)
	}
	if n := changes(t, w, "form:add_keyword"); n != 1 {
		t.Errorf("%d change-log entries, want 1", n)
	}
	if src := one[string](t, w.DB, "SELECT source FROM _jenab_changes WHERE id = ?", string(sub.Change)); src != "form:add_keyword" {
		t.Errorf("source %s", src)
	}

	// Type errors, each on its field; nothing is written.
	sub, err = w.Submit(ctx, v, nil, map[string]Value{
		"keyword": {Text: "  "}, "weight": {Text: "1.5"}, "score": {Text: "abc"},
		"added": {Text: "20-11"}, "kind": {Text: "weather"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"keyword": "needs a value", "weight": "must be a whole number", "score": "is not a number",
		"added": "is not a date; write it as 2026-11-20", "kind": "is not one of the choices"}
	if fmt.Sprint(sub.Errors) != fmt.Sprint(want) || sub.Change != "" {
		t.Errorf("errors %v, want %v", sub.Errors, want)
	}
	if n := changes(t, w, "form:add_keyword"); n != 1 {
		t.Errorf("%d change-log entries after the type errors", n)
	}

	// The keyword is UNIQUE: the error is on its field.
	sub, err = w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "bitcoin"}})
	if err != nil || sub.Errors["keyword"] != "a row with this value already exists" || len(sub.Errors) != 1 || sub.Change != "" {
		t.Fatalf("duplicate %+v, %v", sub, err)
	}
	// The table's CHECK names weight.
	sub, err = w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "eth"}, "weight": {Int: new(int64(-1))}})
	if err != nil || !strings.HasPrefix(sub.Errors["weight"], "doesn't pass the table's check") {
		t.Fatalf("check %+v, %v", sub, err)
	}
	// An empty insert form: active's NOT NULL takes its default.
	sub, err = w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "eth"}})
	if err != nil || !sub.OK() {
		t.Fatalf("minimal %+v, %v", sub, err)
	}
	if a := one[int](t, w.DB, "SELECT active FROM watch_keywords WHERE keyword = 'eth'"); a != 0 {
		// The check box is there and unchecked: false, not the default.
		t.Errorf("active %d, want 0", a)
	}
}

// An update form: the load fills the fields, type errors stop it, a
// write is one entry its own undo reverts, a gone row is Missing, and a
// unique constraint lands on its field.
func TestUpdate(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	exec(t, w.DB, `INSERT INTO watch_keywords(id, keyword, weight, added, active, kind) VALUES
		(1, 'bitcoin', 3, '2026-11-01', 1, 'news'), (2, 'eth', NULL, NULL, 0, NULL)`)
	v := parse(t, editKeyword)

	f, err := w.Open(ctx, v, int64(1))
	if err != nil || f.Missing || f.Error != "" {
		t.Fatalf("open %+v, %v", f, err)
	}
	if f.Values["keyword"].Text != "bitcoin" || *f.Values["weight"].Int != 3 || f.Values["added"].Date.Format(time.DateOnly) != "2026-11-01" ||
		!f.Values["active"].Checked || f.Values["kind"].Text != "news" || f.Key != int64(1) {
		t.Errorf("values %+v", f.Values)
	}
	if f, err := w.Open(ctx, v, int64(99)); err != nil || !f.Missing {
		t.Errorf("gone row %+v, %v", f, err)
	}

	sub, err := w.Submit(ctx, v, int64(1), every(map[string]Value{"keyword": {Text: "bitcoin"}, "weight": {Text: "lots"}}))
	if err != nil || sub.Errors["weight"] != "is not a number" || sub.Change != "" {
		t.Fatalf("type error %+v, %v", sub, err)
	}

	sub, err = w.Submit(ctx, v, int64(1), every(map[string]Value{"id": {Text: "1"}, "keyword": {Text: "BTC"}, "weight": {Text: "7"}, "kind": {Text: "price"}}))
	if err != nil || !sub.OK() || sub.Change == "" || sub.Notice != "Saved the row in watch_keywords." {
		t.Fatalf("update %+v, %v", sub, err)
	}
	// Fields left empty are set to NULL; the unchecked box is false.
	if got := one[string](t, w.DB, "SELECT keyword || '|' || weight || '|' || ifnull(added, 'null') || '|' || active || '|' || kind FROM watch_keywords WHERE id = 1"); got != "BTC|7|null|0|price" {
		t.Errorf("row %s", got)
	}
	if n := changes(t, w, "form:edit_keyword"); n != 1 {
		t.Errorf("%d entries", n)
	}
	// Its own undo puts the row back.
	if _, err := w.DB.UndoChange(ctx, sub.Change, store.UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := one[string](t, w.DB, "SELECT keyword || '|' || weight || '|' || added || '|' || active || '|' || kind FROM watch_keywords WHERE id = 1"); got != "bitcoin|3|2026-11-01|1|news" {
		t.Errorf("after undo %s", got)
	}
	if _, err := w.DB.UndoChange(ctx, sub.Change, store.UndoOptions{}); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("undo twice: %v", err)
	}

	// The key can't be changed; the row with it is.
	sub, err = w.Submit(ctx, v, int64(1), every(map[string]Value{"id": {Text: "5"}, "keyword": {Text: "bitcoin"}}))
	if err != nil || sub.Errors["id"] != "is the row's key, which can't be changed" {
		t.Errorf("key change %+v, %v", sub, err)
	}
	// Another row's keyword: on the keyword field.
	sub, err = w.Submit(ctx, v, int64(2), every(map[string]Value{"keyword": {Text: "bitcoin"}}))
	if err != nil || sub.Errors["keyword"] != "a row with this value already exists" {
		t.Errorf("duplicate %+v, %v", sub, err)
	}
	// The unchecked box writes false into the NOT NULL column.
	sub, err = w.Submit(ctx, v, int64(2), every(map[string]Value{"keyword": {Text: "x"}, "kind": {Text: "news"}}))
	if err != nil || !sub.OK() {
		t.Errorf("unchecked box on NOT NULL %+v, %v", sub, err)
	}
	// The row is gone.
	exec(t, w.DB, "DELETE FROM watch_keywords WHERE id = 2")
	sub, err = w.Submit(ctx, v, int64(2), every(map[string]Value{"keyword": {Text: "y"}}))
	if err != nil || !sub.Missing || sub.Problem != "This row is no longer there." {
		t.Errorf("gone %+v, %v", sub, err)
	}
	// An update form needs its key.
	if _, err := w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "y"}}); !errors.Is(err, ErrRequest) {
		t.Errorf("no key: %v", err)
	}
	// An update lacking a declared field writes nothing: it would set
	// the column to NULL unseen.
	exec(t, w.DB, `INSERT INTO watch_keywords(id, keyword, added) VALUES (3, 'sol', '2026-11-02')`)
	sub, err = w.Submit(ctx, v, int64(3), map[string]Value{"id": {Text: "3"}, "keyword": {Text: "SOL"}, "weight": {}, "active": {}, "kind": {}})
	if err != nil || sub.Problem != "Not saved: missing value for added." || sub.Change != "" {
		t.Errorf("missing field %+v, %v", sub, err)
	}
	if got := one[string](t, w.DB, "SELECT keyword || '|' || added FROM watch_keywords WHERE id = 3"); got != "sol|2026-11-02" {
		t.Errorf("row after the refused update %s", got)
	}
}

// every gives each field of editKeyword a value, empty where m has none,
// as the page sends an update.
func every(m map[string]Value) map[string]Value {
	out := map[string]Value{"id": {}, "keyword": {}, "weight": {}, "added": {}, "active": {}, "kind": {}}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// issue makes w take keys as handed out by Upload for the file field of
// add_keyword.
func issue(w *Writer, keys ...string) {
	w.Issued = func(form, field, key string) bool {
		return form == "add_keyword" && field == "file" && slices.Contains(keys, key)
	}
}

// A file field takes the key of a file under its prefix; one that is gone
// by the submit is an error on the field.
func TestFileField(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	v := parse(t, addKeyword)
	if _, err := w.DB.PutObject(ctx, limit.Interactive, Source("add_keyword"), "receipts/a.pdf", strings.NewReader("%PDF-1.4"), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	issue(w, "receipts/a.pdf", "receipts/gone.pdf")
	sub, err := w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "a"}, "file": {Text: "receipts/a.pdf"}})
	if err != nil || !sub.OK() {
		t.Fatalf("submit %+v, %v", sub, err)
	}
	if f := one[string](t, w.DB, "SELECT file FROM watch_keywords WHERE keyword = 'a'"); f != "receipts/a.pdf" {
		t.Errorf("file %s", f)
	}
	sub, _ = w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "b"}, "file": {Text: "other/a.pdf"}})
	if sub.Errors["file"] != "is not a file uploaded to this field" {
		t.Errorf("other prefix %+v", sub)
	}
	sub, err = w.Submit(ctx, v, nil, map[string]Value{"keyword": {Text: "b"}, "file": {Text: "receipts/gone.pdf"}})
	if err != nil || sub.Errors["file"] != "the file is no longer there; upload it again" {
		t.Errorf("gone file %+v, %v", sub, err)
	}
}

const keywordTable = `version: 1
id: keyword_table
title: Keywords
type: table
query: "SELECT id, keyword FROM watch_keywords"
default_sort: { field: keyword, direction: asc }
rows_from: { table: watch_keywords, key: id }
columns:
  - { field: keyword }
row_actions:
  - { label: Edit, open_form: edit_keyword }
  - { label: Delete, delete: true, confirm: true, bulk: true }
  - { label: Seen, set: { active: 0, seen_at: "now()", added: "today()" }, bulk: true }
  - { label: Weigh, set: { weight: "heavy" } }
  - { label: Drop, delete: true }
`

// Each row action: open_form with the row's key, delete after its
// confirm, set with literals, now() and today(), bulk at 500 as one
// change-log entry and over 500 refused, each undone on its own.
func TestRowActions(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	exec(t, w.DB, `WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 600)
		INSERT INTO watch_keywords(id, keyword, active) SELECT i, 'k' || i, 1 FROM c`)
	v := parse(t, keywordTable)
	keys := func(from, to int) []any {
		var out []any
		for i := from; i <= to; i++ {
			out = append(out, int64(i))
		}
		return out
	}
	src := Source("keyword_table")

	a, err := w.RowAction(ctx, v, 0, keys(7, 7), false)
	if err != nil || a.OpenForm != "edit_keyword" || a.Key != int64(7) || a.Change != "" {
		t.Errorf("open_form %+v, %v", a, err)
	}

	// Delete asks first.
	a, err = w.RowAction(ctx, v, 1, keys(1, 3), false)
	if err != nil || a.Confirm != "Delete 3 rows?" || a.Change != "" || one[int](t, w.DB, "SELECT count(*) FROM watch_keywords") != 600 {
		t.Fatalf("unconfirmed %+v, %v", a, err)
	}
	a, err = w.RowAction(ctx, v, 1, keys(1, 3), true)
	if err != nil || a.Rows != 3 || a.Change == "" || a.Notice != "Deleted 3 rows from watch_keywords." || a.Table != "watch_keywords" {
		t.Fatalf("delete %+v, %v", a, err)
	}
	if n := one[int](t, w.DB, "SELECT count(*) FROM watch_keywords"); n != 597 {
		t.Errorf("%d rows after the delete", n)
	}
	if _, err := w.DB.UndoChange(ctx, a.Change, store.UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := one[int](t, w.DB, "SELECT count(*) FROM watch_keywords"); n != 600 {
		t.Errorf("%d rows after its undo", n)
	}
	// Without confirm, one row; a row that is gone.
	a, err = w.RowAction(ctx, v, 4, keys(9999, 9999), false)
	if err != nil || a.Problem != "This row is no longer there." || a.Change != "" {
		t.Errorf("gone row %+v, %v", a, err)
	}

	// Set, on one row: now() is RFC 3339 in UTC, as pipelines write it
	// (SPEC 7.2), today() the project's date.
	a, err = w.RowAction(ctx, v, 2, keys(10, 10), false)
	if err != nil || a.Rows != 1 || a.Notice != "Updated 1 row in watch_keywords." {
		t.Fatalf("set %+v, %v", a, err)
	}
	if got := one[string](t, w.DB, "SELECT active || '|' || seen_at || '|' || added FROM watch_keywords WHERE id = 10"); got != "0|2026-11-20T22:00:00Z|2026-11-21" {
		t.Errorf("set row %s", got)
	}
	// A literal that doesn't fit its column.
	a, err = w.RowAction(ctx, v, 3, keys(10, 10), false)
	if err != nil || a.Problem != "Not saved: weight must be a whole number." {
		t.Errorf("bad literal %+v, %v", a, err)
	}

	// Bulk: 500 rows, one request, one entry.
	before := changes(t, w, src)
	a, err = w.RowAction(ctx, v, 2, keys(1, 500), false)
	if err != nil || a.Rows != 500 || a.Notice != "Updated 500 rows in watch_keywords." {
		t.Fatalf("bulk %+v, %v", a, err)
	}
	if n := changes(t, w, src) - before; n != 1 {
		t.Errorf("bulk of 500: %d entries, want 1", n)
	}
	if n := one[int](t, w.DB, "SELECT count(*) FROM _jenab_change_rows WHERE change_id = ?", string(a.Change)); n != 500 {
		t.Errorf("bulk entry has %d rows", n)
	}
	if n := one[int](t, w.DB, "SELECT count(*) FROM watch_keywords WHERE active = 0"); n != 500 {
		t.Errorf("%d rows set", n)
	}
	if _, err := w.DB.UndoChange(ctx, a.Change, store.UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	// Row 10 was set before the bulk, so it keeps that.
	if n := one[int](t, w.DB, "SELECT count(*) FROM watch_keywords WHERE active = 0"); n != 1 {
		t.Errorf("%d rows set after the bulk's undo, want 1", n)
	}
	// 501 is refused, and nothing is written.
	before = changes(t, w, src)
	a, err = w.RowAction(ctx, v, 2, keys(1, 501), false)
	if err != nil || a.Problem != "At most 500 rows at once; 501 are chosen." || changes(t, w, src) != before {
		t.Errorf("501 %+v, %v", a, err)
	}
	a, err = w.RowAction(ctx, v, 1, keys(1, 501), true)
	if err != nil || a.Problem == "" || one[int](t, w.DB, "SELECT count(*) FROM watch_keywords") != 600 {
		t.Errorf("501 deletes %+v, %v", a, err)
	}

	// No row, several rows without bulk, an action that isn't there.
	if a, err := w.RowAction(ctx, v, 4, nil, false); err != nil || a.Problem != "Choose a row first." {
		t.Errorf("no row %+v, %v", a, err)
	}
	if _, err := w.RowAction(ctx, v, 4, keys(1, 2), false); !errors.Is(err, ErrRequest) {
		t.Errorf("two rows without bulk: %v", err)
	}
	if _, err := w.RowAction(ctx, v, 9, keys(1, 1), false); !errors.Is(err, ErrRequest) {
		t.Errorf("no such action: %v", err)
	}
}

// Undo turn reverts a turn's writes and leaves a form's alone; the
// form's own undo is by its change.
func TestUndoTurnLeavesForms(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	turn := id.SourceOf("message", id.New())
	if _, err := w.DB.Insert(ctx, limit.Interactive, turn, "watch_keywords", []store.Row{{"keyword": "agent"}}, store.ConflictFail); err != nil {
		t.Fatal(err)
	}
	sub, err := w.Submit(ctx, parse(t, addKeyword), nil, map[string]Value{"keyword": {Text: "mine"}})
	if err != nil || !sub.OK() {
		t.Fatalf("submit %+v, %v", sub, err)
	}
	if _, err := w.DB.Undo(ctx, []id.Source{turn}, store.UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := one[string](t, w.DB, "SELECT group_concat(keyword) FROM watch_keywords"); got != "mine" {
		t.Errorf("after Undo turn: %s", got)
	}
	st, err := w.DB.UndoState(ctx, []id.Source{turn})
	if err != nil {
		t.Fatal(err)
	}
	if st.Redo.Changes == 0 {
		t.Errorf("state %+v", st)
	}
}

type fakePipelines struct {
	running *RunningError
	calls   []string
	inputs  map[string]any
	src     id.Source
}

func (f *fakePipelines) Run(ctx context.Context, p id.Project, pipeline string, inputs map[string]any, src id.Source) (id.Run, error) {
	f.calls = append(f.calls, string(p)+"/"+pipeline)
	f.inputs, f.src = inputs, src
	if f.running != nil {
		return "", f.running
	}
	if pipeline == "missing" {
		return "", fmt.Errorf("no pipeline: %w", store.ErrNotFound)
	}
	return "01JRUN", nil
}

const runForm = `version: 1
id: fetch_form
title: Fetch
type: form
fields:
  - { field: coin, control: text, required: true }
  - { field: days, control: number }
  - { field: from, control: date }
submit: { action: run_pipeline, pipeline: fetch_prices }
actions:
  - { label: Refresh, run_pipeline: fetch_prices, confirm: true }
  - { label: Missing, run_pipeline: missing }
  - { label: Prices, open_page: prices }
`

// run_pipeline goes through the seam: a form's fields as typed inputs, a
// pipeline running already as "already running since 08:00", no pipeline
// service as "can't run yet".
func TestPipelines(t *testing.T) {
	ctx := context.Background()
	w := project(t)
	w.Now = func() time.Time { return time.Date(2026, 11, 21, 9, 30, 0, 0, tehran) }
	v := parse(t, runForm)
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	sub, err := w.Submit(ctx, v, nil, map[string]Value{"coin": {Text: "BTC"}, "days": {Text: "30"}, "from": {Date: &from}})
	if err != nil || sub.Problem != "Pipelines can't run yet." || sub.Run != "" {
		t.Errorf("no pipelines %+v, %v", sub, err)
	}

	fake := &fakePipelines{}
	w.Pipelines = fake
	sub, err = w.Submit(ctx, v, nil, map[string]Value{"coin": {Text: "BTC"}, "days": {Text: "30"}, "from": {Date: &from}})
	if err != nil || sub.Run != "01JRUN" || sub.Notice != "Started fetch_prices." {
		t.Fatalf("run %+v, %v", sub, err)
	}
	if fmt.Sprint(fake.calls) != "[01JPROJECT/fetch_prices]" || fake.src != "form:fetch_form" ||
		fake.inputs["coin"] != "BTC" || fake.inputs["days"] != int64(30) || fake.inputs["from"] != "2026-11-01" {
		t.Errorf("calls %v, src %s, inputs %#v", fake.calls, fake.src, fake.inputs)
	}
	// A field error starts nothing.
	sub, _ = w.Submit(ctx, v, nil, map[string]Value{"days": {Text: "30"}})
	if sub.Errors["coin"] != "needs a value" || len(fake.calls) != 1 {
		t.Errorf("field error %+v, %d calls", sub, len(fake.calls))
	}

	fake.running = &RunningError{Pipeline: "fetch_prices", Run: "01JOLD", Since: time.Date(2026, 11, 21, 4, 30, 0, 0, time.UTC)}
	sub, err = w.Submit(ctx, v, nil, map[string]Value{"coin": {Text: "BTC"}})
	if err != nil || sub.Problem != "already running since 08:00" || sub.Run != "" {
		t.Errorf("running %+v, %v", sub, err)
	}
	other := *fake.running
	other.Since = time.Date(2026, 11, 19, 4, 30, 0, 0, time.UTC)
	if m := other.Message(w.now()); m != "already running since Nov 19, 08:00" {
		t.Errorf("another day: %s", m)
	}

	// The view's actions: confirm first, then the same answer.
	a, err := w.Action(ctx, v, 0, false)
	if err != nil || a.Confirm != "Run fetch_prices?" {
		t.Errorf("unconfirmed %+v, %v", a, err)
	}
	a, err = w.Action(ctx, v, 0, true)
	if err != nil || a.Problem != "already running since 08:00" {
		t.Errorf("action running %+v, %v", a, err)
	}
	fake.running = nil
	if a, err := w.Action(ctx, v, 0, true); err != nil || a.Run != "01JRUN" || fake.inputs != nil {
		t.Errorf("action %+v, %v, inputs %v", a, err, fake.inputs)
	}
	if a, err := w.Action(ctx, v, 1, false); err != nil || a.Problem != "There is no pipeline missing." {
		t.Errorf("missing %+v, %v", a, err)
	}
	if a, err := w.Action(ctx, v, 2, false); err != nil || a.OpenPage != "prices" {
		t.Errorf("open_page %+v, %v", a, err)
	}
}

// Values read from text as typed: Persian digits and separators, and
// what doesn't fit a column.
func TestFit(t *testing.T) {
	for _, c := range []struct {
		v      any
		decl   string
		want   any
		reason string
	}{
		{"۱۲٫۵", "REAL", 12.5, ""},
		{"−۳", "INTEGER", int64(-3), ""},
		{"۱٬۲۰۰", "INTEGER", int64(1200), ""},
		{"0x10", "INTEGER", nil, "must be a whole number"},
		{"abc", "NUMERIC", nil, "must be a number"},
		{"2026-11-20", "DATE", "2026-11-20", ""},
		{dateText("2026-11-20"), "DATETIME", "2026-11-20", ""},
		{dateText("2026-11-20"), "INTEGER", nil, "is a date, but the column takes whole numbers"},
		{true, "INTEGER", int64(1), ""},
		{true, "TEXT", nil, "is true or false, but the column takes text"},
		{int64(5), "TEXT", "5", ""},
		{2.0, "INTEGER", int64(2), ""},
		{2.5, "INTEGER", nil, "must be a whole number"},
		{"x", "BLOB", "x", ""},
	} {
		got, reason := fit(c.v, c.decl, false)
		if got != c.want || reason != c.reason {
			t.Errorf("fit(%#v, %s) = %#v, %q; want %#v, %q", c.v, c.decl, got, reason, c.want, c.reason)
		}
	}
	if _, reason := fit("x", "BLOB", true); reason == "" {
		t.Error("a STRICT BLOB column took text")
	}
	if v, reason := fit("x", "ANY", true); v != "x" || reason != "" {
		t.Errorf("a STRICT ANY column: %#v, %q", v, reason)
	}
}
