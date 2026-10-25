package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

type changeRow struct {
	ID, At, Source, Target, Op string
	Before, After              sql.NullString
}

type entryRow struct {
	Table, Key, Kind string
	Before           sql.NullString
}

func changeList(t *testing.T, p *ProjectDB) []changeRow {
	t.Helper()
	cs, err := Query(context.Background(), p.DB, `SELECT id, at, source, target, op, before, after FROM _jenab_changes ORDER BY id`, nil,
		func(r *sql.Rows) (changeRow, error) {
			var c changeRow
			return c, r.Scan(&c.ID, &c.At, &c.Source, &c.Target, &c.Op, &c.Before, &c.After)
		})
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func entriesOf(t *testing.T, p *ProjectDB, cid id.Change) []entryRow {
	t.Helper()
	es, err := Query(context.Background(), p.DB, `SELECT table_name, row_key, kind, before FROM _jenab_change_rows WHERE change_id = ? ORDER BY rowid`,
		[]any{string(cid)}, func(r *sql.Rows) (entryRow, error) {
			var e entryRow
			return e, r.Scan(&e.Table, &e.Key, &e.Kind, &e.Before)
		})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// lastChange is the newest entry, which must be c.
func lastChange(t *testing.T, p *ProjectDB, c id.Change) changeRow {
	t.Helper()
	cs := changeList(t, p)
	if len(cs) == 0 || cs[len(cs)-1].ID != string(c) {
		t.Fatalf("the newest change is not %q: %v", c, cs)
	}
	return cs[len(cs)-1]
}

func jsonEq(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("%s: %v", got, err)
	}
	json.Unmarshal([]byte(want), &w)
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("got %s, want %s", got, want)
	}
}

var msgSrc = id.SourceOf("message", "01JTEST")

