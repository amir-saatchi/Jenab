package project

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// seedProject makes a project with table t and n objects of size bytes
// each, and opens it. It returns the objects' contents by key.
func seedProject(t *testing.T, e *env, n, size int) (*Project, map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	p := open(t, e.m, create(t, e.m, "صادرات BTC"))
	_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		_, err := tx.Exec("CREATE TABLE t (batch INTEGER, i INTEGER, pad TEXT, PRIMARY KEY (batch, i))")
		return 0, err
	})
	if err != nil {
		t.Fatal(err)
	}
	objs := map[string][]byte{}
	for i := range n {
		b := make([]byte, size)
		rand.Read(b)
		key := fmt.Sprintf("seed/%d.bin", i)
		if _, err := p.DB.PutObject(ctx, limit.Interactive, id.SourceUser, key, bytes.NewReader(b), store.PutOptions{Source: "upload"}); err != nil {
			t.Fatal(err)
		}
		objs[key] = b
	}
	return p, objs
}

// writeBatch writes batch b as one turn: a message in chats.db, then 200
// rows in t with that message as the source (SPEC 2.3).
func writeBatch(ctx context.Context, p *Project, mother id.Chat, b int) error {
	m, _, err := p.Chats.AppendMessage(ctx, chat.Message{Chat: mother, Turn: b + 1, Role: chat.RoleUser,
		Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: fmt.Sprintf("batch %d", b)}}}})
	if err != nil {
		return err
	}
	rows := make([]store.Row, 200)
	for i := range rows {
		rows[i] = store.Row{"batch": b, "i": i, "pad": "padding padding padding padding"}
	}
	_, err = p.DB.Insert(ctx, limit.Background, id.SourceOf("message", string(m.ID)), "t", rows, store.ConflictFail)
	return err
}

func mother(t *testing.T, p *Project) id.Chat {
	t.Helper()
	cs, err := p.Chats.Chats(context.Background())
	if err != nil || len(cs) == 0 {
		t.Fatalf("chats = %v, %v", cs, err)
	}
	return cs[0].ID
}

// place puts an export's project folder into projects, as a user copying
// it into the data folder would.
func place(t *testing.T, res ExportResult, projects string) string {
	t.Helper()
	dst := filepath.Join(projects, string(res.ID))
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if res.Format == ExportFolder {
		if err := copyDir(filepath.Join(res.Path, string(res.ID)), dst); err != nil {
			t.Fatal(err)
		}
		return dst
	}
	if err := unzip(res.Path, projects); err != nil {
		t.Fatal(err)
	}
	return dst
}

