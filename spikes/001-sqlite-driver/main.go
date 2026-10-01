// SPIKE-001: compare cgo-free SQLite drivers against the needs of SPEC v0.3.
//
// Usage: go run . modernc|ncruces
// Prints one markdown table row per check.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ncruces "github.com/ncruces/go-sqlite3"
	ncdriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
	modernc "modernc.org/sqlite"
)

var ctx = context.Background()

const pragmas = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

func dsn(path string, readOnly bool) string {
	s := "file:" + filepath.ToSlash(path) + "?" + pragmas
	if readOnly {
		s += "&_pragma=query_only(1)"
	}
	return s
}

// ---------------------------------------------------------------- drivers

type target struct {
	name string
	open func(path string, readOnly bool) (*sql.DB, error)
	// driver-specific checks
	authorizer func(path string) (bool, string)
	columnInfo func(db *sql.DB, query string) (bool, string)
}

var targets = map[string]target{
	"modernc": {
		name: "modernc.org/sqlite",
		open: func(path string, ro bool) (*sql.DB, error) { return sql.Open("sqlite", dsn(path, ro)) },
		authorizer: func(path string) (bool, string) {
			db, err := sql.Open("sqlite", dsn(path, true))
			if err != nil {
				return false, err.Error()
			}
			defer db.Close()
			var methods []string
			err = rawConn(db, func(c any) error {
				t := reflect.TypeOf(c)
				for i := 0; i < t.NumMethod(); i++ {
					if strings.Contains(strings.ToLower(t.Method(i).Name), "author") {
						methods = append(methods, t.Method(i).Name)
					}
				}
				return nil
			})
			if err != nil {
				return false, err.Error()
			}
			if len(methods) == 0 {
				return false, "no authorizer method on the driver connection (only pre-update, commit and rollback hooks)"
			}
			return true, strings.Join(methods, ", ")
		},
		columnInfo: func(db *sql.DB, query string) (bool, string) {
			var out string
			err := rawConn(db, func(c any) error {
				ci, ok := c.(interface {
					ColumnInfo(string) ([]modernc.ColumnInfo, error)
				})
				if !ok {
					return fmt.Errorf("ColumnInfo not available")
				}
				info, err := ci.ColumnInfo(query)
				if err != nil {
					return err
				}
				var cols []string
				for _, i := range info {
					cols = append(cols, fmt.Sprintf("%s(from %q)", i.Name, i.TableName))
				}
				out = strings.Join(cols, ", ")
				return nil
			})
			if err != nil {
				return false, err.Error()
			}
			return true, out
		},
	},
	"ncruces": {
		name: "github.com/ncruces/go-sqlite3",
		// FTS5 is not in the default build; it is a separate module registered per connection.
		open: func(path string, ro bool) (*sql.DB, error) { return ncdriver.Open(dsn(path, ro), fts5.Register) },
		authorizer: func(path string) (bool, string) {
			db, err := ncdriver.Open(dsn(path, true), func(c *ncruces.Conn) error {
				if err := fts5.Register(c); err != nil {
					return err
				}
				return c.SetAuthorizer(denyInternal)
			})
			if err != nil {
				return false, err.Error()
			}
			defer db.Close()
			return runAuthorizerCases(db)
		},
		columnInfo: func(db *sql.DB, query string) (bool, string) {
			var out string
			err := rawConn(db, func(c any) error {
				stmt, _, err := c.(ncdriver.Conn).Raw().Prepare(query)
				if err != nil {
					return err
				}
				defer stmt.Close()
				var cols []string
				for i := 0; i < stmt.ColumnCount(); i++ {
					cols = append(cols, fmt.Sprintf("%s(from %q)", stmt.ColumnName(i), stmt.ColumnTableName(i)))
				}
				out = strings.Join(cols, ", ")
				return nil
			})
			if err != nil {
				return false, err.Error()
			}
			return true, out
		},
	},
}

func rawConn(db *sql.DB, fn func(any) error) error {
	c, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Raw(fn)
}

