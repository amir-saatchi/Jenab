package project

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// recorder is a Publisher that keeps what it gets.
type recorder struct {
	mu       sync.Mutex
	notices  []Notice
	activity []Activity
}

func (r *recorder) Notice(n Notice) {
	r.mu.Lock()
	r.notices = append(r.notices, n)
	r.mu.Unlock()
}

func (r *recorder) Activity(a Activity) {
	r.mu.Lock()
	r.activity = append(r.activity, a)
	r.mu.Unlock()
}

func (r *recorder) noticeKinds() []NoticeKind {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ks []NoticeKind
	for _, n := range r.notices {
		ks = append(ks, n.Kind)
	}
	return ks
}

func testPaths(root string) config.Paths {
	return config.Paths{Root: root, Registry: filepath.Join(root, "registry.db")}.WithDataFolder(filepath.Join(root, "data"))
}

// env is a manager on a fresh app folder.
type env struct {
	paths config.Paths
	reg   *store.Registry
	pub   *recorder
	m     *Manager
}

func newEnv(t *testing.T, root string) *env {
	t.Helper()
	e := &env{paths: testPaths(root), pub: &recorder{}}
	var err error
	if e.reg, err = store.OpenRegistry(context.Background(), e.paths.Registry); err != nil {
		t.Fatal(err)
	}
	e.m = NewManager(Deps{Paths: e.paths, Registry: e.reg, Events: e.pub})
	return e
}

// stop closes every project and the registry.
func (e *env) stop(t *testing.T) {
	t.Helper()
	if err := e.m.CloseAll(context.Background()); err != nil {
		t.Error(err)
	}
	if err := e.reg.Close(); err != nil {
		t.Error(err)
	}
}

