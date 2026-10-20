package sqlguard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Real SQLite in t.TempDir(). The full SPIKE-007 cases run through
// store.ProjectDB.AgentQuery; these test each layer on its own.

// testSchema is SPIKE-007's project, with jenab's internal prefix.
var testSchema = []string{
	"CREATE TABLE _jenab_meta(key TEXT PRIMARY KEY, value TEXT)",
	"CREATE TABLE _jenab_changes(id INTEGER PRIMARY KEY, target TEXT, op TEXT)",
	"CREATE INDEX _jenab_changes_op ON _jenab_changes(op)",
	"CREATE TABLE _jenab_flags(name TEXT PRIMARY KEY) WITHOUT ROWID",
	"CREATE TABLE data(id INTEGER PRIMARY KEY, name TEXT, val REAL, tags TEXT)",
	"CREATE INDEX data_name ON data(name)",
	"CREATE TABLE child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES data(id), note TEXT)",
	"INSERT INTO _jenab_meta VALUES ('project_id', '01J'), ('schema_version', '3')",
	"INSERT INTO _jenab_changes(target, op) VALUES ('table:data', 'insert')",
	"INSERT INTO _jenab_flags VALUES ('a')",
	`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 2000)
	 INSERT INTO data(name, val, tags) SELECT 'n' || (i % 50), i * 1.5, '["a","b"]' FROM n`,
	`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100)
	 INSERT INTO child(parent_id, note) SELECT i, 'c' FROM n`,
}

// openTest returns the database and a guarded reader connection.
func openTest(tb testing.TB) (*sql.DB, *sql.Conn) {
	tb.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(tb.TempDir(), "project.db"))+"?_pragma=journal_mode(WAL)")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	for _, s := range testSchema {
		if _, err := db.Exec(s); err != nil {
			tb.Fatalf("%s: %v", s, err)
		}
	}
	c, err := db.Conn(context.Background())
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { c.Close() })
	if err := Prepare(context.Background(), c); err != nil {
		tb.Fatal(err)
	}
	return db, c
}

