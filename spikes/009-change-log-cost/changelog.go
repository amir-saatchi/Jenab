package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ---------------------------------------------------------------- schema

type layout struct {
	name, ddl string
}

var layouts = []layout{
	{"A: WITHOUT ROWID, key (table_name, row_key, change_id)", `
CREATE TABLE _burrow_change_rows(change_id INTEGER NOT NULL, table_name TEXT NOT NULL, row_key TEXT NOT NULL,
  kind TEXT, before TEXT, PRIMARY KEY (table_name, row_key, change_id)) WITHOUT ROWID;
CREATE INDEX _burrow_change_rows_change ON _burrow_change_rows(change_id);`},
	{"B: rowid table + index (table_name, row_key, change_id)", `
CREATE TABLE _burrow_change_rows(change_id INTEGER NOT NULL, table_name TEXT NOT NULL, row_key TEXT NOT NULL,
  kind TEXT, before TEXT);
CREATE INDEX _burrow_change_rows_key ON _burrow_change_rows(table_name, row_key, change_id);
CREATE INDEX _burrow_change_rows_change ON _burrow_change_rows(change_id);`},
}

const baseSchema = `
CREATE TABLE _burrow_meta(key TEXT PRIMARY KEY, value);
CREATE TABLE _burrow_changes(id INTEGER PRIMARY KEY, at TEXT, source TEXT, target TEXT, op TEXT, before TEXT, after TEXT);
CREATE TABLE _burrow_memory(scope TEXT, section TEXT, content TEXT, revision INTEGER, PRIMARY KEY (scope, section));
CREATE TABLE _burrow_views(id TEXT PRIMARY KEY, config TEXT, revision INTEGER);
CREATE TABLE prices(coin TEXT, date TEXT, price REAL, volume REAL, source TEXT, _run_id INTEGER, _fetched_at TEXT,
  PRIMARY KEY (coin, date));
CREATE TABLE notes(id INTEGER PRIMARY KEY, chat TEXT, text TEXT);
CREATE TEMP TABLE _chunk_before(row_key TEXT PRIMARY KEY, before TEXT);
`

// ---------------------------------------------------------------- tables

type table struct {
	name     string
	cols, pk []string
}

var prices = table{"prices", []string{"coin", "date", "price", "volume", "source", "_run_id", "_fetched_at"}, []string{"coin", "date"}}
var notes = table{"notes", []string{"id", "chat", "text"}, []string{"id"}}

func (t table) upsertSQL() string {
	var set []string
	for _, c := range t.cols {
		if !slices.Contains(t.pk, c) {
			set = append(set, c+" = excluded."+c)
		}
	}
	return fmt.Sprintf("INSERT INTO %s(%s) VALUES (%s) ON CONFLICT(%s) DO UPDATE SET %s",
		t.name, strings.Join(t.cols, ", "), strings.TrimSuffix(strings.Repeat("?, ", len(t.cols)), ", "),
		strings.Join(t.pk, ", "), strings.Join(set, ", "))
}

// keyMatch is "(coin, date) IN (SELECT value->>0, value->>1 FROM <src>)" for a JSON array of keys.
func (t table) keyIn(expr, src string) string {
	var parts []string
	for i := range t.pk {
		parts = append(parts, fmt.Sprintf("%s->>%d", expr, i))
	}
	return fmt.Sprintf("(%s) IN (SELECT %s FROM %s)", strings.Join(t.pk, ", "), strings.Join(parts, ", "), src)
}

func (t table) jsonArray() string { return "json_array(" + strings.Join(t.pk, ", ") + ")" }

func (t table) jsonObject() string {
	var parts []string
	for _, c := range t.cols {
		parts = append(parts, fmt.Sprintf("'%s', %s", c, c))
	}
	return "json_object(" + strings.Join(parts, ", ") + ")"
}

func (t table) keysJSON(rows [][]any) string {
	var keys [][]any
	for _, r := range rows {
		var k []any
		for _, p := range t.pk {
			k = append(k, r[slices.Index(t.cols, p)])
		}
		keys = append(keys, k)
	}
	b, _ := json.Marshal(keys)
	return string(b)
}

// ---------------------------------------------------------------- writes

type mode int

const (
	modePlain  mode = iota // no change log at all
	modeKeys               // change entry + row keys, no before-images
	modeBefore             // change entry + row keys + before-images (SPEC)
)

var modeNames = []string{"plain upsert, no change log", "change log, row keys only", "change log, row keys + before-images"}

// writeChunk upserts rows in one transaction and records the change.
func writeChunk(db *sql.DB, t table, rows [][]any, m mode, source, at string) int64 {
	tx, err := db.Begin()
	must(err)
	defer tx.Rollback()
	keys := t.keysJSON(rows)
	if m == modeBefore {
		exec(tx, "DELETE FROM temp._chunk_before")
		exec(tx, fmt.Sprintf("INSERT INTO temp._chunk_before SELECT %s, %s FROM %s WHERE %s",
			t.jsonArray(), t.jsonObject(), t.name, t.keyIn("value", "json_each(?)")), keys)
	}
	st, err := tx.Prepare(t.upsertSQL())
	must(err)
	for _, r := range rows {
		_, err := st.Exec(r...)
		must(err)
	}
	st.Close()
	var cid int64
	if m != modePlain {
		res := exec(tx, "INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, ?, ?, 'upsert')", at, source, "table:"+t.name)
		cid, _ = res.LastInsertId()
		if m == modeKeys {
			exec(tx, "INSERT INTO _burrow_change_rows(change_id, table_name, row_key) SELECT ?, ?, value FROM json_each(?)", cid, t.name, keys)
		} else {
			exec(tx, `INSERT INTO _burrow_change_rows(change_id, table_name, row_key, kind, before)
				SELECT ?, ?, k.value, CASE WHEN b.before IS NULL THEN 'i' ELSE 'u' END, b.before
				FROM json_each(?) k LEFT JOIN temp._chunk_before b ON b.row_key = k.value`, cid, t.name, keys)
		}
	}
	must(tx.Commit())
	return cid
}