func unzip(name, dst string) error {
	zr, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		to := filepath.Join(dst, filepath.FromSlash(f.Name))
		if strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(to, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyReport is what checkCopy found in a copy.
type copyReport struct {
	batches, objects int
}

// checkCopy opens the copy with a manager on root, as a project of that
// data folder, and checks it: listed with its name, quick_check passes,
// whole batches 0..n-1, every change from a message finds the message, and
// every object opens with the bytes the original had (want, when given).
func checkCopy(t *testing.T, root string, cid id.Project, name string, want map[string][]byte) copyReport {
	t.Helper()
	ctx := context.Background()
	e := newEnv(t, root)
	defer e.stop(t)
	ps, err := e.m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(ps, func(p store.ProjectEntry) bool { return p.ID == cid && p.Name == name }) {
		t.Fatalf("the copy %s is not listed: %+v", cid, ps)
	}
	p := open(t, e.m, cid)
	defer p.Release()
	if p.Damage != nil {
		t.Fatalf("the copy is damaged: %+v", p.Damage)
	}
	for _, db := range []*store.DB{p.DB.DB, p.Chats.DB} {
		if r := query[string](t, db, "PRAGMA quick_check"); len(r) != 1 || r[0] != "ok" {
			t.Fatalf("quick_check of %s: %v", filepath.Base(db.Path()), r)
		}
	}
	meta, err := p.DB.Meta(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta[metaID] != string(cid) || meta[metaExportedFrom] == "" || meta[metaExportedFrom] == string(cid) {
		t.Errorf("meta = %v", meta)
	}
	var rep copyReport
	sizes := query[int](t, p.DB.DB, "SELECT count(*) FROM t GROUP BY batch ORDER BY batch")
	for _, n := range sizes {
		if n != 200 {
			t.Fatalf("a batch with %d rows in the copy", n)
		}
	}
	rep.batches = len(sizes)
	if max := query[int](t, p.DB.DB, "SELECT coalesce(max(batch), -1) FROM t"); max[0] != rep.batches-1 {
		t.Fatalf("batches are not 0..%d: the last is %d", rep.batches-1, max[0])
	}
	msgs := map[string]bool{}
	for _, m := range query[string](t, p.Chats.DB, "SELECT id FROM messages") {
		msgs["message:"+m] = true
	}
	for _, src := range query[string](t, p.DB.DB, "SELECT source FROM _jenab_changes WHERE source LIKE 'message:%'") {
		if !msgs[src] {
			t.Fatalf("change from %s, which is not in the copy's chats.db", src)
		}
	}
	var keys []string
	prefixes := []string{""}
	for len(prefixes) > 0 {
		list, err := p.DB.ListObjects(ctx, prefixes[0], "", 1000)
		if err != nil {
			t.Fatal(err)
		}
		prefixes = append(prefixes[1:], list.Folders...)
		for _, o := range list.Objects {
			keys = append(keys, o.Key)
		}
	}
	for _, k := range keys {
		_, r, err := p.DB.OpenObject(ctx, k)
		if err != nil {
			t.Fatalf("object %s: %v", k, err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if w, ok := want[k]; ok && !bytes.Equal(b, w) {
			t.Fatalf("object %s differs", k)
		}
		rep.objects++
	}
	for k := range want {
		if !slices.Contains(keys, k) {
			t.Fatalf("object %s is missing from the copy", k)
		}
	}
	// The copy takes writes.
	if err := writeBatch(ctx, p, mother(t, p), rep.batches); err != nil {
		t.Fatalf("a write to the copy: %v", err)
	}
	return rep
}

func query[T any](t *testing.T, db *store.DB, q string) []T {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := store.Query(ctx, db, q, nil, func(r *sql.Rows) (T, error) {
		var v T
		return v, r.Scan(&v)
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestExportDuringWrites is the first done-when item: an export taken
// during a run of writes to both databases and the bucket opens as a
// valid project with every row committed before it started, whole turns
// only, and no change without its message.
func TestExportDuringWrites(t *testing.T) {
	for _, f := range []ExportFormat{ExportZip, ExportFolder} {
		t.Run(string(f), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			e := newEnv(t, root)
			defer e.stop(t)
			p, objs := seedProject(t, e, 20, 64<<10)
			defer p.Release()
			ch := mother(t, p)
			for b := range 50 {
				if err := writeBatch(ctx, p, ch, b); err != nil {
					t.Fatal(err)
				}
			}

			var committed atomic.Int64 // batches whose write returned
			committed.Store(50)
			stop := make(chan struct{})
			var wg sync.WaitGroup
			var werr error
			wg.Go(func() {
				for b := 50; ; b++ {
					select {
					case <-stop:
						return
					default:
					}
					if err := writeBatch(ctx, p, ch, b); err != nil {
						werr = err
						return
					}
					committed.Store(int64(b + 1))
				}
			})
			wg.Go(func() { // the bucket changes too
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					default:
					}
					if _, err := p.DB.PutObject(ctx, limit.Background, id.SourceApp, fmt.Sprintf("live/%d", i%7), strings.NewReader(strings.Repeat("x", i)), store.PutOptions{Source: "tool"}); err != nil {
						werr = err
						return
					}
				}
			})
			time.Sleep(50 * time.Millisecond)
			before := int(committed.Load())
			var steps []ExportStep
			var mu sync.Mutex
			target := filepath.Join(t.TempDir(), "out")
			if f == ExportZip {
				target += ".zip"
			}
			res, err := p.Export(ctx, ExportOptions{Target: target, Format: f, Progress: func(pr ExportProgress) {
				mu.Lock()
				if len(steps) == 0 || steps[len(steps)-1] != pr.Step {
					steps = append(steps, pr.Step)
				}
				mu.Unlock()
			}})
			close(stop)
			wg.Wait()
			if err != nil {
				t.Fatal(err)
			}
			if werr != nil {
				t.Fatal(werr)
			}
			after := int(committed.Load())
			if res.ID == p.ID || !id.Valid(string(res.ID)) || res.Path != target || res.Size <= 0 || res.Missing != 0 {
				t.Errorf("result = %+v", res)
			}
			if left := partials(target); len(left) != 0 {
				t.Errorf("the .partial is still there: %v", left)
			}
			if left, _ := os.ReadDir(filepath.Join(p.Dir, "tmp")); slices.ContainsFunc(left, func(d os.DirEntry) bool { return strings.HasPrefix(d.Name(), "export-") }) {
				t.Error("the database copies are still in tmp/")
			}
			if !slices.Contains(steps, StepProject) || !slices.Contains(steps, StepChats) || steps[len(steps)-1] != StepDone {
				t.Errorf("steps = %v", steps)
			}

			// Copied into a data folder, it opens as a project.
			other := t.TempDir()
			place(t, res, testPaths(other).Projects)
			rep := checkCopy(t, other, res.ID, "صادرات BTC", objs)
			if rep.batches < before || rep.batches > after {
				t.Errorf("the copy has %d batches; %d were committed before the export and %d after", rep.batches, before, after)
			}
			t.Logf("%s: %d batches in the copy (%d before, %d after), %d objects, %d bytes, %d ms",
				f, rep.batches, before, after, rep.objects, res.Size, res.Millis)
		})
	}
}

// TestExportNextToTheOriginal: a copy in the same data folder opens next
// to the project it came from.
func TestExportNextToTheOriginal(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, root)
	p, objs := seedProject(t, e, 3, 1000)
	res, err := p.Export(context.Background(), ExportOptions{Target: filepath.Join(t.TempDir(), "copy"), Format: ExportFolder})
	if err != nil {
		t.Fatal(err)
	}
	pid := p.ID
	p.Release()
	e.stop(t)
	place(t, res, testPaths(root).Projects)
	checkCopy(t, root, res.ID, "صادرات BTC", objs)
	e = newEnv(t, root)
	defer e.stop(t)
	ps, err := e.m.List(context.Background())
	if err != nil || len(ps) != 2 {
		t.Fatalf("projects = %+v, %v", ps, err)
	}
	open(t, e.m, pid).Release()
}

func TestExportZipLayout(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 3, 1000)
	defer p.Release()
	os.MkdirAll(filepath.Join(p.Dir, "skills", "sql"), 0o755)
	os.WriteFile(filepath.Join(p.Dir, "skills", "sql", "SKILL.md"), []byte("---\nname: sql\n---\nbody"), 0o644)
	os.WriteFile(filepath.Join(p.Dir, "snapshots", "project-before.db"), []byte("not exported"), 0o644)
	target := filepath.Join(t.TempDir(), "out.zip")
	res, err := p.Export(context.Background(), ExportOptions{Target: target, Format: ExportZip})
	if err != nil {
		t.Fatal(err)
	}
	if res.Objects != 3 {
		t.Errorf("objects = %d", res.Objects)
	}
	zr, err := zip.OpenReader(target)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if !f.Modified.Equal(zr.File[0].Modified) {
			t.Errorf("%s has its own time", f.Name)
		}
	}
	if !slices.IsSorted(names) {
		t.Errorf("entries not in name order: %v", names)
	}
	top := string(res.ID) + "/"
	for _, want := range []string{top, top + "project.db", top + "chats.db", top + "objects/", top + "snapshots/", top + "tmp/", top + "skills/sql/SKILL.md"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s missing from %v", want, names)
		}
	}
	for _, n := range names {
		if !strings.HasPrefix(n, top) || strings.Contains(n, "jenab.lock") || strings.Contains(n, "before.db") || strings.HasSuffix(n, "-wal") {
			t.Errorf("unexpected entry %s", n)
		}
	}
	if n := len(names); n != 8+3 {
		t.Errorf("%d entries: %v", n, names)
	}
}

func TestExportRefuses(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 1, 10)
	defer p.Release()
	dir := t.TempDir()

	full := filepath.Join(dir, "full")
	os.MkdirAll(full, 0o755)
	os.WriteFile(filepath.Join(full, "keep.txt"), []byte("mine"), 0o644)
	if _, err := p.Export(ctx, ExportOptions{Target: full, Format: ExportFolder}); !errors.Is(err, ErrExportExists) {
		t.Errorf("into a folder with files: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(full, "keep.txt")); string(b) != "mine" {
		t.Error("the folder's file was touched")
	}
	empty := filepath.Join(dir, "empty")
	os.MkdirAll(empty, 0o755)
	if res, err := p.Export(ctx, ExportOptions{Target: empty, Format: ExportFolder}); err != nil || !exists(filepath.Join(empty, string(res.ID), "project.db")) {
		t.Errorf("into an empty folder: %+v, %v", res, err)
	}

	z := filepath.Join(dir, "a.zip")
	os.WriteFile(z, []byte("old"), 0o644)
	if _, err := p.Export(ctx, ExportOptions{Target: z, Format: ExportZip}); !errors.Is(err, ErrExportExists) {
		t.Errorf("over a file without Replace: %v", err)
	}
	if res, err := p.Export(ctx, ExportOptions{Target: z, Format: ExportZip, Replace: true}); err != nil || res.Size <= 3 {
		t.Errorf("over a file with Replace: %+v, %v", res, err)
	}
	if _, err := p.Export(ctx, ExportOptions{Target: filepath.Join(dir, "a.zip"), Format: ExportFolder, Replace: true}); !errors.Is(err, ErrExportExists) {
		t.Errorf("a folder over a file: %v", err)
	}

	if _, err := p.Export(ctx, ExportOptions{Target: filepath.Join(p.Dir, "tmp", "x.zip"), Format: ExportZip}); err == nil {
		t.Error("an export into the project's own folder was allowed")
	}
	if _, err := p.Export(ctx, ExportOptions{Target: filepath.Join(dir, "x"), Format: "tar"}); err == nil {
		t.Error("an unknown format was allowed")
	}

	p.mu.Lock()
	p.exportDone = make(chan struct{})
	p.mu.Unlock()
	_, err := p.Export(ctx, ExportOptions{Target: filepath.Join(dir, "b.zip"), Format: ExportZip})
	p.mu.Lock()
	p.exportDone = nil
	p.mu.Unlock()
	if !errors.Is(err, ErrExportRunning) {
		t.Errorf("a second export at once: %v", err)
	}

	defer func(f func(string) (uint64, string, bool)) { freeSpace = f }(freeSpace)
	freeSpace = func(string) (uint64, string, bool) { return 1 << 20, "v", true }
	_, err = p.Export(ctx, ExportOptions{Target: filepath.Join(dir, "c.zip"), Format: ExportZip})
	if !errors.Is(err, ErrNoSpace) || len(partials(filepath.Join(dir, "c.zip"))) != 0 {
		t.Errorf("with 1 MB free: %v", err)
	}
	t.Log(err)
}

// TestExportCancel: a cancelled export, and one whose project closes,
// leave nothing behind.
func TestExportCancel(t *testing.T) {
	for _, f := range []ExportFormat{ExportZip, ExportFolder} {
		t.Run(string(f), func(t *testing.T) {
			e := newEnv(t, t.TempDir())
			defer e.stop(t)
			p, _ := seedProject(t, e, 30, 32<<10)
			defer p.Release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			target := filepath.Join(t.TempDir(), "out")
			_, err := p.Export(ctx, ExportOptions{Target: target, Format: f, Progress: func(pr ExportProgress) {
				if pr.Step == StepObjects {
					cancel()
				}
			}})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v", err)
			}
			if exists(target) || len(partials(target)) != 0 {
				t.Error("a cancelled export left files")
			}
			if left, _ := os.ReadDir(filepath.Join(p.Dir, "tmp")); len(left) != 0 {
				t.Errorf("tmp/ has %d entries", len(left))
			}
			if res, err := p.Export(context.Background(), ExportOptions{Target: target, Format: f}); err != nil || res.Objects != 30 {
				t.Errorf("the next export: %+v, %v", res, err)
			}
		})
	}
}

