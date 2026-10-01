// SPIKE-007: guard agent SQL without relying on an authorizer.
//
// Usage: go run . modernc|ncruces
// Prints markdown tables.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ncruces "github.com/ncruces/go-sqlite3"
	ncdriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
	modernc "modernc.org/sqlite"
)

var ctx = context.Background()

const (
	limitLength   = 0 // SQLITE_LIMIT_LENGTH
	limitAttached = 7 // SQLITE_LIMIT_ATTACHED
	maxLength     = 16 << 20
	queryTimeout  = 2 * time.Second
)

type driverT struct {
	name  string
	open  func(dsn string) (*sql.DB, error)
	limit func(c *sql.Conn, id, val int) (int, error)
}

var drivers = map[string]driverT{
	"modernc": {
		name:  "modernc.org/sqlite v1.59.0",
		open:  func(dsn string) (*sql.DB, error) { return sql.Open("sqlite", dsn) },
		limit: func(c *sql.Conn, id, val int) (int, error) { return modernc.Limit(c, id, val) },
	},
	"ncruces": {
		name: "github.com/ncruces/go-sqlite3 v0.35.6",
		open: func(dsn string) (*sql.DB, error) { return ncdriver.Open(dsn, fts5.Register) },
		limit: func(c *sql.Conn, id, val int) (old int, err error) {
			err = c.Raw(func(dc any) error {
				old = dc.(ncdriver.Conn).Raw().Limit(ncruces.LimitCategory(id), val)
				return nil
			})
			return old, err
		},
	},
}

func dsn(path string, readOnly bool) string {
	s := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	if readOnly {
		s += "&_pragma=query_only(1)"
	}
	return s
}

// conn opens a single fresh connection. Guarded readers get the limits.
func (d driverT) conn(path string, readOnly, limits bool) (*sql.Conn, func()) {
	db, err := d.open(dsn(path, readOnly))
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)
	c, err := db.Conn(ctx)
	if err != nil {
		panic(err)
	}
	if limits {
		if _, err := d.limit(c, limitAttached, 0); err != nil {
			panic(err)
		}
		if _, err := d.limit(c, limitLength, maxLength); err != nil {
			panic(err)
		}
	}
	return c, func() { c.Close(); db.Close() }
}

func main() {
	if len(os.Args) != 2 || drivers[os.Args[1]].open == nil {
		fmt.Println("usage: go run . modernc|ncruces")
		os.Exit(2)
	}
	d := drivers[os.Args[1]]
	dir, _ := os.MkdirTemp("", "spike007")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "project.db")
	other := filepath.ToSlash(filepath.Join(dir, "other.db"))

	w, closeW := d.conn(path, false, false)
	defer closeW()
	setup(w)
	o, closeO := d.conn(other, false, false)
	o.ExecContext(ctx, "CREATE TABLE secret(x); INSERT INTO secret VALUES ('other project data')")
	closeO()

	fmt.Printf("# SPIKE-007 results: %s\n\n", d.name)
	var ver string
	w.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&ver)
	fmt.Printf("SQLite %s. Guarded readers: `query_only`, `SQLITE_LIMIT_ATTACHED = 0`, `SQLITE_LIMIT_LENGTH = 16 MiB`, timeout %v.\n", ver, queryTimeout)

	denied := modules(w)
	special(d, path, other)
	runCases(d, path, other, dir, w, denied)
	perf(d, path, denied)

	// The writer also gets the ATTACH limit; Go code never attaches.
	d.limit(w, limitAttached, 0)
	runSchemaGuard(w, other)
}

func setup(w *sql.Conn) {
	stmts := []string{
		"CREATE TABLE _burrow_meta(key TEXT PRIMARY KEY, value TEXT)",
		"CREATE TABLE _burrow_changes(id INTEGER PRIMARY KEY, target TEXT, op TEXT)",
		"CREATE TABLE _burrow_flags(name TEXT PRIMARY KEY) WITHOUT ROWID",
		"CREATE TABLE data(id INTEGER PRIMARY KEY, name TEXT, val REAL, tags TEXT)",
		"CREATE INDEX data_name ON data(name)",
		"CREATE TABLE child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES data(id), note TEXT)",
		"INSERT INTO _burrow_meta VALUES ('project_id', '01J...'), ('schema_version', '3')",
		"INSERT INTO _burrow_changes(target, op) VALUES ('table:data', 'insert')",
		"INSERT INTO _burrow_flags VALUES ('a')",
		`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 2000)
		 INSERT INTO data(name, val, tags) SELECT 'n' || (i % 50), i * 1.5, '["a","b"]' FROM n`,
		`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 100)
		 INSERT INTO child(parent_id, note) SELECT i, 'c' FROM n`,
	}
	for _, s := range stmts {
		if _, err := w.ExecContext(ctx, s); err != nil {
			panic(s + ": " + err.Error())
		}
	}
}

