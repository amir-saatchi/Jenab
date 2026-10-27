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