func pricesProject(t *testing.T) *ProjectDB {
	t.Helper()
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE prices (coin TEXT, date TEXT, price REAL, note TEXT, PRIMARY KEY (coin, date))`)
	return p
}

func TestChangeLogFormat(t *testing.T) {
	p, _ := openTestProject(t)
	for _, n := range []string{"_jenab_changes", "_jenab_change_rows", "_jenab_columns",
		"_jenab_change_rows_key", "_jenab_change_rows_change", "_jenab_changes_source"} {
		if one[int](t, p.DB, "SELECT count(*) FROM sqlite_schema WHERE name = ?", n) != 1 {
			t.Errorf("%s is missing", n)
		}
	}
	// The key index is on (table_name, row_key, change_id), in that order.
	cols, err := Query(context.Background(), p.DB, `SELECT name FROM pragma_index_info('_jenab_change_rows_key') ORDER BY seqno`, nil,
		func(r *sql.Rows) (string, error) {
			var s string
			return s, r.Scan(&s)
		})
	if err != nil || strings.Join(cols, ",") != "table_name,row_key,change_id" {
		t.Errorf("key index columns %v, %v", cols, err)
	}

	// A project at format 2 gets the log at its next open, after a backup.
	dir := t.TempDir()
	old := Format{Name: "project", Steps: ProjectFormat.Steps[:2]}
	db := openTest(t, filepath.Join(dir, "project.db"), old)
	db.Close(context.Background())
	p2, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close(context.Background())
	if v := one[int](t, p2.DB, "PRAGMA user_version"); v != ProjectFormat.current() {
		t.Errorf("version %d after the update", v)
	}
	if one[int](t, p2.DB, "SELECT count(*) FROM _jenab_changes") != 0 {
		t.Error("the new log is not empty")
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "project-pre-update-*-v2.db")); len(m) != 1 {
		t.Errorf("backups: %v", m)
	}
}

func TestInsertConflictRules(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	r, err := p.Insert(ctx, limit.Interactive, msgSrc, "prices", []Row{
		{"coin": "BTC", "date": "2026-09-27", "price": 100.0, "note": "first"},
		{"coin": "BTC", "date": "2026-09-28", "price": 101},
		{"coin": "ETH", "date": "2026-09-27", "price": json.Number("5.5")},
	}, ConflictFail)
	if err != nil || r.Inserted != 3 || r.Change == "" {
		t.Fatalf("insert: %+v, %v", r, err)
	}
	c := lastChange(t, p, r.Change)
	if c.Source != string(msgSrc) || c.Target != "table:prices" || c.Op != "insert" || c.Before.Valid || c.After.Valid {
		t.Errorf("entry %+v", c)
	}
	if _, err := parseTime(c.At); err != nil {
		t.Errorf("at %q: %v", c.At, err)
	}
	es := entriesOf(t, p, r.Change)
	if len(es) != 3 || es[0].Key != `["BTC","2026-09-27"]` || es[0].Kind != "i" || es[0].Before.Valid || es[0].Table != "prices" {
		t.Errorf("row entries %+v", es)
	}

	// fail: a taken key fails the request and logs nothing.
	n := len(changeList(t, p))
	_, err = p.Insert(ctx, limit.Interactive, msgSrc, "prices", []Row{{"coin": "BTC", "date": "2026-09-27", "price": 1}}, ConflictFail)
	if !errors.Is(err, ErrRowExists) || !strings.Contains(err.Error(), "row 0") {
		t.Errorf("duplicate insert: %v", err)
	}
	if len(changeList(t, p)) != n {
		t.Error("a failed insert left an entry")
	}

	// skip: the taken key stays as it is.
	r, err = p.Insert(ctx, limit.Interactive, msgSrc, "prices", []Row{
		{"coin": "BTC", "date": "2026-09-27", "price": 1},
		{"coin": "BTC", "date": "2026-09-29", "price": 102},
	}, ConflictSkip)
	if err != nil || r.Inserted != 1 || r.Skipped != 1 {
		t.Fatalf("skip: %+v, %v", r, err)
	}
	if es := entriesOf(t, p, r.Change); len(es) != 1 || es[0].Key != `["BTC","2026-09-29"]` {
		t.Errorf("skip entries %+v", es)
	}
	if v := one[float64](t, p.DB, `SELECT price FROM prices WHERE coin = 'BTC' AND date = '2026-09-27'`); v != 100 {
		t.Errorf("skip changed the price to %v", v)
	}

	// upsert: given columns change, others stay; the before-image is the old row.
	r, err = p.Insert(ctx, limit.Interactive, msgSrc, "prices", []Row{
		{"coin": "BTC", "date": "2026-09-27", "price": 200},
		{"coin": "BTC", "date": "2026-09-27", "price": 300}, // the same row again
		{"coin": "SOL", "date": "2026-09-27", "price": 7},
		{"coin": "ETH", "date": "2026-09-27"}, // the key alone: unchanged
	}, ConflictUpsert)
	if err != nil || r.Inserted != 1 || r.Updated != 1 || r.Skipped != 1 {
		t.Fatalf("upsert: %+v, %v", r, err)
	}
	if lastChange(t, p, r.Change).Op != "upsert" {
		t.Error("op is not upsert")
	}
	es = entriesOf(t, p, r.Change)
	if len(es) != 2 || es[0].Kind != "u" || es[1].Kind != "i" || es[1].Before.Valid {
		t.Fatalf("upsert entries %+v", es)
	}
	jsonEq(t, es[0].Before.String, `{"coin":"BTC","date":"2026-09-27","price":100.0,"note":"first"}`)
	if v := one[string](t, p.DB, `SELECT price || '/' || note FROM prices WHERE coin = 'BTC' AND date = '2026-09-27'`); v != "300.0/first" {
		t.Errorf("after the upsert: %s", v)
	}

	// Nothing changed: no entry.
	n = len(changeList(t, p))
	r, err = p.Insert(ctx, limit.Interactive, msgSrc, "prices", []Row{{"coin": "BTC", "date": "2026-09-27"}}, ConflictSkip)
	if err != nil || r.Change != "" || len(changeList(t, p)) != n {
		t.Errorf("an insert that changed nothing: %+v, %v", r, err)
	}
	if r, err := p.Insert(ctx, limit.Interactive, msgSrc, "prices", nil, ConflictFail); err != nil || r.Change != "" {
		t.Errorf("an empty insert: %+v, %v", r, err)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{
		{"coin": "BTC", "date": "d1", "price": 1, "note": "a"},
		{"coin": "BTC", "date": "d2", "price": 2, "note": "b"},
	}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	r, err := p.Update(ctx, limit.Interactive, id.SourceOf("form", "v1"), UpdateReq{Table: "prices", Set: Row{"note": "checked"},
		Keys: []Row{{"coin": "BTC", "date": "d1"}, {"coin": "BTC", "date": "nope"}}})
	if err != nil || r.Updated != 1 || r.Skipped != 1 {
		t.Fatalf("update: %+v, %v", r, err)
	}
	c := lastChange(t, p, r.Change)
	if c.Op != "update" || c.Source != "form:v1" || c.Target != "table:prices" {
		t.Errorf("entry %+v", c)
	}
	es := entriesOf(t, p, r.Change)
	if len(es) != 1 || es[0].Kind != "u" || es[0].Key != `["BTC","d1"]` {
		t.Fatalf("update entries %+v", es)
	}
	jsonEq(t, es[0].Before.String, `{"coin":"BTC","date":"d1","price":1.0,"note":"a"}`)

	if _, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "prices", Set: Row{"coin": "ETH"},
		Keys: []Row{{"coin": "BTC", "date": "d1"}}}); err == nil {
		t.Error("an update set a key column")
	}
	for _, k := range []Row{{"coin": "BTC"}, {"coin": "BTC", "price": 1}, {"coin": "BTC", "date": nil}} {
		if _, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "prices", Set: Row{"note": "x"}, Keys: []Row{k}}); err == nil {
			t.Errorf("key %v was taken", k)
		}
	}
	if _, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "prices", Keys: []Row{{"coin": "BTC", "date": "d1"}}}); err == nil {
		t.Error("an update with nothing to set")
	}

	r, err = p.Delete(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "BTC", "date": "d2"}, {"coin": "BTC", "date": "nope"}})
	if err != nil || r.Deleted != 1 || r.Skipped != 1 {
		t.Fatalf("delete: %+v, %v", r, err)
	}
	if lastChange(t, p, r.Change).Op != "delete" {
		t.Error("op is not delete")
	}
	es = entriesOf(t, p, r.Change)
	if len(es) != 1 || es[0].Kind != "d" {
		t.Fatalf("delete entries %+v", es)
	}
	jsonEq(t, es[0].Before.String, `{"coin":"BTC","date":"d2","price":2.0,"note":"b"}`)
	if one[int](t, p.DB, "SELECT count(*) FROM prices") != 1 {
		t.Error("the row is still there")
	}
}

func TestWritesRefused(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE nokey (a TEXT, b TEXT)`)
	mustExec(t, p.DB, `CREATE VIEW v AS SELECT * FROM prices`)
	row := []Row{{"a": "x"}}
	for _, tab := range []string{"nokey", "v", "_jenab_meta", "_JENAB_changes", "sqlite_schema", "SQLITE_sequence"} {
		if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, tab, row, ConflictFail); !errors.Is(err, ErrNotAgentTable) {
			t.Errorf("insert into %s: %v", tab, err)
		}
		if _, err := p.Delete(ctx, limit.Interactive, id.SourceUser, tab, row); !errors.Is(err, ErrNotAgentTable) {
			t.Errorf("delete from %s: %v", tab, err)
		}
	}
	for _, tab := range []string{"missing", `prices"; DROP TABLE prices; --`, "prices x"} {
		if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, tab, row, ConflictFail); !errors.Is(err, ErrNotFound) {
			t.Errorf("insert into %q: %v", tab, err)
		}
	}
	bad := []Row{
		{"coin": "BTC", "date": "d", `price" = 1; --`: 1}, // not a column
		{"coin": "BTC"},            // no full key
		{"coin": nil, "date": "d"}, // a NULL key
		{"coin": "BTC", "date": "d", "price": map[string]any{"a": 1}},
		{"coin": "BTC", "date": "d", "price": 1, "PRICE": 2}, // twice
		{"coin": "BTC", "date": []byte("d")},
		{"coin": "a\xffb", "date": "d"}, // not UTF-8: JSON keys would not match
		{"coin": "BTC", "date": "d", "note": "\xff"},
		{},
	}
	for _, r := range bad {
		if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{r}, ConflictUpsert); !errors.Is(err, ErrInvalid) {
			t.Errorf("row %q: %v", r, err)
		}
	}
	if _, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "a\xffb", "date": "d"}}); !errors.Is(err, ErrInvalid) ||
		!strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("delete of a key that is not UTF-8: %v", err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, "", "prices", []Row{{"coin": "a", "date": "b"}}, ConflictFail); !errors.Is(err, ErrInvalid) {
		t.Errorf("a write without a source: %v", err)
	}

	// Triggers would write where the log can't see (SPEC 2.2), and a row of
	// more than 500 columns doesn't fit json_object.
	mustExec(t, p.DB, `CREATE TABLE trig (id INTEGER PRIMARY KEY, n INTEGER)`)
	mustExec(t, p.DB, `CREATE TRIGGER trig_n AFTER INSERT ON trig BEGIN UPDATE trig SET n = 1; END`)
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "TRIG", []Row{{"id": 1}}, ConflictFail); !errors.Is(err, ErrNotAgentTable) ||
		!strings.Contains(err.Error(), "triggers") {
		t.Errorf("table with a trigger: %v", err)
	}
	cols := []string{"id INTEGER PRIMARY KEY"}
	for i := range 500 {
		cols = append(cols, fmt.Sprintf("c%d TEXT", i))
	}
	mustExec(t, p.DB, "CREATE TABLE wide ("+strings.Join(cols, ", ")+")")
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "wide", []Row{{"id": 1}}, ConflictFail); !errors.Is(err, ErrInvalid) ||
		!strings.Contains(err.Error(), "501 columns") || !strings.Contains(err.Error(), "at most 500") {
		t.Errorf("wide table: %v", err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM _jenab_changes") != 0 || one[int](t, p.DB, "SELECT count(*) FROM prices") != 0 {
		t.Error("a refused write changed something")
	}

	// A taken UNIQUE value is not a taken key.
	mustExec(t, p.DB, `CREATE TABLE coins (id TEXT PRIMARY KEY, symbol TEXT UNIQUE)`)
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "coins", []Row{{"id": "bitcoin", "symbol": "BTC"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	for _, c := range []Conflict{ConflictFail, ConflictUpsert} {
		_, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "coins", []Row{{"id": "bitcoin2", "symbol": "BTC"}}, c)
		if !errors.Is(err, ErrNotUnique) || errors.Is(err, ErrRowExists) || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "coins.symbol") {
			t.Errorf("conflict rule %d, taken unique value: %v", c, err)
		}
	}
	_, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "coins", []Row{{"id": "bitcoin", "symbol": "XBT"}}, ConflictFail)
	if !errors.Is(err, ErrRowExists) || errors.Is(err, ErrNotUnique) {
		t.Errorf("taken key: %v", err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "coins", []Row{{"id": "eth", "symbol": "ETH"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "coins", Set: Row{"symbol": "BTC"}, Keys: []Row{{"id": "eth"}}}); !errors.Is(err, ErrNotUnique) {
		t.Errorf("update to a taken unique value: %v", err)
	}
	// None of these are the database's errors.
	if s := p.DB.Stats().LastError; s != "" {
		t.Errorf("a refused write is in the writer's stats: %s", s)
	}

	// Names are matched without case and quoted, so odd ones work.
	mustExec(t, p.DB, `CREATE TABLE "odd ""table""" ("the key" INTEGER PRIMARY KEY, "it's" BLOB)`)
	r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, `ODD "TABLE"`, []Row{{"It's": []byte{1, 2}}, {"it's": "text"}}, ConflictFail)
	if err != nil || r.Inserted != 2 {
		t.Fatalf("odd names: %+v, %v", r, err)
	}
	es := entriesOf(t, p, r.Change)
	if len(es) != 2 || es[0].Key != "[1]" || es[1].Key != "[2]" || es[0].Table != `odd "table"` {
		t.Errorf("rowid keys %+v", es)
	}
	r, err = p.Delete(ctx, limit.Interactive, id.SourceUser, `odd "table"`, []Row{{"the key": 1}})
	if err != nil || r.Deleted != 1 {
		t.Fatalf("delete: %+v, %v", r, err)
	}
	jsonEq(t, entriesOf(t, p, r.Change)[0].Before.String, `{"the key":1,"it's":{"$blob":"0102"}}`)
}