// denyInternal blocks every access to _burrow_* tables, plus ATTACH.
func denyInternal(action ncruces.AuthorizerActionCode, n3, n4, schema, inner string) ncruces.AuthorizerReturnCode {
	internal := func(s string) bool { return strings.HasPrefix(strings.ToLower(s), "_burrow_") }
	switch action {
	case ncruces.AUTH_READ:
		// pragma_* table-valued functions expose schema details; the agent uses describe_table instead.
		l := strings.ToLower(n3)
		if internal(n3) || strings.HasPrefix(l, "pragma_") {
			return ncruces.AUTH_DENY
		}
		// Direct reads of the schema table would list _burrow_* names. SQLite's own schema
		// loading does not go through the authorizer, so this only affects user SQL.
		if (l == "sqlite_schema" || l == "sqlite_master") && inner == "" {
			return ncruces.AUTH_DENY
		}
	case ncruces.AUTH_INSERT, ncruces.AUTH_UPDATE, ncruces.AUTH_DELETE,
		ncruces.AUTH_DROP_TABLE, ncruces.AUTH_ANALYZE:
		if internal(n3) {
			return ncruces.AUTH_DENY
		}
	case ncruces.AUTH_ALTER_TABLE, ncruces.AUTH_CREATE_INDEX, ncruces.AUTH_CREATE_TRIGGER:
		if internal(n4) {
			return ncruces.AUTH_DENY
		}
	case ncruces.AUTH_PRAGMA:
		if internal(n4) {
			return ncruces.AUTH_DENY
		}
	case ncruces.AUTH_ATTACH, ncruces.AUTH_DETACH:
		return ncruces.AUTH_DENY
	}
	return ncruces.AUTH_OK
}

func runAuthorizerCases(db *sql.DB) (bool, string) {
	cases := []struct {
		sql     string
		allowed bool
	}{
		{"SELECT * FROM data", true},
		{"SELECT * FROM _burrow_meta", false},
		{`SELECT * FROM "_BURROW_META"`, false},
		{"SELECT * FROM main._burrow_meta", false},
		{"SELECT * FROM v_leak", false},
		{"WITH x AS (SELECT * FROM _burrow_meta) SELECT * FROM x", false},
		{"SELECT * FROM data WHERE id IN (SELECT rowid FROM _burrow_meta)", false},
		{"PRAGMA table_info(_burrow_meta)", false},
		{"SELECT * FROM pragma_table_info('_burrow_meta')", false},
		{"ATTACH 'x.db' AS x", false},
		{"SELECT name FROM sqlite_schema", false},
		{"SELECT d.name, count(*) FROM data d GROUP BY d.name", true},
	}
	var fails []string
	for _, c := range cases {
		rows, err := db.Query(c.sql)
		if err == nil {
			rows.Close()
		}
		if (err == nil) != c.allowed {
			fails = append(fails, fmt.Sprintf("`%s` expected allowed=%v, got err=%v", c.sql, c.allowed, err))
		}
	}
	if len(fails) > 0 {
		return false, strings.Join(fails, "; ")
	}
	return true, fmt.Sprintf("all %d cases correct (direct, quoted, schema-qualified, view, CTE, subquery, PRAGMA, pragma_ function, ATTACH, sqlite_schema)", len(cases))
}

// ---------------------------------------------------------------- shared checks