// TestExportClose: closing the project stops a running export and waits
// until it has removed what it wrote, its database copies in tmp/ too.
func TestExportClose(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, root)
	p, _ := seedProject(t, e, 30, 32<<10)
	dir := p.Dir
	target := filepath.Join(t.TempDir(), "out.zip")
	started := make(chan struct{})
	var once sync.Once
	errc := make(chan error, 1)
	go func() {
		_, err := p.Export(context.Background(), ExportOptions{Target: target, Format: ExportZip, Progress: func(pr ExportProgress) {
			if pr.Step == StepObjects {
				once.Do(func() { close(started) })
				time.Sleep(20 * time.Millisecond) // keep it running while the project closes
			}
		}})
		errc <- err
	}()
	<-started
	e.stop(t) // CloseAll, with the export's lease still held
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	default:
		t.Fatal("the project closed before the export stopped")
	}
	if _, err := p.Export(context.Background(), ExportOptions{Target: filepath.Join(t.TempDir(), "late.zip"), Format: ExportZip}); !errors.Is(err, ErrClosed) {
		t.Errorf("an export after close: %v", err)
	}
	p.Release()
	if exists(target) || len(partials(target)) != 0 {
		t.Error("the stopped export left files")
	}
	if left, _ := os.ReadDir(filepath.Join(dir, "tmp")); len(left) != 0 {
		t.Errorf("tmp/ has %d entries", len(left))
	}
	if exists(lockPath(dir)) {
		t.Error("the project did not close cleanly")
	}
}

