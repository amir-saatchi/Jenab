package main

import (
	"errors"
	"testing"
)

func TestLockRoot(t *testing.T) {
	dir := t.TempDir()
	f, err := lockRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockRoot(dir); !errors.Is(err, errRunning) {
		t.Fatalf("second lock: %v; want errRunning", err)
	}
	f.Close()
	g, err := lockRoot(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	g.Close()
}
