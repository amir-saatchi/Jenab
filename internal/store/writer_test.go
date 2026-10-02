package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/amir-saatchi/jenab/internal/limit"
)

// These tests use fake jobs on a writer without a connection, inside a
// synctest bubble (Q35). The real-SQLite tests are in store_test.go.

func fakeDB(reset func(any)) *DB {
	db := &DB{}
	db.w = newWriter(func() *sql.Conn { return nil }, reset)
	go db.w.loop()
	return db
}

// do sends a fake request that runs fn.
func do(ctx context.Context, db *DB, p limit.Priority, fn func() (string, error)) (string, error) {
	return doConn(ctx, db, p, func(context.Context, *sql.Conn) (string, error) { return fn() })
}

// block starts a request that holds the writer until release is closed.
func block(t *testing.T, db *DB, p limit.Priority) (release func()) {
	ch := make(chan struct{})
	go do(context.Background(), db, p, func() (string, error) { <-ch; return "", nil })
	synctest.Wait()
	if !db.Stats().Busy {
		t.Fatal("the blocking request is not running")
	}
	return func() { close(ch) }
}

func TestTenToOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := fakeDB(nil)
		release := block(t, db, limit.Background)

		var mu sync.Mutex
		var order []string
		add := func(p limit.Priority, name string) {
			go do(context.Background(), db, p, func() (string, error) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return name, nil
			})
		}
		for range 25 {
			add(limit.Interactive, "I")
		}
		for range 3 {
			add(limit.Background, "B")
		}
		synctest.Wait()
		if s := db.Stats(); s.Interactive != 25 || s.Background != 3 {
			t.Fatalf("queued %d interactive and %d background, want 25 and 3", s.Interactive, s.Background)
		}
		release()
		synctest.Wait()

		got := strings.Join(order, "")
		want := strings.Repeat("I", 10) + "B" + strings.Repeat("I", 10) + "B" + strings.Repeat("I", 5) + "B"
		if got != want {
			t.Errorf("order\n got %s\nwant %s", got, want)
		}
		db.w.close(context.Background())
	})
}

func TestCancelledWhileQueuedNeverRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := fakeDB(nil)
		release := block(t, db, limit.Interactive)

		ran := false
		ctx, cancel := context.WithCancel(context.Background())
		var err error
		done := make(chan struct{})
		go func() {
			_, err = do(ctx, db, limit.Interactive, func() (string, error) { ran = true; return "", nil })
			close(done)
		}()
		synctest.Wait()
		cancel()
		<-done
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		release()
		synctest.Wait()
		if ran {
			t.Error("a request given up while queued ran")
		}
		if n := db.Stats().Done; n != 1 {
			t.Errorf("Done = %d, want 1 (the skipped request doesn't count)", n)
		}
		db.w.close(context.Background())
	})
}

func TestCancelledAfterTakenFinishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := fakeDB(nil)
		ch := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		var v string
		var err error
		done := make(chan struct{})
		go func() {
			v, err = do(ctx, db, limit.Interactive, func() (string, error) { <-ch; return "committed", nil })
			close(done)
		}()
		synctest.Wait() // the request is running
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("Do returned while its request was still running")
		default:
		}
		close(ch)
		<-done
		if v != "committed" || err != nil {
			t.Errorf("got %q, %v; want the result of the request that ran", v, err)
		}
		db.w.close(context.Background())
	})
}

func TestPanicIsRecovered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resets := 0
		db := fakeDB(func(any) { resets++ })
		_, err := do(context.Background(), db, limit.Interactive, func() (string, error) { panic("boom") })
		var pe *PanicError
		if !errors.As(err, &pe) || pe.Value != "boom" || len(pe.Stack) == 0 {
			t.Fatalf("err = %v, want a PanicError with the value and stack", err)
		}
		if resets != 1 {
			t.Errorf("connection reset %d times, want 1", resets)
		}
		v, err := do(context.Background(), db, limit.Interactive, func() (string, error) { return "next", nil })
		if v != "next" || err != nil {
			t.Errorf("the writer stopped serving after a panic: %q, %v", v, err)
		}
		if !strings.Contains(db.Stats().LastError, "boom") {
			t.Errorf("LastError = %q", db.Stats().LastError)
		}
		db.w.close(context.Background())
	})
}

func TestCloseDrainsThenRefuses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := fakeDB(nil)
		release := block(t, db, limit.Interactive)
		var results []string
		var mu sync.Mutex
		for range 3 {
			go func() {
				v, err := do(context.Background(), db, limit.Background, func() (string, error) { return "ran", nil })
				mu.Lock()
				results = append(results, v+errString(err))
				mu.Unlock()
			}()
		}
		synctest.Wait()
		closed := make(chan struct{})
		go func() { db.w.close(context.Background()); close(closed) }()
		synctest.Wait()
		if _, err := do(context.Background(), db, limit.Interactive, func() (string, error) { return "", nil }); !errors.Is(err, ErrClosed) {
			t.Errorf("a request after Close: err = %v, want ErrClosed", err)
		}
		release()
		<-closed
		synctest.Wait()
		if strings.Join(results, ",") != "ran,ran,ran" {
			t.Errorf("queued requests: %v, want all three to run before Close returns", results)
		}
	})
}

func TestCloseTimeoutFailsQueued(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := fakeDB(nil)
		release := block(t, db, limit.Interactive)
		ran := false
		var err error
		done := make(chan struct{})
		go func() {
			_, err = do(context.Background(), db, limit.Interactive, func() (string, error) { ran = true; return "", nil })
			close(done)
		}()
		synctest.Wait()
		ctx, cancel := context.WithCancel(context.Background())
		closed := make(chan struct{})
		go func() { db.w.close(ctx); close(closed) }()
		synctest.Wait()
		cancel() // Close ran out of time
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("Close returned while a request was still running")
		default:
		}
		release()
		<-closed
		<-done
		if ran || !errors.Is(err, ErrClosed) {
			t.Errorf("queued request: ran = %v, err = %v; want not run and ErrClosed", ran, err)
		}
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return " " + err.Error()
}
