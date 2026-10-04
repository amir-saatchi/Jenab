package store

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// SPIKE-008's crash tests for the store, kept as permanent tests: a worker
// process (this test binary) does real work, gets killed at a random moment
// (TerminateProcess on Windows, SIGKILL elsewhere), and the test then opens
// the files and checks them. A kill is not a power cut: the OS still writes
// what it has buffered. The recovery scenario is in project; chats belong
// to P1-06.
//
// go test -run Crash ./internal/store; JENAB_CRASH_KILLS sets the kills per
// scenario (default 5). -short skips them.

const crashEnv = "JENAB_STORE_CRASH_WORKER"

func kills() int {
	if n, err := strconv.Atoi(os.Getenv("JENAB_CRASH_KILLS")); err == nil && n > 0 {
		return n
	}
	return 5
}

// TestCrashWorker is the worker side; it only runs inside a crash test.
func TestCrashWorker(t *testing.T) {
	spec := os.Getenv(crashEnv)
	if spec == "" {
		t.Skip("runs only as a crash test worker")
	}
	scenario, dir, _ := strings.Cut(spec, "|")
	ctx := context.Background()
	switch scenario {
	case "chunks":
		workChunks(ctx, dir)
	case "snapshots":
		workSnapshots(ctx, dir)
	case "format":
		workFormat(ctx, dir)
	case "objects":
		workObjects(ctx, dir)
	}
	t.Fatal("the worker returned instead of being killed")
}

// killDuring starts the worker for scenario and kills it a random time
// after it reports READY.
func killDuring(t *testing.T, scenario, dir string, maxDelay time.Duration) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashWorker$", "-test.v")
	cmd.Env = append(os.Environ(), crashEnv+"="+scenario+"|"+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if sc.Text() == "READY" {
				ready <- true
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			cmd.Wait()
			t.Fatal("the worker exited before READY")
		}
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		t.Fatal("the worker never got READY")
	}
	time.Sleep(time.Duration(rand.Int64N(int64(maxDelay))))
	cmd.Process.Kill()
	cmd.Wait()
}

func quickCheck(t *testing.T, db *DB) {
	t.Helper()
	if r := one[string](t, db, "PRAGMA quick_check"); r != "ok" {
		t.Fatalf("quick_check: %s", r)
	}
}

// ---- chunked writes: whole chunks or nothing ----

const chunkRows = 500

func workChunks(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		_, err := tx.Exec("CREATE TABLE IF NOT EXISTS t (chunk INTEGER, i INTEGER, pad TEXT, PRIMARY KEY (chunk, i))")
		return 0, err
	})
	fmt.Println("READY")
	for {
		_, err := Do(ctx, p.DB, limit.Background, func(tx *sql.Tx) (int, error) {
			var c int
			if err := tx.QueryRow("SELECT coalesce(max(chunk), 0) + 1 FROM t").Scan(&c); err != nil {
				return 0, err
			}
			st, err := tx.Prepare("INSERT INTO t VALUES (?, ?, randomblob(200))")
			if err != nil {
				return 0, err
			}
			defer st.Close()
			for i := range chunkRows {
				if _, err := st.Exec(c, i); err != nil {
					return 0, err
				}
			}
			return c, nil
		})
		if err != nil {
			panic(err)
		}
	}
}

func TestCrashChunkedWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	for k := range kills() {
		killDuring(t, "chunks", dir, 400*time.Millisecond)
		p, err := OpenProject(context.Background(), dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		if n := one[int](t, p.DB, "SELECT count(*) FROM (SELECT chunk FROM t GROUP BY chunk HAVING count(*) <> ?)", chunkRows); n != 0 {
			t.Errorf("kill %d: %d chunks are partly written", k+1, n)
		}
		if gaps := one[int](t, p.DB, "SELECT max(chunk) - count(DISTINCT chunk) FROM t"); gaps != 0 {
			t.Errorf("kill %d: %d chunks missing between committed ones", k+1, gaps)
		}
		t.Logf("kill %d: %d chunks committed", k+1, one[int](t, p.DB, "SELECT count(DISTINCT chunk) FROM t"))
		p.Close(context.Background())
	}
}