func TestInsertInChunks(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	rows := make([]Row, 1234)
	for i := range rows {
		rows[i] = Row{"coin": "BTC", "date": fmt.Sprintf("d%04d", i), "price": i}
	}
	r, err := p.Insert(ctx, limit.Background, id.SourceOf("run", "r1"), "prices", rows, ConflictUpsert)
	if err != nil || r.Inserted != 1234 {
		t.Fatalf("%+v, %v", r, err)
	}
	if cs := changeList(t, p); len(cs) != 1 {
		t.Fatalf("%d entries for one request", len(cs))
	}
	es := entriesOf(t, p, r.Change)
	if len(es) != 1234 || es[1233].Key != `["BTC","d1233"]` {
		t.Errorf("%d row entries", len(es))
	}

	// With ConflictFail, a taken key in the second chunk, or a key given
	// twice, fails the request before anything is written.
	eth := func() []Row {
		rows := make([]Row, 800)
		for i := range rows {
			rows[i] = Row{"coin": "ETH", "date": fmt.Sprintf("d%04d", i)}
		}
		return rows
	}
	rows = eth()
	rows[700] = Row{"coin": "BTC", "date": "d0001"}
	r, err = p.Insert(ctx, limit.Background, id.SourceOf("run", "r2"), "prices", rows, ConflictFail)
	if !errors.Is(err, ErrRowExists) || !strings.Contains(err.Error(), "row 700") || r.Inserted != 0 || r.Change != "" {
		t.Fatalf("taken key: %+v, %v", r, err)
	}
	rows = eth()
	rows[650] = Row{"coin": "ETH", "date": "d0100"}
	r, err = p.Insert(ctx, limit.Background, id.SourceOf("run", "r2"), "prices", rows, ConflictFail)
	if !errors.Is(err, ErrRowExists) || !strings.Contains(err.Error(), "row 650") || r.Inserted != 0 {
		t.Fatalf("key given twice: %+v, %v", r, err)
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM prices WHERE coin = 'ETH'"); n != 0 || len(changeList(t, p)) != 1 {
		t.Errorf("%d rows written before the check failed", n)
	}

	// So does a bad row, whatever the conflict rule.
	rows = eth()
	rows[700] = Row{"coin": "ETH", "date": "x", "nope": 1}
	r, err = p.Insert(ctx, limit.Background, id.SourceOf("run", "r3"), "prices", rows, ConflictUpsert)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "row 700") || r.Inserted != 0 {
		t.Fatalf("bad row: %+v, %v", r, err)
	}

	// A constraint that only the write itself checks, in the second chunk,
	// keeps the first one, with its entry.
	mustExec(t, p.DB, `CREATE TABLE checked (id INTEGER PRIMARY KEY, n INTEGER CHECK (n >= 0))`)
	rows = make([]Row, 800)
	for i := range rows {
		rows[i] = Row{"id": i, "n": i}
	}
	rows[700]["n"] = -1
	r, err = p.Insert(ctx, limit.Background, id.SourceOf("run", "r3"), "checked", rows, ConflictUpsert)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "row 700") || r.Inserted != 500 {
		t.Fatalf("%+v, %v", r, err)
	}
	if n := len(entriesOf(t, p, r.Change)); n != 500 {
		t.Errorf("%d row entries for the committed chunk", n)
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM checked"); n != 500 {
		t.Errorf("%d rows committed", n)
	}
}