// TestExportOddTarget: the chosen place may hold characters that SQLite
// reads in a URI filename, and a .partial of the same name that isn't
// the export's own stays untouched.
func TestExportOddTarget(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 2, 100)
	defer p.Release()
	dir := filepath.Join(t.TempDir(), "C# 100% ready")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []ExportFormat{ExportFolder, ExportZip} {
		target := filepath.Join(dir, "out #1 %41")
		if f == ExportZip {
			target += ".zip"
		}
		mine := target + ".partial"
		if err := os.WriteFile(mine, []byte("the user's"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := p.Export(context.Background(), ExportOptions{Target: target, Format: f})
		if err != nil || res.Objects != 2 {
			t.Fatalf("%s: %+v, %v", f, res, err)
		}
		other := t.TempDir()
		place(t, res, testPaths(other).Projects)
		checkCopy(t, other, res.ID, "صادرات BTC", nil)
		if b, err := os.ReadFile(mine); err != nil || string(b) != "the user's" {
			t.Errorf("%s: the user's file became %q, %v", f, b, err)
		}
	}
	if ents, _ := os.ReadDir(filepath.Dir(dir)); len(ents) != 1 {
		t.Errorf("a file was made outside the chosen folder: %v", ents)
	}
}

// TestExportTargetTaken: what appears at the target while the export runs
// is kept, and the export fails as ErrExportExists without leaving
// anything.
func TestExportTargetTaken(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 5, 1000)
	defer p.Release()
	cases := []struct {
		name   string
		f      ExportFormat
		before func(target string) error // at the start
		during func(target string) error // in the objects step
		check  func(target string) error // the user's things are still there
	}{
		{"a file at a zip", ExportZip, nil, userFile, hasUserFile},
		{"a file at a folder", ExportFolder, nil, userFile, hasUserFile},
		{"a folder at a zip", ExportZip, nil, userDir, hasUserDir},
		{"a folder at a folder", ExportFolder, nil, userDir, hasUserDir},
		{"a file in the empty folder", ExportFolder, func(t string) error { return os.Mkdir(t, 0o755) },
			func(t string) error { return userFile(filepath.Join(t, "mine.txt")) },
			func(t string) error { return hasUserFile(filepath.Join(t, "mine.txt")) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "out")
			if c.before != nil {
				if err := c.before(target); err != nil {
					t.Fatal(err)
				}
			}
			var once sync.Once
			var derr error
			_, err := p.Export(context.Background(), ExportOptions{Target: target, Format: c.f, Progress: func(pr ExportProgress) {
				if pr.Step == StepObjects {
					once.Do(func() { derr = c.during(target) })
				}
			}})
			if derr != nil {
				t.Fatal(derr)
			}
			if !errors.Is(err, ErrExportExists) {
				t.Errorf("err = %v", err)
			}
			if err := c.check(target); err != nil {
				t.Errorf("the user's things: %v", err)
			}
			if left := partials(target); len(left) != 0 {
				t.Errorf("left %v", left)
			}
		})
	}
}

