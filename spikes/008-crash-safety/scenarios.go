package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const projectSchema = `
CREATE TABLE _burrow_meta(key TEXT PRIMARY KEY, value);
CREATE TABLE _burrow_changes(id INTEGER PRIMARY KEY, at TEXT, source TEXT, target TEXT, op TEXT, before TEXT, after TEXT);
CREATE TABLE _burrow_change_rows(change_id INTEGER NOT NULL REFERENCES _burrow_changes(id), table_name TEXT NOT NULL,
  row_key TEXT NOT NULL, PRIMARY KEY (table_name, row_key, change_id)) WITHOUT ROWID;
CREATE TABLE _burrow_runs(id INTEGER PRIMARY KEY, status TEXT, started_at TEXT, ended_at TEXT, rows INTEGER DEFAULT 0);
CREATE TABLE _burrow_views(id TEXT PRIMARY KEY, config TEXT, revision INTEGER);
CREATE TABLE _burrow_objects(key TEXT PRIMARY KEY, sha256 TEXT, size INTEGER, updated_at TEXT);
CREATE TABLE _burrow_object_versions(key TEXT, version INTEGER, sha256 TEXT, size INTEGER, PRIMARY KEY (key, version));
CREATE TABLE prices(coin TEXT, date TEXT, price REAL, _run_id INTEGER, PRIMARY KEY (coin, date));
INSERT INTO _burrow_meta VALUES ('schema_version', 1), ('rows_total', 0);
INSERT INTO _burrow_views VALUES ('price_table', '{"query":"SELECT coin, date, price FROM prices"}', 1);
`

func setupProject(dir string, baseRows int) error {
	db := openDB(filepath.Join(dir, "project.db"))
	defer db.Close()
	if _, err := db.Exec(projectSchema); err != nil {
		return err
	}
	if baseRows > 0 {
		_, err := db.Exec(`WITH RECURSIVE s(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM s WHERE i < ?)
			INSERT INTO prices SELECT 'BASE', printf('%08d', i), i * 0.5, 0 FROM s`, baseRows-1)
		if err != nil {
			return err
		}
		_, err = db.Exec("UPDATE _burrow_meta SET value = ? WHERE key = 'rows_total'", baseRows)
		return err
	}
	return nil
}

var scenarios = []scenario{
	{
		id: "S1", desc: "chunked pipeline write (100k rows in 500-row transactions)",
		minKill: 50 * time.Millisecond, maxKill: 1200 * time.Millisecond,
		setup: func(dir string) error { return setupProject(dir, 0) },
		work:  workPipeline,
		recover: func(dir string, db *sql.DB) map[string]int {
			r, _ := db.Exec("UPDATE _burrow_runs SET status = 'interrupted', ended_at = ? WHERE status = 'running'", now())
			n, _ := r.RowsAffected()
			return map[string]int{"runs marked interrupted": int(n)}
		},
		verify: verifyPipeline,
	},
	{
		id: "S2", desc: "migration with a table rebuild (200k rows)",
		minKill: 100 * time.Millisecond, maxKill: 3000 * time.Millisecond,
		setup:   func(dir string) error { return setupProject(dir, 200_000) },
		work:    workMigration,
		recover: func(string, *sql.DB) map[string]int { return nil },
		verify:  verifyMigration,
	},
	{
		id: "S3", desc: "bucket put of 50 MB files, with deletes",
		minKill: 100 * time.Millisecond, maxKill: 3000 * time.Millisecond,
		setup:   func(dir string) error { return setupProject(dir, 0) },
		work:    workBucket,
		recover: recoverBucket,
		verify:  verifyBucket,
		cleanup: cleanupBucket,
	},
	{
		id: "S3c", desc: "crash exactly after the rename into objects/, before the index row",
		crashAt: "index", minKill: time.Millisecond, maxKill: 2 * time.Millisecond,
		setup:   func(dir string) error { return setupProject(dir, 0) },
		work:    workBucket,
		recover: recoverBucket,
		verify:  verifyBucket,
		cleanup: cleanupBucket,
	},
	{
		id: "S4a", desc: "VACUUM INTO straight to the final file name", demo: true,
		minKill: 50 * time.Millisecond, maxKill: 1500 * time.Millisecond,
		setup: func(dir string) error { return setupProject(dir, 300_000) },
		work:  func(dir string) { workSnapshot(dir, false) },
		recover: func(dir string, _ *sql.DB) map[string]int {
			j, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*-journal"))
			return map[string]int{"snapshots with a hot -journal next to them": len(j)}
		},
		verify: func(dir string, db *sql.DB) []string { return verifySnapshots(dir, db, true) },
	},
	{
		id: "S4b", desc: "VACUUM INTO a .partial file, fsync, rename",
		minKill: 50 * time.Millisecond, maxKill: 1500 * time.Millisecond,
		setup: func(dir string) error { return setupProject(dir, 300_000) },
		work:  func(dir string) { workSnapshot(dir, true) },
		recover: func(dir string, _ *sql.DB) map[string]int {
			m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.partial"))
			j, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.partial-journal"))
			for _, f := range append(m, j...) {
				os.Remove(f)
			}
			return map[string]int{"partial snapshots removed": len(m), "their -journal files removed": len(j)}
		},
		verify: func(dir string, db *sql.DB) []string { return verifySnapshots(dir, db, false) },
	},
	{
		id: "S5a", desc: "chat message written to chats.db before the project change that refers to it",
		minKill: 50 * time.Millisecond, maxKill: 1500 * time.Millisecond,
		setup:   setupChats,
		work:    func(dir string) { workChats(dir, true) },
		recover: func(string, *sql.DB) map[string]int { return nil },
		verify:  verifyChats,
		cleanup: cleanupChats,
	},
	{
		id: "S5b", desc: "project change written before its chat message", demo: true,
		minKill: 50 * time.Millisecond, maxKill: 1500 * time.Millisecond,
		setup:   setupChats,
		work:    func(dir string) { workChats(dir, false) },
		recover: func(string, *sql.DB) map[string]int { return nil },
		verify:  verifyChats,
		cleanup: cleanupChats,
	},
}

