package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/limit"
)

// Real SQLite in t.TempDir(), outside synctest.

func openTestProject(t *testing.T) (*ProjectDB, string) {
	t.Helper()
	dir := t.TempDir()
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(context.Background()) })
	return p, dir
}

func mustExec(t *testing.T, db *DB, q string, args ...any) {
	t.Helper()
	if _, err := Do(context.Background(), db, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec(q, args...)
		return struct{}{}, err
	}); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func one[T any](t *testing.T, db *DB, q string, args ...any) T {
	t.Helper()
	rows, err := Query(context.Background(), db, q, args, func(r *sql.Rows) (T, error) {
		var v T
		return v, r.Scan(&v)
	})
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if len(rows) != 1 {
		t.Fatalf("%s: %d rows, want 1", q, len(rows))
	}
	return rows[0]
}

func TestOpenNewProject(t *testing.T) {
	p, dir := openTestProject(t)
	if _, err := os.Stat(filepath.Join(dir, "project.db")); err != nil {
		t.Fatal(err)
	}
	if v := one[int](t, p.DB, "PRAGMA user_version"); v != ProjectFormat.current() {
		t.Errorf("user_version = %d, want %d", v, ProjectFormat.current())
	}
	if m := one[string](t, p.DB, "PRAGMA journal_mode"); m != "wal" {
		t.Errorf("journal_mode = %s", m)
	}
	for _, tbl := range []string{"_jenab_meta", "_jenab_approvals"} {
		if n := one[int](t, p.DB, "SELECT count(*) FROM sqlite_schema WHERE name = ?", tbl); n != 1 {
			t.Errorf("table %s missing", tbl)
		}
	}
	if err := p.SetMeta(context.Background(), map[string]string{"name": "Bitcoin", "project_id": "01J"}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetMeta(context.Background(), map[string]string{"name": "بیت‌کوین"}); err != nil {
		t.Fatal(err)
	}
	m, err := p.Meta(context.Background())
	if err != nil || m["name"] != "بیت‌کوین" || m["project_id"] != "01J" {
		t.Errorf("Meta = %v, %v", m, err)
	}
}

func TestWriterSettings(t *testing.T) {
	p, _ := openTestProject(t)
	got, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) ([3]int, error) {
		var r [3]int
		for i, q := range []string{"PRAGMA cache_size", "PRAGMA journal_size_limit", "PRAGMA foreign_keys"} {
			if err := tx.QueryRow(q).Scan(&r[i]); err != nil {
				return r, err
			}
		}
		return r, nil
	})
	if err != nil || got != [3]int{-65536, 67108864, 1} {
		t.Errorf("cache_size, journal_size_limit, foreign_keys = %v, %v", got, err)
	}
	other := filepath.Join(t.TempDir(), "other.db")
	_, err = Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec("ATTACH ? AS o", other)
		return struct{}{}, err
	})
	if err == nil {
		t.Error("the writer could ATTACH another file")
	}
}

func TestReadersAreGuarded(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	mustExec(t, p.DB, "INSERT INTO t (v) VALUES ('a')")
	ctx := context.Background()
	noRows := func(*sql.Rows) (int, error) { return 0, nil }

	if v := one[int](t, p.DB, "PRAGMA query_only"); v != 1 {
		t.Errorf("query_only = %d", v)
	}
	if _, err := Query(ctx, p.DB, "INSERT INTO t (v) VALUES ('b')", nil, noRows); err == nil {
		t.Error("a reader could write")
	}
	other := filepath.Join(t.TempDir(), "other.db")
	if _, err := Query(ctx, p.DB, "ATTACH ? AS o", []any{other}, noRows); err == nil {
		t.Error("a reader could ATTACH another file")
	}
	if _, err := Query(ctx, p.DB, "SELECT length(zeroblob(20000000))", nil, noRows); err == nil || !strings.Contains(err.Error(), "too big") {
		t.Errorf("a 20 MB blob on a reader: %v, want 'too big'", err)
	}
	// A guard switched off on a connection comes back when it is taken again.
	for range 8 {
		Query(ctx, p.DB, "PRAGMA query_only = OFF", nil, noRows)
	}
	for range 8 {
		if v := one[int](t, p.DB, "PRAGMA query_only"); v != 1 {
			t.Fatal("query_only stayed off on a pooled connection")
		}
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM t"); n != 1 {
		t.Errorf("rows = %d", n)
	}
}

func TestQueryTimeoutInterrupts(t *testing.T) {
	p, _ := openTestProject(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Query(ctx, p.DB, "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM r) SELECT count(*) FROM r", nil,
		func(r *sql.Rows) (int, error) { var n int; return n, r.Scan(&n) })
	if err == nil {
		t.Fatal("a runaway query finished")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("stopped after %v", d)
	}
}

// waitBusy waits until the writer is running a request.
func waitBusy(t *testing.T, db *DB) {
	t.Helper()
	for i := 0; !db.Stats().Busy; i++ {
		if i > 500 {
			t.Fatal("the writer never started the request")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// holdWriter starts a write transaction that stays open until release.
func holdWriter(t *testing.T, db *DB) (release func()) {
	ch := make(chan struct{})
	go Do(context.Background(), db, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		if _, err := tx.Exec("INSERT INTO t (v) VALUES ('held')"); err != nil {
			return struct{}{}, err
		}
		<-ch
		return struct{}{}, nil
	})
	waitBusy(t, db)
	return func() { close(ch) }
}

func TestReadsDontWaitForTheWriter(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	release := holdWriter(t, p.DB)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rows, err := Query(ctx, p.DB, "SELECT count(*) FROM t", nil, func(r *sql.Rows) (int, error) { var n int; return n, r.Scan(&n) })
	if err != nil || rows[0] != 0 {
		t.Errorf("read during an open write: %v, %v; want 0 rows at once", rows, err)
	}
}

func TestCancelledWriteNeverCommits(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	release := holdWriter(t, p.DB)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
			_, err := tx.Exec("INSERT INTO t (v) VALUES ('cancelled')")
			return struct{}{}, err
		})
		errc <- err
	}()
	for p.Stats().Interactive == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	release()
	mustExec(t, p.DB, "INSERT INTO t (v) VALUES ('after')") // waits for the held one too
	if n := one[int](t, p.DB, "SELECT count(*) FROM t WHERE v = 'cancelled'"); n != 0 {
		t.Error("a cancelled write committed")
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM t"); n != 2 {
		t.Errorf("rows = %d, want 2", n)
	}
}