// writeConfig records a memory or config edit: one row, before-image in _burrow_changes.before.
func writeConfig(db *sql.DB, kind, id, content, source, at string) {
	tx, err := db.Begin()
	must(err)
	defer tx.Rollback()
	var old sql.NullString
	var q, up string
	if kind == "memory" {
		q = "SELECT content FROM _burrow_memory WHERE scope = 'project' AND section = ?"
		up = "INSERT INTO _burrow_memory VALUES ('project', ?, ?, 1) ON CONFLICT DO UPDATE SET content = excluded.content, revision = revision + 1"
	} else {
		q = "SELECT config FROM _burrow_views WHERE id = ?"
		up = "INSERT INTO _burrow_views VALUES (?, ?, 1) ON CONFLICT DO UPDATE SET config = excluded.config, revision = revision + 1"
	}
	tx.QueryRow(q, id).Scan(&old)
	exec(tx, up, id, content)
	res := exec(tx, "INSERT INTO _burrow_changes(at, source, target, op, before) VALUES (?, ?, ?, 'save', ?)", at, source, kind+":"+id, old)
	cid, _ := res.LastInsertId()
	exec(tx, "INSERT INTO _burrow_change_rows(change_id, table_name, row_key, kind) VALUES (?, ?, ?, 'u')", cid, kind, id)
	must(tx.Commit())
}

// ---------------------------------------------------------------- undo

func idsJSON(ids []int64) string { b, _ := json.Marshal(ids); return string(b) }

// conflicts returns row keys touched by the given changes and by a later change outside them.
func conflicts(db *sql.DB, ids []int64) []string {
	rows, err := db.Query(`SELECT DISTINCT r.row_key FROM _burrow_change_rows r
		JOIN _burrow_change_rows l ON l.table_name = r.table_name AND l.row_key = r.row_key AND l.change_id > r.change_id
		WHERE r.change_id IN (SELECT value FROM json_each(?1))
		  AND l.change_id NOT IN (SELECT value FROM json_each(?1))`, idsJSON(ids))
	must(err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		rows.Scan(&k)
		out = append(out, k)
	}
	return out
}

// undo reverts the given changes in reverse order, skipping the given row keys.
// The undo itself is recorded as a change with before-images, so it can be redone.
func undo(db *sql.DB, t table, ids []int64, skip []string, source, at string) int64 {
	tx, err := db.Begin()
	must(err)
	defer tx.Rollback()
	skipJSON, _ := json.Marshal(append([]string{}, skip...))
	ids = slices.Clone(ids)
	slices.Sort(ids)
	slices.Reverse(ids)
	affected := fmt.Sprintf(`(SELECT row_key FROM _burrow_change_rows WHERE change_id IN (SELECT value FROM json_each('%s'))
		AND row_key NOT IN (SELECT value FROM json_each('%s')))`, idsJSON(ids), skipJSON)

	// Current values of the rows the undo will touch, for the undo's own change entry.
	exec(tx, "DELETE FROM temp._chunk_before")
	exec(tx, fmt.Sprintf("INSERT OR IGNORE INTO temp._chunk_before SELECT %s, %s FROM %s WHERE %s",
		t.jsonArray(), t.jsonObject(), t.name, t.keyIn("row_key", affected)))

	// Rows without a before-image did not exist before the change: delete them.
	// Rows with one are put back with an upsert, which also re-creates deleted rows.
	var extract []string
	for _, c := range t.cols {
		extract = append(extract, fmt.Sprintf("before->>'%s'", c))
	}
	restore := strings.Replace(t.upsertSQL(), "VALUES ("+strings.TrimSuffix(strings.Repeat("?, ", len(t.cols)), ", ")+")",
		fmt.Sprintf(`SELECT %s FROM _burrow_change_rows WHERE change_id = ? AND before IS NOT NULL
			AND row_key NOT IN (SELECT value FROM json_each(?))`, strings.Join(extract, ", ")), 1)
	for _, id := range ids {
		exec(tx, fmt.Sprintf(`DELETE FROM %s WHERE %s`, t.name,
			t.keyIn("row_key", `_burrow_change_rows WHERE change_id = ? AND before IS NULL AND row_key NOT IN (SELECT value FROM json_each(?))`)),
			id, string(skipJSON))
		exec(tx, restore, id, string(skipJSON))
	}
	res := exec(tx, "INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, ?, ?, 'undo')", at, source, "table:"+t.name)
	cid, _ := res.LastInsertId()
	exec(tx, fmt.Sprintf(`INSERT INTO _burrow_change_rows(change_id, table_name, row_key, kind, before)
		SELECT DISTINCT ?, ?, a.row_key, CASE WHEN b.before IS NULL THEN 'i' ELSE 'u' END, b.before
		FROM %s a LEFT JOIN temp._chunk_before b ON b.row_key = a.row_key`, affected), cid, t.name)
	must(tx.Commit())
	return cid
}

// ---------------------------------------------------------------- helpers

type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

func exec(e execer, q string, args ...any) sql.Result {
	r, err := e.Exec(q, args...)
	if err != nil {
		panic(fmt.Sprintf("%v\n%s", err, q))
	}
	return r
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