type result struct {
	check  string
	ok     bool
	detail string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run . modernc|ncruces")
		os.Exit(2)
	}
	t, found := targets[os.Args[1]]
	if !found {
		fmt.Fprintln(os.Stderr, "unknown driver", os.Args[1])
		os.Exit(2)
	}

	dir, err := os.MkdirTemp("", "spike001-"+os.Args[1]+"-")
	must(err)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "project.db")

	w, err := t.open(path, false)
	must(err)
	w.SetMaxOpenConns(1) // single writer
	r, err := t.open(path, true)
	must(err)
	r.SetMaxOpenConns(8) // reader pool

	setup(w)

	var results []result
	add := func(check string, ok bool, detail string) { results = append(results, result{check, ok, detail}) }

	// 1. version and compile options
	var version string
	must(w.QueryRow("SELECT sqlite_version()").Scan(&version))
	opts := compileOptions(w)
	add("SQLite version", true, version)
	add("FTS5 compiled in", opts["ENABLE_FTS5"], optDetail(opts, "ENABLE_FTS5"))
	add("Bytecode vtab (tables_used) compiled in", opts["ENABLE_BYTECODE_VTAB"], optDetail(opts, "ENABLE_BYTECODE_VTAB"))
	add("Column metadata compiled in", opts["ENABLE_COLUMN_METADATA"], optDetail(opts, "ENABLE_COLUMN_METADATA"))

	// 2. FTS5
	ok, d := checkFTS5(w)
	add("FTS5 create, MATCH, bm25, snippet", ok, d)

	// 3. authorizer / tables_used
	ok, d = t.authorizer(path)
	add("Authorizer blocks _burrow_* tables", ok, d)
	ok, d = checkTablesUsed(r)
	add("tables_used() lists tables behind views and CTEs", ok, d)

	ok, d = checkExplainTables(r)
	add("EXPLAIN-based table detection (driver-independent)", ok, d)

	// 4. result columns without executing (SPEC 10, step 5)
	ok, d = t.columnInfo(r, "SELECT name, AVG(val) AS avg_val FROM data GROUP BY name")
	add("Result column names without executing", ok, d)

	// 5. VACUUM INTO on a query_only connection
	ok, d = checkVacuumInto(r, filepath.Join(dir, "snapshot.db"))
	add("VACUUM INTO on query_only reader", ok, d)

	ok, d = checkSnapshotAlternatives(t, path, w, filepath.Join(dir, "snap"))
	add("Snapshot: VACUUM INTO on a separate plain connection, writer latency during it", ok, d)

	// 6. DDL rollback
	ok, d = checkDDLRollback(w)
	add("DDL rolls back in a transaction", ok, d)

	// 7. table rebuild with foreign keys off
	ok, d = checkTableRebuild(w)
	add("Table rebuild (foreign_keys OFF, foreign_key_check)", ok, d)

	// 8. WAL: reader not blocked by open write transaction
	ok, d = checkWALReader(w, r)
	add("WAL reader not blocked by open write txn", ok, d)

	// 9. SQL features used by the spec
	ok, d = checkSQLFeatures(w)
	add("UPSERT, RETURNING, JSON ->>", ok, d)

	// 10. bulk write with concurrent readers
	ok, d = checkBulk(w, r)
	add("100k rows in 500-row txns, 4 concurrent readers", ok, d)

	// 11. memory per reader connection
	ok, d = checkMemory(t, path)
	add("Memory per extra connection", ok, d)

	w.Close()
	r.Close()

	fmt.Printf("### %s\n\n| Check | Result | Detail |\n|---|---|---|\n", t.name)
	for _, res := range results {
		mark := "PASS"
		if !res.ok {
			mark = "FAIL"
		}
		fmt.Printf("| %s | %s | %s |\n", res.check, mark, strings.ReplaceAll(res.detail, "|", "\\|"))
	}
}

func setup(w *sql.DB) {
	for _, s := range []string{
		"CREATE TABLE data(id INTEGER PRIMARY KEY, name TEXT, val REAL)",
		"CREATE TABLE _burrow_meta(key TEXT PRIMARY KEY, value TEXT)",
		"CREATE VIEW v_leak AS SELECT * FROM _burrow_meta",
		"INSERT INTO data(name, val) VALUES ('a', 1), ('b', 2)",
		"INSERT INTO _burrow_meta VALUES ('schema_version', '1')",
	} {
		_, err := w.Exec(s)
		must(err)
	}
}

func compileOptions(db *sql.DB) map[string]bool {
	rows, err := db.Query("PRAGMA compile_options")
	must(err)
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var o string
		must(rows.Scan(&o))
		m[strings.SplitN(o, "=", 2)[0]] = true
	}
	return m
}

func optDetail(m map[string]bool, k string) string {
	if m[k] {
		return "SQLITE_" + k + " present"
	}
	return "SQLITE_" + k + " not in compile_options"
}