func create(t *testing.T, m *Manager, name string) id.Project {
	t.Helper()
	pid, err := m.Create(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func open(t *testing.T, m *Manager, pid id.Project) *Project {
	t.Helper()
	p, err := m.Open(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCreateMakesTheLayout(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	pid := create(t, e.m, "  بیت‌کوین  ")
	dir := filepath.Join(e.paths.Projects, string(pid))
	for _, f := range []string{"project.db", "objects", "snapshots", "tmp"} {
		if !exists(filepath.Join(dir, f)) {
			t.Errorf("%s is missing", f)
		}
	}
	if exists(lockPath(dir)) {
		t.Error("Create left a lock file")
	}
	got, err := e.reg.Project(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "بیت‌کوین" || got.Folder != dir {
		t.Errorf("registry entry = %+v", got)
	}
	p := open(t, e.m, pid)
	defer p.Release()
	meta, err := p.DB.Meta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta[metaID] != string(pid) || meta[metaName] != "بیت‌کوین" || meta[metaCreated] == "" {
		t.Errorf("meta = %v", meta)
	}
	if p.Name != "بیت‌کوین" || p.Dir != dir {
		t.Errorf("project = %q in %s", p.Name, p.Dir)
	}
	if _, err := e.m.Create(context.Background(), " "); !errors.Is(err, ErrNoName) {
		t.Errorf("empty name: err = %v", err)
	}
}

func TestLeasesAndLock(t *testing.T) {
	e := newEnv(t, t.TempDir())
	pid := create(t, e.m, "BTC")
	a := open(t, e.m, pid)
	b := open(t, e.m, pid)
	if a != b {
		t.Fatal("two opens gave two projects")
	}
	if !exists(lockPath(a.Dir)) {
		t.Error("no lock file while open")
	}
	if n := a.Activity().Leases; n != 2 {
		t.Errorf("leases = %d, want 2", n)
	}
	if !e.m.Busy() {
		t.Error("not busy with two leases")
	}
	a.Release()
	b.Release()
	if e.m.Busy() {
		t.Error("busy with no leases")
	}
	if kinds := e.pub.noticeKinds(); len(kinds) != 0 {
		t.Errorf("notices after a clean open: %v", kinds)
	}
	e.stop(t)
	if exists(lockPath(a.Dir)) {
		t.Error("the lock file is still there after CloseAll")
	}
	if a.Context().Err() == nil {
		t.Error("the project context was not cancelled")
	}
	if _, err := e.m.Open(context.Background(), pid); !errors.Is(err, ErrClosed) {
		t.Errorf("open after CloseAll: err = %v", err)
	}
}

func TestOpenUnknownProject(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	if _, err := e.m.Open(context.Background(), id.Project(id.New())); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestConcurrentOpensShareOneProject(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	pid := create(t, e.m, "BTC")
	const n = 20
	got := make([]*Project, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { got[i] = open(t, e.m, pid) })
	}
	wg.Wait()
	for _, p := range got {
		if p != got[0] {
			t.Fatal("concurrent opens gave different projects")
		}
	}
	if l := got[0].Activity().Leases; l != n {
		t.Errorf("leases = %d, want %d", l, n)
	}
	for _, p := range got {
		p.Release()
	}
	if e.m.Busy() {
		t.Error("busy after every release")
	}
}

func TestIdleClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEnv(t, t.TempDir())
		defer e.stop(t)
		pid := create(t, e.m, "BTC")
		p := open(t, e.m, pid)
		closed := false
		p.OnClose(func() { closed = true })
		p.Release()

		time.Sleep(IdleAfter - time.Minute)
		synctest.Wait()
		if closed || !exists(lockPath(p.Dir)) {
			t.Fatal("closed before the idle time")
		}
		// An open in the window stops the timer; the time starts again at
		// the next release, so the first timer's time passes without a close.
		q := open(t, e.m, pid)
		if q != p {
			t.Fatal("reopen in the window gave a new project")
		}
		q.Release()
		time.Sleep(IdleAfter - time.Second)
		synctest.Wait()
		if closed {
			t.Fatal("the first timer closed the project")
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if !closed || exists(lockPath(p.Dir)) || p.Context().Err() == nil {
			t.Fatalf("not closed after the idle time: OnClose %v, lock %v", closed, exists(lockPath(p.Dir)))
		}
		e.pub.mu.Lock()
		last := e.pub.activity[len(e.pub.activity)-1]
		e.pub.mu.Unlock()
		if last.Open || last.Project != pid {
			t.Errorf("last activity = %+v, want closed", last)
		}
		// Opening again opens a new instance.
		r := open(t, e.m, pid)
		if r == p {
			t.Error("open after idle close gave the closed project")
		}
		r.Release()
	})
}

func TestOnCloseNewestFirstAndPanics(t *testing.T) {
	e := newEnv(t, t.TempDir())
	p := open(t, e.m, create(t, e.m, "BTC"))
	var order []int
	p.OnClose(func() { order = append(order, 1) })
	remove := p.OnClose(func() { order = append(order, 2) })
	p.OnClose(func() { panic("boom") })
	p.OnClose(func() { order = append(order, 3) })
	remove()
	p.Release()
	e.stop(t)
	if !slices.Equal(order, []int{3, 1}) {
		t.Errorf("OnClose order = %v, want [3 1]", order)
	}
	if exists(lockPath(p.Dir)) {
		t.Error("a panic in OnClose kept the lock")
	}
}

type fixedReporter []Status

func (f fixedReporter) Status() []Status { return f }

func TestActivityCollectsReporters(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p := open(t, e.m, create(t, e.m, "BTC"))
	defer p.Release()
	p.Report(fixedReporter{{ID: "turn-1", Kind: "turn", State: "running"}})
	un := p.Report(fixedReporter{{ID: "run-1", Kind: "run", State: "running"}, {ID: "run-2", Kind: "run", State: "waiting"}})
	if a := p.Activity(); len(a.Work) != 3 || a.Work[0].ID != "turn-1" || !a.Open || a.Leases != 1 {
		t.Errorf("activity = %+v", a)
	}
	un()
	if a := p.Activity(); len(a.Work) != 1 {
		t.Errorf("after unregister: %d items", len(a.Work))
	}
}

func TestRegistryRebuiltFromFolders(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, root)
	ids := []id.Project{create(t, e.m, "BTC"), create(t, e.m, "ETH"), create(t, e.m, "Notes")}
	before, err := e.m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e.stop(t)
	// A folder without a project and a folder that isn't a project ID are
	// skipped.
	os.MkdirAll(filepath.Join(e.paths.Projects, id.New()), 0o755)
	os.MkdirAll(filepath.Join(e.paths.Projects, "not-a-project"), 0o755)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(e.paths.Registry + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}

	e = newEnv(t, root)
	defer e.stop(t)
	after, err := e.m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(ids) {
		t.Fatalf("listed %d projects after the rebuild, want %d", len(after), len(ids))
	}
	byID := map[id.Project]store.ProjectEntry{}
	for _, p := range before {
		byID[p.ID] = p
	}
	for _, p := range after {
		b, ok := byID[p.ID]
		if !ok || b.Name != p.Name || b.Folder != p.Folder || !b.CreatedAt.Equal(p.CreatedAt) {
			t.Errorf("rebuilt %+v, was %+v", p, b)
		}
	}
	// The rebuilt projects open.
	p := open(t, e.m, ids[1])
	if p.Name != "ETH" {
		t.Errorf("name = %q", p.Name)
	}
	p.Release()
}

func TestDamagedProjectOpensReadOnly(t *testing.T) {
	e := newEnv(t, t.TempDir())
	ctx := context.Background()
	pid := create(t, e.m, "BTC")
	p := open(t, e.m, pid)
	dir := p.Dir
	_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		if _, err := tx.Exec("CREATE TABLE prices (day TEXT PRIMARY KEY, close REAL, pad TEXT)"); err != nil {
			return 0, err
		}
		for i := range 2000 {
			if _, err := tx.Exec("INSERT INTO prices VALUES (?, ?, ?)", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format(time.DateOnly), i, "padding padding padding"); err != nil {
				return 0, err
			}
		}
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Release()
	e.stop(t)

	// Garbage over the table's pages, and a lock file as after a crash.
	f, err := os.OpenFile(filepath.Join(dir, "project.db"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := f.Stat()
	junk := make([]byte, st.Size()/2)
	for i := range junk {
		junk[i] = byte(i * 7)
	}
	f.WriteAt(junk, st.Size()/4)
	f.Close()
	os.WriteFile(lockPath(dir), nil, 0o644)
	snap := filepath.Join(dir, "snapshots", "project-pre-update-20261001-120000.000-v1.db")
	os.WriteFile(snap, []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "snapshots", "chats-pre-update-20261002-120000.000-v1.db"), []byte("x"), 0o644)

	e = newEnv(t, filepath.Dir(e.paths.DataFolder))
	defer e.stop(t)
	p = open(t, e.m, pid)
	defer p.Release()
	if p.Damage == nil || !p.ReadOnly() || !p.DB.ReadOnly() {
		t.Fatalf("damage = %+v, read-only %v", p.Damage, p.DB.ReadOnly())
	}
	if p.Damage.File != "project.db" || p.Damage.Snapshot != snap || p.Damage.Problem == "" {
		t.Errorf("damage = %+v", p.Damage)
	}
	if err := p.DB.SetMeta(ctx, map[string]string{"x": "y"}); !errors.Is(err, store.ErrReadOnly) {
		t.Errorf("write: err = %v, want ErrReadOnly", err)
	}
	if kinds := e.pub.noticeKinds(); !slices.Equal(kinds, []NoticeKind{NoticeDamaged}) {
		t.Errorf("notices = %v", kinds)
	}
}
