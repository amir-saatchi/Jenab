package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

// tablesFromExplain returns the tables a statement would open, by reading its bytecode.
// OpenRead/OpenWrite carry a b-tree root page, which maps to a table (or an index of a table)
// through sqlite_schema. VOpen means a virtual table (e.g. pragma_* functions).
func tablesFromExplain(db *sql.DB, query string) (tables []string, virtual bool, err error) {
	roots := map[int64]string{}
	rows, err := db.Query("SELECT rootpage, tbl_name FROM sqlite_schema WHERE rootpage > 0")
	if err != nil {
		return nil, false, err
	}
	for rows.Next() {
		var rp int64
		var name string
		rows.Scan(&rp, &name)
		roots[rp] = name
	}
	rows.Close()

	rows, err = db.Query("EXPLAIN " + query)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var addr, p1, p2, p3, p5 sql.NullInt64
		var opcode, p4, comment sql.NullString
		if err := rows.Scan(&addr, &opcode, &p1, &p2, &p3, &p4, &p5, &comment); err != nil {
			return nil, false, err
		}
		switch opcode.String {
		case "OpenRead", "OpenWrite", "ReopenIdx":
			if p3.Int64 != 0 { // another schema (temp or attached)
				seen[fmt.Sprintf("<schema %d>", p3.Int64)] = true
				continue
			}
			if name, ok := roots[p2.Int64]; ok {
				seen[name] = true
			} else {
				seen[fmt.Sprintf("<root %d>", p2.Int64)] = true
			}
		case "VOpen":
			virtual = true
		}
	}
	for n := range seen {
		tables = append(tables, n)
	}
	return tables, virtual, nil
}

func checkExplainTables(r *sql.DB) (bool, string) {
	cases := []struct {
		sql     string
		blocked bool
	}{
		{"SELECT * FROM data", false},
		{"SELECT d.name, count(*) FROM data d GROUP BY d.name", false},
		{"SELECT * FROM _burrow_meta", true},
		{"SELECT * FROM v_leak", true},
		{"WITH x AS (SELECT * FROM _burrow_meta) SELECT * FROM x", true},
		{"SELECT * FROM data WHERE id IN (SELECT rowid FROM _burrow_meta)", true},
		{"SELECT value FROM _burrow_meta WHERE key = 'schema_version'", true}, // uses the PK index
		{"SELECT * FROM pragma_table_info('_burrow_meta')", true},
	}
	var fails []string
	for _, c := range cases {
		tables, virtual, err := tablesFromExplain(r, c.sql)
		if err != nil {
			fails = append(fails, c.sql+": "+err.Error())
			continue
		}
		blocked := virtual
		for _, t := range tables {
			if strings.HasPrefix(strings.ToLower(t), "_burrow_") || strings.HasPrefix(t, "<") {
				blocked = true
			}
		}
		if blocked != c.blocked {
			fails = append(fails, fmt.Sprintf("`%s` expected blocked=%v, got tables=%v virtual=%v", c.sql, c.blocked, tables, virtual))
		}
	}
	if len(fails) > 0 {
		return false, strings.Join(fails, "; ")
	}
	return true, fmt.Sprintf("all %d cases correct (direct, view, CTE, subquery, index-only lookup, pragma_ function)", len(cases))
}

func checkSnapshotAlternatives(t target, path string, w *sql.DB, prefix string) (bool, string) {
	// A plain connection (not query_only), outside the reader pool, used only for snapshots.
	snap, err := t.open(path, false)
	if err != nil {
		return false, err.Error()
	}
	defer snap.Close()
	snap.SetMaxOpenConns(1)

	// Make the database big enough that the snapshot takes measurable time.
	tx, err := w.Begin()
	if err != nil {
		return false, err.Error()
	}
	for i := 0; i < 200_000; i++ {
		tx.Exec("INSERT INTO data(name, val) VALUES (?, ?)", "snapshot-filler-row", float64(i))
	}
	if err := tx.Commit(); err != nil {
		return false, err.Error()
	}

	var vacErr error
	var vacTime time.Duration
	done := make(chan struct{})
	go func() {
		start := time.Now()
		_, vacErr = snap.Exec("VACUUM INTO ?", prefix+"-vacuum.db")
		vacTime = time.Since(start)
		close(done)
	}()
	// The writer keeps committing small transactions while the snapshot runs.
	var maxLat time.Duration
	commits := 0
loop:
	for {
		select {
		case <-done:
			break loop
		default:
		}
		start := time.Now()
		if _, err := w.Exec("INSERT INTO data(name, val) VALUES ('during-snapshot', 0)"); err != nil {
			<-done
			return false, "writer failed during snapshot: " + err.Error()
		}
		if l := time.Since(start); l > maxLat {
			maxLat = l
		}
		commits++
	}
	if vacErr != nil {
		return false, vacErr.Error()
	}
	st, err := os.Stat(prefix + "-vacuum.db")
	if err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("VACUUM INTO took %v for %.1f MB; writer made %d commits meanwhile, max commit latency %v",
		vacTime.Round(time.Millisecond), float64(st.Size())/(1<<20), commits, maxLat.Round(time.Microsecond))
}
