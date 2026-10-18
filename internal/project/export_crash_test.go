package project

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The P2-12 crash test, with the P1-04 worker: the worker exports the
// project again and again, zip and folder in turn, while it writes, and is
// killed at a random moment. No export may then carry a final name unless
// it is complete, and the next open removes what the killed export left:
// the database copies of a zip export in tmp/, and its .partial.
//
// go test -run CrashExport ./internal/project; JENAB_CRASH_KILLS sets the
// kills (default 5). -short skips it.

// exportWorker is the worker side; spec is root|pid|exports.
func exportWorker(spec string) {
	rest, exports, _ := cutLast(spec)
	root, pid, _ := cutLast(rest)
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
	cs, err := p.Chats.Chats(ctx)
	if err != nil {
		panic(err)
	}
	next, err := store.Query(ctx, p.DB.DB, "SELECT coalesce(max(batch), -1) + 1 FROM t", nil, func(r *sql.Rows) (int, error) {
		var n int
		return n, r.Scan(&n)
	})
	if err != nil {
		panic(err)
	}
	go func() {
		for b := next[0]; ; b++ {
			if err := writeBatch(ctx, p, cs[0].ID, b); err != nil {
				panic(err)
			}
		}
	}()
	go func() {
		for i := 0; ; i++ {
			if _, err := p.DB.PutObject(ctx, limit.Background, id.SourceApp, fmt.Sprintf("live/%d", i%11), strings.NewReader(strings.Repeat("y", 1000+i%50000)), store.PutOptions{Source: "tool"}); err != nil {
				panic(err)
			}
		}
	}()
	fmt.Println("READY")
	for i := 0; ; i++ {
		o := ExportOptions{Format: ExportFolder, Target: filepath.Join(exports, fmt.Sprintf("%d-%d", os.Getpid(), i))}
		if i%2 == 0 {
			o.Format, o.Target = ExportZip, o.Target+".zip"
		}
		if _, err := p.Export(ctx, o); err != nil {
			panic(err)
		}
	}
}

func TestCrashExport(t *testing.T) {
	if testing.Short() {
		t.Skip("kills a worker process; skipped with -short")
	}
	root := t.TempDir()
	exports := t.TempDir()
	e := newEnv(t, root)
	p, _ := seedProject(t, e, 10, 64<<10)
	ch := mother(t, p)
	for b := range 20 {
		if err := writeBatch(context.Background(), p, ch, b); err != nil {
			t.Fatal(err)
		}
	}
	pid, dir := p.ID, p.Dir
	p.Release()
	e.stop(t)

	checked := map[string]bool{}
	partials := 0
	for k := range kills() {
		killWorker(t, "export|"+root+"|"+string(pid)+"|"+exports, 1500*time.Millisecond)
		ents, err := os.ReadDir(exports)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ents {
			name := filepath.Join(exports, d.Name())
			if strings.HasSuffix(d.Name(), ".partial") {
				if !checked[name] {
					partials++
					checkPartial(t, name)
				}
				checked[name] = true
				continue
			}
			if checked[name] {
				continue
			}
			checked[name] = true
			// A final name: the export is complete and opens.
			f := ExportFolder
			if strings.HasSuffix(d.Name(), ".zip") {
				f = ExportZip
			}
			cid := exportID(t, name, f)
			other := t.TempDir()
			place(t, ExportResult{Path: name, Format: f, ID: cid}, testPaths(other).Projects)
			rep := checkCopy(t, other, cid, "صادرات BTC", nil)
			if rep.batches < 20 {
				t.Errorf("kill %d: %s has %d batches", k, d.Name(), rep.batches)
			}
		}

		// The next open recovers and removes the database copies and the
		// .partial.
		e := newEnv(t, root)
		p := open(t, e.m, pid)
		if left, _ := os.ReadDir(filepath.Join(dir, "tmp")); len(left) != 0 {
			t.Errorf("kill %d: tmp/ still has %d entries", k, len(left))
		}
		if left, _ := filepath.Glob(filepath.Join(exports, "*.partial")); len(left) != 0 {
			t.Errorf("kill %d: the next open left %v", k, left)
		}
		p.Release()
		e.stop(t)
	}
	complete := len(checked) - partials
	t.Logf("%d kills: %d complete exports checked, %d stopped by a kill", kills(), complete, partials)
	if partials == 0 {
		t.Error("no kill landed in an export")
	}
}

// exportID is the project ID inside a finished export: its one folder.
func exportID(t *testing.T, name string, f ExportFormat) id.Project {
	t.Helper()
	if f == ExportFolder {
		ents, err := os.ReadDir(name)
		if err != nil || len(ents) != 1 || !id.Valid(ents[0].Name()) {
			t.Fatalf("%s holds %v, %v", name, ents, err)
		}
		return id.Project(ents[0].Name())
	}
	tmp := t.TempDir()
	if err := unzip(name, tmp); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	ents, err := os.ReadDir(tmp)
	if err != nil || len(ents) != 1 || !id.Valid(ents[0].Name()) {
		t.Fatalf("%s holds %v, %v", name, ents, err)
	}
	return id.Project(ents[0].Name())
}

// checkPartial checks what a killed export left: a database with its final
// name inside a partial folder is complete, by the safe-copy rule.
func checkPartial(t *testing.T, name string) {
	t.Helper()
	st, err := os.Stat(name)
	if err != nil || !st.IsDir() {
		return // a zip file is checked by its name alone
	}
	ents, _ := os.ReadDir(name)
	for _, d := range ents {
		for _, f := range []string{"project.db", "chats.db"} {
			db := filepath.Join(name, d.Name(), f)
			if !exists(db) {
				continue
			}
			problem, err := store.QuickCheck(context.Background(), db)
			if err != nil || problem != "" {
				t.Errorf("%s: %q, %v", db, problem, err)
			}
			if f == "project.db" {
				meta, err := store.ReadProjectMeta(context.Background(), filepath.Join(name, d.Name()))
				if err != nil || meta[metaID] != d.Name() {
					t.Errorf("%s: meta %v, %v", db, meta, err)
				}
			}
		}
	}
}