// ---------------------------------------------------------------- S1 pipeline

func workPipeline(dir string) {
	db := openDB(filepath.Join(dir, "project.db"))
	fmt.Println("READY")
	for {
		phase("run start")
		res, err := db.Exec("INSERT INTO _burrow_runs(status, started_at) VALUES ('running', ?)", now())
		must(err)
		runID, _ := res.LastInsertId()
		for c := 0; c < 200; c++ {
			phase("chunk write")
			tx, err := db.Begin()
			must(err)
			res, err := tx.Exec("INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, ?, 'table:prices', 'insert')", now(), fmt.Sprint("run:", runID))
			must(err)
			changeID, _ := res.LastInsertId()
			ins, _ := tx.Prepare("INSERT INTO prices VALUES (?, ?, ?, ?)")
			cr, _ := tx.Prepare("INSERT INTO _burrow_change_rows VALUES (?, 'prices', ?)")
			coin := fmt.Sprint("R", runID)
			for r := 0; r < 500; r++ {
				date := fmt.Sprintf("%06d", c*500+r)
				_, err := ins.Exec(coin, date, float64(r), runID)
				must(err)
				_, err = cr.Exec(changeID, fmt.Sprintf(`["%s","%s"]`, coin, date))
				must(err)
			}
			ins.Close()
			cr.Close()
			_, err = tx.Exec("UPDATE _burrow_meta SET value = value + 500 WHERE key = 'rows_total'")
			must(err)
			_, err = tx.Exec("UPDATE _burrow_runs SET rows = rows + 500 WHERE id = ?", runID)
			must(err)
			phase("chunk commit")
			must(tx.Commit())
		}
		phase("run finish")
		_, err = db.Exec("UPDATE _burrow_runs SET status = 'ok', ended_at = ? WHERE id = ?", now(), runID)
		must(err)
	}
}

