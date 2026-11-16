package lifecycle

import (
	"errors"
	"testing"
	"time"
)

func TestLockRoot(t *testing.T) {
	dir := t.TempDir()
	f, err := LockRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockRoot(dir); !errors.Is(err, ErrRunning) {
		t.Fatalf("second lock: %v; want ErrRunning", err)
	}
	f.Close()
	g, err := LockRoot(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	g.Close()
}

func TestLockRootWait(t *testing.T) {
	dir := t.TempDir()
	f, err := LockRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockRootWait(dir, 150*time.Millisecond); !errors.Is(err, ErrRunning) {
		t.Fatalf("lock held past the wait: %v; want ErrRunning", err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		f.Close()
	}()
	g, err := LockRootWait(dir, 5*time.Second)
	if err != nil {
		t.Fatalf("lock once the other copy quit: %v", err)
	}
	g.Close()
}