// modules returns the virtual table modules in this build, minus the allowed ones.
func modules(w *sql.Conn) map[string]bool {
	denied := map[string]bool{"bytecode": true, "tables_used": true, "dbstat": true}
	for _, f := range deniedFuncs {
		denied[f] = true
	}
	rows, err := w.QueryContext(ctx, "SELECT name FROM pragma_module_list ORDER BY name")
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var all []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		all = append(all, n)
		if !allowedVtabs[n] {
			denied[n] = true
		}
	}
	fmt.Printf("\nVirtual table modules in this build: %s.\n", strings.Join(all, ", "))
	return denied
}

// fingerprint summarizes data and schema, to detect any change made by a case.
func fingerprint(w *sql.Conn) string {
	var s string
	err := w.QueryRowContext(ctx, `SELECT
		(SELECT count(*) || ':' || total(val) || ':' || coalesce(sum(length(name)), 0) FROM data) || '|' ||
		(SELECT count(*) FROM _burrow_meta) || '|' || (SELECT count(*) FROM _burrow_changes) || '|' ||
		(SELECT count(*) FROM child) || '|' ||
		(SELECT group_concat(name) FROM (SELECT name FROM sqlite_schema ORDER BY name))`).Scan(&s)
	if err != nil {
		return "error: " + err.Error()
	}
	return s
}

func short(s string) string {
	s = strings.NewReplacer("\n", " ", "|", "\\|").Replace(s)
	if len(s) > 70 {
		s = s[:67] + "..."
	}
	return s
}

