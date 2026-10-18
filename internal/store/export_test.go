package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestExportCopyWithWorker: a query worker in the middle of a long read
// holds a read transaction on project.db; the export's VACUUM INTO, a
// reader too, doesn't wait for it, and its copy is whole.
func TestExportCopyWithWorker(t *testing.T) {
	p, _ := openAgentProject(t, 1)
	ctx := context.Background()
	var ended atomic.Bool
	errc := make(chan error, 1)
	go func() {
		_, err := p.guardedQuery(ctx, "SELECT count(*) FROM data a, data b, data c", nil, 1, 3*time.Second)
		ended.Store(true)
		errc <- err
	}()
	time.Sleep(300 * time.Millisecond) // the worker is reading

	target := filepath.Join(t.TempDir(), "project.db")
	start := time.Now()
	if _, err := p.ExportCopy(ctx, target, CopyEdit{Meta: map[string]string{"id": "copy"}, DropApprovals: true}); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	if ended.Load() {
		t.Errorf("the copy took %v and ended after the worker's read", took)
	}
	t.Logf("the copy took %v while a worker read", took)
	if bad, err := QuickCheck(ctx, target); bad != "" || err != nil {
		t.Errorf("quick_check: %q %v", bad, err)
	}
	if err := <-errc; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the slow read: %v", err)
	}
	c, err := OpenProject(ctx, filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if n := one[int64](t, c.DB, "SELECT count(*) FROM data"); n != one[int64](t, p.DB, "SELECT count(*) FROM data") || n == 0 {
		t.Errorf("the copy has %d rows of data", n)
	}
}