// Rows that foreign-key actions delete or change are logged in the same
// entry, before the rows that caused them and deepest first (SPEC 2.5).
func TestForeignKeyActions(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE parent (id INTEGER PRIMARY KEY, code TEXT UNIQUE, name TEXT)`,
		`CREATE TABLE child (id INTEGER PRIMARY KEY, pid INTEGER REFERENCES parent ON DELETE CASCADE)`,
		`CREATE TABLE grand (id INTEGER PRIMARY KEY, cid INTEGER REFERENCES child (id) ON DELETE CASCADE)`,
		`CREATE TABLE tag (id INTEGER PRIMARY KEY, pid INTEGER REFERENCES parent ON DELETE SET NULL)`,
		`CREATE TABLE link (id INTEGER PRIMARY KEY, pcode TEXT REFERENCES parent (code) ON UPDATE CASCADE ON DELETE SET NULL)`,
		`INSERT INTO parent VALUES (1, 'a', 'one'), (2, 'b', 'two')`,
		`INSERT INTO child VALUES (10, 1), (11, 1), (12, 2)`,
		`INSERT INTO grand VALUES (100, 10), (101, 12)`,
		`INSERT INTO tag VALUES (20, 1), (21, 2)`,
		`INSERT INTO link VALUES (30, 'b'), (31, 'b')`,
	} {
		mustExec(t, p.DB, q)
	}
	type entry struct{ table, key, kind, before string }
	check := func(what string, cid id.Change, want []entry) {
		t.Helper()
		es := entriesOf(t, p, cid)
		if len(es) != len(want) {
			t.Fatalf("%s: %d entries, want %d: %+v", what, len(es), len(want), es)
		}
		for i, e := range es {
			if e.Table != want[i].table || e.Key != want[i].key || e.Kind != want[i].kind {
				t.Errorf("%s: entry %d is %s %s %s, want %+v", what, i, e.Table, e.Key, e.Kind, want[i])
				continue
			}
			jsonEq(t, e.Before.String, want[i].before)
		}
	}

	// A delete cascades two levels down and nulls a column.
	r, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "parent", []Row{{"id": 1}})
	if err != nil || r.Deleted != 1 || r.Cascaded != 4 {
		t.Fatalf("delete: %+v, %v", r, err)
	}
	check("delete", r.Change, []entry{
		{"grand", "[100]", "d", `{"id":100,"cid":10}`},
		{"child", "[10]", "d", `{"id":10,"pid":1}`},
		{"child", "[11]", "d", `{"id":11,"pid":1}`},
		{"tag", "[20]", "u", `{"id":20,"pid":1}`},
		{"parent", "[1]", "d", `{"id":1,"code":"a","name":"one"}`},
	})
	if one[int](t, p.DB, "SELECT count(*) FROM child") != 1 || one[int](t, p.DB, "SELECT count(*) FROM grand") != 1 ||
		one[int](t, p.DB, "SELECT count(*) FROM tag WHERE pid IS NULL") != 1 {
		t.Error("the actions did not run")
	}

	// A changed key that others reference cascades through ON UPDATE, by
	// update and by upsert. The primary key itself can't be set (SPEC 5.3),
	// so the referenced key here is a UNIQUE column.
	r, err = p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "parent", Set: Row{"code": "bb"}, Keys: []Row{{"id": 2}}})
	if err != nil || r.Updated != 1 || r.Cascaded != 2 {
		t.Fatalf("update: %+v, %v", r, err)
	}
	check("update", r.Change, []entry{
		{"link", "[30]", "u", `{"id":30,"pcode":"b"}`},
		{"link", "[31]", "u", `{"id":31,"pcode":"b"}`},
		{"parent", "[2]", "u", `{"id":2,"code":"b","name":"two"}`},
	})
	r, err = p.Insert(ctx, limit.Interactive, id.SourceUser, "parent", []Row{{"id": 2, "code": "c"}, {"id": 3, "code": "d"}}, ConflictUpsert)
	if err != nil || r.Updated != 1 || r.Inserted != 1 || r.Cascaded != 2 {
		t.Fatalf("upsert: %+v, %v", r, err)
	}
	if es := entriesOf(t, p, r.Change); len(es) != 4 || es[0].Table != "link" || es[1].Table != "link" {
		t.Errorf("upsert entries %+v", es)
	}
	// Changing a column nothing references, or setting the same value,
	// cascades nothing.
	r, err = p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "parent", Set: Row{"name": "x"}, Keys: []Row{{"id": 2}}})
	if err != nil || r.Cascaded != 0 || len(entriesOf(t, p, r.Change)) != 1 {
		t.Errorf("update of a plain column: %+v, %v", r, err)
	}

	// A tree that references itself goes as a whole.
	mustExec(t, p.DB, `CREATE TABLE node (id INTEGER PRIMARY KEY, up INTEGER REFERENCES node ON DELETE CASCADE)`)
	mustExec(t, p.DB, `INSERT INTO node VALUES (1, NULL), (2, 1), (3, 2), (4, 3), (5, NULL)`)
	r, err = p.Delete(ctx, limit.Interactive, id.SourceUser, "node", []Row{{"id": 1}, {"id": 3}})
	if err != nil || r.Deleted != 2 || r.Cascaded != 2 || one[int](t, p.DB, "SELECT count(*) FROM node") != 1 {
		t.Fatalf("tree: %+v, %v", r, err)
	}
	// Children before parents, so undo can go backwards: 3 is deleted both
	// by the request and as a grandchild of 1.
	if es := entriesOf(t, p, r.Change); len(es) != 4 || es[0].Key != "[4]" || es[1].Key != "[3]" || es[2].Key != "[2]" || es[3].Key != "[1]" {
		t.Errorf("tree entries %+v", es)
	}

	// Rows the log can't key are refused, and nothing changes.
	mustExec(t, p.DB, `CREATE TABLE loose (pid INTEGER REFERENCES parent ON DELETE CASCADE)`)
	mustExec(t, p.DB, `INSERT INTO loose VALUES (2)`)
	n := len(changeList(t, p))
	if _, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "parent", []Row{{"id": 2}}); !errors.Is(err, ErrNotAgentTable) ||
		!strings.Contains(err.Error(), "loose") {
		t.Errorf("cascade into a table without a key: %v", err)
	}
	if len(changeList(t, p)) != n || one[int](t, p.DB, "SELECT count(*) FROM parent WHERE id = 2") != 1 {
		t.Error("the refused delete changed something")
	}
}

func TestObjectChanges(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	o1 := put(t, p, "docs/a.txt", "one")
	cs := changeList(t, p)
	if len(cs) != 1 || cs[0].Op != "save" || cs[0].Target != "object:docs/a.txt" || cs[0].Source != "user" || cs[0].Before.Valid {
		t.Fatalf("put entry %+v", cs)
	}
	var after struct {
		Key, SHA256 string
		Version     int
	}
	json.Unmarshal([]byte(cs[0].After.String), &after)
	if after.Key != "docs/a.txt" || after.SHA256 != o1.SHA256 || after.Version != 1 {
		t.Errorf("after %s", cs[0].After.String)
	}
	if es := entriesOf(t, p, id.Change(cs[0].ID)); len(es) != 1 || es[0].Kind != "i" || es[0].Key != "object:docs/a.txt" || es[0].Table != "_jenab_objects" {
		t.Errorf("put row entries %+v", es)
	}

	o2 := put(t, p, "docs/a.txt", "two")
	c := changeList(t, p)[1]
	if !strings.Contains(c.Before.String, o1.SHA256) || !strings.Contains(c.After.String, o2.SHA256) {
		t.Errorf("second put %+v", c)
	}
	if es := entriesOf(t, p, id.Change(c.ID)); es[0].Kind != "u" || !strings.Contains(es[0].Before.String, o1.SHA256) {
		t.Errorf("second put rows %+v", es)
	}

	// Object references: the key check and the move.
	mustExec(t, p.DB, `CREATE TABLE docs (id INTEGER PRIMARY KEY, file TEXT, thumb TEXT, title TEXT)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref'), ('docs', 'THUMB', 'object_ref'), ('docs', 'gone', 'object_ref'), ('old', 'x', 'object_ref')`)
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{{"file": "docs/missing.txt"}}, ConflictFail); !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing key: %v", err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{{"file": 5}}, ConflictFail); err == nil {
		t.Error("a number as an object key")
	}
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{
		{"id": 1, "file": "docs/a.txt", "thumb": "docs/a.txt"}, {"id": 2, "file": nil, "title": "docs/missing.txt"}, {"id": 3, "file": "docs/a.txt"},
	}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "docs", Set: Row{"thumb": "docs/missing.txt"}, Keys: []Row{{"id": 2}}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update to a missing key: %v", err)
	}

	if _, err := p.MoveObject(ctx, limit.Interactive, msgSrc, "docs/a.txt", "docs/b.txt"); err != nil {
		t.Fatal(err)
	}
	if v := one[string](t, p.DB, `SELECT group_concat(id || ':' || coalesce(file, '-') || ':' || coalesce(thumb, '-'), ' ') FROM docs`); v != "1:docs/b.txt:docs/b.txt 2:-:- 3:docs/b.txt:-" {
		t.Errorf("after the move: %s", v)
	}
	cs = changeList(t, p)
	m := cs[len(cs)-1]
	if m.Op != "move" || m.Target != "object:docs/a.txt" || m.Source != string(msgSrc) || !strings.Contains(m.After.String, `"key":"docs/b.txt"`) {
		t.Errorf("move entry %+v", m)
	}
	es := entriesOf(t, p, id.Change(m.ID))
	if len(es) != 4 || es[0].Key != "object:docs/a.txt" || es[0].Kind != "d" || es[1].Key != "object:docs/b.txt" || es[1].Kind != "i" ||
		es[2].Table != "docs" || es[2].Key != "[1]" || es[3].Key != "[3]" || es[2].Kind != "u" {
		t.Fatalf("move rows %+v", es)
	}
	jsonEq(t, es[2].Before.String, `{"id":1,"file":"docs/a.txt","thumb":"docs/a.txt","title":null}`)

	if err := p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "docs/b.txt"); err != nil {
		t.Fatal(err)
	}
	cs = changeList(t, p)
	d := cs[len(cs)-1]
	if d.Op != "delete" || d.Target != "object:docs/b.txt" || d.After.Valid || !strings.Contains(d.Before.String, o2.SHA256) {
		t.Errorf("delete entry %+v", d)
	}
	if es := entriesOf(t, p, id.Change(d.ID)); len(es) != 1 || es[0].Kind != "d" {
		t.Errorf("delete rows %+v", es)
	}

	// Failed operations log nothing.
	n := len(cs)
	p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "docs/b.txt")
	p.MoveObject(ctx, limit.Interactive, id.SourceUser, "docs/none", "docs/c.txt")
	if len(changeList(t, p)) != n {
		t.Error("a failed object operation left an entry")
	}
}

