package project

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The P1-04 crash test, in the style of SPIKE-008 and store's crash tests: a
// worker process (this test binary) opens a project and writes until it is
// killed (TerminateProcess on Windows, SIGKILL elsewhere). The next open must
// find the lock, recover, and give a project that works.
//
// go test -run Crash ./internal/project; JENAB_CRASH_KILLS sets the kills
// (default 5). -short skips it.

const crashEnv = "JENAB_PROJECT_CRASH_WORKER"

func kills() int {
	if n, err := strconv.Atoi(os.Getenv("JENAB_CRASH_KILLS")); err == nil && n > 0 {
		return n
	}
	return 5
}

// TestCrashWorker is the worker side; it only runs inside the crash test.
func TestCrashWorker(t *testing.T) {
	spec := os.Getenv(crashEnv)
	if spec == "" {
		t.Skip("runs only as a crash test worker")
	}
	root, pid, _ := cutLast(spec)
	ctx := context.Background()
	reg, err := store.OpenRegistry(ctx, testPaths(root).Registry)
	if err != nil {
		panic(err)
	}
	m := NewManager(Deps{Paths: testPaths(root), Registry: reg})
	p, err := m.Open(ctx, id.Project(pid))
	if err != nil {
		panic(err)
	}
	// Leftovers that recovery must clear.
	os.WriteFile(filepath.Join(p.Dir, "tmp", "download.part"), []byte("half"), 0o644)
	os.MkdirAll(filepath.Join(p.Dir, "tmp", "dry-run"), 0o755)
	os.WriteFile(filepath.Join(p.Dir, "snapshots", "project-before-migration.db.partial"), []byte("half"), 0o644)
	first, err := store.Query(ctx, p.DB.DB, "SELECT coalesce(max(batch), -1) + 1 FROM t", nil, func(r *sql.Rows) (int, error) {
		var n int
		return n, r.Scan(&n)
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("READY")
	for i := first[0]; ; i++ {
		_, err := store.Do(ctx, p.DB.DB, limit.Background, func(tx *sql.Tx) (int, error) {
			for j := range 200 {
				if _, err := tx.Exec("INSERT INTO t (batch, i, pad) VALUES (?, ?, ?)", i, j, "padding padding padding padding"); err != nil {
					return 0, err
				}
			}
			return 0, nil
		})
		if err != nil {
			panic(err)
		}
	}
}

func cutLast(s string) (string, string, bool) {
	i := len(s) - 1
	for i >= 0 && s[i] != '|' {
		i--
	}
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

func TestCrashRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("kills a worker process; skipped with -short")
	}
	root := t.TempDir()
	e := newEnv(t, root)
	ctx := context.Background()
	pid := create(t, e.m, "BTC")
	p := open(t, e.m, pid)
	_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		_, err := tx.Exec("CREATE TABLE t (batch INTEGER, i INTEGER, pad TEXT, PRIMARY KEY (batch, i))")
		return 0, err
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := p.Dir
	p.Release()
	e.stop(t)

	for k := range kills() {
		killWorker(t, root+"|"+string(pid), 300*time.Millisecond)
		if !fileExists(lockPath(dir)) {
			t.Fatalf("kill %d: no lock file after the kill", k)
		}

		e := newEnv(t, root)
		p := open(t, e.m, pid)
		if kinds := e.pub.noticeKinds(); !slices.Equal(kinds, []NoticeKind{NoticeRecovered}) {
			t.Errorf("kill %d: notices = %v, want [recovered]", k, kinds)
		}
		if p.Damage != nil {
			t.Fatalf("kill %d: damage after a kill: %+v", k, p.Damage)
		}
		if left, _ := os.ReadDir(filepath.Join(dir, "tmp")); len(left) != 0 {
			t.Errorf("kill %d: tmp/ still has %d entries", k, len(left))
		}
		if fileExists(filepath.Join(dir, "snapshots", "project-before-migration.db.partial")) {
			t.Errorf("kill %d: the partial snapshot is still there", k)
		}
		// Whole batches or nothing, and the project takes writes.
		var rows, partial int
		qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		counts, err := store.Query(qctx, p.DB.DB, "SELECT count(*) FROM t WHERE batch >= 0 GROUP BY batch HAVING count(*) <> 200", nil, func(r *sql.Rows) (int, error) {
			var n int
			return n, r.Scan(&n)
		})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		partial = len(counts)
		if partial != 0 {
			t.Errorf("kill %d: %d partial batches", k, partial)
		}
		rows, err = store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			if _, err := tx.Exec("INSERT INTO t (batch, i, pad) VALUES (-1, ?, 'after')", k); err != nil { // batch -1: the checks' own rows
				return 0, err
			}
			var n int
			return n, tx.QueryRow("SELECT count(*) FROM t").Scan(&n)
		})
		if err != nil {
			t.Fatalf("kill %d: write after recovery: %v", k, err)
		}
		t.Logf("kill %d: recovered, %d rows", k, rows)
		p.Release()
		e.stop(t)
		if fileExists(lockPath(dir)) {
			t.Fatalf("kill %d: lock left after a clean close", k)
		}
	}

	// A clean close means no recovery next time.
	e = newEnv(t, root)
	defer e.stop(t)
	p = open(t, e.m, pid)
	p.Release()
	if kinds := e.pub.noticeKinds(); len(kinds) != 0 {
		t.Errorf("notices after a clean close: %v", kinds)
	}
}

// killWorker starts the worker and kills it a random time after READY.
func killWorker(t *testing.T, spec string, maxDelay time.Duration) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashWorker$", "-test.v")
	cmd.Env = append(os.Environ(), crashEnv+"="+spec)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	gone := make(chan struct{}) // closed when the worker's output ends
	go func() {
		defer close(gone)
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
		cmd.Wait()
		t.Fatal("the worker never got READY")
	}
	select {
	case <-gone:
		cmd.Wait()
		t.Fatal("the worker stopped before it was killed")
	case <-time.After(50*time.Millisecond + time.Duration(rand.Int64N(int64(maxDelay)))):
	}
	cmd.Process.Kill()
	cmd.Wait()
}

func fileExists(p string) bool { return exists(p) }
