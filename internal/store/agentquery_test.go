package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// TestMain lets the test binary be its own query worker.
func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(m.Run())
}

// agentTables is SPIKE-007's project on top of project.db's own tables:
// one more internal table, a row in the change log, and agent tables with
// 2,000 and 100 rows.
var agentTables = []string{
	"CREATE TABLE _jenab_flags(name TEXT PRIMARY KEY) WITHOUT ROWID",
	"CREATE TABLE data(id INTEGER PRIMARY KEY, name TEXT, val REAL, tags TEXT)",
	"CREATE INDEX data_name ON data(name)",
	"CREATE TABLE child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES data(id), note TEXT)",
	"INSERT INTO _jenab_meta VALUES ('project_id', '01J'), ('schema_version', '3')",
	"INSERT INTO _jenab_changes(id, at, source, target, op) VALUES ('01J', '2026-10-18T08:00:00Z', 'user', 'table:data', 'insert')",
	"INSERT INTO _jenab_flags VALUES ('a')",
	`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 2000)
	 INSERT INTO data(name, val, tags) SELECT 'n' || (i % 50), i * 1.5, '["a","b"]' FROM n`,
	`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100)
	 INSERT INTO child(parent_id, note) SELECT i, 'c' FROM n`,
}

// openAgentProject opens a project with the given number of readers and
// SPIKE-007's tables.
func openAgentProject(t testing.TB, readers int) (*ProjectDB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(context.Background(), filepath.Join(dir, "project.db"), Options{
		Format:        ProjectFormat,
		Snapshots:     filepath.Join(dir, "snapshots"),
		Readers:       readers,
		WriterPragmas: projectPragmas,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := newProjectDB(db, dir)
	t.Cleanup(func() { p.Close(context.Background()) })
	for _, s := range agentTables {
		mustExec(t, p.DB, s)
	}
	// Start a worker, so that tests with short deadlines don't wait for
	// one: the first start of a newly built test binary can take a second
	// while the virus scanner reads it.
	start := time.Now()
	if _, err := p.AgentQuery(context.Background(), "SELECT 1", nil, 1); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Logf("the first worker took %v to start", d)
	}
	return p, dir
}

// fingerprint sums up the data and the schema, to see any change.
func fingerprint(t testing.TB, p *ProjectDB) string {
	t.Helper()
	return one[string](t, p.DB, `SELECT
		(SELECT count(*) || ':' || total(val) || ':' || coalesce(sum(length(name)), 0) FROM data) || '|' ||
		(SELECT count(*) FROM _jenab_meta) || '|' || (SELECT count(*) FROM _jenab_changes) || '|' ||
		(SELECT count(*) FROM child) || '|' ||
		(SELECT group_concat(name) FROM (SELECT name FROM sqlite_schema ORDER BY name))`)
}

// agentCases are SPIKE-007's 11 normal queries and 52 attacks. layer is
// the first layer that stops the query, 0 for one that runs; rows is what
// a normal query returns with max 200. {OTHER} and {DIR} become paths.
var agentCases = []struct {
	kind  string
	q     string
	layer int
	rows  int
	args  []any
}{
	{"allowed", "SELECT * FROM data", 0, 200, nil},
	{"allowed", "SELECT name, avg(val) AS a FROM data WHERE val > :v GROUP BY name ORDER BY a DESC LIMIT 10", 0, 10, []any{sql.Named("v", 10)}},
	{"allowed", "WITH t AS (SELECT * FROM data) SELECT count(*) FROM t", 0, 1, nil},
	{"allowed", "SELECT d.name, c.note FROM data d JOIN child c ON c.parent_id = d.id", 0, 100, nil},
	{"allowed", "SELECT * FROM data WHERE name = 'a;b'", 0, 0, nil},
	{"allowed", "SELECT * FROM data WHERE name = '_jenab_meta'", 0, 0, nil},
	{"allowed", "SELECT 1;", 0, 1, nil},
	{"allowed", "SELECT \"name\" FROM \"data\" /* note */ -- trailing comment", 0, 200, nil},
	{"allowed", "SELECT d.id, j.value FROM data d, json_each(d.tags) j LIMIT 5", 0, 5, nil},
	{"allowed", "VALUES (1), (2)", 0, 2, nil},
	{"allowed", "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100) SELECT sum(i) FROM n", 0, 1, nil},

	// Reading internal tables
	{"internal read", "SELECT * FROM _jenab_meta", 1, 0, nil},
	{"internal read", "SELECT * FROM \"_jenab_meta\"", 1, 0, nil},
	{"internal read", "SELECT * FROM [_JENAB_META]", 1, 0, nil},
	{"internal read", "SELECT * FROM `_jenab_meta`", 1, 0, nil},
	{"internal read", "SELECT * FROM main._jenab_meta", 1, 0, nil},
	{"internal read", "WITH x AS (SELECT * FROM _jenab_meta) SELECT * FROM x", 1, 0, nil},
	{"internal read", "SELECT * FROM (SELECT * FROM _jenab_meta)", 1, 0, nil},
	{"internal read", "SELECT * FROM data WHERE id IN (SELECT id FROM _jenab_changes)", 1, 0, nil},
	{"internal read", "SELECT * FROM data WHERE name IN _jenab_flags", 1, 0, nil},
	{"internal read", "SELECT (SELECT value FROM _jenab_meta LIMIT 1) FROM data", 1, 0, nil},
	{"internal read", "SELECT name FROM data UNION SELECT key FROM _jenab_meta", 1, 0, nil},
	{"internal read", "SELECT * FROM data ORDER BY (SELECT count(*) FROM _jenab_changes)", 1, 0, nil},
	{"internal read", "SELECT * FROM data WHERE EXISTS (SELECT 1 FROM _jenab_meta WHERE key = 'x')", 1, 0, nil},
	{"internal read", "SELECT key FROM _jenab_meta WHERE key = 'project_id'", 1, 0, nil},
	{"schema read", "SELECT * FROM sqlite_schema", 1, 0, nil},
	{"schema read", "SELECT * FROM sqlite_master", 1, 0, nil},
	{"schema read", "SELECT * FROM main.sqlite_schema", 1, 0, nil},
	{"schema read", "SELECT * FROM sqlite_temp_schema", 1, 0, nil},

	// Schema information through virtual tables
	{"virtual table", "SELECT * FROM pragma_table_info('_jenab_meta')", 1, 0, nil},
	{"virtual table", "SELECT * FROM \"pragma_table_info\"('_jenab_meta')", 1, 0, nil},
	{"virtual table", "SELECT * FROM pragma_table_list", 1, 0, nil},
	{"virtual table", "SELECT * FROM pragma_database_list", 1, 0, nil},
	{"virtual table", "SELECT * FROM dbstat", 1, 0, nil},
	{"virtual table", "SELECT * FROM sqlite_dbpage", 1, 0, nil},
	{"virtual table", "SELECT * FROM bytecode('SELECT * FROM _jenab_meta')", 1, 0, nil},

	// Writes
	{"write", "DELETE FROM data", 1, 0, nil},
	{"write", "UPDATE data SET name = 'x'", 1, 0, nil},
	{"write", "REPLACE INTO data(id, name) VALUES (1, 'x')", 1, 0, nil},
	{"write", "WITH x AS (SELECT 1) DELETE FROM data", 2, 0, nil},
	{"write", "WITH x AS (SELECT 1) INSERT INTO data(name) VALUES ('x')", 2, 0, nil},
	{"write", "CREATE TEMP TABLE t AS SELECT * FROM _jenab_meta", 1, 0, nil},
	{"write", "CREATE TEMP VIEW v AS SELECT * FROM data", 1, 0, nil},

	// Connection state and files
	{"connection", "PRAGMA query_only = 0", 1, 0, nil},
	{"connection", "PRAGMA writable_schema = 1", 1, 0, nil},
	{"connection", "ATTACH '{OTHER}' AS o", 1, 0, nil},
	{"connection", "VACUUM INTO '{DIR}/copy.db'", 1, 0, nil},
	{"connection", "BEGIN IMMEDIATE", 1, 0, nil},
	{"connection", "ANALYZE", 1, 0, nil},

	// A second statement hidden after the first
	{"multi-statement", "SELECT 1; DELETE FROM data", 1, 0, nil},
	{"multi-statement", "SELECT 1; PRAGMA query_only = 0", 1, 0, nil},
	{"multi-statement", "SELECT ';'; DELETE FROM data", 1, 0, nil},
	{"multi-statement", "SELECT 1 /* ; */; DELETE FROM data", 1, 0, nil},
	{"multi-statement", "SELECT 1 -- x\n; DELETE FROM data", 1, 0, nil},
	{"multi-statement", "SELECT 1; ATTACH '{OTHER}' AS o", 1, 0, nil},
	{"multi-statement", "SELECT 1\x00; DELETE FROM data", 1, 0, nil},

	// Dangerous functions
	{"function", "SELECT load_extension('evil')", 1, 0, nil},
	{"function", "SELECT fts3_tokenizer('simple')", 1, 0, nil},

	// Resource exhaustion: the deadline and the length limit
	{"exhaustion", "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r", 3, 0, nil},
	{"exhaustion", "SELECT count(*) FROM data a, data b, data c, data d", 3, 0, nil},
	{"exhaustion", "SELECT length(randomblob(200000000))", 3, 0, nil},
	{"exhaustion", "SELECT length(printf('%.*c', 200000000, 'x'))", 3, 0, nil},
	{"exhaustion", "WITH RECURSIVE r(i, s) AS (SELECT 1, 'x' UNION ALL SELECT i+1, s || s FROM r WHERE i < 40) SELECT max(length(s)) FROM r", 3, 0, nil},
}

func TestAgentQueryCases(t *testing.T) {
	if n := len(agentCases); n != 63 {
		t.Fatalf("%d cases, want SPIKE-007's 63", n)
	}
	p, dir := openAgentProject(t, 4)
	other, err := sql.Open("sqlite", dsn(filepath.Join(dir, "other.db"), false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Exec("CREATE TABLE secret(x); INSERT INTO secret VALUES ('other project data')"); err != nil {
		t.Fatal(err)
	}
	other.Close()
	paths := strings.NewReplacer("{OTHER}", filepath.ToSlash(filepath.Join(dir, "other.db")), "{DIR}", filepath.ToSlash(dir))
	base := fingerprint(t, p)

	for i, c := range agentCases {
		t.Run(fmt.Sprintf("%02d %s", i+1, c.kind), func(t *testing.T) {
			// The exhaustion cases stop after 1 s instead of 10.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, err := p.AgentQuery(ctx, paths.Replace(c.q), c.args, 200)
			var re *sqlguard.RejectError
			switch {
			case c.layer == 0 && err != nil:
				t.Errorf("%s: %v", c.q, err)
			case c.layer == 0 && len(r.Values) != c.rows:
				t.Errorf("%s: %d rows, want %d", c.q, len(r.Values), c.rows)
			case c.layer == 1 || c.layer == 2:
				if !errors.As(err, &re) || re.Layer != c.layer {
					t.Errorf("%s: %v, want layer %d", c.q, err, c.layer)
				}
			case c.layer == 3:
				if err == nil || errors.As(err, &re) {
					t.Errorf("%s: %v, want an error from the reader", c.q, err)
				}
			}
			if f := fingerprint(t, p); f != base {
				t.Fatalf("%s changed the project: %s", c.q, f)
			}
		})
	}
}

func TestAgentQueryDeadline(t *testing.T) {
	// One reader, so the next query gets the same connection back.
	p, _ := openAgentProject(t, 1)
	ctx := context.Background()
	for _, q := range []string{
		// All the work is in the first row.
		"WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r",
		// The first row is quick, the second never comes: without Wrap,
		// modernc doesn't interrupt it.
		"WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT i FROM r WHERE i = 1 OR i = 1000000000000",
	} {
		start := time.Now()
		_, err := p.guardedQuery(ctx, q, nil, 200, 300*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "ran longer than 300ms") {
			t.Errorf("%s: %v", q, err)
		}
		if el := time.Since(start); el > 3*time.Second {
			t.Errorf("%s: stopped after %v", q, el)
		}
		r, err := p.AgentQuery(ctx, "SELECT count(*) FROM data", nil, 10)
		if err != nil || len(r.Values) != 1 || r.Values[0][0] != int64(2000) {
			t.Fatalf("next query: %v, %v", r, err)
		}
		// The reader holds no read transaction, so a checkpoint finishes
		// after a write.
		mustExec(t, p.DB, "INSERT INTO child(note) VALUES ('after')")
		if ok, err := p.Checkpoint(ctx); !ok || err != nil {
			t.Errorf("checkpoint after a stopped query: %v, %v", ok, err)
		}
	}
	// A caller that gives up gets its own error.
	cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if _, err := p.AgentQuery(cctx, "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r", nil, 1); err != context.DeadlineExceeded {
		t.Errorf("caller's deadline: %v", err)
	}
}

func TestAgentQueryTotal(t *testing.T) {
	p, _ := openAgentProject(t, 2)
	ctx := context.Background()
	cases := []struct {
		q      string
		args   []any
		max    int
		rows   int
		total  int
		cut    bool
		capped bool
	}{
		{"SELECT * FROM data", nil, 200, 200, 2000, true, false},
		{"SELECT * FROM child", nil, 200, 100, 100, false, false},
		{"SELECT * FROM data WHERE id <= 200", nil, 200, 200, 200, false, false},
		{"SELECT * FROM data WHERE id < 0", nil, 200, 0, 0, false, false},
		{"WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT i FROM r", nil, 10, 10, sqlworker.CountCap, true, true},
		{"SELECT name FROM data GROUP BY name", nil, 0, 1, 50, true, false},
		// Both copies of the query share the parameters.
		{"SELECT id FROM data WHERE id IN (?, ?) ORDER BY id", []any{3, 5}, 1, 1, 2, true, false},
		{"SELECT id FROM data WHERE id IN (?2, ?) OR id = :n ORDER BY id;", []any{3, 5, 9, sql.Named("n", 7)}, 1, 1, 3, true, false},
		{"SELECT id FROM data WHERE name = :n AND id > ?", []any{sql.Named("n", "n1"), 1000}, 3, 3, 20, true, false},
		// "id:1" is a string here, not the column of Wrap's CTE: the count
		// sees what the query alone sees.
		{`SELECT d.id, c.id FROM data d JOIN child c ON c.parent_id = d.id WHERE "id:1" = 'id:1'`, nil, 1, 1, 100, true, false},
	}
	for _, c := range cases {
		r, err := p.AgentQuery(ctx, c.q, c.args, c.max)
		if err != nil {
			t.Errorf("%s: %v", c.q, err)
			continue
		}
		if len(r.Values) != c.rows || r.Total != c.total || r.Cut != c.cut || r.Capped != c.capped {
			t.Errorf("%s: %d rows, total %d, cut %v, capped %v; want %d, %d, %v, %v",
				c.q, len(r.Values), r.Total, r.Cut, r.Capped, c.rows, c.total, c.cut, c.capped)
		}
	}
}

func TestAgentQueryRows(t *testing.T) {
	p, _ := openAgentProject(t, 2)
	ctx := context.Background()

	r, err := p.AgentQuery(ctx, "SELECT id, val FROM data ORDER BY val DESC -- newest first", nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, v := range r.Values {
		ids = append(ids, v[0].(int64))
	}
	if !slices.Equal(r.Columns, []string{"id", "val"}) || !slices.Equal(ids, []int64{2000, 1999, 1998}) {
		t.Errorf("columns %v, ids %v", r.Columns, ids)
	}

	r, err = p.AgentQuery(ctx, "SELECT 1 AS i, 1.5 AS f, 'بیت‌کوین' AS s, x'00ff' AS b, NULL AS n", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if v := r.Values[0]; v[0] != int64(1) || v[1] != 1.5 || v[2] != "بیت‌کوین" || string(v[3].([]byte)) != "\x00\xff" || v[4] != nil {
		t.Errorf("values %#v", v)
	}

	r, err = p.AgentQuery(ctx, "SELECT d.id, c.id FROM data d JOIN child c ON c.parent_id = d.id", nil, 1)
	if err != nil || !slices.Equal(r.Columns, []string{"id", "id:1"}) {
		t.Errorf("duplicate names: %v, %v", r.Columns, err)
	}

	// SQLite's own errors pass through with their text.
	_, err = p.AgentQuery(ctx, "SELECT nope FROM data", nil, 10)
	var re *sqlguard.RejectError
	if err == nil || errors.As(err, &re) || !strings.Contains(err.Error(), "no such column: nope") {
		t.Errorf("SQLite error: %v", err)
	}

	// The ticket's example of an error the model can fix.
	_, err = p.AgentQuery(ctx, "SELECT * FROM _jenab_changes", nil, 10)
	if err == nil || err.Error() != "line 1:15: `_jenab_changes` is internal" {
		t.Errorf("layer 1 error: %v", err)
	}
}

const bom = "\xef\xbb\xbf" // U+FEFF, white space to SQLite at a token's start

// guardAttacks are queries that got past the first version of the guard.
var guardAttacks = []string{
	// '#' starts a parameter, and :a(...) is a Tcl-style one: SQLite read
	// more than one statement where layer 1 saw a string, and EXPLAIN ran
	// them all.
	"SELECT :a('), #a(') ; PRAGMA query_only=0; INSERT INTO data(name) SELECT value FROM _jenab_meta; COMMIT; SELECT 1 --'",
	"SELECT :a('), 1 ; PRAGMA query_only=0; DELETE FROM data; COMMIT; SELECT 1 --'",
	"SELECT $a::b(') ; DELETE FROM data; SELECT 1 --'",
	"SELECT #a",
	// A BOM is white space to SQLite but was part of the name to layer 1.
	"SELECT * FROM " + bom + "_jenab_meta",
	"SELECT * FROM " + bom + "pragma_table_info('_jenab_meta')",
	"SELECT * FROM " + bom + "pragma_database_list",
	"SELECT * FROM " + bom + "pragma_table_list",
	"SELECT * FROM " + bom + "dbstat",
	"SELECT 1;" + bom + "PRAGMA query_only = 0",
	// json_each as a name let any virtual table through layer 2.
	"SELECT 1 AS json_each, * FROM " + bom + "dbstat",
	"SELECT 1 AS json_each, * FROM notes",
	"SELECT * FROM json_each('[1]'), notes",
	"SELECT * FROM notes, json_tree('{}')",
	// rtreecheck runs SQL of its own on any table, which EXPLAIN doesn't show.
	"SELECT rtreecheck('_jenab_meta')",
	"SELECT \"rtreecheck\"('_jenab' || '_meta')",
}

// cteChain and cteDoubling are sqlguard_test.go's: n CTEs, each using the
// one before once or twice.
func cteChain(n int) string {
	var b strings.Builder
	b.WriteString("WITH c0 AS (SELECT 1 AS x)")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, ", c%d AS (SELECT x FROM c%d)", i, i-1)
	}
	fmt.Fprintf(&b, " SELECT * FROM c%d", n-1)
	return b.String()
}

func cteDoubling(n int) string {
	var b strings.Builder
	b.WriteString("WITH a0 AS (SELECT 1 AS x)")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, ", a%d AS (SELECT (SELECT x FROM a%d) + (SELECT x FROM a%d) AS x)", i, i-1, i-1)
	}
	fmt.Fprintf(&b, " SELECT x FROM a%d", n)
	return b.String()
}

// TestAgentQueryPrepareCost: the review's queries that took 31 s and 3 s
// to prepare, past the deadline, are refused at once by layer 1's budget.
func TestAgentQueryPrepareCost(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	for _, q := range []string{cteChain(8000), cteChain(150), cteDoubling(14), cteDoubling(16)} {
		start := time.Now()
		_, err := p.guardedQuery(context.Background(), q, nil, 10, 2*time.Second)
		var re *sqlguard.RejectError
		if !errors.As(err, &re) || re.Layer != 1 {
			t.Errorf("%.40s: %v, want layer 1", q, err)
		}
		if d := time.Since(start); d > 100*time.Millisecond {
			t.Errorf("%.40s: %v", q, d)
		}
	}
	// What the budget lets through runs, well within a view's deadline.
	for _, q := range []string{cteChain(100), cteDoubling(8)} {
		start := time.Now()
		if _, err := p.guardedQuery(context.Background(), q, nil, 10, 2*time.Second); err != nil {
			t.Errorf("%.40s: %v", q, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%.40s: %v", q, d)
		}
	}
}

// withMemory gives p's agent queries workers capped at n bytes.
func withMemory(p *ProjectDB, n uint64) {
	p.workers.Close()
	p.workers = sqlworker.New(p.Path(), sqlworker.Options{Memory: n})
}

// nest is the third review's flattening blowup: each level uses the one
// below twice, and SQLite copies it into both when it flattens the
// subqueries, so the prepare doubles with each level.
func nest(depth int) string {
	q := "SELECT 1 AS a FROM data WHERE id = 1"
	for range depth {
		q = "SELECT a+a AS a FROM (" + q + ")"
	}
	return q
}

// resourceAttacks are the third review's queries that took seconds or
// gigabytes inside the app; in a worker each ends at its deadline or at
// the memory cap. Their arguments are a 1 MB string.
var resourceAttacks = []struct{ name, q string }{
	{"flattening 19", nest(19)},
	{"flattening 20", nest(20)},
	{"string CTE names", func() string {
		var b strings.Builder
		b.WriteString("WITH 'a0' AS (SELECT 1 AS x)")
		for i := 1; i <= 16; i++ {
			fmt.Fprintf(&b, ", 'a%d' AS (SELECT (SELECT x FROM 'a%d') + (SELECT x FROM 'a%d') AS x)", i, i-1, i-1)
		}
		return b.String() + " SELECT x FROM 'a16'"
	}()},
	{"jsonb_group_array", "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r WHERE i < 300) SELECT length(jsonb_group_array(printf('%.*c', 1000000, 'x'))) FROM r"},
	{"json_group_array", "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r WHERE i < 300) SELECT length(json_group_array(printf('%.*c', 1000000, 'x'))) FROM r"},
	{"instr", "SELECT instr(printf('%.*c', 200000, 'x'), printf('%.*c', 60000, 'x') || 'y')"},
	{"replace", "SELECT length(replace(printf('%.*c', 900000, 'x'), printf('%.*c', 60000, 'x') || 'y', ''))"},
	{"like", "SELECT printf('%.*c', 200000, 'a') LIKE '%' || printf('%.*c', 40000, 'a') || 'b'"},
	{"variables", "SELECT length(concat(" + strings.TrimSuffix(strings.Repeat("?1, ", 1000), ", ") + "))"},
}

// TestAgentQueryResourceAttacks: each attack returns an error by its
// deadline and a little more, the app's own memory stays where it was,
// and the next query works.
func TestAgentQueryResourceAttacks(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	withMemory(p, 128<<20)
	ctx := context.Background()
	arg := []any{strings.Repeat("x", 1<<20)}
	const d = 2 * time.Second
	if _, err := p.AgentQuery(ctx, "SELECT 1", nil, 1); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	debug.FreeOSMemory()
	before := processMemory()
	for _, a := range resourceAttacks {
		var args []any
		if strings.Contains(a.q, "?1") {
			args = arg
		}
		start := time.Now()
		_, err := p.guardedQuery(ctx, a.q, args, 10, d)
		took := time.Since(start)
		if err == nil {
			t.Errorf("%s: no error", a.name)
		}
		if took > d+time.Second {
			t.Errorf("%s: %v", a.name, took)
		}
		t.Logf("%-18s %6v  %v", a.name, took.Round(time.Millisecond), err)
		r, err := p.AgentQuery(ctx, "SELECT count(*) FROM data", nil, 1)
		if err != nil || r.Values[0][0] != int64(2000) {
			t.Fatalf("%s: next query: %v %v", a.name, r, err)
		}
	}
	runtime.GC()
	debug.FreeOSMemory()
	after := processMemory()
	if after > before+4<<20 {
		t.Errorf("the test process grew from %d to %d KiB", before>>10, after>>10)
	}
	t.Logf("test process: %d KiB before, %d KiB after", before>>10, after>>10)
}

// TestAgentQueryMemoryCap: a query over the cap is stopped, and says so.
func TestAgentQueryMemoryCap(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	withMemory(p, 128<<20)
	_, err := p.AgentQuery(context.Background(), resourceAttacks[3].q, nil, 10)
	var me *sqlworker.MemoryError
	if !errors.As(err, &me) || err.Error() != "the query used more than 128 MiB and was stopped" {
		t.Errorf("%v", err)
	}
}

// TestAgentQueryKilledReader: killing a worker in the middle of a read
// leaves the file, the WAL and the writer as they were.
func TestAgentQueryKilledReader(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	ctx := context.Background()
	done := make(chan struct{})
	writes := 0
	go func() {
		defer close(done)
		for range 50 {
			mustExec(t, p.DB, "INSERT INTO child(note) VALUES ('during')")
			writes++
		}
	}()
	for range 3 {
		_, err := p.guardedQuery(ctx, "SELECT count(*) FROM data a, data b, data c", nil, 1, 200*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("slow read: %v", err)
		}
	}
	<-done
	if ok, err := p.Checkpoint(ctx); !ok || err != nil {
		t.Errorf("checkpoint after killed readers: %v, %v", ok, err)
	}
	if n := one[int64](t, p.DB, "SELECT count(*) FROM child WHERE note = 'during'"); n != int64(writes) {
		t.Errorf("%d rows written, %d read", writes, n)
	}
	if bad, err := QuickCheck(ctx, p.Path()); bad != "" || err != nil {
		t.Errorf("quick_check: %q %v", bad, err)
	}
}

// TestAgentQueryMemory: values are at most sqlguard.AgentLength, so wide
// rows of big values fail early, and the rows handed back stop at
// sqlworker.ResultCap bytes.
func TestAgentQueryMemory(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	ctx := context.Background()
	wide := "SELECT " + strings.TrimSuffix(strings.Repeat("printf('%.*c', 2000000, 'a'), ", 40), ", ")
	_, err := p.AgentQuery(ctx, wide, nil, 10)
	var re *sqlguard.RejectError
	if err == nil || errors.As(err, &re) || !strings.Contains(err.Error(), "too big") {
		t.Errorf("40 values of 2 MB: %v", err)
	}
	// 20 rows of 900 KB: under the value limit, over resultCap together.
	r, err := p.AgentQuery(ctx, "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r WHERE i < 20) SELECT i, printf('%.*c', 900000, 'a') AS s FROM r", nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Full || !r.Cut || r.Total != 20 || len(r.Values) == 0 || len(r.Values) >= 10 {
		t.Errorf("%d rows, total %d, cut %v, full %v", len(r.Values), r.Total, r.Cut, r.Full)
	}
	// A query that needs no more values than the limit is unaffected.
	r, err = p.AgentQuery(ctx, "SELECT length(printf('%.*c', 1000000, 'a'))", nil, 1)
	if err != nil || r.Values[0][0] != int64(1000000) || r.Full {
		t.Errorf("1 MB value: %v %v", r, err)
	}
}

// TestAgentQueryParams: ?999 is the largest parameter, and binding NULL to
// all of them for EXPLAIN is quick.
func TestAgentQueryParams(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	start := time.Now()
	r, err := p.guardedQuery(context.Background(), "SELECT 1 FROM data WHERE id = ?999", make([]any, 999), 10, 2*time.Second)
	if err != nil || len(r.Values) != 0 {
		t.Errorf("?999: %v %v", r, err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("?999 took %v", d)
	}
	_, err = p.AgentQuery(context.Background(), "SELECT 1 FROM data WHERE id = ?1000", nil, 10)
	var re *sqlguard.RejectError
	if !errors.As(err, &re) || re.Layer != 1 {
		t.Errorf("?1000: %v", err)
	}
}

// TestAgentQueryJSONSchemas: json_each named with a schema, main or temp,
// is the same eponymous table (layer 2 compares its address), so it runs
// and changes nothing.
func TestAgentQueryJSONSchemas(t *testing.T) {
	p := openAttackProject(t)
	base := fingerprint(t, p)
	for _, q := range []string{
		"SELECT value FROM main.json_each('[1,2]')",
		"SELECT value FROM temp.json_each('[1,2]')",
		"SELECT j.key FROM data, temp.json_tree(data.tags) j WHERE data.id = 1 AND j.type = 'text'",
	} {
		r, err := p.AgentQuery(context.Background(), q, nil, 10)
		if err != nil || len(r.Values) != 2 {
			t.Errorf("%s: %v %v", q, r, err)
		}
	}
	if f := fingerprint(t, p); f != base {
		t.Fatalf("changed the project: %s", f)
	}
}

// openAttackProject is openAgentProject with one reader, so that
// queryOnly sees the connection the last query ran on, and an FTS5
// table: a virtual table whose name layer 1 doesn't know.
func openAttackProject(tb testing.TB) *ProjectDB {
	p, _ := openAgentProject(tb, 1)
	mustExec(tb, p.DB, "CREATE VIRTUAL TABLE notes USING fts5(body)")
	mustExec(tb, p.DB, "INSERT INTO notes VALUES ('a note')")
	return p
}

// queryOnly reads query_only on the reader as the last query left it,
// without sqlguard.Prepare setting it again.
func queryOnly(tb testing.TB, p *ProjectDB) int {
	tb.Helper()
	ctx := context.Background()
	c, err := p.readers.Conn(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	defer c.Close()
	var on int
	if err := c.QueryRowContext(ctx, "PRAGMA query_only").Scan(&on); err != nil {
		tb.Fatal(err)
	}
	return on
}

// runUnchanged runs q, if it passes layer 1, and fails when it changed the
// data, the schema or the reader's query_only.
func runUnchanged(t *testing.T, p *ProjectDB, base, q string) {
	t.Helper()
	if sqlguard.Check(q) != nil {
		return
	}
	p.guardedQuery(context.Background(), q, nil, 50, 200*time.Millisecond)
	if f := fingerprint(t, p); f != base {
		t.Fatalf("%q changed the project: %s", q, f)
	}
	if on := queryOnly(t, p); on != 1 {
		t.Fatalf("%q turned query_only off", q)
	}
}

func TestAgentQueryAttacks(t *testing.T) {
	p := openAttackProject(t)
	base := fingerprint(t, p)
	for _, q := range guardAttacks {
		_, err := p.AgentQuery(context.Background(), q, nil, 10)
		var re *sqlguard.RejectError
		if !errors.As(err, &re) {
			t.Errorf("%q: %v, want a *RejectError", q, err)
		}
		if f := fingerprint(t, p); f != base {
			t.Fatalf("%q changed the project: %s", q, f)
		}
		if on := queryOnly(t, p); on != 1 {
			t.Fatalf("%q turned query_only off", q)
		}
	}
	// Layer 1 lets these through; layer 2 refuses them by the virtual
	// table they open, not by its name.
	for _, q := range []string{"SELECT * FROM notes", "SELECT 1 AS json_each, * FROM notes", "SELECT * FROM json_each('[1]'), notes"} {
		_, err := p.AgentQuery(context.Background(), q, nil, 10)
		var re *sqlguard.RejectError
		if !errors.As(err, &re) || re.Layer != 2 {
			t.Errorf("%q: %v, want layer 2", q, err)
		}
	}
}

// TestAgentQueryByteSweep puts every byte, and a BOM, in front of the
// characters an attack hides behind. Each query is refused by layer 1 or
// runs without changing anything.
func TestAgentQueryByteSweep(t *testing.T) {
	p := openAttackProject(t)
	base := fingerprint(t, p)
	templates := []string{
		"SELECT %s ; PRAGMA query_only=0; DELETE FROM data; COMMIT; SELECT 1 --'\"",
		"SELECT :a%s ; PRAGMA query_only=0; DELETE FROM data; COMMIT; SELECT 1 --'\"",
		"SELECT * FROM %s",
		"SELECT 1 AS json_each, * FROM data WHERE name = %s",
	}
	var prefixes []string
	for b := range 256 {
		prefixes = append(prefixes, string([]byte{byte(b)}))
	}
	prefixes = append(prefixes, bom)
	passed := 0
	for _, tmpl := range templates {
		for _, target := range []string{"'", `"`, ";", "_jenab_meta"} {
			for _, pre := range prefixes {
				q := fmt.Sprintf(tmpl, pre+target)
				if sqlguard.Check(q) == nil {
					passed++
				}
				runUnchanged(t, p, base, q)
			}
		}
	}
	t.Logf("%d of %d queries passed layer 1", passed, len(templates)*4*len(prefixes))
}

// FuzzAgentQuery: whatever passes layer 1 leaves the data, the schema and
// query_only as they were. Seeded with SPIKE-007's cases and the attacks
// above; run it with go test -run '^$' -fuzz FuzzAgentQuery.
func FuzzAgentQuery(f *testing.F) {
	for _, c := range agentCases {
		f.Add(c.q)
	}
	for _, q := range guardAttacks {
		f.Add(q)
	}
	p := openAttackProject(f)
	base := fingerprint(f, p)
	f.Fuzz(func(t *testing.T, q string) {
		runUnchanged(t, p, base, q)
	})
}

// TestOddPaths: a project in a folder whose name means something in a URI
// is opened in that folder by the writer, the readers and the workers.
func TestOddPaths(t *testing.T) {
	ctx := context.Background()
	names := []string{"a#b", "%41", "with space", "پروژه‌ها"}
	if runtime.GOOS != "windows" {
		names = append(names, "what?") // Windows refuses '?' in names
	}
	for _, name := range names {
		parent := t.TempDir()
		dir := filepath.Join(parent, name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p, err := OpenProject(ctx, dir)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		mustExec(t, p.DB, "CREATE TABLE t1 (a)")
		mustExec(t, p.DB, "INSERT INTO t1 VALUES (1), (2)")
		if err := p.SetMeta(ctx, map[string]string{"name": name}); err != nil {
			t.Fatal(err)
		}
		if m, err := p.Meta(ctx); err != nil || m["name"] != name {
			t.Errorf("%s: reader: %v %v", name, m, err)
		}
		r, err := p.AgentQuery(ctx, "SELECT count(*) FROM t1", nil, 1)
		if err != nil || len(r.Values) != 1 || r.Values[0][0] != int64(2) {
			t.Errorf("%s: worker: %v %v", name, r.Values, err)
		}
		p.Close(ctx)
		if _, err := os.Stat(filepath.Join(dir, "project.db")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if es, _ := os.ReadDir(parent); len(es) != 1 || es[0].Name() != name {
			t.Errorf("%s: the parent folder holds %v", name, es)
		}
	}
}
