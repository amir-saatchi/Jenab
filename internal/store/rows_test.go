package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// The pre-check of a request of more than one chunk compares keys as the
// primary key does, with its collation and its columns' affinity.
func TestPrecheckKeys(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE nc (k TEXT COLLATE NOCASE PRIMARY KEY, v INTEGER)`,
		`CREATE TABLE pc (k TEXT, v INTEGER, PRIMARY KEY (k COLLATE NOCASE)) WITHOUT ROWID`,
		`CREATE TABLE ni (a INTEGER, b TEXT, PRIMARY KEY (a, b))`,
		`CREATE TABLE ri (id INTEGER PRIMARY KEY, v INTEGER)`,
		`INSERT INTO nc VALUES ('Zed', 0)`,
		`INSERT INTO pc VALUES ('Zed', 0)`,
	} {
		mustExec(t, p.DB, q)
	}
	text := func(n int) []Row {
		rows := make([]Row, n)
		for i := range rows {
			rows[i] = Row{"k": fmt.Sprintf("k%d", i), "v": i}
		}
		return rows
	}
	for _, tab := range []string{"nc", "pc"} {
		rows := text(600)
		rows[550]["k"] = "K0"
		if _, err := p.Insert(ctx, limit.Background, id.SourceUser, tab, rows, ConflictFail); !errors.Is(err, ErrRowExists) || !strings.Contains(err.Error(), "row 550") {
			t.Errorf("%s: a key given twice in two cases: %v", tab, err)
		}
		rows = text(600)
		rows[520]["k"] = "zed"
		if _, err := p.Insert(ctx, limit.Background, id.SourceUser, tab, rows, ConflictFail); !errors.Is(err, ErrRowExists) || !strings.Contains(err.Error(), "row 520") {
			t.Errorf("%s: a taken key in another case: %v", tab, err)
		}
		if n := one[int](t, p.DB, "SELECT count(*) FROM "+tab); n != 1 {
			t.Errorf("%s: %d rows written before the check failed", tab, n)
		}
	}
	rows := make([]Row, 600)
	for i := range rows {
		rows[i] = Row{"a": i, "b": "x"}
	}
	rows[580] = Row{"a": "1", "b": "x"}
	if _, err := p.Insert(ctx, limit.Background, id.SourceUser, "ni", rows, ConflictFail); !errors.Is(err, ErrRowExists) || !strings.Contains(err.Error(), "row 580") {
		t.Errorf("1 and '1' in an INTEGER key: %v", err)
	}
	rows[580] = Row{"a": "580", "b": "x"}
	if r, err := p.Insert(ctx, limit.Background, id.SourceUser, "ni", rows, ConflictFail); err != nil || r.Inserted != 600 {
		t.Errorf("distinct keys: %+v, %v", r, err)
	}
	if n, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		var n int
		return n, tx.QueryRow("SELECT count(*) FROM sqlite_temp_schema").Scan(&n)
	}); err != nil || n != 0 {
		t.Errorf("%d temporary tables left, %v", n, err)
	}

	// In a rowid table, rows that leave the key out and rows that give it
	// can't share a request of more than one chunk: SQLite picks the key
	// of the first when their chunk is written.
	rows = make([]Row, 600)
	for i := range rows {
		rows[i] = Row{"v": i}
		if i >= 300 {
			rows[i]["id"] = i + 1000
		}
	}
	if _, err := p.Insert(ctx, limit.Background, id.SourceUser, "ri", rows, ConflictSkip); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "row 300") {
		t.Errorf("left-out and given keys: %v", err)
	}
	if r, err := p.Insert(ctx, limit.Background, id.SourceUser, "ri", rows[290:310], ConflictFail); err != nil || r.Inserted != 20 {
		t.Errorf("a mix in one chunk: %+v, %v", r, err)
	}
	if r, err := p.Insert(ctx, limit.Background, id.SourceUser, "ri", rows[:300], ConflictFail); err != nil || r.Inserted != 300 {
		t.Errorf("all keys left out: %+v, %v", r, err)
	}
}

// OR ABORT overrides the ON CONFLICT clauses of the schema: none deletes a
// row the log can't see, or drops one without a word.
func TestSchemaConflictClauses(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE kr (id TEXT PRIMARY KEY ON CONFLICT REPLACE, v INTEGER)`,
		`CREATE TABLE ur (id INTEGER PRIMARY KEY, code TEXT UNIQUE ON CONFLICT REPLACE)`,
		`CREATE TABLE ig (id INTEGER PRIMARY KEY ON CONFLICT IGNORE, code TEXT UNIQUE ON CONFLICT IGNORE,
			n INTEGER NOT NULL ON CONFLICT IGNORE DEFAULT 0)`,
		`INSERT INTO kr VALUES ('a', 1)`,
		`INSERT INTO ur VALUES (1, 'x'), (2, 'y')`,
		`INSERT INTO ig VALUES (1, 'x', 1), (2, 'w', 1)`,
	} {
		mustExec(t, p.DB, q)
	}
	tables := []string{"kr", "ur", "ig"}
	was := dump(t, p, tables...)
	n := len(changeList(t, p))
	src := id.SourceUser
	for _, c := range []struct {
		what string
		err  error
		do   func() (WriteResult, error)
	}{
		{"PK REPLACE, insert", ErrRowExists, func() (WriteResult, error) {
			return p.Insert(ctx, limit.Interactive, src, "kr", []Row{{"id": "a", "v": 2}}, ConflictFail)
		}},
		{"UNIQUE REPLACE, insert", ErrNotUnique, func() (WriteResult, error) {
			return p.Insert(ctx, limit.Interactive, src, "ur", []Row{{"id": 3, "code": "x"}}, ConflictFail)
		}},
		{"UNIQUE REPLACE, upsert", ErrNotUnique, func() (WriteResult, error) {
			return p.Insert(ctx, limit.Interactive, src, "ur", []Row{{"id": 2, "code": "x"}}, ConflictUpsert)
		}},
		{"UNIQUE REPLACE, upsert of a new row", ErrNotUnique, func() (WriteResult, error) {
			return p.Insert(ctx, limit.Interactive, src, "ur", []Row{{"id": 3, "code": "x"}}, ConflictUpsert)
		}},
		{"UNIQUE REPLACE, update", ErrNotUnique, func() (WriteResult, error) {
			return p.Update(ctx, limit.Interactive, src, UpdateReq{Table: "ur", Set: Row{"code": "x"}, Keys: []Row{{"id": 2}}})
		}},
		{"PK IGNORE, insert", ErrRowExists, func() (WriteResult, error) {
			return p.Insert(ctx, limit.Interactive, src, "ig", []Row{{"id": 1, "code": "z"}}, ConflictFail)
		}},
		{"UNIQUE IGNORE, insert", ErrNotUnique, func() (WriteResult, error) {
			return p.Insert(ctx, limit.Interactive, src, "ig", []Row{{"id": 3, "code": "x"}}, ConflictFail)
		}},
		{"UNIQUE IGNORE, update", ErrNotUnique, func() (WriteResult, error) {
			return p.Update(ctx, limit.Interactive, src, UpdateReq{Table: "ig", Set: Row{"code": "x"}, Keys: []Row{{"id": 2}}})
		}},
		{"NOT NULL IGNORE, update", ErrInvalid, func() (WriteResult, error) {
			return p.Update(ctx, limit.Interactive, src, UpdateReq{Table: "ig", Set: Row{"n": nil}, Keys: []Row{{"id": 1}}})
		}},
	} {
		if r, err := c.do(); !errors.Is(err, c.err) {
			t.Errorf("%s: %+v, %v, want %v", c.what, r, err, c.err)
		}
		if now := dump(t, p, tables...); now != was || len(changeList(t, p)) != n {
			t.Errorf("%s changed something:\n%s\nwant:\n%s", c.what, now, was)
		}
	}
	// The conflict rules still work as they say.
	if r, err := p.Insert(ctx, limit.Interactive, src, "kr", []Row{{"id": "a", "v": 2}}, ConflictUpsert); err != nil || r.Updated != 1 {
		t.Errorf("upsert: %+v, %v", r, err)
	}
	if r, err := p.Insert(ctx, limit.Interactive, src, "ig", []Row{{"id": 1, "code": "z"}}, ConflictSkip); err != nil || r.Skipped != 1 {
		t.Errorf("skip: %+v, %v", r, err)
	}
}