func TestCleanChanges(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	ins := func(coin string, n int) id.Change {
		rows := make([]Row, n)
		for i := range rows {
			rows[i] = Row{"coin": coin, "date": fmt.Sprint(i), "price": i}
		}
		r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", rows, ConflictUpsert)
		if err != nil {
			t.Fatal(err)
		}
		return r.Change
	}
	old1 := ins("BTC", 3)
	put(t, p, "a.txt", "x")
	old2 := ins("BTC", 5) // 3 updates, 2 inserts
	recent := ins("ETH", 2)
	old := time.Now().Add(-100 * 24 * time.Hour)
	mustExec(t, p.DB, `UPDATE _jenab_changes SET at = ? WHERE id <= ?`, formatTime(old), string(old2))

	rep, err := p.CleanChanges(ctx, 0)
	if err != nil || rep.Changes != 3 || rep.Rows != 9 {
		t.Fatalf("clean: %+v, %v", rep, err)
	}
	cs := changeList(t, p)
	if len(cs) != 4 {
		t.Fatalf("%d entries left; cleanup keeps them all", len(cs))
	}
	jsonEq(t, cs[0].After.String, `{"i":3,"u":0,"d":0}`)
	jsonEq(t, cs[2].After.String, `{"i":2,"u":3,"d":0}`)
	if !strings.Contains(cs[1].After.String, `"key":"a.txt"`) {
		t.Errorf("the object keeps its revision: %s", cs[1].After.String)
	}
	if len(entriesOf(t, p, old1)) != 0 || len(entriesOf(t, p, old2)) != 0 || len(entriesOf(t, p, recent)) != 2 {
		t.Error("the wrong row entries were dropped")
	}
	if m, _ := p.Meta(ctx); m[metaCleanedTo] != string(old2) {
		t.Errorf("watermark %q, want %q", m[metaCleanedTo], old2)
	}
	if at, err := p.CleanedAt(ctx); err != nil || time.Since(at) > time.Minute {
		t.Errorf("cleaned at %v, %v", at, err)
	}

	// Again: nothing to do. A shorter limit reaches the recent one.
	if rep, err := p.CleanChanges(ctx, 0); err != nil || rep.Changes != 0 {
		t.Errorf("second clean: %+v, %v", rep, err)
	}
	if rep, err := p.CleanChanges(ctx, time.Nanosecond); err != nil || rep.Changes != 1 || rep.Rows != 2 {
		t.Errorf("clean with a short limit: %+v, %v", rep, err)
	}
}

