package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------- schema guard (SPEC 7.5)

// internalSnapshot lists every schema entry that belongs to an internal table.
func internalSnapshot(c *sql.Conn) (string, error) {
	rows, err := c.QueryContext(ctx, `SELECT type, name, tbl_name, coalesce(sql, '') FROM sqlite_schema
		WHERE name LIKE '\_burrow\_%' ESCAPE '\' OR tbl_name LIKE '\_burrow\_%' ESCAPE '\'
		ORDER BY type, name`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var t, n, tn, s string
		if err := rows.Scan(&t, &n, &tn, &s); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s|%s|%s|%s\n", t, n, tn, s)
	}
	return b.String(), rows.Err()
}

// schemaGuard runs inside the migration transaction, after all steps, before commit.
func schemaGuard(c *sql.Conn, before string) string {
	now, err := internalSnapshot(c)
	if err != nil {
		return err.Error()
	}
	if now != before {
		return "internal schema changed"
	}
	var name, typ string
	err = c.QueryRowContext(ctx, `SELECT type, name FROM sqlite_schema WHERE type IN ('trigger', 'view') LIMIT 1`).Scan(&typ, &name)
	if err == nil {
		return fmt.Sprintf("%s %s not allowed", typ, name)
	}
	var n int
	c.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_temp_schema`).Scan(&n)
	if n > 0 {
		return "temp schema objects not allowed"
	}
	rows, err := c.QueryContext(ctx, `SELECT name, sql FROM sqlite_schema WHERE type = 'table'
		AND name NOT LIKE 'sqlite\_%' ESCAPE '\' AND name NOT LIKE '\_burrow\_%' ESCAPE '\'`)
	if err != nil {
		return err.Error()
	}
	type tbl struct{ name, sql string }
	var tables []tbl
	for rows.Next() {
		var t tbl
		rows.Scan(&t.name, &t.sql)
		tables = append(tables, t)
	}
	rows.Close()
	for _, t := range tables {
		if strings.HasPrefix(strings.ToUpper(t.sql), "CREATE VIRTUAL") {
			return "virtual table " + t.name + " not allowed"
		}
		var pk int
		if err := c.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info(?) WHERE pk > 0`, t.name).Scan(&pk); err != nil {
			return err.Error()
		}
		if pk == 0 {
			return "table " + t.name + " has no primary key"
		}
	}
	fk, err := c.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err.Error()
	}
	defer fk.Close()
	if fk.Next() {
		var table string
		var rowid sql.NullInt64
		var parent string
		var idx int
		fk.Scan(&table, &rowid, &parent, &idx)
		return fmt.Sprintf("foreign key violation in %s (parent %s)", table, parent)
	}
	return ""
}

type ddlCase struct {
	name   string
	stmts  []string
	fkOff  bool // table rebuild procedure: foreign keys off before the transaction
	wantOK bool
}

var rebuild = []string{
	"CREATE TABLE data_new(id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', val REAL, tags TEXT)",
	"INSERT INTO data_new(id, name, val, tags) SELECT id, coalesce(name, ''), val, tags FROM data",
	"DROP TABLE data",
	"ALTER TABLE data_new RENAME TO data",
	"CREATE INDEX data_name ON data(name)",
}

var ddlCases = []ddlCase{
	{"create table with composite key", []string{"CREATE TABLE prices(coin TEXT, date TEXT, price REAL, PRIMARY KEY (coin, date))"}, false, true},
	{"create WITHOUT ROWID table", []string{"CREATE TABLE coins(symbol TEXT PRIMARY KEY) WITHOUT ROWID"}, false, true},
	{"add column", []string{"ALTER TABLE data ADD COLUMN note TEXT"}, false, true},
	{"rename column", []string{"ALTER TABLE data RENAME COLUMN name TO label"}, false, true},
	{"create index", []string{"CREATE INDEX data_val ON data(val)"}, false, true},
	{"rebuild table (documented procedure)", rebuild, true, true},

	{"drop internal table", []string{"DROP TABLE _burrow_changes"}, false, false},
	{"rename internal table", []string{"ALTER TABLE _burrow_meta RENAME TO meta_old"}, false, false},
	{"add column to internal table", []string{"ALTER TABLE _burrow_meta ADD COLUMN evil TEXT"}, false, false},
	{"index on internal table", []string{"CREATE INDEX evil ON _burrow_changes(op)"}, false, false},
	{"new table with internal name", []string{"CREATE TABLE _burrow_evil(x PRIMARY KEY)"}, false, false},
	{"rename agent table to internal name", []string{"ALTER TABLE data RENAME TO _burrow_data"}, false, false},
	{"trigger that clears the change log", []string{"CREATE TRIGGER t AFTER INSERT ON data BEGIN DELETE FROM _burrow_changes; END"}, false, false},
	{"temp trigger on the writer connection", []string{"CREATE TEMP TRIGGER t AFTER INSERT ON main.data BEGIN DELETE FROM _burrow_changes; END"}, false, false},
	{"SQL view", []string{"CREATE VIEW v AS SELECT * FROM data"}, false, false},
	{"table without primary key", []string{"CREATE TABLE nopk(a, b)"}, false, false},
	{"virtual table", []string{"CREATE VIRTUAL TABLE f USING fts5(x)"}, false, false},
	{"rebuild that loses parent rows", []string{rebuild[0], strings.Replace(rebuild[1], "FROM data", "FROM data WHERE id > 10", 1), rebuild[2], rebuild[3]}, true, false},
	{"attach on the writer", []string{"ATTACH '{OTHER}' AS o"}, false, false},
}

func fullSchema(c *sql.Conn) string {
	var s string
	c.QueryRowContext(ctx, `SELECT group_concat(type || name || coalesce(sql, ''), ';') FROM (SELECT * FROM sqlite_schema ORDER BY name)`).Scan(&s)
	return s
}

func runSchemaGuard(w *sql.Conn, other string) {
	fmt.Println("\n## Schema guard (migrations)\n")
	fmt.Println("Each case runs in `BEGIN IMMEDIATE`, then the guard, then `ROLLBACK`. The writer connection has `SQLITE_LIMIT_ATTACHED = 0`.\n")
	fmt.Println("| Case | Expect | SQLite | Guard | Rolled back clean | Result |")
	fmt.Println("|---|---|---|---|---|---|")
	initial := fullSchema(w)
	fails := 0
	for _, dc := range ddlCases {
		before, err := internalSnapshot(w)
		if err != nil {
			panic(err)
		}
		if dc.fkOff {
			w.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
		}
		if _, err := w.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			panic(err)
		}
		sqliteRes, guardRes := "ok", "-"
		var stmtErr error
		for _, s := range dc.stmts {
			if _, stmtErr = w.ExecContext(ctx, strings.ReplaceAll(s, "{OTHER}", other)); stmtErr != nil {
				sqliteRes = "error: " + short(stmtErr.Error())
				break
			}
		}
		g := ""
		if stmtErr == nil {
			g = schemaGuard(w, before)
			guardRes = "pass"
			if g != "" {
				guardRes = "**reject**: " + g
			}
		}
		w.ExecContext(ctx, "ROLLBACK")
		if dc.fkOff {
			w.ExecContext(ctx, "PRAGMA foreign_keys = ON")
		}
		clean := fullSchema(w) == initial
		var temp int
		w.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_temp_schema").Scan(&temp)
		clean = clean && temp == 0
		ok := stmtErr == nil && g == ""
		res := "OK"
		if ok != dc.wantOK || !clean {
			res = "**FAIL**"
			fails++
		}
		expect := "reject"
		if dc.wantOK {
			expect = "pass"
		}
		fmt.Printf("| %s | %s | %s | %s | %v | %s |\n", dc.name, expect, sqliteRes, guardRes, clean, res)
	}
	fmt.Printf("\n**Schema guard: %d of %d cases correct.**\n", len(ddlCases)-fails, len(ddlCases))

	// Guard cost with a larger schema: 100 extra tables, inside a rolled-back transaction.
	w.ExecContext(ctx, "BEGIN IMMEDIATE")
	for i := 0; i < 100; i++ {
		w.ExecContext(ctx, fmt.Sprintf("CREATE TABLE t%03d(id INTEGER PRIMARY KEY, a TEXT, b REAL)", i))
	}
	before, _ := internalSnapshot(w)
	start := time.Now()
	const n = 50
	for i := 0; i < n; i++ {
		if g := schemaGuard(w, before); g != "" {
			fmt.Println("unexpected guard result:", g)
		}
	}
	w.ExecContext(ctx, "ROLLBACK")
	fmt.Printf("\nGuard cost with 104 agent tables: %v per migration.\n", (time.Since(start) / n).Round(time.Microsecond))
}