func TestCheck(t *testing.T) {
	cases := []struct{ q, want string }{
		{"SELECT * FROM data", ""},
		{"SELECT 1;", ""},
		{"select 1; -- done", ""},
		{"  With t AS (SELECT 1) SELECT * FROM t", ""},
		{"VALUES (1)", ""},
		{"SELECT 'it''s _jenab_meta; sqlite_schema'", ""},
		{`SELECT "a""b" FROM data -- _jenab_meta`, ""},
		{"SELECT 1 /* ; DELETE */", ""},
		{"SELECT value FROM data, json_each(data.tags)", ""},
		{"SELECT * FROM data WHERE id = ? OR name = :n OR val = @v OR tags = $t OR id = ?3", ""},

		{"SELECT * FROM _jenab_changes", "line 1:15: `_jenab_changes` is internal"},
		{"SELECT *\nFROM data\nWHERE id IN (SELECT 1 FROM [_JENAB_META])", "line 3:28: `_JENAB_META` is internal"},
		{"SELECT 'بیت‌کوین' AS n, * FROM `_jenab_meta`", "line 1:32: `_jenab_meta` is internal"},
		{"SELECT * FROM main.sqlite_master", "line 1:20: `sqlite_master` is reserved"},
		{"SELECT * FROM pragma_table_info('data')", "line 1:15: `pragma_table_info` is not allowed: queries can't use PRAGMA functions"},
		{"SELECT * FROM fts5vocab('x', 'row')", "line 1:15: `fts5vocab` is not allowed: json_each and json_tree are the only virtual tables"},
		{"SELECT load_extension('x')", "line 1:8: `load_extension` is not allowed"},
		{"DELETE FROM data", "line 1:1: a query must start with SELECT, WITH or VALUES, not `DELETE`"},
		{"PRAGMA query_only = 0", "line 1:1: a query must start with SELECT, WITH or VALUES, not `PRAGMA`"},
		{"(SELECT 1)", "line 1:1: a query must start with SELECT, WITH or VALUES, not `(`"},
		{"SELECT 1; SELECT 2", "line 1:9: only one statement is allowed"},
		{"SELECT ';';;", "line 1:11: only one statement is allowed"},
		{"SELECT 1\x00; DELETE FROM data", "line 1:9: the query contains a NUL byte"},
		{"", "the query is empty"},
		{" ; ", "the query is empty"},
		{"-- nothing", "the query is empty"},
		{"SELECT 'abc", "line 1:8: unterminated string"},
		{`SELECT "abc`, "line 1:8: unterminated quoted name"},
		{"SELECT [abc", "line 1:8: unterminated quoted name"},
		{"SELECT 1 /* x", "line 1:10: unterminated comment"},
		{"SELECT ?0", "line 1:8: parameter ?0 must be between ?1 and ?999"},
		{"SELECT ?99999", "line 1:8: parameter ?99999 must be between ?1 and ?999"},

		// The conservative lexer: what SQLite might read differently is
		// refused, not guessed at.
		{"SELECT 1 AS نام, 'ok\xff', x'00'", ""},
		{"SELECT :a('), #a(') ; PRAGMA query_only=0; INSERT INTO data(name) SELECT value FROM _jenab_meta; COMMIT; SELECT 1 --'",
			"line 1:8: parameter names are letters, digits and '_' only"},
		{"SELECT #a", "line 1:8: `#` is not allowed here"},
		{"SELECT * FROM data WHERE id = :a::b", "line 1:31: parameter names are letters, digits and '_' only"},
		{"SELECT $a$b", "line 1:8: parameter names are letters, digits and '_' only"},
		{"SELECT a$b FROM data", "line 1:9: `$` can only start a parameter name"},
		{"SELECT : a", "line 1:8: `:` must start a parameter name, like :name"},
		{"SELECT @", "line 1:8: `@` must start a parameter name, like @name"},
		{"SELECT :_c", "line 1:8: parameter names start with a letter, like :name; for a numbered parameter write ?1"},
		{"SELECT $1", "line 1:8: parameter names start with a letter, like $name; for a numbered parameter write ?1"},
		{"SELECT * FROM \uFEFF_jenab_meta", "line 1:15: the character U+FEFF is not allowed outside strings"},
		{"SELECT * FROM \uFEFFpragma_table_info('data')", "line 1:15: the character U+FEFF is not allowed outside strings"},
		{"\uFEFFSELECT 1", "line 1:1: the character U+FEFF is not allowed outside strings"},
		{"SELECT 1 FROM data\xef\xbb", "line 1:19: the query is not valid UTF-8"},
		{"SELECT 1 ^ 2", "line 1:10: `^` is not allowed here"},
		{"SELECT 1\v", "line 1:9: the byte 0x0B is not allowed here"},
		{"SELECT 1 \\ 2", "line 1:10: `\\` is not allowed here"},
		{"SELECT (1", "line 1:8: this `(` is never closed"},
		{"SELECT 1) UNION SELECT (2", "line 1:9: this `)` has no matching `(`"},

		// Functions: refused when called, not as names.
		{"SELECT rtreecheck('_jenab_meta')", "line 1:8: `rtreecheck` is not allowed"},
		{`SELECT "RTREECHECK" ('x')`, "line 1:8: `RTREECHECK` is not allowed"},
		{"SELECT snippet, offsets FROM data", ""},

		// The budget for preparing.
		{cteChain(100), ""},
		{cteChain(101), "line 1:2782: the query defines more than 100 CTEs; split it into smaller queries"},
		{cteDoubling(8), ""},
		{cteDoubling(9), expands},
		{"WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 9) SELECT n.i FROM n, n AS m", ""},
		// SQLite takes a 'string' for a CTE's name and for a table after
		// FROM, and a quoted "." is an alias, not a dot.
		{cteNamed(9, "'a%d'", "'a%d'"), expands},
		{cteNamed(9, "a%d", "'a%d'"), expands},
		{cteNamed(9, "a%d", `a%d "."`), expands},
		{cteNamed(9, "a%d", "a%d '.'"), expands},
		{cteNamed(9, `"a%d"`, "[a%d] AS `.`"), expands},
		{"SELECT 1" + strings.Repeat(" ", MaxQuery), "the query is 65544 bytes; at most 65536 are allowed"},
		{strings.Repeat("SELECT 1 UNION ALL ", 500) + "SELECT 1", "line 1:9501: the query has more than 500 SELECT, WITH and VALUES; split it into smaller queries"},
	}
	for _, c := range cases {
		err := Check(c.q)
		got := ""
		if err != nil {
			got = err.Error()
			var re *RejectError
			if !errors.As(err, &re) || re.Layer != 1 {
				t.Errorf("%q: %T %v, want a layer 1 *RejectError", c.q, err, err)
			} else if re.Schema != schemaReason(c.want) {
				t.Errorf("%q: Schema = %v", c.q, re.Schema)
			}
		}
		if got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.q, got, c.want)
		}
	}
}