func userFile(name string) error { return os.WriteFile(name, []byte("USER FILE"), 0o644) }

func hasUserFile(name string) error {
	b, err := os.ReadFile(name)
	if err == nil && string(b) != "USER FILE" {
		err = fmt.Errorf("%s holds %q", name, b)
	}
	return err
}

func userDir(name string) error {
	if err := os.Mkdir(name, 0o755); err != nil {
		return err
	}
	return userFile(filepath.Join(name, "mine.txt"))
}

func hasUserDir(name string) error {
	if ents, err := os.ReadDir(name); err != nil || len(ents) != 1 {
		return fmt.Errorf("%s holds %v, %v", name, ents, err)
	}
	return hasUserFile(filepath.Join(name, "mine.txt"))
}

// TestCleanExports: the next open removes a recorded .partial, and only
// one whose name is that export's.
func TestCleanExports(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tmp", "export-"+id.New()), 0o755); err != nil { // a zip's database copies
		t.Fatal(err)
	}
	ours, other := id.New(), id.New()
	partial := filepath.Join(out, "a.zip."+ours+".partial")
	notOurs := filepath.Join(out, "b.zip."+other+".partial")
	mine := filepath.Join(out, "mine.txt")
	for _, f := range []string{partial, notOurs, mine} {
		if err := userFile(f); err != nil {
			t.Fatal(err)
		}
	}
	for cid, path := range map[string]string{ours: partial, id.New(): notOurs, id.New() + "x": mine} {
		if err := writeRecord(filepath.Join(dir, "tmp", "export-"+cid+recordExt), path); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(dir, "tmp", "other.txt")
	if err := userFile(keep); err != nil {
		t.Fatal(err)
	}
	cleanExports(dir, slog.New(slog.DiscardHandler))
	if exists(partial) {
		t.Error("the recorded .partial is still there")
	}
	if !exists(notOurs) || !exists(mine) || !exists(keep) {
		t.Error("a file that isn't the export's was removed")
	}
	left, _ := os.ReadDir(filepath.Join(dir, "tmp"))
	if len(left) != 1 || left[0].Name() != "other.txt" { // every record goes, used or not
		t.Errorf("tmp/ holds %v", left)
	}
}