// Temporary triggers on a table are refused like others, and a deferred
// foreign key that fails at commit is the caller's error, not the
// database's.
func TestTempTriggersAndDeferredKeys(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE tt (id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `CREATE TEMP TRIGGER tt_ins AFTER INSERT ON main.tt BEGIN SELECT 1; END`)
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "tt", []Row{{"id": 1}}, ConflictFail); !errors.Is(err, ErrNotAgentTable) || !strings.Contains(err.Error(), "triggers") {
		t.Errorf("a table with a temporary trigger: %v", err)
	}
	mustExec(t, p.DB, `DROP TRIGGER temp.tt_ins`)

	mustExec(t, p.DB, `CREATE TABLE dp (id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `CREATE TABLE dc (id INTEGER PRIMARY KEY, pid INTEGER REFERENCES dp DEFERRABLE INITIALLY DEFERRED)`)
	r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "dc", []Row{{"id": 1, "pid": 99}}, ConflictFail)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Errorf("a deferred foreign key: %+v, %v", r, err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM dc") != 0 || len(changeList(t, p)) != 0 {
		t.Error("the failed commit kept something")
	}
	if s := p.DB.Stats().LastError; s != "" {
		t.Errorf("a refused write counts as a database error: %s", s)
	}
	if r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "tt", []Row{{"id": 1}}, ConflictFail); err != nil || r.Inserted != 1 {
		t.Errorf("the writer after a failed commit: %+v, %v", r, err)
	}
}