func verifyPipeline(dir string, db *sql.DB) []string {
	p := integrity(db, "project.db")
	rows := count(db, "SELECT count(*) FROM prices")
	if t := count(db, "SELECT value FROM _burrow_meta WHERE key = 'rows_total'"); t != rows {
		p = append(p, fmt.Sprintf("rows_total %d but %d rows", t, rows))
	}
	if rows%500 != 0 {
		p = append(p, fmt.Sprintf("%d rows is not a whole number of chunks", rows))
	}
	if n := count(db, "SELECT count(*) FROM _burrow_change_rows"); n != rows {
		p = append(p, fmt.Sprintf("%d change rows for %d rows", n, rows))
	}
	if n := count(db, `SELECT count(*) FROM prices WHERE NOT EXISTS (SELECT 1 FROM _burrow_change_rows r
		WHERE r.table_name = 'prices' AND r.row_key = json_array(prices.coin, prices.date))`); n > 0 {
		p = append(p, fmt.Sprintf("%d rows without a change-log entry", n))
	}
	if n := count(db, "SELECT count(*) FROM _burrow_change_rows WHERE change_id NOT IN (SELECT id FROM _burrow_changes)"); n > 0 {
		p = append(p, fmt.Sprintf("%d change rows without a change", n))
	}
	if n := count(db, "SELECT coalesce(sum(rows), 0) FROM _burrow_runs"); n != rows {
		p = append(p, fmt.Sprintf("runs report %d rows, table has %d", n, rows))
	}
	if n := count(db, "SELECT count(*) FROM _burrow_runs WHERE status = 'running'"); n > 0 {
		p = append(p, fmt.Sprintf("%d runs still 'running'", n))
	}
	return p
}

// ---------------------------------------------------------------- S2 migration

var shapes = [2]struct{ create, view string }{
	{"CREATE TABLE prices_new(coin TEXT, date TEXT, price REAL, _run_id INTEGER, PRIMARY KEY (coin, date))",
		`{"query":"SELECT coin, date, price FROM prices"}`},
	{"CREATE TABLE prices_new(coin TEXT, date TEXT, price REAL, _run_id INTEGER, note TEXT, PRIMARY KEY (date, coin))",
		`{"query":"SELECT coin, date, price, note FROM prices"}`},
}

func workMigration(dir string) {
	db := openDB(filepath.Join(dir, "project.db"))
	c, err := db.Conn(context.Background())
	must(err)
	ctx := context.Background()
	fmt.Println("READY")
	for {
		var v int
		must(c.QueryRowContext(ctx, "SELECT value FROM _burrow_meta WHERE key = 'schema_version'").Scan(&v))
		target := shapes[v%2] // v odd = shape 0 now, so target shape 1; and back
		step := func(name, q string, args ...any) {
			phase(name)
			_, err := c.ExecContext(ctx, q, args...)
			must(err)
		}
		step("foreign keys off", "PRAGMA foreign_keys = OFF")
		step("begin", "BEGIN IMMEDIATE")
		step("create", target.create)
		step("copy", "INSERT INTO prices_new(coin, date, price, _run_id) SELECT coin, date, price, _run_id FROM prices")
		step("drop", "DROP TABLE prices")
		step("rename", "ALTER TABLE prices_new RENAME TO prices")
		step("dependents", "UPDATE _burrow_views SET config = ?, revision = revision + 1 WHERE id = 'price_table'", target.view)
		step("version", "UPDATE _burrow_meta SET value = value + 1 WHERE key = 'schema_version'")
		step("change log", "INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, ?, 'schema', 'migrate')", now(), fmt.Sprint("migration:", v))
		phase("foreign_key_check")
		rows, err := c.QueryContext(ctx, "PRAGMA foreign_key_check")
		must(err)
		rows.Close()
		step("commit", "COMMIT")
		step("foreign keys on", "PRAGMA foreign_keys = ON")
	}
}

func verifyMigration(dir string, db *sql.DB) []string {
	p := integrity(db, "project.db")
	v := count(db, "SELECT value FROM _burrow_meta WHERE key = 'schema_version'")
	var sqlText, view string
	db.QueryRow("SELECT sql FROM sqlite_schema WHERE name = 'prices'").Scan(&sqlText)
	db.QueryRow("SELECT config FROM _burrow_views WHERE id = 'price_table'").Scan(&view)
	hasNote := strings.Contains(sqlText, "note")
	if want := v%2 == 0; hasNote != want {
		p = append(p, fmt.Sprintf("schema_version %d does not match table shape (note column: %v)", v, hasNote))
	}
	if strings.Contains(view, "note") != hasNote {
		p = append(p, "view config does not match table shape")
	}
	if n := count(db, "SELECT count(*) FROM _burrow_changes WHERE op = 'migrate'"); n != v-1 {
		p = append(p, fmt.Sprintf("%d migrate entries for schema_version %d", n, v))
	}
	if n := count(db, "SELECT count(*) FROM sqlite_schema WHERE name = 'prices_new'"); n > 0 {
		p = append(p, "prices_new left behind")
	}
	if n := count(db, "SELECT count(*) FROM prices"); n != 200_000 {
		p = append(p, fmt.Sprintf("prices has %d rows, want 200000", n))
	}
	var fk int
	db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		p = append(p, "foreign keys off after reopen")
	}
	return p
}