// ---- snapshots: only complete copies carry a final name ----

const snapRows = 20000

func workSnapshots(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			Do(ctx, p.DB, limit.Background, func(tx *sql.Tx) (int, error) {
				_, err := tx.Exec("INSERT INTO t (v) VALUES (randomblob(200))")
				return 0, err
			})
		}
	}()
	fmt.Println("READY")
	snaps := filepath.Join(dir, "snapshots")
	for {
		name := filepath.Join(snaps, fmt.Sprintf("snap-%d.db", time.Now().UnixNano()))
		if err := p.Snapshot(ctx, name); err != nil {
			panic(err)
		}
		m, _ := filepath.Glob(filepath.Join(snaps, "snap-*.db"))
		sort.Strings(m)
		for len(m) > 5 {
			os.Remove(m[0])
			m = m[1:]
		}
	}
}

func TestCrashSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v BLOB)")
	mustExec(t, p.DB, "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?) INSERT INTO t (v) SELECT randomblob(200) FROM n", snapRows)
	p.Close(context.Background())
	os.MkdirAll(filepath.Join(dir, "snapshots"), 0o755)

	checked, midCopy := 0, 0
	for k := range kills() {
		killDuring(t, "snapshots", dir, 300*time.Millisecond)
		if m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.partial")); len(m) > 0 {
			midCopy++
		}
		p, err := OpenProject(context.Background(), dir) // removes half-written copies
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		p.Close(context.Background())
		m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*"))
		for _, f := range m {
			if filepath.Ext(f) != ".db" {
				t.Errorf("kill %d: %s left in snapshots", k+1, filepath.Base(f))
				continue
			}
			// Opened the way a user restoring it would: read-write, so SQLite
			// would roll back a hot journal.
			s, err := sql.Open("sqlite", dsn(f, false))
			if err != nil {
				t.Fatal(err)
			}
			var res string
			var n int
			err = s.QueryRow("PRAGMA quick_check").Scan(&res)
			if err == nil {
				err = s.QueryRow("SELECT count(*) FROM t").Scan(&n)
			}
			s.Close()
			if err != nil || res != "ok" || n < snapRows {
				t.Errorf("kill %d: snapshot %s is broken: %v, %s, %d rows", k+1, filepath.Base(f), err, res, n)
			}
			checked++
		}
	}
	t.Logf("%d snapshots checked; %d of %d kills hit a copy in progress", checked, midCopy, kills())
}

// ---- format update: the old version or the new one, never between ----

const formatRows = 60000

// bigFormat's step 2 rebuilds t, which takes a moment with formatRows rows.
func bigFormat(n int) Format {
	f := Format{Name: "big", Steps: []func(*sql.Tx) error{
		execAll("CREATE TABLE t (id INTEGER PRIMARY KEY, v BLOB)"),
	}}
	if n >= 2 {
		f.Steps = append(f.Steps, execAll(
			"CREATE TABLE t2 (id INTEGER PRIMARY KEY, v BLOB, extra TEXT NOT NULL DEFAULT 'new')",
			"INSERT INTO t2 (id, v) SELECT id, v FROM t",
			"DROP TABLE t",
			"ALTER TABLE t2 RENAME TO t",
		))
	}
	return f
}

func workFormat(ctx context.Context, dir string) {
	fmt.Println("READY")
	db, err := Open(ctx, filepath.Join(dir, "big.db"), Options{Format: bigFormat(2), Snapshots: filepath.Join(dir, "snapshots")})
	if err != nil {
		panic(err)
	}
	db.Close(ctx)
	time.Sleep(time.Hour) // wait to be killed
}

func TestCrashFormatUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	tmpl := filepath.Join(t.TempDir(), "big.db")
	db := openTest(t, tmpl, bigFormat(1))
	mustExec(t, db, "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?) INSERT INTO t (v) SELECT randomblob(300) FROM n", formatRows)
	db.Close(context.Background())
	orig, err := os.ReadFile(tmpl)
	if err != nil {
		t.Fatal(err)
	}

	// Time one update, so the kills land while it runs.
	trial := filepath.Join(t.TempDir(), "big.db")
	os.WriteFile(trial, orig, 0o644)
	start := time.Now()
	db = openTest(t, trial, bigFormat(2))
	took := time.Since(start)
	db.Close(context.Background())
	t.Logf("one update with its backup takes %v", took.Round(time.Millisecond))

	seen := map[int]int{}
	for k := range kills() {
		dir := t.TempDir()
		path := filepath.Join(dir, "big.db")
		os.WriteFile(path, orig, 0o644)
		killDuring(t, "format", dir, took)

		v := rawVersion(t, path)
		seen[v]++
		db, err := Open(context.Background(), path, Options{Format: bigFormat(v), Snapshots: filepath.Join(dir, "snapshots")})
		if err != nil {
			t.Fatalf("kill %d: open at version %d: %v", k+1, v, err)
		}
		quickCheck(t, db)
		if n := one[int](t, db, "SELECT count(*) FROM t"); n != formatRows {
			t.Errorf("kill %d: version %d has %d rows, want %d", k+1, v, n, formatRows)
		}
		cols := one[int](t, db, "SELECT count(*) FROM pragma_table_info('t')")
		if want := map[int]int{1: 2, 2: 3}[v]; cols != want {
			t.Errorf("kill %d: version %d has %d columns, want %d", k+1, v, cols, want)
		}
		db.Close(context.Background())

		// The backup, if one was finished, is the complete old version.
		m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*"))
		for _, f := range m {
			if filepath.Ext(f) != ".db" {
				continue // removed at the next open
			}
			if bv := rawVersion(t, f); bv != 1 {
				t.Errorf("kill %d: backup %s has version %d", k+1, filepath.Base(f), bv)
			}
		}
	}
	t.Logf("after the kills: %d at version 1, %d at version 2", seen[1], seen[2])
}

// ---- objects: no row ever points to missing bytes (N-30) ----

func workObjects(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			if _, err := p.SweepObjects(ctx); err != nil {
				panic(err)
			}
		}
	}()
	fmt.Println("READY")
	for i := 0; ; i++ {
		data := make([]byte, rand.IntN(64<<10))
		for j := range data {
			data[j] = byte(rand.IntN(256))
		}
		if i%5 == 0 {
			data = []byte(fmt.Sprintf("repeated %d", i%3)) // stored once
		}
		if i%7 == 0 {
			p.objects.Put(ctx, bytes.NewReader(data), 0) // bytes with no row, for the sweep
			continue
		}
		if _, err := p.PutObject(ctx, limit.Background, id.SourceApp, fmt.Sprintf("k/%d", i%50), bytes.NewReader(data), PutOptions{Source: "tool"}); err != nil {
			panic(err)
		}
	}
}

func TestCrashObjects(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	ctx := context.Background()
	for k := range kills() {
		killDuring(t, "objects", dir, 500*time.Millisecond)
		p, err := OpenProject(ctx, dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		used, err := p.usedHashes(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for h := range used {
			r, err := p.objects.Open(ctx, h)
			if err != nil {
				t.Errorf("kill %d: a row points to missing bytes: %v", k+1, err)
				continue
			}
			sum := sha256.New()
			io.Copy(sum, r)
			r.Close()
			if hex.EncodeToString(sum.Sum(nil)) != h {
				t.Errorf("kill %d: the bytes of %s don't match their name", k+1, h)
			}
		}
		rep, err := p.SweepObjects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		files := 0
		for s, err := range p.objects.All(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			files++
			if !used[s.Hash] {
				t.Errorf("kill %d: %s has no row after the sweep", k+1, s.Hash)
			}
		}
		t.Logf("kill %d: %d hashes in use, %d files, the sweep removed %d", k+1, len(used), files, rep.Files)
		p.Close(ctx)
	}
}
