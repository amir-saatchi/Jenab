// Quick check: can modernc guard agent SQL without an authorizer?
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
)

const limitAttached = 7 // SQLITE_LIMIT_ATTACHED

func main() {
	dir, _ := os.MkdirTemp("", "guard")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "p.db")
	other := filepath.Join(dir, "other.db")

	w, _ := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	w.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT); INSERT INTO t(v) VALUES('a'),('b');`)
	o, _ := sql.Open("sqlite", "file:"+other)
	o.Exec(`CREATE TABLE secret(x); INSERT INTO secret VALUES('leak')`)
	o.Close()

	r, _ := sql.Open("sqlite", "file:"+path+"?_pragma=query_only(1)")
	r.SetMaxOpenConns(1)
	ctx := context.Background()
	c, _ := r.Conn(ctx)

	// 1. ATTACH before and after the limit
	_, err := c.ExecContext(ctx, "ATTACH ? AS o", other)
	fmt.Println("attach, no limit:", errStr(err))
	c.ExecContext(ctx, "DETACH o")
	old, err := sqlite.Limit(c, limitAttached, 0)
	fmt.Println("Limit(ATTACHED,0): old =", old, "err =", errStr(err))
	_, err = c.ExecContext(ctx, "ATTACH ? AS o", other)
	fmt.Println("attach, limit 0:", errStr(err))

	// 2. Writes on query_only
	_, err = c.ExecContext(ctx, "DELETE FROM t")
	fmt.Println("delete on query_only:", errStr(err))
	_, err = c.ExecContext(ctx, "CREATE TABLE x(a)")
	fmt.Println("create on query_only:", errStr(err))

	// 3. Multi-statement in one Query call
	rows, err := c.QueryContext(ctx, "SELECT 1; DELETE FROM t")
	if err == nil {
		rows.Close()
	}
	fmt.Println("query with 2 statements:", errStr(err))
	var n int
	c.QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&n)
	fmt.Println("rows in t after:", n)

	// 4. Timeout interrupts a runaway query
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = c.QueryRowContext(tctx, `WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r`).Scan(&n)
	fmt.Printf("runaway query: stopped after %v, err = %s\n", time.Since(start).Round(time.Millisecond), errStr(err))
	c.Close()
}

func errStr(err error) string {
	if err == nil {
		return "OK (no error)"
	}
	return err.Error()
}
