package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

func TestAffinity(t *testing.T) {
	for _, c := range []struct {
		decl   string
		strict bool
		want   string
	}{
		{"INTEGER", false, "INTEGER"},
		{"BLOBINT", false, "INTEGER"}, // INT comes before BLOB
		{"CHARINT", false, "INTEGER"}, // and before CHAR
		{"FLOATING POINT", false, "INTEGER"},
		{"VARCHAR(10)", false, "TEXT"},
		{"TEXTBLOB", false, "TEXT"},
		{"BLOB", false, ""},
		{"", false, ""},
		{"DOUBLE PRECISION", false, "REAL"},
		{"STRING", false, "NUMERIC"},
		{"DECIMAL(10,5)", false, "NUMERIC"},
		{"ANY", false, "NUMERIC"},
		{"ANY", true, ""},
		{"INT", true, "INTEGER"},
		{"TEXT", true, "TEXT"},
	} {
		if got := affinity(c.decl, c.strict); got != c.want {
			t.Errorf("affinity(%q, strict %v) = %q, want %q", c.decl, c.strict, got, c.want)
		}
	}

	// The pre-check sees 1 and '1' as one key in a BLOBINT column too.
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE bi (k BLOBINT PRIMARY KEY, v TEXT)`)
	rows := make([]Row, 600)
	for i := range rows {
		rows[i] = Row{"k": i, "v": "x"}
	}
	rows[550] = Row{"k": "1", "v": "x"}
	if _, err := p.Insert(context.Background(), limit.Background, id.SourceUser, "bi", rows, ConflictFail); !errors.Is(err, ErrRowExists) ||
		!strings.Contains(err.Error(), "row 550") {
		t.Errorf("1 and '1' in a BLOBINT key: %v", err)
	}
}

// Only an INTEGER PRIMARY KEY without its own index is the rowid; any other
// key must be given, as SQLite would store NULL for it.
func TestRowidKeys(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE kd (id INTEGER PRIMARY KEY DESC, v INTEGER)`)
	mustExec(t, p.DB, `CREATE TABLE ka (id INTEGER, v INTEGER, PRIMARY KEY (id DESC))`) // the rowid, by SQLite's rules
	if r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "kd", []Row{{"v": 1}, {"v": 2}}, ConflictFail); !errors.Is(err, ErrInvalid) ||
		!strings.Contains(err.Error(), "key column id is missing") || r.Inserted != 0 {
		t.Errorf("INTEGER PRIMARY KEY DESC left out: %+v, %v", r, err)
	}
	if r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "kd", []Row{{"id": 1, "v": 1}, {"id": nil, "v": 2}}, ConflictFail); !errors.Is(err, ErrInvalid) || r.Inserted != 0 {
		t.Errorf("a NULL key: %+v, %v", r, err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM kd") != 0 {
		t.Error("a row without a key went in")
	}
	r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "ka", []Row{{"v": 1}, {"v": 2}}, ConflictFail)
	if err != nil || r.Inserted != 2 {
		t.Fatalf("a rowid key left out: %+v, %v", r, err)
	}
	if es := entriesOf(t, p, r.Change); len(es) != 2 || es[0].Key != "[1]" || es[1].Key != "[2]" {
		t.Errorf("entries %+v", es)
	}

	// An action that would set a child's key is refused: the change log
	// knows rows by their key.
	mustExec(t, p.DB, `CREATE TABLE kp (id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `CREATE TABLE kc (pid INTEGER REFERENCES kp ON DELETE SET NULL, n INTEGER, PRIMARY KEY (pid, n))`)
	mustExec(t, p.DB, `INSERT INTO kp VALUES (1)`)
	mustExec(t, p.DB, `INSERT INTO kc VALUES (1, 1)`)
	if _, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "kp", []Row{{"id": 1}}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "primary key") {
		t.Errorf("SET NULL on a key column: %v", err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM kc WHERE pid = 1") != 1 {
		t.Error("the refused delete changed something")
	}
}

// Rows whose referenced values stay the same walk nowhere, however many
// rows reference them.
func TestUpsertKeepsReferencedValues(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE coin (id INTEGER PRIMARY KEY, code TEXT UNIQUE, price REAL)`)
	mustExec(t, p.DB, `CREATE TABLE trade (id INTEGER PRIMARY KEY, code TEXT REFERENCES coin (code) ON UPDATE CASCADE)`)
	mustExec(t, p.DB, `CREATE INDEX trade_code ON trade (code)`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 500)
		INSERT INTO coin SELECT i, printf('c%d', i), i FROM n`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 12500 - 1)
		INSERT INTO trade SELECT i, printf('c%d', i % 500 + 1) FROM n`)
	rows := make([]Row, 500)
	for i := range rows {
		rows[i] = Row{"id": i + 1, "code": fmt.Sprintf("c%d", i+1), "price": 2.5}
	}
	r, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "coin", rows, ConflictUpsert)
	if err != nil || r.Updated != 500 || r.Cascaded != 0 {
		t.Fatalf("upsert of the price: %+v, %v", r, err)
	}
	keys := make([]Row, 500)
	for i := range keys {
		keys[i] = Row{"id": i + 1}
	}
	if r, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "coin", Set: Row{"price": 3}, Keys: keys}); err != nil || r.Cascaded != 0 {
		t.Fatalf("update of the price: %+v, %v", r, err)
	}
	// One row that really changes its code takes its 25 trades with it.
	rows[7]["code"] = "new"
	r, err = p.Insert(ctx, limit.Interactive, id.SourceUser, "coin", rows, ConflictUpsert)
	if err != nil || r.Cascaded != 25 || one[int](t, p.DB, "SELECT count(*) FROM trade WHERE code = 'new'") != 25 {
		t.Fatalf("upsert of a code: %+v, %v", r, err)
	}
	if r, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "coin", Set: Row{"code": "c8"}, Keys: []Row{{"id": 8}}}); err != nil || r.Cascaded != 25 {
		t.Fatalf("update of a code: %+v, %v", r, err)
	}
}