// TestExportApprovals: by default the copy trusts nothing: no approvals,
// no private hosts, and the level a new project gets now. With
// IncludeApprovals it trusts what the original does.
func TestExportApprovals(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 1, 100)
	defer p.Release()
	for i, target := range []string{"api.example.com", "abc123"} {
		a := store.Approval{ID: id.Approval(id.New()), Kind: []string{"host", "starlark"}[i], Target: target, Answer: chat.GrantAlways, Source: id.SourceUser}
		if err := p.DB.RecordApproval(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.SetLevel(ctx, Auto); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPrivateHosts(ctx, []string{"nas.local"}); err != nil {
		t.Fatal(err)
	}
	e.m.d.Level = func() Level { return Strict } // approvals.default_level now

	type trust struct {
		level     Level
		hosts     []string
		approvals []store.Approval
	}
	read := func(p *Project) trust {
		t.Helper()
		l, err := p.Level(ctx)
		if err != nil {
			t.Fatal(err)
		}
		h, err := p.PrivateHosts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		as, err := p.DB.Approvals(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return trust{l, h, as}
	}
	src := read(p)
	if len(src.approvals) != 2 || src.level != Auto || len(src.hosts) != 1 {
		t.Fatalf("the source: %+v", src)
	}
	for _, include := range []bool{false, true} {
		res, err := p.Export(ctx, ExportOptions{Target: filepath.Join(t.TempDir(), "out.zip"), Format: ExportZip, IncludeApprovals: include})
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		place(t, res, testPaths(root).Projects)
		ce := newEnv(t, root)
		if _, err := ce.m.List(ctx); err != nil { // finds the copy
			t.Fatal(err)
		}
		cp := open(t, ce.m, res.ID)
		got := read(cp)
		meta, err := cp.DB.Meta(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cp.Release()
		ce.stop(t)
		if include {
			if got.level != src.level || !slices.Equal(got.hosts, src.hosts) || fmt.Sprint(got.approvals) != fmt.Sprint(src.approvals) {
				t.Errorf("with approvals: %+v, want %+v", got, src)
			}
			continue
		}
		if _, ok := meta[metaPrivateHosts]; ok || got.level != Strict || len(got.hosts) != 0 || len(got.approvals) != 0 {
			t.Errorf("without approvals: %+v, meta %v", got, meta)
		}
	}
	if now := read(p); fmt.Sprint(now) != fmt.Sprint(src) {
		t.Errorf("the source changed: %+v", now)
	}
}

// partials lists what exports to target are writing or left.
func partials(target string) []string {
	m, _ := filepath.Glob(target + ".*.partial")
	return m
}

// TestExportMissingBytes: an object whose bytes are gone is counted and
// left out; the rest is exported.
func TestExportMissingBytes(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, objs := seedProject(t, e, 3, 100)
	defer p.Release()
	o, err := p.DB.HeadObject(context.Background(), "seed/1.bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(p.Dir, "objects", o.SHA256[:2], o.SHA256[2:4], o.SHA256)); err != nil {
		t.Fatal(err)
	}
	res, err := p.Export(context.Background(), ExportOptions{Target: filepath.Join(t.TempDir(), "out.zip"), Format: ExportZip})
	if err != nil || res.Missing != 1 || res.Objects != 2 {
		t.Fatalf("%+v, %v", res, err)
	}
	_ = objs
}

func TestBytesText(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 kB", 123_456_789: "123.5 MB", 2_000_000_000_000: "2.0 TB"} {
		if got := bytesText(n); got != want {
			t.Errorf("bytesText(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestExportMeasure measures an export of a project with 100,000 rows and
// 200 MB of objects, half random and half text, while writes go on. It
// runs only with JENAB_EXPORT_MEASURE=1.
func TestExportMeasure(t *testing.T) {
	if os.Getenv("JENAB_EXPORT_MEASURE") == "" {
		t.Skip("set JENAB_EXPORT_MEASURE=1 to measure")
	}
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 100, 1<<20) // 100 MB random
	defer p.Release()
	for i := range 100 { // 100 MB text
		body := strings.Repeat(fmt.Sprintf("line %d of a text file that compresses well\n", i), (1<<20)/45)
		if _, err := p.DB.PutObject(ctx, limit.Interactive, id.SourceUser, fmt.Sprintf("text/%d.txt", i), strings.NewReader(body), store.PutOptions{Source: "upload"}); err != nil {
			t.Fatal(err)
		}
	}
	ch := mother(t, p)
	for b := range 500 { // 100,000 rows
		if err := writeBatch(ctx, p, ch, b); err != nil {
			t.Fatal(err)
		}
	}
	est, err := p.Estimate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("project: %s, of which databases %s, %d objects", bytesText(est.Bytes), bytesText(est.Databases), est.Objects)
	for _, f := range []ExportFormat{ExportZip, ExportFolder} {
		stop := make(chan struct{})
		done := make(chan int)
		go func() {
			n := 0
			for b := 500 + 1000*len(f); ; b++ {
				select {
				case <-stop:
					done <- n
					return
				default:
				}
				if err := writeBatch(ctx, p, ch, b); err != nil {
					panic(err)
				}
				n++
			}
		}()
		target := filepath.Join(t.TempDir(), "out")
		start := time.Now()
		res, err := p.Export(ctx, ExportOptions{Target: target, Format: f})
		took := time.Since(start)
		close(stop)
		n := <-done
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %s in %v, %d objects, %d turns of 200 rows written meanwhile", f, bytesText(res.Size), took.Round(time.Millisecond), res.Objects, n)
	}
}