func checkFTS5(w *sql.DB) (bool, string) {
	for _, s := range []string{
		"CREATE VIRTUAL TABLE msgs USING fts5(body)",
		"INSERT INTO msgs(body) VALUES ('bitcoin dropped on tuesday'), ('ethereum news'), ('bitcoin price in EUR')",
	} {
		if _, err := w.Exec(s); err != nil {
			return false, err.Error()
		}
	}
	rows, err := w.Query("SELECT snippet(msgs, 0, '[', ']', '…', 4) FROM msgs WHERE msgs MATCH 'bitcoin' ORDER BY bm25(msgs)")
	if err != nil {
		return false, err.Error()
	}
	defer rows.Close()
	n := 0
	var first string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		if n == 0 {
			first = s
		}
		n++
	}
	return n == 2, fmt.Sprintf("%d matches, first snippet %q", n, first)
}

func checkTablesUsed(r *sql.DB) (bool, string) {
	q := "WITH x AS (SELECT key FROM v_leak) SELECT d.name FROM data d WHERE d.name IN (SELECT key FROM x)"
	rows, err := r.Query("SELECT DISTINCT name FROM tables_used(?) WHERE type = 'table'", q)
	if err != nil {
		return false, err.Error()
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	joined := strings.Join(names, ", ")
	return strings.Contains(joined, "_burrow_meta") && strings.Contains(joined, "data"), "tables: " + joined
}

func checkVacuumInto(r *sql.DB, target string) (bool, string) {
	if _, err := r.Exec("VACUUM INTO ?", target); err != nil {
		return false, err.Error()
	}
	st, err := os.Stat(target)
	if err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("snapshot written, %d bytes", st.Size())
}

func checkDDLRollback(w *sql.DB) (bool, string) {
	tx, err := w.Begin()
	if err != nil {
		return false, err.Error()
	}
	for _, s := range []string{
		"CREATE TABLE t_new(x)",
		"ALTER TABLE data ADD COLUMN extra TEXT",
		"UPDATE _burrow_meta SET value = '2' WHERE key = 'schema_version'",
	} {
		if _, err := tx.Exec(s); err != nil {
			tx.Rollback()
			return false, err.Error()
		}
	}
	tx.Rollback()
	var tables, extra int
	var ver string
	w.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name = 't_new'").Scan(&tables)
	w.QueryRow("SELECT count(*) FROM pragma_table_info('data') WHERE name = 'extra'").Scan(&extra)
	w.QueryRow("SELECT value FROM _burrow_meta WHERE key = 'schema_version'").Scan(&ver)
	ok := tables == 0 && extra == 0 && ver == "1"
	return ok, fmt.Sprintf("after rollback: new table=%d, new column=%d, schema_version=%s", tables, extra, ver)
}

func checkTableRebuild(w *sql.DB) (bool, string) {
	c, err := w.Conn(ctx)
	if err != nil {
		return false, err.Error()
	}
	defer c.Close()
	steps := []string{
		"CREATE TABLE parent(id INTEGER PRIMARY KEY, code TEXT)",
		"CREATE TABLE child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id) ON DELETE CASCADE)",
		"INSERT INTO parent VALUES (1, 'BTC'), (2, 'ETH')",
		"INSERT INTO child(parent_id) VALUES (1), (1), (2)",
		// rebuild, following https://sqlite.org/lang_altertable.html#otheralter
		"PRAGMA foreign_keys = OFF",
		"BEGIN",
		"CREATE TABLE parent_new(id INTEGER PRIMARY KEY, code TEXT NOT NULL, region TEXT NOT NULL DEFAULT 'eu')",
		"INSERT INTO parent_new(id, code) SELECT id, code FROM parent",
		"DROP TABLE parent",
		"ALTER TABLE parent_new RENAME TO parent",
	}
	for _, s := range steps {
		if _, err := c.ExecContext(ctx, s); err != nil {
			return false, s + ": " + err.Error()
		}
	}
	var violations int
	rows, err := c.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return false, err.Error()
	}
	for rows.Next() {
		violations++
	}
	rows.Close()
	if _, err := c.ExecContext(ctx, "COMMIT"); err != nil {
		return false, err.Error()
	}
	c.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	var children, fk int
	c.QueryRowContext(ctx, "SELECT count(*) FROM child").Scan(&children)
	c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
	ok := violations == 0 && children == 3 && fk == 1
	return ok, fmt.Sprintf("fk violations=%d, child rows kept=%d/3, foreign_keys back on=%v", violations, children, fk == 1)
}

