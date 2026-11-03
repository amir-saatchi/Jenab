package app

import (
	"context"
	"testing"
)

func TestMemory(t *testing.T) {
	e := newEnv(t, t.TempDir())
	r, err := e.svc.System.Memory(context.Background())
	if err != nil || r.Total == 0 || r.Level == "" {
		t.Fatalf("Memory = %+v, %v", r, err)
	}
}

func TestVersion(t *testing.T) {
	e := newEnv(t, t.TempDir())
	v, err := e.svc.System.Version(context.Background())
	if err != nil || v.Version == "" || v.Go == "" {
		t.Fatalf("Version = %+v, %v", v, err)
	}
}