func code(s string) string {
	s = strings.NewReplacer("\n", "↵", "\x00", "\\0", "|", "\\|").Replace(s)
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

func queryOnly(c *sql.Conn) int {
	var v int
	c.QueryRowContext(ctx, "PRAGMA query_only").Scan(&v)
	return v
}

// special checks that explain why the order of the layers matters.
func special(d driverT, path, other string) {
	fmt.Println("\n## Connection behaviour\n")
	fmt.Println("| Check | Result |")
	fmt.Println("|---|---|")

	c, done := d.conn(path, true, false)
	old, _ := d.limit(c, limitAttached, -1)
	oldLen, _ := d.limit(c, limitLength, -1)
	fmt.Printf("| Default `SQLITE_LIMIT_ATTACHED` / `SQLITE_LIMIT_LENGTH` | %d / %d |\n", old, oldLen)
	_, err := c.ExecContext(ctx, "ATTACH ? AS o", other)
	fmt.Printf("| `ATTACH` on `query_only` without the limit | %s |\n", errOrOK(err))
	done()

	c, done = d.conn(path, true, true)
	_, err = c.ExecContext(ctx, "ATTACH ? AS o", other)
	fmt.Printf("| `ATTACH` with `SQLITE_LIMIT_ATTACHED = 0` | %s |\n", errOrOK(err))
	done()

	c, done = d.conn(path, true, true)
	st, err := c.PrepareContext(ctx, "PRAGMA query_only = 0")
	if err == nil {
		st.Close()
	}
	fmt.Printf("| Only *prepare* `PRAGMA query_only = 0`, never execute | prepare: %s; `query_only` is now **%d** |\n", errOrOK(err), queryOnly(c))
	done()

	c, done = d.conn(path, true, true)
	rows, err := c.QueryContext(ctx, "EXPLAIN PRAGMA query_only = 0")
	if err == nil {
		for rows.Next() {
		}
		rows.Close()
	}
	fmt.Printf("| `EXPLAIN PRAGMA query_only = 0` | %s; `query_only` is now **%d** |\n", errOrOK(err), queryOnly(c))
	done()
}

func errOrOK(err error) string {
	if err == nil {
		return "succeeded"
	}
	return "error: " + short(err.Error())
}

type layer3 struct {
	text    string
	blocked bool
	changed string
}

// runL3 executes the raw query on a fresh guarded reader and reports what happened.
func runL3(d driverT, path, q string, w *sql.Conn, base string) layer3 {
	c, done := d.conn(path, true, true)
	defer done()
	tctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	start := time.Now()
	var res layer3
	rows, err := c.QueryContext(tctx, q, nullParams(q)...)
	if err == nil {
		n := 0
		for n < 200 && rows.Next() {
			n++
		}
		err = rows.Err()
		rows.Close()
		if err == nil {
			res.text = fmt.Sprintf("ran, %d rows", n)
		}
	}
	if err != nil {
		res.blocked = true
		res.text = "error: " + short(err.Error())
		if el := time.Since(start); el > 500*time.Millisecond {
			res.text += fmt.Sprintf(" (%v)", el.Round(10*time.Millisecond))
		}
	}
	var changes []string
	if queryOnly(c) != 1 {
		changes = append(changes, "query_only turned off")
	}
	var ndb int
	c.QueryRowContext(ctx, "SELECT count(*) FROM pragma_database_list WHERE name NOT IN ('main', 'temp')").Scan(&ndb)
	if ndb > 0 {
		changes = append(changes, "database attached")
	}
	if fingerprint(w) != base {
		changes = append(changes, "project.db changed")
	}
	res.changed = strings.Join(changes, ", ")
	return res
}

func runCases(d driverT, path, other, dir string, w *sql.Conn, denied map[string]bool) {
	fmt.Println("\n## Agent query cases\n")
	fmt.Println("Each layer is tested on its own, on a fresh connection. In Burrow they run in order: 1 → 2 → 3, and a query stops at the first layer that rejects it.\n")
	fmt.Println("- **1 Text**: single statement, starts with SELECT/WITH/VALUES, no internal, reserved, `pragma_*` or denied names")
	fmt.Println("- **2 EXPLAIN**: compiled program opens no internal table, `sqlite_schema` or virtual table (except `json_each`/`json_tree`), no write opcodes")
	fmt.Println("- **3 Connection**: the raw query on a guarded reader (`query_only`, limits, timeout)\n")
	fmt.Println("| # | Kind | Query | Expect | 1 Text | 2 EXPLAIN | 3 Connection | Result |")
	fmt.Println("|---|---|---|---|---|---|---|---|")

	base := fingerprint(w)
	fails, unsafe := 0, 0
	byLayer := map[string]int{}
	for i, qc := range queryCases {
		q := strings.NewReplacer("{OTHER}", other, "{DIR}", filepath.ToSlash(dir)).Replace(qc.sql)

		t, vtab := textCheck(q, denied)
		l1 := "pass"
		if t != "" {
			l1 = "**reject**: " + short(t)
		}

		c, done := d.conn(path, true, true)
		rc := &rootCache{}
		e, err := explainCheck(c, q, rc, vtab)
		l2 := "pass"
		if err != nil {
			l2 = "error: " + short(err.Error())
		} else if e != "" {
			l2 = "**reject**: " + short(e)
		}
		if queryOnly(c) != 1 || fingerprint(w) != base {
			l2 += " ⚠ state changed"
		}
		done()

		l3 := runL3(d, path, q, w, base)
		l3s := l3.text
		if l3.changed != "" {
			l3s += " ⚠ " + l3.changed
			base = fingerprint(w)
		}

		// Burrow's order: the first layer that rejects stops the query.
		var blockedBy string
		switch {
		case t != "":
			blockedBy = "1"
		case err != nil || e != "":
			blockedBy = "2"
		case l3.blocked && l3.changed == "":
			blockedBy = "3"
		}
		res := "OK"
		if qc.allow && blockedBy != "" {
			res = "**FAIL** (false positive)"
			fails++
		}
		if !qc.allow && blockedBy == "" {
			res = "**FAIL** (not blocked)"
			fails++
			unsafe++
		}
		if !qc.allow && blockedBy != "" {
			byLayer[blockedBy]++
		}
		expect := "block"
		if qc.allow {
			expect = "allow"
		}
		fmt.Printf("| %d | %s | %s | %s | %s | %s | %s | %s |\n", i+1, qc.cat, code(qc.sql), expect, l1, l2, l3s, res)
	}
	fmt.Printf("\n**Agent queries: %d of %d cases correct.** Blocked cases stopped by layer 1: %d, layer 2: %d, layer 3: %d.\n",
		len(queryCases)-fails, len(queryCases), byLayer["1"], byLayer["2"], byLayer["3"])
	if unsafe > 0 {
		fmt.Printf("**%d cases got through all layers.**\n", unsafe)
	}
}

func perf(d driverT, path string, denied map[string]bool) {
	c, done := d.conn(path, true, true)
	defer done()
	q := "SELECT name, avg(val) AS a FROM data WHERE val > :v GROUP BY name ORDER BY a DESC LIMIT 10"
	rc := &rootCache{}
	const n = 2000
	start := time.Now()
	for i := 0; i < n; i++ {
		if r, _ := textCheck(q, denied); r != "" {
			panic(r)
		}
	}
	textT := time.Since(start) / n
	start = time.Now()
	for i := 0; i < n; i++ {
		if r, err := explainCheck(c, q, rc, false); r != "" || err != nil {
			panic(fmt.Sprint(r, err))
		}
	}
	explT := time.Since(start) / n
	start = time.Now()
	for i := 0; i < n; i++ {
		rows, err := c.QueryContext(ctx, q, sql.Named("v", 10))
		if err != nil {
			panic(err)
		}
		for rows.Next() {
		}
		rows.Close()
	}
	runT := time.Since(start) / n
	fmt.Println("\n## Cost per query\n")
	fmt.Println("Typical view query (GROUP BY over 2,000 rows), average of 2,000 runs, root-page map cached by `schema_version`.\n")
	fmt.Println("| Step | Time |")
	fmt.Println("|---|---|")
	fmt.Printf("| 1 Text check | %v |\n| 2 EXPLAIN check | %v |\n| Running the query itself | %v |\n",
		textT.Round(100*time.Nanosecond), explT.Round(time.Microsecond), runT.Round(time.Microsecond))
}
