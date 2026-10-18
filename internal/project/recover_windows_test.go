package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

// TestExportRecordThroughRecovery: a killed export's .partial that is
// still locked, here held open, can't be removed at the next open. Its
// record then stays, through the crash recovery that empties tmp/, and
// the open after that removes both.
func TestExportRecordThroughRecovery(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, root)
	p, _ := seedProject(t, e, 1, 100)
	pid, dir := p.ID, p.Dir
	p.Release()
	e.stop(t)

	cid := id.New()
	partial := filepath.Join(t.TempDir(), "out.zip."+cid+".partial")
	if err := userFile(partial); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(dir, "tmp", "export-"+cid+recordExt)
	if err := writeRecord(rec, partial); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(dir, "tmp", "export-"+cid)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(dir); err != nil { // as if the app died
		t.Fatal(err)
	}
	f, err := os.Open(partial) // Windows refuses to delete an open file
	if err != nil {
		t.Fatal(err)
	}

	e = newEnv(t, root)
	open(t, e.m, pid).Release()
	e.stop(t)
	if !exists(partial) || !exists(rec) {
		t.Fatalf("after the recovery: partial %v, record %v", exists(partial), exists(rec))
	}
	if exists(stage) {
		t.Error("the recovery kept the database copies")
	}

	f.Close()
	e = newEnv(t, root)
	open(t, e.m, pid).Release()
	e.stop(t)
	if exists(partial) || exists(rec) {
		t.Errorf("after the next open: partial %v, record %v", exists(partial), exists(rec))
	}
}