// schemaReason says a refusal is of a schema name, which RejectError.Schema
// marks.
func schemaReason(why string) bool {
	return strings.HasSuffix(why, "is internal") || strings.HasSuffix(why, "is reserved") || strings.Contains(why, "PRAGMA functions")
}

// cteChain is n CTEs, each reading the one before.
func cteChain(n int) string {
	var b strings.Builder
	b.WriteString("WITH c0 AS (SELECT 1 AS x)")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, ", c%d AS (SELECT x FROM c%d)", i, i-1)
	}
	fmt.Fprintf(&b, " SELECT * FROM c%d", n-1)
	return b.String()
}

const expands = "this query expands to too many subqueries (over 2000): each use of a CTE copies it and the CTEs it uses; use each CTE fewer times"

// cteDoubling is n CTEs, each using the one before twice: 800 bytes for
// n = 12, and its prepare doubles with each step.
func cteDoubling(n int) string { return cteNamed(n, "a%d", "a%d") }

// cteNamed is cteDoubling with the CTEs named by the format def, and
// used as the format use says.
func cteNamed(n int, def, use string) string {
	var b strings.Builder
	b.WriteString("WITH " + fmt.Sprintf(def, 0) + " AS (SELECT 1 AS x)")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, ", %s AS (SELECT (SELECT x FROM %s) + (SELECT x FROM %s) AS x)",
			fmt.Sprintf(def, i), fmt.Sprintf(use, i-1), fmt.Sprintf(use, i-1))
	}
	fmt.Fprintf(&b, " SELECT x FROM %s", fmt.Sprintf(use, n))
	return b.String()
}

// TestBudgetBoundsPrepare prepares the most expensive shapes the budget
// lets through: none may take long, as the deadline can't stop a prepare.
func TestBudgetBoundsPrepare(t *testing.T) {
	_, c := openTest(t)
	star := "WITH t AS (SELECT * FROM data WHERE val > 3) SELECT 1"
	for i := range 240 {
		star += fmt.Sprintf(", (SELECT count(*) FROM t WHERE id > %d)", i)
	}
	for _, q := range []string{
		cteChain(100),
		cteChain(100) + strings.Repeat(" UNION ALL SELECT * FROM c99", 18),
		cteDoubling(8),
		cteNamed(8, "'a%d'", "'a%d'"),
		cteNamed(8, "a%d", `a%d "."`),
		cteNamed(8, `"a%d"`, "[a%d] AS `.`"),
		star,
		strings.Repeat("SELECT 1 UNION ALL ", 498) + "SELECT 1",
	} {
		if err := Check(q); err != nil {
			t.Fatalf("%.40s: %v", q, err)
		}
		start := time.Now()
		if _, err := firstColumn(c, q); err != nil {
			t.Fatalf("%.40s: %v", q, err)
		}
		if d := time.Since(start); d > 300*time.Millisecond {
			t.Errorf("%.40s: prepared in %v", q, d)
		}
	}
}

// TestFunctionsCovered keeps the function lists complete: every function
// in this build is allowed or refused, so a driver update that adds one
// fails here until it is sorted.
func TestFunctionsCovered(t *testing.T) {
	db, _ := openTest(t)
	rows, err := db.Query("SELECT DISTINCT name FROM pragma_function_list")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			t.Fatal(err)
		}
		n++
		switch denied := deniedFuncs[f] || strings.HasPrefix(f, "sqlite_"); {
		case allowedFuncs[f] && denied:
			t.Errorf("function %s is both allowed and denied", f)
		case !allowedFuncs[f] && !denied:
			t.Errorf("function %s is neither allowed nor denied", f)
		}
	}
	if err := rows.Err(); err != nil || n == 0 {
		t.Fatalf("%d functions, %v", n, err)
	}
}