func checkWALReader(w, r *sql.DB) (bool, string) {
	c, err := w.Conn(ctx)
	if err != nil {
		return false, err.Error()
	}
	defer c.Close()
	var before int
	r.QueryRow("SELECT count(*) FROM data").Scan(&before)
	if _, err := c.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, err.Error()
	}
	c.ExecContext(ctx, "INSERT INTO data(name, val) VALUES ('uncommitted', 0)")
	start := time.Now()
	var during int
	err = r.QueryRow("SELECT count(*) FROM data").Scan(&during)
	elapsed := time.Since(start)
	c.ExecContext(ctx, "COMMIT")
	if err != nil {
		return false, err.Error()
	}
	ok := during == before && elapsed < 200*time.Millisecond
	return ok, fmt.Sprintf("read took %v, saw %d rows (committed: %d)", elapsed.Round(time.Microsecond), during, before)
}

func checkSQLFeatures(w *sql.DB) (bool, string) {
	var v string
	err := w.QueryRow(`INSERT INTO _burrow_meta(key, value) VALUES ('schema_version', '5')
		ON CONFLICT(key) DO UPDATE SET value = excluded.value RETURNING value`).Scan(&v)
	if err != nil {
		return false, err.Error()
	}
	var j string
	if err := w.QueryRow(`SELECT '{"a":{"b":2}}' ->> '$.a.b'`).Scan(&j); err != nil {
		return false, err.Error()
	}
	return v == "5" && j == "2", fmt.Sprintf("upsert returned %s, json ->> returned %s", v, j)
}

func checkBulk(w, r *sql.DB) (bool, string) {
	const total, chunk = 100_000, 500
	var done atomic.Bool
	var reads, readErrs atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !done.Load() {
				var n, m sql.NullInt64
				if err := r.QueryRow("SELECT count(*), max(id) FROM data").Scan(&n, &m); err != nil {
					readErrs.Add(1)
				}
				reads.Add(1)
			}
		}()
	}
	start := time.Now()
	for i := 0; i < total; i += chunk {
		tx, err := w.Begin()
		if err != nil {
			done.Store(true)
			return false, err.Error()
		}
		stmt, err := tx.Prepare("INSERT INTO data(name, val) VALUES (?, ?)")
		if err != nil {
			tx.Rollback()
			done.Store(true)
			return false, err.Error()
		}
		for j := 0; j < chunk; j++ {
			if _, err := stmt.Exec(fmt.Sprintf("row-%d", i+j), float64(i+j)); err != nil {
				tx.Rollback()
				done.Store(true)
				return false, err.Error()
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			done.Store(true)
			return false, err.Error()
		}
	}
	elapsed := time.Since(start)
	done.Store(true)
	wg.Wait()
	return readErrs.Load() == 0, fmt.Sprintf("write %v (%.0f rows/s); %d concurrent reads, %d read errors",
		elapsed.Round(time.Millisecond), float64(total)/elapsed.Seconds(), reads.Load(), readErrs.Load())
}

func checkMemory(t target, path string) (bool, string) {
	const n = 8
	db, err := t.open(path, true)
	if err != nil {
		return false, err.Error()
	}
	defer db.Close()
	db.SetMaxOpenConns(n)
	db.SetMaxIdleConns(n)
	before := privateBytes()
	conns := make([]*sql.Conn, n)
	for i := range conns {
		c, err := db.Conn(ctx)
		if err != nil {
			return false, err.Error()
		}
		var cnt int
		c.QueryRowContext(ctx, "SELECT count(*) FROM data WHERE val > 10").Scan(&cnt)
		conns[i] = c
	}
	after := privateBytes()
	for _, c := range conns {
		c.Close()
	}
	if before == 0 {
		return true, "not measured on this OS"
	}
	per := float64(after-before) / n / (1 << 20)
	return true, fmt.Sprintf("~%.1f MB private memory per connection (%d connections, after a full-table query)", per, n)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