func TestPanicRollsBackAndWriterGoesOn(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	_, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		tx.Exec("INSERT INTO t (v) VALUES ('half')")
		panic("bug")
	})
	var pe *PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v", err)
	}
	mustExec(t, p.DB, "INSERT INTO t (v) VALUES ('next')")
	if n := one[int](t, p.DB, "SELECT count(*) FROM t WHERE v = 'half'"); n != 0 {
		t.Error("the panicking transaction committed")
	}
	if v := one[int](t, p.DB, "PRAGMA user_version"); v != ProjectFormat.current() {
		t.Error("the file is not usable after the reset")
	}
	// The new write connection has the writer settings again.
	cs, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		var n int
		return n, tx.QueryRow("PRAGMA cache_size").Scan(&n)
	})
	if err != nil || cs != -65536 {
		t.Errorf("cache_size after the reset = %d, %v", cs, err)
	}
}

func TestErrorRollsBack(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	_, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		tx.Exec("INSERT INTO t (v) VALUES ('x')")
		return struct{}{}, ErrConflict
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM t"); n != 0 {
		t.Error("a failed request committed")
	}
	if s := p.Stats().LastError; s != "" {
		t.Errorf("a revision conflict counted as a writer error: %q", s)
	}
}

func TestCheckpointAndClose(t *testing.T) {
	dir := t.TempDir()
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	for range 20 {
		mustExec(t, p.DB, "INSERT INTO t (v) VALUES (randomblob(4000))")
	}
	wal := filepath.Join(dir, "project.db-wal")
	if st, err := os.Stat(wal); err != nil || st.Size() == 0 {
		t.Fatalf("no WAL to checkpoint: %v", err)
	}
	ok, err := p.Checkpoint(context.Background())
	if !ok || err != nil {
		t.Fatalf("Checkpoint = %v, %v", ok, err)
	}
	if st, _ := os.Stat(wal); st != nil && st.Size() != 0 {
		t.Errorf("WAL is %d bytes after TRUNCATE", st.Size())
	}

	// A request queued before Close still runs; later ones are refused.
	release := holdWriter(t, p.DB)
	errc := make(chan error, 1)
	go func() {
		_, err := Do(context.Background(), p.DB, limit.Background, func(tx *sql.Tx) (struct{}, error) {
			_, err := tx.Exec("INSERT INTO t (v) VALUES ('queued')")
			return struct{}{}, err
		})
		errc <- err
	}()
	for p.Stats().Background == 0 {
		time.Sleep(time.Millisecond)
	}
	closed := make(chan error, 1)
	go func() { closed <- p.Close(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	release()
	if err := <-errc; err != nil {
		t.Errorf("queued request: %v", err)
	}
	if err := <-closed; err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := Do(context.Background(), p.DB, limit.Interactive, func(*sql.Tx) (int, error) { return 0, nil }); !errors.Is(err, ErrClosed) {
		t.Errorf("Do after Close: %v", err)
	}
	if _, err := Query(context.Background(), p.DB, "SELECT 1", nil, func(*sql.Rows) (int, error) { return 0, nil }); !errors.Is(err, ErrClosed) {
		t.Errorf("Query after Close: %v", err)
	}
	if _, err := os.Stat(wal); err == nil {
		if st, _ := os.Stat(wal); st.Size() != 0 {
			t.Errorf("WAL left with %d bytes after Close", st.Size())
		}
	}

	q, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(context.Background())
	if n := one[int](t, q.DB, "SELECT count(*) FROM t"); n != 22 {
		t.Errorf("rows after reopen = %d, want 22", n)
	}
}

func TestCheckpointBusyReader(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	mustExec(t, p.DB, "INSERT INTO t (v) VALUES ('a')")
	// A reader in the middle of a read transaction holds the WAL.
	c, err := p.readers.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	tx.QueryRow("SELECT count(*) FROM t").Scan(&n)
	mustExec(t, p.DB, "INSERT INTO t (v) VALUES ('b')")
	start := time.Now()
	ok, err := p.Checkpoint(context.Background())
	if err != nil || ok {
		t.Errorf("Checkpoint with a busy reader = %v, %v; want false, nil", ok, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("the checkpoint held the writer for %v", d)
	}
	tx.Rollback()
	if ok, err := p.Checkpoint(context.Background()); !ok || err != nil {
		t.Errorf("Checkpoint after the reader left = %v, %v", ok, err)
	}
}