// TestRestrict: the agent limits hold on a restricted reader, and Prepare
// undoes them.
func TestRestrict(t *testing.T) {
	_, c := openTest(t)
	ctx := context.Background()
	if err := Restrict(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExecContext(ctx, "SELECT length(randomblob(?))", AgentLength+1); err == nil {
		t.Error("a blob over AgentLength works")
	}
	if _, err := c.ExecContext(ctx, "SELECT ?1000", 1); err == nil {
		t.Error("?1000 works")
	}
	if err := Prepare(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExecContext(ctx, "SELECT length(randomblob(?))", AgentLength+1); err != nil {
		t.Errorf("after Prepare: %v", err)
	}
}

// TestModulesCovered keeps deniedModules complete: every virtual table
// module in this build is allowed or refused by layer 1.
func TestModulesCovered(t *testing.T) {
	db, _ := openTest(t)
	rows, err := db.Query("SELECT name FROM pragma_module_list")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			t.Fatal(err)
		}
		n++
		if err := Check("SELECT * FROM " + m); err == nil && !allowedModules[m] {
			t.Errorf("module %s passes layer 1", m)
		}
	}
	if err := rows.Err(); err != nil || n == 0 {
		t.Fatalf("%d modules, %v", n, err)
	}
}

func TestParams(t *testing.T) {
	const q = "SELECT ?, :a, ?, ?05, ?, :a, $c, @b;"
	c, err := check(q)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{1, 2, 3, 5, 6, 2, 7, 8}; !slices.Equal(c.params, want) {
		t.Errorf("params = %v, want %v", c.params, want)
	}
	if got, want := c.body(), "SELECT ?1, :a, ?3, ?5, ?6, :a, $c, @b"; got != want { // ?05 as ?5: modernc binds ?05 by name
		t.Errorf("body = %q, want %q", got, want)
	}
	args := c.nullArgs()
	if len(args) != 11 { // 8 by position, then a, c and b by name
		t.Errorf("%d null args, want 11: %v", len(args), args)
	}
	for _, a := range args[:8] {
		if a != nil {
			t.Errorf("positional arg %v", a)
		}
	}
	if a, ok := args[8].(sql.NamedArg); !ok || a.Name != "a" || a.Value != nil {
		t.Errorf("args[8] = %#v", args[8])
	}
	ps, perr := Params(q)
	want := []Param{{1, ""}, {2, ":a"}, {3, ""}, {5, ""}, {6, ""}, {7, "$c"}, {8, "@b"}}
	if perr != nil || !slices.Equal(ps, want) {
		t.Errorf("Params = %v, %v; want %v", ps, perr, want)
	}
	// database/sql binds only names that start with a letter.
	for _, q := range []string{"SELECT :_c", "SELECT $1", "SELECT @9x"} {
		if _, err := Params(q); err == nil || !strings.Contains(err.Error(), "parameter names start with a letter") {
			t.Errorf("%s: %v", q, err)
		}
	}
	if ps, err := Params("SELECT :بها, :b2"); err != nil || len(ps) != 2 {
		t.Errorf("Persian name: %v, %v", ps, err)
	}
}