// Many old changes go in several short batches.
func TestCleanChangesInBatches(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	at := formatTime(time.Now().Add(-200 * 24 * time.Hour))
	// 450 changes of 20 rows, then one of 6,000: more than one batch both ways.
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 451)
		INSERT INTO _jenab_changes (id, at, source, target, op) SELECT printf('00%024d', i), ?, 'run:x', 'table:t', 'insert' FROM n`, at)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 450 * 20 - 1)
		INSERT INTO _jenab_change_rows (change_id, table_name, row_key, kind) SELECT printf('00%024d', i / 20 + 1), 't', json_array(i), 'i' FROM n`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 5999)
		INSERT INTO _jenab_change_rows (change_id, table_name, row_key, kind) SELECT printf('00%024d', 451), 't', json_array(i), 'd' FROM n`)
	rep, err := p.CleanChanges(ctx, 0)
	if err != nil || rep.Changes != 451 || rep.Rows != 450*20+6000 {
		t.Fatalf("%+v, %v", rep, err)
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM _jenab_change_rows"); n != 0 {
		t.Errorf("%d row entries left", n)
	}
	if v := one[string](t, p.DB, "SELECT after FROM _jenab_changes WHERE id = printf('00%024d', 451)"); v != `{"d":6000,"i":0,"u":0}` {
		t.Errorf("after %s", v)
	}
}

// One change of 12,000 rows takes three transactions; the watermark is on
// it from the first one, and its counts grow with each.
func TestCleanBigChange(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	at := formatTime(time.Now().Add(-200 * 24 * time.Hour))
	mustExec(t, p.DB, `INSERT INTO _jenab_changes (id, at, source, target, op) VALUES ('01A', ?, 'run:x', 'table:t', 'upsert'), ('01B', ?, 'run:x', 'table:t', 'insert')`, at, at)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 11999)
		INSERT INTO _jenab_change_rows (change_id, table_name, row_key, kind) SELECT '01A', 't', json_array(i), iif(i % 4 = 0, 'u', 'i') FROM n`)
	mustExec(t, p.DB, `INSERT INTO _jenab_change_rows (change_id, table_name, row_key, kind) VALUES ('01B', 't', '[1]', 'i')`)
	cutoff := formatTime(time.Now().Add(-ChangeKeep))
	batch := func() cleaned {
		t.Helper()
		b, err := Do(ctx, p.DB, limit.Background, func(tx *sql.Tx) (cleaned, error) { return cleanBatch(tx, cutoff) })
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for i, want := range []struct {
		rows  int
		more  bool
		after string
		mark  string
	}{
		{5000, true, `{"d":0,"i":3750,"u":1250}`, "01A"},
		{5000, true, `{"d":0,"i":7500,"u":2500}`, "01A"},
		{2001, false, `{"d":0,"i":9000,"u":3000}`, "01B"}, // the rest, and the next change
	} {
		b := batch()
		if b.Rows != want.rows || b.more != want.more {
			t.Errorf("batch %d: %+v", i+1, b)
		}
		if a := one[string](t, p.DB, "SELECT after FROM _jenab_changes WHERE id = '01A'"); a != want.after {
			t.Errorf("batch %d: after %s, want %s", i+1, a, want.after)
		}
		if m, _ := p.Meta(ctx); m[metaCleanedTo] != want.mark {
			t.Errorf("batch %d: watermark %q, want %q", i+1, m[metaCleanedTo], want.mark)
		}
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM _jenab_change_rows"); n != 0 {
		t.Errorf("%d row entries left", n)
	}
	if rep, err := p.CleanChanges(ctx, 0); err != nil || rep.Rows != 0 {
		t.Errorf("clean after: %+v, %v", rep, err)
	}
}