// ---------------------------------------------------------------- S3 bucket

const blockSize = 1 << 20
const blocks = 50

func objectPath(dir, sum string) string {
	return filepath.Join(dir, "objects", sum[:2], sum[2:4], sum)
}

func workBucket(dir string) {
	db := openDB(filepath.Join(dir, "project.db"))
	block := make([]byte, blockSize)
	for i := range block {
		block[i] = byte(rand.N(256))
	}
	seed := time.Now().UnixNano()
	keys := []string{"images/a.png", "reports/b.pdf", "data/c.bin"}
	fmt.Println("READY")
	for i := 0; ; i++ {
		key := keys[i%len(keys)]
		if i%4 == 3 {
			phase("delete")
			tx, _ := db.Begin()
			tx.Exec("DELETE FROM _burrow_objects WHERE key = ?", key)
			tx.Exec("INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, 'user', ?, 'delete')", now(), "object:"+key)
			must(tx.Commit())
			continue
		}
		// 1. Stream into tmp/, hashing on the way.
		phase("stream to tmp")
		f, err := os.CreateTemp(filepath.Join(dir, "tmp"), "put-*")
		must(err)
		h := sha256.New()
		w := io.MultiWriter(f, h)
		var header [16]byte
		for b := 0; b < blocks; b++ {
			copy(header[:], fmt.Sprintf("%x", seed+int64(i*1000+b)))
			w.Write(header[:])
			w.Write(block)
		}
		phase("fsync")
		must(f.Sync())
		must(f.Close())
		sum := hex.EncodeToString(h.Sum(nil))
		final := objectPath(dir, sum)
		// 2. Rename into objects/.
		phase("rename")
		must(os.MkdirAll(filepath.Dir(final), 0o755))
		if _, err := os.Stat(final); err == nil {
			os.Remove(f.Name())
		} else {
			must(os.Rename(f.Name(), final))
		}
		// 3. Index row, version row and change-log entry in one transaction.
		phase("index")
		size := int64(blocks * (blockSize + 16))
		tx, err := db.Begin()
		must(err)
		var ver int64
		tx.QueryRow("SELECT coalesce(max(version), 0) + 1 FROM _burrow_object_versions WHERE key = ?", key).Scan(&ver)
		_, err = tx.Exec(`INSERT INTO _burrow_objects VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET
			sha256 = excluded.sha256, size = excluded.size, updated_at = excluded.updated_at`, key, sum, size, now())
		must(err)
		_, err = tx.Exec("INSERT INTO _burrow_object_versions VALUES (?, ?, ?, ?)", key, ver, sum, size)
		must(err)
		_, err = tx.Exec("INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, 'agent', ?, 'save')", now(), "object:"+key)
		must(err)
		phase("index commit")
		must(tx.Commit())
	}
}

func referenced(db *sql.DB) map[string]bool {
	ref := map[string]bool{}
	rows, err := db.Query("SELECT sha256 FROM _burrow_objects UNION SELECT sha256 FROM _burrow_object_versions")
	must(err)
	defer rows.Close()
	for rows.Next() {
		var s string
		rows.Scan(&s)
		ref[s] = true
	}
	return ref
}