// TestVtabFlag: only a call of json_each or json_tree lets layer 2 allow
// virtual tables, not the name (SELECT 1 AS json_each).
func TestVtabFlag(t *testing.T) {
	for q, want := range map[string]bool{
		"SELECT * FROM json_each('[1]')":            true,
		"SELECT * FROM data, JSON_TREE (data.tags)": true,
		`SELECT * FROM "json_each"('[1]')`:          true,
		"SELECT 1 AS json_each, * FROM data":        false,
		"SELECT * FROM json_each":                   false,
		"SELECT json_each FROM data":                false,
	} {
		c, err := check(q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if c.vtab != want {
			t.Errorf("%s: vtab = %v, want %v", q, c.vtab, want)
		}
	}
}

// TestExplain runs layer 2's program check without layer 1 and without
// oneStatement, on attacks those would refuse first, so it shows that the
// program check stands on its own. PRAGMAs and several statements are left
// out: preparing them is what layer 1 exists to prevent.
func TestExplain(t *testing.T) {
	_, c := openTest(t)
	ctx := context.Background()
	tables, err := New().tableMap(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	json, err := jsonTables(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		q    string
		vtab bool
		want string // "" passes; "error" is SQLite's own error
	}{
		{"SELECT * FROM data", false, ""},
		{"SELECT d.name, c.note FROM data d JOIN child c ON c.parent_id = d.id", false, ""},
		{"SELECT name, count(*) FROM data WHERE name > 'n1' GROUP BY name", false, ""},
		{"SELECT d.id, j.value FROM data d, json_each(d.tags) j", true, ""},
		{"WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100) SELECT sum(i) FROM n", false, ""},
		{"SELECT DISTINCT name FROM data ORDER BY 1", false, ""},

		{"SELECT * FROM _jenab_meta", false, "`_jenab_meta` is internal"},
		{`SELECT * FROM "_JENAB_META"`, false, "`_jenab_meta` is internal"},
		{"WITH x AS (SELECT * FROM _jenab_meta) SELECT * FROM x", false, "`_jenab_meta` is internal"},
		{"SELECT * FROM data WHERE name IN _jenab_flags", false, "`_jenab_flags` is internal"},
		{"SELECT * FROM data WHERE id IN (SELECT id FROM _jenab_changes)", false, "`_jenab_changes` is internal"},
		{"SELECT op FROM _jenab_changes WHERE op > ''", false, "`_jenab_changes` is internal"}, // the index only
		{"SELECT name FROM data UNION SELECT key FROM _jenab_meta", false, "`_jenab_meta` is internal"},
		{"SELECT * FROM sqlite_schema", false, "`sqlite_schema` is reserved"},
		{"SELECT * FROM sqlite_temp_schema", false, "the query reads the temp schema or another database file"},
		{"SELECT * FROM pragma_table_info('data')", false, vtabOnly},
		{"SELECT * FROM dbstat", false, vtabOnly},
		{"SELECT * FROM json_each", false, vtabOnly},
		// The module check: json_each( somewhere doesn't let other virtual
		// tables in.
		{"SELECT * FROM json_each('[1]'), dbstat", true, vtabOnly},
		{"SELECT * FROM pragma_table_list, json_tree('[1]')", true, vtabOnly},
		{"SELECT * FROM json_each('[1]') a, json_each('[2]') b, json_tree('{}') c", true, ""},
		{"SELECT load_extension('x')", false, "`load_extension` is not allowed"},
		{"SELECT rtreecheck('_jenab_meta')", false, "`rtreecheck` is not allowed"},
		{"SELECT sqlite_version()", false, "`sqlite_version` is not allowed"},
		{"SELECT name, row_number() OVER (ORDER BY val) FROM data", false, ""},
		{"ATTACH 'other.db' AS o", false, "`sqlite_attach` is not allowed"},
		{"DELETE FROM data", false, "only read queries are allowed; this one would change the database"},
		{"WITH x AS (SELECT 1) INSERT INTO data(name) VALUES ('x')", false, "only read queries are allowed; this one would change the database"},
		{"CREATE TEMP VIEW v AS SELECT * FROM data", false, "only read queries are allowed; this one would change the database"},
		{"BEGIN IMMEDIATE", false, "only read queries are allowed; this one would change the database"},
		{"ANALYZE", false, "only read queries are allowed; this one would change the database"},
		{"VACUUM INTO 'copy.db'", false, "only read queries are allowed; this one would change the database"},
		{"SELECT * FROM nope", false, "error"},
	}
	for _, tc := range cases {
		var vtabs map[string]bool
		if tc.vtab {
			vtabs = json
		}
		err := program(ctx, c, tc.q, nil, tables, vtabs)
		var re *RejectError
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: %v", tc.q, err)
		case tc.want == "error" && (err == nil || errors.As(err, &re)):
			t.Errorf("%s: %v, want SQLite's error", tc.q, err)
		case tc.want != "" && tc.want != "error" && (!errors.As(err, &re) || re.Layer != 2 || re.Error() != tc.want):
			t.Errorf("%s:\n got %v\nwant %s", tc.q, err, tc.want)
		case re != nil && re.Schema != schemaReason(tc.want):
			t.Errorf("%s: Schema = %v", tc.q, re.Schema)
		}
	}
	var on int
	if err := c.QueryRowContext(context.Background(), "PRAGMA query_only").Scan(&on); err != nil || on != 1 {
		t.Errorf("query_only = %d, %v", on, err)
	}
}

const vtabOnly = "json_each(...) and json_tree(...) are the only virtual tables a query can use"

// TestJSONTablesStable: the addresses the module check compares stay the
// same on a connection, through other queries and other virtual tables.
func TestJSONTablesStable(t *testing.T) {
	db, c := openTest(t)
	ctx := context.Background()
	first, err := jsonTables(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"SELECT * FROM json_each('[1,2]')", "SELECT count(*) FROM pragma_table_list",
		"SELECT count(*) FROM dbstat", "SELECT * FROM json_tree('{\"a\":1}')",
	} {
		rows, err := c.QueryContext(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	if _, err := db.Exec("CREATE TABLE later(x)"); err != nil {
		t.Fatal(err)
	}
	again, err := jsonTables(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(first, again) {
		t.Errorf("addresses changed: %v, then %v", first, again)
	}
}

// TestOneStatement runs oneStatement on text layer 1 would refuse: it
// must refuse more than one statement without running any of them.
func TestOneStatement(t *testing.T) {
	db, c := openTest(t)
	cases := []struct{ q, want string }{
		{"SELECT 1", ""},
		{"SELECT 1 -- a comment to the end", ""},
		{"WITH t(x) AS (VALUES (1), (2)) SELECT x FROM t ORDER BY x DESC LIMIT 1", ""},
		{"SELECT name FROM data UNION ALL SELECT note FROM child", ""},
		{"SELECT nope FROM data", "error"},
		{"SELECT 1 FROM", "error"},
		{"WITH x AS (SELECT 1) DELETE FROM data", oneOnly},
		{"SELECT 1; INSERT INTO data(name) VALUES ('x')", oneOnly},
		{"SELECT :a('), #a(') ; PRAGMA query_only=0; INSERT INTO data(name) SELECT value FROM _jenab_meta; COMMIT; SELECT 1 --'", oneOnly},
		{"SELECT 1;\uFEFFPRAGMA query_only=0", oneOnly},
	}
	for _, tc := range cases {
		err := oneStatement(c, tc.q)
		var re *RejectError
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: %v", tc.q, err)
		case tc.want == "error" && (err == nil || errors.As(err, &re)):
			t.Errorf("%s: %v, want SQLite's error", tc.q, err)
		case tc.want != "" && tc.want != "error" && (!errors.As(err, &re) || re.Layer != 2 || re.Error() != tc.want):
			t.Errorf("%s:\n got %v\nwant %s", tc.q, err, tc.want)
		}
	}
	var on, n int
	if err := c.QueryRowContext(context.Background(), "PRAGMA query_only").Scan(&on); err != nil || on != 1 {
		t.Errorf("query_only = %d, %v", on, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM data").Scan(&n); err != nil || n != 2000 {
		t.Errorf("data has %d rows, %v", n, err)
	}
}

const oneOnly = "only one read query is allowed; SQLite reads this as a write or as more than one statement"

// TestExplainSchemaChange: a table made after the map was read is found,
// because the map follows schema_version.
func TestExplainSchemaChange(t *testing.T) {
	db, c := openTest(t)
	g := New()
	ctx := context.Background()
	if err := g.Explain(ctx, c, "SELECT * FROM data"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE fresh(id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := g.Explain(ctx, c, "SELECT * FROM fresh"); err != nil {
		t.Errorf("new table: %v", err)
	}
	if err := g.Explain(ctx, c, "SELECT * FROM data WHERE id = :id AND name = ?"); err != nil {
		t.Errorf("parameters: %v", err)
	}
	if err := g.Explain(ctx, c, "SELECT * FROM _jenab_meta"); err == nil || !strings.Contains(err.Error(), "line 1:15") {
		t.Errorf("Explain runs layer 1 first: %v", err)
	}
}

func TestPrepare(t *testing.T) {
	_, c := openTest(t)
	ctx := context.Background()
	if _, err := c.ExecContext(ctx, "ATTACH ? AS o", filepath.Join(t.TempDir(), "other.db")); err == nil {
		t.Error("ATTACH works on a guarded reader")
	}
	if _, err := c.ExecContext(ctx, "SELECT length(randomblob(?))", MaxLength+1); err == nil {
		t.Error("a blob over the length limit works")
	}
	if _, err := c.ExecContext(ctx, "INSERT INTO data(name) VALUES ('x')"); err == nil {
		t.Error("a guarded reader can write")
	}
}

// The typical view query of SPIKE-007: GROUP BY over 2,000 rows.
const benchQuery = "SELECT name, avg(val) AS a FROM data WHERE val > :v GROUP BY name ORDER BY a DESC LIMIT 10"

func BenchmarkCheck(b *testing.B) {
	for b.Loop() {
		if err := Check(benchQuery); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExplain(b *testing.B) {
	_, c := openTest(b)
	g := New()
	ctx := context.Background()
	for b.Loop() {
		if err := g.Explain(ctx, c, benchQuery); err != nil {
			b.Fatal(err)
		}
	}
}