// A BLOB is always a change: JSON would hold it as base64 text, which may
// even equal the stored value.
func TestBlobValueCascades(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE bx (id INTEGER PRIMARY KEY, x UNIQUE)`)
	mustExec(t, p.DB, `CREATE TABLE by (id INTEGER PRIMARY KEY, px REFERENCES bx (x) ON UPDATE CASCADE)`)
	mustExec(t, p.DB, `INSERT INTO bx VALUES (1, 'AQI=')`)
	mustExec(t, p.DB, `INSERT INTO by VALUES (10, 'AQI=')`)
	r, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "bx", Set: Row{"x": []byte{1, 2}}, Keys: []Row{{"id": 1}}})
	if err != nil || r.Cascaded != 1 || one[string](t, p.DB, "SELECT hex(px) FROM by") != "0102" {
		t.Fatalf("update: %+v, %v", r, err)
	}
	if es := entriesOf(t, p, r.Change); len(es) != 2 || es[0].Table != "by" || es[0].Kind != "u" {
		t.Errorf("entries %+v", es)
	}
	// The same by upsert, back to text and then to the BLOB again.
	mustExec(t, p.DB, `UPDATE bx SET x = 'AQI='`)
	r, err = p.Insert(ctx, limit.Interactive, id.SourceUser, "bx", []Row{{"id": 1, "x": []byte{1, 2}}}, ConflictUpsert)
	if err != nil || r.Cascaded != 1 {
		t.Fatalf("upsert: %+v, %v", r, err)
	}
}

// The cap counts each row each foreign key reaches, so many foreign keys
// to one parent can't make a write slow.
func TestCascadeManyKeys(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE hub (id INTEGER PRIMARY KEY)`)
	cols := make([]string, 30)
	for i := range cols {
		cols[i] = fmt.Sprintf("r%d INTEGER REFERENCES hub ON DELETE SET NULL", i)
	}
	mustExec(t, p.DB, `CREATE TABLE spoke (id INTEGER PRIMARY KEY, `+strings.Join(cols, ", ")+`)`)
	vals := strings.TrimSuffix(strings.Repeat("1, ", 30), ", ")
	mustExec(t, p.DB, `INSERT INTO hub VALUES (1)`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 9000)
		INSERT INTO spoke SELECT i, `+vals+` FROM n`)
	start := time.Now()
	_, err := p.Delete(ctx, limit.Interactive, id.SourceUser, "hub", []Row{{"id": 1}})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "row changes") || !strings.Contains(err.Error(), "spoke") {
		t.Fatalf("30 foreign keys to 9,000 rows: %v", err)
	}
	t.Logf("refused in %v: %v", time.Since(start).Round(time.Millisecond), err)
	if one[int](t, p.DB, "SELECT count(*) FROM spoke WHERE r0 = 1") != 9000 {
		t.Error("the refused delete changed something")
	}
}

// The pre-check covers UNIQUE indexes too, declared or created, so a
// clash in a later chunk writes nothing. A partial index is only checked
// by the write, so the chunks before it stay and are counted.
func TestPrecheckUnique(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE uq (id INTEGER PRIMARY KEY, code TEXT UNIQUE, w TEXT, x INTEGER)`,
		`CREATE UNIQUE INDEX uq_w ON uq (w COLLATE NOCASE)`,
		`CREATE UNIQUE INDEX uq_x ON uq (x) WHERE x > 0`,
		`INSERT INTO uq VALUES (100000, 'taken', 'Taken', NULL)`,
	} {
		mustExec(t, p.DB, q)
	}
	fresh := func() []Row {
		rows := make([]Row, 600)
		for i := range rows {
			rows[i] = Row{"id": i, "code": fmt.Sprintf("c%d", i)} // w and x left out: NULL
		}
		return rows
	}
	for _, c := range []struct {
		what, want string
		row        int
		edit       func(rows []Row)
	}{
		{"a code given twice", "uq.code", 599, func(rows []Row) { rows[599]["code"] = "c3" }},
		{"a taken code", "uq.code", 550, func(rows []Row) { rows[550]["code"] = "taken" }},
		{"w in two cases", "uq.w", 560, func(rows []Row) { rows[10]["w"] = "a"; rows[560]["w"] = "A" }},
		{"a taken w", "uq.w", 570, func(rows []Row) { rows[570]["w"] = "TAKEN" }},
	} {
		rows := fresh()
		c.edit(rows)
		r, err := p.Insert(ctx, limit.Background, id.SourceUser, "uq", rows, ConflictFail)
		if !errors.Is(err, ErrNotUnique) || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), fmt.Sprintf("row %d", c.row)) ||
			r.Inserted != 0 {
			t.Errorf("%s: %+v, %v", c.what, r, err)
		}
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM uq"); n != 1 {
		t.Fatalf("%d rows written before the check failed", n)
	}

	// With ConflictSkip only rows that go in count: one whose key is taken
	// is skipped, whatever its code.
	rows := fresh()
	rows[0] = Row{"id": 100000, "code": "c5"}
	rows[540]["code"] = "taken"
	if r, err := p.Insert(ctx, limit.Background, id.SourceUser, "uq", rows, ConflictSkip); !errors.Is(err, ErrNotUnique) || r.Inserted != 0 {
		t.Errorf("skip, a new row with a taken code: %+v, %v", r, err)
	}
	rows[540]["code"] = "c540"
	if r, err := p.Insert(ctx, limit.Background, id.SourceUser, "uq", rows, ConflictSkip); err != nil || r.Inserted != 599 || r.Skipped != 1 {
		t.Errorf("skip, a skipped row with a taken code: %+v, %v", r, err)
	}

	// A clash in a partial index fails the second chunk only.
	rows = fresh()
	for i := range rows {
		rows[i]["id"] = 200000 + i
		rows[i]["code"] = fmt.Sprintf("p%d", i)
	}
	rows[3]["x"], rows[590]["x"] = 5, 5
	r, err := p.Insert(ctx, limit.Background, id.SourceUser, "uq", rows, ConflictFail)
	if !errors.Is(err, ErrNotUnique) || r.Inserted != 500 || r.Change == "" {
		t.Errorf("a partial index: %+v, %v", r, err)
	}
}