func objectFiles(dir string) []string {
	var files []string
	filepath.WalkDir(filepath.Join(dir, "objects"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	return files
}

func recoverBucket(dir string, db *sql.DB) map[string]int {
	tmp, _ := filepath.Glob(filepath.Join(dir, "tmp", "*"))
	for _, f := range tmp {
		os.Remove(f)
	}
	ref := referenced(db)
	orphans := 0
	for _, f := range objectFiles(dir) {
		if !ref[filepath.Base(f)] {
			os.Remove(f)
			orphans++
		}
	}
	return map[string]int{"half-written tmp files removed": len(tmp), "orphan object files swept": orphans}
}

func verifyBucket(dir string, db *sql.DB) []string {
	p := integrity(db, "project.db")
	rows, err := db.Query("SELECT key, sha256, size FROM _burrow_objects UNION ALL SELECT key, sha256, size FROM _burrow_object_versions")
	must(err)
	for rows.Next() {
		var key, sum string
		var size int64
		rows.Scan(&key, &sum, &size)
		st, err := os.Stat(objectPath(dir, sum))
		if err != nil {
			p = append(p, fmt.Sprintf("index row %s points to a missing file", key))
		} else if st.Size() != size {
			p = append(p, fmt.Sprintf("%s: file size %d, index says %d", key, st.Size(), size))
		}
	}
	rows.Close()
	for _, f := range objectFiles(dir) {
		fh, err := os.Open(f)
		must(err)
		h := sha256.New()
		io.Copy(h, fh)
		fh.Close()
		if hex.EncodeToString(h.Sum(nil)) != filepath.Base(f) {
			p = append(p, "object file content does not match its name: "+filepath.Base(f)[:12])
		}
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "tmp", "*")); len(tmp) > 0 {
		p = append(p, "tmp/ not empty after recovery")
	}
	return p
}

// cleanupBucket keeps only current versions so disk use stays small between iterations.
func cleanupBucket(dir string, db *sql.DB) {
	db.Exec("DELETE FROM _burrow_object_versions WHERE sha256 NOT IN (SELECT sha256 FROM _burrow_objects)")
	ref := referenced(db)
	for _, f := range objectFiles(dir) {
		if !ref[filepath.Base(f)] {
			os.Remove(f)
		}
	}
}

// ---------------------------------------------------------------- S4 snapshots

func workSnapshot(dir string, partial bool) {
	path := filepath.Join(dir, "project.db")
	w := openDB(path)
	snap := openDB(path) // separate plain connection, outside the reader pool
	fmt.Println("READY")
	go func() {
		for i := 0; ; i++ {
			w.Exec("INSERT INTO prices VALUES ('W', ?, 1, 0)", fmt.Sprintf("%s-%d", now(), i))
			time.Sleep(time.Millisecond)
		}
	}()
	for {
		name := filepath.Join(dir, "snapshots", fmt.Sprintf("snap-%d.db", time.Now().UnixNano()))
		target := name
		if partial {
			target = name + ".partial"
		}
		phase("vacuum into")
		_, err := snap.Exec("VACUUM INTO ?", target)
		must(err)
		if partial {
			phase("fsync")
			f, err := os.OpenFile(target, os.O_RDWR, 0)
			must(err)
			must(f.Sync())
			f.Close()
			phase("rename")
			must(os.Rename(target, name))
		}
		phase("prune")
		m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "snap-*.db"))
		sort.Strings(m)
		for len(m) > 5 {
			os.Remove(m[0])
			m = m[1:]
		}
	}
}

func verifySnapshots(dir string, db *sql.DB, removeBroken bool) []string {
	p := integrity(db, "project.db")
	m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "snap-*.db"))
	for _, f := range m {
		problem := ""
		// Opened the way a user restoring it would: read-write, so SQLite rolls back a hot journal.
		sdb, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f))
		if err == nil {
			var res string
			if err := sdb.QueryRow("PRAGMA quick_check").Scan(&res); err != nil {
				problem = err.Error()
			} else if res != "ok" {
				problem = res
			} else {
				var n int64
				if err := sdb.QueryRow("SELECT count(*) FROM prices").Scan(&n); err != nil {
					problem = err.Error()
				} else if n < 300_000 {
					problem = fmt.Sprintf("only %d rows", n)
				}
			}
			sdb.Close()
		} else {
			problem = err.Error()
		}
		if _, err := os.Stat(f + "-journal"); err == nil && problem == "" {
			problem = "left a -journal file"
		}
		if problem != "" {
			st, _ := os.Stat(f)
			sz := int64(0)
			if st != nil {
				sz = st.Size()
			}
			if len(problem) > 60 {
				problem = problem[:60]
			}
			p = append(p, fmt.Sprintf("snapshot %s (%.1f MB) is broken: %s", filepath.Base(f), float64(sz)/(1<<20), problem))
			if removeBroken { // report each broken file once
				os.Remove(f)
				os.Remove(f + "-journal")
			}
		}
	}
	return p
}

