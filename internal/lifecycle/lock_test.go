package lifecycle

import (
	"errors"
	"testing"
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
