package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// dump is every row of tables, for comparing states.
func dump(t *testing.T, p *ProjectDB, tables ...string) string {
	t.Helper()
	var out []string
	for _, tab := range tables {
		rows, err := Query(context.Background(), p.DB, "SELECT * FROM "+quote(tab)+" ORDER BY 1", nil, func(r *sql.Rows) (string, error) {
			cols, _ := r.Columns()
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				return "", err
			}
			return fmt.Sprint(vals), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, tab+": "+strings.Join(rows, " "))
	}
	return strings.Join(out, "\n")
}

// undo replays a change's row entries backwards in one transaction, with
// foreign keys checked at each statement, as P2-03's undo will: a deleted
// row is put back, an updated one gets its before-image, an inserted one
// goes. It fails if a row comes back before one it references.
func undo(t *testing.T, p *ProjectDB, cid id.Change) {
	t.Helper()
	es := entriesOf(t, p, cid)
	_, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for i := len(es) - 1; i >= 0; i-- {
			e := es[i]
			tab, err := loadTable(tx, e.Table)
			if err != nil {
				return struct{}{}, err
			}
			var key []any
			json.Unmarshal([]byte(e.Key), &key)
			where := make([]string, len(tab.pk))
			for j, c := range tab.pk {
				where[j] = quote(c) + " = ?"
			}
			var row map[string]any
			json.Unmarshal([]byte(e.Before.String), &row)
			cols := slices.Sorted(maps.Keys(row))
			vals := make([]any, len(cols))
			set := make([]string, len(cols))
			for j, c := range cols {
				vals[j], set[j] = row[c], quote(c)+" = ?"
			}
			var q string
			var args []any
			switch e.Kind {
			case "d":
				q = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quote(tab.name), strings.Join(quoteAll(cols), ", "),
					strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "))
				args = vals
			case "u":
				q = fmt.Sprintf("UPDATE %s SET %s WHERE %s", quote(tab.name), strings.Join(set, ", "), strings.Join(where, " AND "))
				args = append(vals, key...)
			case "i":
				q = fmt.Sprintf("DELETE FROM %s WHERE %s", quote(tab.name), strings.Join(where, " AND "))
				args = key
			}
			if _, err := tx.Exec(q, args...); err != nil {
				return struct{}{}, fmt.Errorf("entry %d (%s %s %s): %w", i, e.Kind, e.Table, e.Key, err)
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
}

// A row that two chains of actions reach is logged at the depth of the
// longer one, and so are the rows below it, so that undo going backwards
// never puts a row back before one it references.
func TestCascadeDiamond(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE tp (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE ta (id INTEGER PRIMARY KEY, p INTEGER REFERENCES tp ON DELETE CASCADE)`,
		`CREATE TABLE tb (id INTEGER PRIMARY KEY, a INTEGER REFERENCES ta ON DELETE CASCADE)`,
		`CREATE TABLE ts (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE tr (id INTEGER PRIMARY KEY, b INTEGER REFERENCES tb ON DELETE CASCADE,
			p INTEGER REFERENCES tp ON DELETE CASCADE, s INTEGER REFERENCES ts ON DELETE CASCADE)`,
		`CREATE TABLE tx (id INTEGER PRIMARY KEY, r INTEGER REFERENCES tr ON DELETE CASCADE)`,
		`CREATE TABLE tn (id INTEGER PRIMARY KEY, r INTEGER REFERENCES tr ON DELETE SET NULL)`,
		`INSERT INTO tp VALUES (1), (2)`,
		`INSERT INTO ta VALUES (10, 1), (11, 2)`,
		`INSERT INTO tb VALUES (20, 10)`,
		`INSERT INTO ts VALUES (5)`,
		`INSERT INTO tr VALUES (30, 20, 1, 5), (31, NULL, 2, 5)`,
		`INSERT INTO tx VALUES (40, 30), (41, 31)`,
		`INSERT INTO tn VALUES (50, 30)`,
	} {
		mustExec(t, p.DB, q)
	}
	tables := []string{"tp", "ta", "tb", "ts", "tr", "tx", "tn"}
	was := dump(t, p, tables...)

	// tr 30 is a child of tp 1 and a great-grandchild of it, through ta and
	// tb; tx 40 and tn 50 hang below it.
	r, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "tp", []Row{{"id": 1}})
	if err != nil || r.Deleted != 1 || r.Cascaded != 5 {
		t.Fatalf("delete: %+v, %v", r, err)
	}
	var got []string
	for _, e := range entriesOf(t, p, r.Change) {
		got = append(got, e.Kind+" "+e.Table+e.Key)
	}
	want := []string{"u tn[50]", "d tx[40]", "d tr[30]", "d tb[20]", "d ta[10]", "d tp[1]"}
	if !slices.Equal(got, want) {
		t.Errorf("entries %v, want %v", got, want)
	}
	undo(t, p, r.Change)
	if now := dump(t, p, tables...); now != was {
		t.Errorf("after undo:\n%s\nwant:\n%s", now, was)
	}

	// Both parents at once, and a self-referencing tree, go back the same way.
	r, err = p.Delete(ctx, limit.Interactive, id.SourceUser, "tp", []Row{{"id": 1}, {"id": 2}})
	if err != nil || r.Deleted != 2 || r.Cascaded != 8 {
		t.Fatalf("delete both: %+v, %v", r, err)
	}
	undo(t, p, r.Change)
	if now := dump(t, p, tables...); now != was {
		t.Errorf("after undo of both:\n%s\nwant:\n%s", now, was)
	}
	mustExec(t, p.DB, `CREATE TABLE node (id INTEGER PRIMARY KEY, up INTEGER REFERENCES node ON DELETE CASCADE)`)
	mustExec(t, p.DB, `INSERT INTO node VALUES (1, NULL), (2, 1), (3, 2), (4, 3), (5, 1), (6, 5), (7, 3), (8, NULL)`)
	was = dump(t, p, "node")
	r, err = p.Delete(ctx, limit.Interactive, id.SourceUser, "node", []Row{{"id": 3}, {"id": 1}})
	if err != nil || r.Deleted != 2 || r.Cascaded != 5 {
		t.Fatalf("tree: %+v, %v", r, err)
	}
	undo(t, p, r.Change)
	if now := dump(t, p, "node"); now != was {
		t.Errorf("tree after undo:\n%s\nwant:\n%s", now, was)
	}
}

// The search for rows an action reaches compares keys as SQLite's own
// action does, with the parent column's collation.
func TestCascadeCollation(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE cp (id TEXT COLLATE NOCASE PRIMARY KEY)`)
	mustExec(t, p.DB, `CREATE TABLE cc (id INTEGER PRIMARY KEY, pid TEXT REFERENCES cp ON DELETE CASCADE)`)
	mustExec(t, p.DB, `INSERT INTO cp VALUES ('abc'), ('xyz')`)
	mustExec(t, p.DB, `INSERT INTO cc VALUES (1, 'ABC'), (2, 'abc'), (3, 'xyz')`)
	r, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "cp", []Row{{"id": "abc"}})
	if err != nil || r.Cascaded != 2 || one[int](t, p.DB, "SELECT count(*) FROM cc") != 1 {
		t.Fatalf("%+v, %v", r, err)
	}
	if es := entriesOf(t, p, r.Change); len(es) != 3 || es[0].Key != "[1]" || es[1].Key != "[2]" {
		t.Errorf("entries %+v", es)
	}
}

// A write that foreign keys would carry to more than maxCascade rows is
// refused before anything changes; one at the cap goes through.
func TestCascadeCap(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE big (id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `CREATE TABLE kid (id INTEGER PRIMARY KEY, pid INTEGER REFERENCES big ON DELETE CASCADE)`)
	mustExec(t, p.DB, `CREATE INDEX kid_pid ON kid (pid)`)
	mustExec(t, p.DB, `INSERT INTO big VALUES (1)`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?) INSERT INTO kid SELECT i, 1 FROM n`, maxCascade+1)
	n := len(changeList(t, p))
	_, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "big", []Row{{"id": 1}})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), fmt.Sprintf("delete %d rows in kid", maxCascade+1)) ||
		!strings.Contains(err.Error(), "smaller batches") {
		t.Fatalf("past the cap: %v", err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM kid") != maxCascade+1 || len(changeList(t, p)) != n {
		t.Error("the refused delete changed something")
	}
	if s := p.DB.Stats().LastError; s != "" {
		t.Errorf("a refused write counts as a database error: %s", s)
	}
	if _, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "kid", []Row{{"id": 1}}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "big", []Row{{"id": 1}})
	if err != nil || r.Cascaded != maxCascade || len(entriesOf(t, p, r.Change)) != maxCascade+1 {
		t.Fatalf("at the cap: %+v, %v", r, err)
	}
	t.Logf("a delete that cascades to %d rows took %v", maxCascade, time.Since(start).Round(time.Millisecond))
}

// A row an action reaches with a BLOB in its key can't be logged, so the
// write is refused.
func TestCascadeBlobKey(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE bp (id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `CREATE TABLE bc (k BLOB PRIMARY KEY, pid INTEGER REFERENCES bp ON DELETE CASCADE)`)
	mustExec(t, p.DB, `INSERT INTO bp VALUES (1)`)
	mustExec(t, p.DB, `INSERT INTO bc VALUES (x'01', 1)`)
	if _, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "bp", []Row{{"id": 1}}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "BLOB") {
		t.Errorf("cascade to a BLOB key: %v", err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM bc") != 1 {
		t.Error("the refused delete changed something")
	}
}

// What one write's cascade costs near maxCascade, for choosing it: a
// delete that cascades, one that sets NULL, and an update that cascades.
// Run with JENAB_PERF_TEST=1; each should stay well under 1 s.
func TestCascadeSpeed(t *testing.T) {
	if os.Getenv("JENAB_PERF_TEST") != "1" {
		t.Skip("set JENAB_PERF_TEST=1")
	}
	ctx := context.Background()
	for _, n := range []int{maxCascade / 4, maxCascade / 2, maxCascade} {
		for _, kind := range []string{"ON DELETE CASCADE", "ON DELETE SET NULL", "ON UPDATE CASCADE"} {
			p, _ := openTestProject(t)
			mustExec(t, p.DB, `CREATE TABLE sp (id INTEGER PRIMARY KEY, code TEXT UNIQUE)`)
			mustExec(t, p.DB, `CREATE TABLE sk (id INTEGER PRIMARY KEY, pcode TEXT REFERENCES sp (code) `+kind+`, note TEXT)`)
			mustExec(t, p.DB, `CREATE INDEX sk_pcode ON sk (pcode)`)
			mustExec(t, p.DB, `INSERT INTO sp VALUES (1, 'a')`)
			mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
				INSERT INTO sk SELECT i, 'a', printf('note %d', i) FROM n`, n)
			start := time.Now()
			var r WriteResult
			var err error
			if kind == "ON UPDATE CASCADE" {
				r, err = p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "sp", Set: Row{"code": "b"}, Keys: []Row{{"id": 1}}})
			} else {
				r, err = p.Delete(ctx, limit.Interactive, id.SourceUser, "sp", []Row{{"id": 1}})
			}
			d := time.Since(start)
			if err != nil || r.Cascaded != n {
				t.Fatalf("%s, %d rows: %+v, %v", kind, n, r, err)
			}
			t.Logf("%s to %d rows: %v", kind, n, d.Round(time.Millisecond))
			if n == maxCascade && d > time.Second {
				t.Errorf("%s to %d rows took %v", kind, n, d)
			}
			p.Close(ctx)
		}
	}
	// 30 foreign keys from one child table to one parent, 333 rows each.
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE hub (id INTEGER PRIMARY KEY)`)
	cols := make([]string, 30)
	for i := range cols {
		cols[i] = fmt.Sprintf("r%d INTEGER REFERENCES hub ON DELETE SET NULL", i)
	}
	mustExec(t, p.DB, `CREATE TABLE spoke (id INTEGER PRIMARY KEY, `+strings.Join(cols, ", ")+`)`)
	mustExec(t, p.DB, `INSERT INTO hub VALUES (1)`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 333)
		INSERT INTO spoke SELECT i, `+strings.TrimSuffix(strings.Repeat("1, ", 30), ", ")+` FROM n`)
	start := time.Now()
	r, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "hub", []Row{{"id": 1}})
	d := time.Since(start)
	if err != nil || r.Cascaded != 333 {
		t.Fatalf("30 foreign keys: %+v, %v", r, err)
	}
	t.Logf("30 foreign keys to 333 rows: %v", d.Round(time.Millisecond))
	if d > time.Second {
		t.Errorf("30 foreign keys to 333 rows took %v", d)
	}
}