// seedPrices fills prices with n rows: 50 coins, n/50 days each.
func seedPrices(t testing.TB, p *ProjectDB, n int) {
	t.Helper()
	if _, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < ? - 1)
			INSERT INTO prices SELECT 'C' || (i % 50), printf('d%06d', i / 50), i * 1.5, 'seed' FROM n`, n)
		return struct{}{}, err
	}); err != nil {
		t.Fatal(err)
	}
}

// upsertRows is n rows, the share upd of them for days that exist (from
// day from on), the rest for new days.
func upsertRows(n int, upd float64, from, fresh int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		day := from + i/50
		if float64(i) >= upd*float64(n) {
			day = fresh + i/50
		}
		rows[i] = Row{"coin": fmt.Sprintf("C%d", i%50), "date": fmt.Sprintf("d%06d", day), "price": float64(i), "note": "upsert"}
	}
	return rows
}

// N-08: a 5,000-row upsert with the change log takes under 1 s, on a table
// of 100,000 rows as in SPIKE-009. The time is checked only with
// JENAB_PERF_TEST=1, as it needs a quiet machine; the counts always.
func TestUpsertSpeed(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	seedPrices(t, p, 100_000)
	fresh := 10_000 // days past the seeded 2,000
	var half []time.Duration
	for i, upd := range []float64{0, 0.5, 0.5, 0.5, 1} {
		rows := upsertRows(5000, upd, i*100, fresh)
		fresh += 100
		start := time.Now()
		r, err := p.Insert(ctx, limit.Background, id.SourceOf("run", "speed"), "prices", rows, ConflictUpsert)
		took := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if r.Inserted+r.Updated != 5000 || r.Updated != int(upd*5000) {
			t.Errorf("%+v", r)
		}
		t.Logf("5,000-row upsert, %3.0f%% updates: %v", upd*100, took.Round(time.Millisecond))
		if upd == 0.5 {
			half = append(half, took)
		}
	}
	// The median, so one slow disk sync on a busy machine doesn't decide.
	slices.Sort(half)
	if os.Getenv("JENAB_PERF_TEST") == "1" && half[1] > time.Second {
		t.Errorf("the median of the 50%% upserts took %v, over 1 s (N-08)", half[1])
	}
}

func BenchmarkUpsert5000(b *testing.B) {
	dir := b.TempDir()
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close(context.Background())
	if _, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		_, err := tx.Exec(`CREATE TABLE prices (coin TEXT, date TEXT, price REAL, note TEXT, PRIMARY KEY (coin, date))`)
		return 0, err
	}); err != nil {
		b.Fatal(err)
	}
	seedPrices(b, p, 100_000)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if _, err := p.Insert(context.Background(), limit.Background, id.SourceUser, "prices", upsertRows(5000, 0.5, (i%20)*100, 10_000+i*100), ConflictUpsert); err != nil {
			b.Fatal(err)
		}
	}
}

// SPIKE-009: with a million row entries, and after the cleanup has dropped
// half of them, the undo conflict check of a 5,000-row run stays an index
// lookup under 10 ms. The check is P2-03's; this is its query shape, with
// the row entries' rowid as their order. It takes about 30 s and its timing
// needs a quiet machine, so it runs only with JENAB_PERF_TEST=1.
func TestConflictLookupAt1M(t *testing.T) {
	if os.Getenv("JENAB_PERF_TEST") != "1" {
		t.Skip("set JENAB_PERF_TEST=1")
	}
	p := pricesProject(t)
	ctx := context.Background()
	seedPrices(t, p, 100_000)
	start := time.Now()
	old, recent := formatTime(time.Now().Add(-200*24*time.Hour)), formatTime(time.Now().Add(-time.Hour))
	if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		// 200 runs of 5,000 rows, the first half past the 90 days; a quarter
		// of the rows are updates with a before-image.
		if _, err := tx.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 200)
			INSERT INTO _jenab_changes (id, at, source, target, op) SELECT printf('00%024d', i), iif(i <= 100, ?, ?), 'run:x', 'table:prices', 'upsert' FROM n`,
			old, recent); err != nil {
			return struct{}{}, err
		}
		_, err := tx.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 1000000 - 1)
			INSERT INTO _jenab_change_rows (change_id, table_name, row_key, kind, before)
			SELECT printf('00%024d', i / 5000 + 1), 'prices', json_array('C' || (i % 50), printf('d%06d', (i / 50) % 2000)),
				iif(i % 4 = 0, 'u', 'i'), iif(i % 4 = 0, json_object('coin', 'C' || (i % 50), 'date', printf('d%06d', (i / 50) % 2000), 'price', i * 1.5, 'note', 'seed'), NULL)
			FROM n`)
		return struct{}{}, err
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("filling 1M row entries took %v", time.Since(start).Round(time.Millisecond))

	run, err := p.Insert(ctx, limit.Background, id.SourceOf("run", "r"), "prices", upsertRows(5000, 0.5, 0, 10_000), ConflictUpsert)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]Row, 100)
	for i := range keys {
		keys[i] = Row{"coin": fmt.Sprintf("C%d", i%50), "date": fmt.Sprintf("d%06d", i/50)}
	}
	if _, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "prices", Set: Row{"note": "later"}, Keys: keys}); err != nil {
		t.Fatal(err)
	}
	const conflicts = `SELECT DISTINCT r.row_key FROM _jenab_change_rows r
		JOIN _jenab_change_rows l ON l.table_name = r.table_name AND l.row_key = r.row_key AND l.rowid > r.rowid
		WHERE r.change_id = ?1 AND l.change_id <> ?1`
	// Both sides are index searches, whatever the machine's load.
	plan, err := Query(ctx, p.DB, "EXPLAIN QUERY PLAN "+conflicts, []any{string(run.Change)}, func(r *sql.Rows) (string, error) {
		var a, b, c int
		var d string
		return d, r.Scan(&a, &b, &c, &d)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range plan {
		if !strings.HasPrefix(d, "SEARCH") && !strings.HasPrefix(d, "USE TEMP B-TREE") {
			t.Errorf("plan step %q is not an index search", d)
		}
	}
	t.Logf("plan: %s", strings.Join(plan, "; "))
	// Five tries on one reader connection, so its page cache is warm as in
	// SPIKE-009; the fastest counts.
	lookup := func() (time.Duration, int) {
		type try struct {
			took time.Duration
			n    int
		}
		r, err := readTx(ctx, p.DB, func(tx *sql.Tx) (try, error) {
			var best try
			for i := range 5 {
				start := time.Now()
				ks, err := queryTx(tx, conflicts, []any{string(run.Change)}, func(r *sql.Rows) (string, error) {
					var k string
					return k, r.Scan(&k)
				})
				if err != nil {
					return best, err
				}
				if took := time.Since(start); i == 0 || took < best.took {
					best.took = took
				}
				best.n = len(ks)
			}
			return best, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return r.took, r.n
	}
	took, n := lookup()
	t.Logf("conflict check at %d row entries: %v, %d conflicts", one[int](t, p.DB, "SELECT count(*) FROM _jenab_change_rows"), took, n)
	if n != 100 {
		t.Errorf("%d conflicts, want 100", n)
	}

	start = time.Now()
	rep, err := p.CleanChanges(ctx, 0)
	if err != nil || rep.Rows != 500_000 {
		t.Fatalf("clean: %+v, %v", rep, err)
	}
	t.Logf("cleanup of %d changes, %d rows: %v", rep.Changes, rep.Rows, time.Since(start).Round(time.Millisecond))
	took, n = lookup()
	t.Logf("conflict check after the cleanup: %v, %d conflicts", took, n)
	if n != 100 || took > 10*time.Millisecond {
		t.Errorf("after the cleanup: %v, %d conflicts", took, n)
	}
}