// ---------------------------------------------------------------- S5 chats.db and project.db

func setupChats(dir string) error {
	if err := setupProject(dir, 0); err != nil {
		return err
	}
	c := openDB(filepath.Join(dir, "chats.db"))
	defer c.Close()
	_, err := c.Exec("CREATE TABLE messages(id INTEGER PRIMARY KEY, chat_id INTEGER, turn INTEGER, role TEXT, content TEXT)")
	return err
}

func workChats(dir string, messageFirst bool) {
	project := openDB(filepath.Join(dir, "project.db"))
	chats := openDB(filepath.Join(dir, "chats.db"))
	var maxID int64
	chats.QueryRow("SELECT coalesce(max(id), 0) FROM messages").Scan(&maxID)
	var nextID atomic.Int64 // IDs are made in Go (ULIDs in Burrow), before anything is written
	nextID.Store(maxID + 1000)
	var turn int64
	project.QueryRow("SELECT count(*) FROM _burrow_changes").Scan(&turn)
	fmt.Println("READY")
	go func() { // another chat streaming its parts
		for {
			chats.Exec("INSERT INTO messages VALUES (?, 2, 0, 'stream', 'part')", nextID.Add(1))
			time.Sleep(500 * time.Microsecond)
		}
	}()
	writeMessage := func(id int64) {
		phase("message")
		_, err := chats.Exec("INSERT INTO messages VALUES (?, 1, ?, 'assistant', 'tool call: insert')", id, turn)
		must(err)
	}
	writeChange := func(id int64) {
		phase("project change")
		tx, err := project.Begin()
		must(err)
		key := fmt.Sprint(id)
		tx.Exec("INSERT INTO prices VALUES ('T', ?, 1, 0)", key)
		res, err := tx.Exec("INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, ?, 'table:prices', 'insert')", now(), fmt.Sprint("message:", id))
		must(err)
		cid, _ := res.LastInsertId()
		tx.Exec("INSERT INTO _burrow_change_rows VALUES (?, 'prices', ?)", cid, fmt.Sprintf(`["T","%s"]`, key))
		must(tx.Commit())
	}
	for ; ; turn++ {
		id := nextID.Add(1)
		if messageFirst {
			writeMessage(id)
			writeChange(id)
		} else {
			writeChange(id)
			writeMessage(id)
		}
	}
}

func verifyChats(dir string, db *sql.DB) []string {
	p := integrity(db, "project.db")
	c := openDB(filepath.Join(dir, "chats.db"))
	p = append(p, integrity(c, "chats.db")...)
	c.Close()
	chatsPath := filepath.ToSlash(filepath.Join(dir, "chats.db"))
	must2(db.Exec("ATTACH ? AS chats", chatsPath))
	defer db.Exec("DETACH chats")
	dangling := count(db, `SELECT count(*) FROM _burrow_changes ch WHERE ch.source LIKE 'message:%'
		AND NOT EXISTS (SELECT 1 FROM chats.messages m WHERE m.id = CAST(substr(ch.source, 9) AS INTEGER))`)
	if dangling > 0 {
		p = append(p, fmt.Sprintf("%d change-log entries point to a message that does not exist", dangling))
	}
	return p
}

func must2(_ any, err error) { must(err) }

const danglingChanges = `SELECT id FROM _burrow_changes ch WHERE ch.source LIKE 'message:%'
	AND NOT EXISTS (SELECT 1 FROM chats.messages m WHERE m.id = CAST(substr(ch.source, 9) AS INTEGER))`

// cleanupChats removes dangling entries so each one is reported only once.
func cleanupChats(dir string, db *sql.DB) {
	must2(db.Exec("ATTACH ? AS chats", filepath.ToSlash(filepath.Join(dir, "chats.db"))))
	db.Exec("DELETE FROM _burrow_change_rows WHERE change_id IN (" + danglingChanges + ")")
	db.Exec("DELETE FROM _burrow_changes WHERE id IN (" + danglingChanges + ")")
	db.Exec("DETACH chats")
}
