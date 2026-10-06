package project

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

func TestMoveProjectsMovesThemOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	e := newEnv(t, root)
	defer e.stop(t)
	log := slog.New(slog.DiscardHandler)

	// The first start only records the folder.
	if r, err := MoveProjects(ctx, e.paths, e.reg, log); r != nil || err != nil {
		t.Fatalf("first start: %v, %v", r, err)
	}
	a := create(t, e.m, "Coins")
	b := create(t, e.m, "Notes")

	moved := e.paths.WithDataFolder(filepath.Join(root, "elsewhere"))
	r, err := MoveProjects(ctx, moved, e.reg, log)
	if err != nil {
		t.Fatal(err)
	}
	if r.Moved != 2 || len(r.Failed) != 0 || r.To != moved.DataFolder {
		t.Fatalf("result = %+v", r)
	}
	for _, pid := range []string{string(a), string(b)} {
		p, err := e.reg.Project(ctx, id.Project(pid))
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(moved.Projects, pid); p.Folder != want || !exists(filepath.Join(want, "project.db")) {
			t.Fatalf("%s: folder %s, want %s", pid, p.Folder, want)
		}
		if exists(filepath.Join(e.paths.Projects, pid)) {
			t.Fatalf("%s: still in the old folder", pid)
		}
	}
	// The next start finds nothing to do.
	if r, err := MoveProjects(ctx, moved, e.reg, log); r != nil || err != nil {
		t.Fatalf("next start: %v, %v", r, err)
	}
	// And the moved projects open.
	m := NewManager(Deps{Paths: moved, Registry: e.reg})
	open(t, m, a).Release()
	if err := m.CloseAll(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMoveProjectsCopiesAcrossDrivesAndKeepsWhatFails(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	e := newEnv(t, root)
	defer e.stop(t)
	log := slog.New(slog.DiscardHandler)
	if _, err := MoveProjects(ctx, e.paths, e.reg, log); err != nil {
		t.Fatal(err)
	}
	a := create(t, e.m, "Coins")
	b := create(t, e.m, "Notes")

	// No rename works, as between drives; b's place is taken.
	rename = crossRename
	defer func() { rename = os.Rename }()
	moved := e.paths.WithDataFolder(filepath.Join(root, "other"))
	if err := os.MkdirAll(filepath.Join(moved.Projects, string(b)), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := MoveProjects(ctx, moved, e.reg, log)
	if err != nil {
		t.Fatal(err)
	}
	if r.Moved != 1 || len(r.Failed) != 1 || r.Failed[0] != "Notes" {
		t.Fatalf("result = %+v", r)
	}
	pa, _ := e.reg.Project(ctx, a)
	pb, _ := e.reg.Project(ctx, b)
	if pa.Folder != filepath.Join(moved.Projects, string(a)) || !exists(filepath.Join(pa.Folder, "chats.db")) || exists(filepath.Join(e.paths.Projects, string(a))) {
		t.Fatalf("a was not copied: %s", pa.Folder)
	}
	if pb.Folder != filepath.Join(e.paths.Projects, string(b)) || !exists(filepath.Join(pb.Folder, "project.db")) {
		t.Fatalf("b moved: %s", pb.Folder)
	}
	// The next start tries again.
	if err := os.Remove(filepath.Join(moved.Projects, string(b))); err != nil {
		t.Fatal(err)
	}
	r, err = MoveProjects(ctx, moved, e.reg, log)
	if err != nil || r == nil || r.Moved != 1 {
		t.Fatalf("retry: %+v, %v", r, err)
	}
}

// crossRename fails as a rename to another drive does.
func crossRename(src, dst string) error {
	return &os.LinkError{Op: "rename", Old: src, New: dst, Err: errCrossDevice}
}

// moveEnv is an env whose data folder is recorded, with two projects.
func moveEnv(t *testing.T) (e *env, a, b id.Project, root string) {
	t.Helper()
	root = t.TempDir()
	e = newEnv(t, root)
	t.Cleanup(func() { e.stop(t) })
	if _, err := MoveProjects(context.Background(), e.paths, e.reg, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	return e, create(t, e.m, "Coins"), create(t, e.m, "Notes"), root
}

func folderOf(t *testing.T, e *env, pid id.Project) string {
	t.Helper()
	p, err := e.reg.Project(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	return p.Folder
}

func TestMoveProjectsCopiesOnlyAcrossDrives(t *testing.T) {
	ctx := context.Background()
	e, a, _, root := moveEnv(t)
	log := slog.New(slog.DiscardHandler)
	// A file still open, say: copying it could lose what is written next.
	rename = func(src, dst string) error {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: errors.New("access denied")}
	}
	defer func() { rename = os.Rename }()
	moved := e.paths.WithDataFolder(filepath.Join(root, "other"))
	r, err := MoveProjects(ctx, moved, e.reg, log)
	if err != nil {
		t.Fatal(err)
	}
	if r.Moved != 0 || len(r.Failed) != 2 {
		t.Fatalf("result = %+v", r)
	}
	if got := folderOf(t, e, a); got != filepath.Join(e.paths.Projects, string(a)) {
		t.Errorf("registry points at %s", got)
	}
	if ents, _ := os.ReadDir(moved.Projects); len(ents) != 0 {
		t.Errorf("something was copied: %v", ents)
	}
	// The next start tries again.
	rename = os.Rename
	if r, err := MoveProjects(ctx, moved, e.reg, log); err != nil || r.Moved != 2 {
		t.Fatalf("retry: %+v, %v", r, err)
	}
}

func TestMoveProjectsRepairsAStoppedRename(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	// The folder was renamed, then the app stopped before the registry was
	// updated.
	stopped := func(t *testing.T) (*env, id.Project, id.Project, config.Paths) {
		e, a, b, root := moveEnv(t)
		moved := e.paths.WithDataFolder(filepath.Join(root, "other"))
		if err := os.MkdirAll(moved.Projects, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(e.paths.Projects, string(a)), filepath.Join(moved.Projects, string(a))); err != nil {
			t.Fatal(err)
		}
		return e, a, b, moved
	}
	t.Run("next start", func(t *testing.T) {
		e, a, b, moved := stopped(t)
		r, err := MoveProjects(ctx, moved, e.reg, log)
		if err != nil || r.Moved != 2 || len(r.Failed) != 0 {
			t.Fatalf("%+v, %v", r, err)
		}
		for _, pid := range []id.Project{a, b} {
			if got, want := folderOf(t, e, pid), filepath.Join(moved.Projects, string(pid)); got != want {
				t.Errorf("%s: folder %s, want %s", pid, got, want)
			}
		}
	})
	t.Run("open", func(t *testing.T) {
		e, a, _, moved := stopped(t)
		m := NewManager(Deps{Paths: moved, Registry: e.reg})
		open(t, m, a).Release()
		if err := m.CloseAll(ctx); err != nil {
			t.Fatal(err)
		}
		if got, want := folderOf(t, e, a), filepath.Join(moved.Projects, string(a)); got != want {
			t.Errorf("folder %s, want %s", got, want)
		}
	})
	t.Run("list", func(t *testing.T) {
		e, a, _, moved := stopped(t)
		m := NewManager(Deps{Paths: moved, Registry: e.reg})
		if _, err := m.List(ctx); err != nil {
			t.Fatal(err)
		}
		if got, want := folderOf(t, e, a), filepath.Join(moved.Projects, string(a)); got != want {
			t.Errorf("folder %s, want %s", got, want)
		}
	})
}

func TestMoveProjectsFinishesAStoppedCopy(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	rename = crossRename
	defer func() { rename = os.Rename }()
	// The copy was finished, then the app stopped before the registry was
	// updated.
	stopped := func(t *testing.T) (*env, id.Project, config.Paths) {
		e, a, _, root := moveEnv(t)
		moved := e.paths.WithDataFolder(filepath.Join(root, "other"))
		if err := os.MkdirAll(moved.Projects, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := copyDir(filepath.Join(e.paths.Projects, string(a)), filepath.Join(moved.Projects, string(a))); err != nil {
			t.Fatal(err)
		}
		return e, a, moved
	}
	t.Run("same files", func(t *testing.T) {
		e, a, moved := stopped(t)
		r, err := MoveProjects(ctx, moved, e.reg, log)
		if err != nil || r.Moved != 2 || len(r.Failed) != 0 {
			t.Fatalf("%+v, %v", r, err)
		}
		if got, want := folderOf(t, e, a), filepath.Join(moved.Projects, string(a)); got != want {
			t.Errorf("folder %s, want %s", got, want)
		}
		if exists(filepath.Join(e.paths.Projects, string(a))) {
			t.Error("the old folder is still there")
		}
	})
	t.Run("changed since", func(t *testing.T) {
		e, a, moved := stopped(t)
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(filepath.Join(e.paths.Projects, string(a), "project.db"), later, later); err != nil {
			t.Fatal(err)
		}
		r, err := MoveProjects(ctx, moved, e.reg, log)
		if err != nil || r.Moved != 1 || len(r.Failed) != 1 || r.Failed[0] != "Coins" {
			t.Fatalf("%+v, %v", r, err)
		}
		if got, want := folderOf(t, e, a), filepath.Join(e.paths.Projects, string(a)); got != want {
			t.Errorf("folder %s, want %s", got, want)
		}
	})
}

func TestMoveProjectsWhenTheRegistryFails(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	saveProject = func(*store.Registry, context.Context, store.ProjectEntry) error { return errors.New("disk full") }
	defer func() { saveProject = (*store.Registry).SaveProject }()
	for name, rn := range map[string]func(string, string) error{"rename": os.Rename, "copy": crossRename} {
		t.Run(name, func(t *testing.T) {
			rename = rn
			defer func() { rename = os.Rename }()
			e, a, _, root := moveEnv(t)
			moved := e.paths.WithDataFolder(filepath.Join(root, "other"))
			r, err := MoveProjects(ctx, moved, e.reg, log)
			if err != nil || r.Moved != 0 || len(r.Failed) != 2 {
				t.Fatalf("%+v, %v", r, err)
			}
			old := filepath.Join(e.paths.Projects, string(a))
			if got := folderOf(t, e, a); got != old || !exists(filepath.Join(old, "project.db")) {
				t.Errorf("folder %s, want %s", got, old)
			}
			if ents, _ := os.ReadDir(moved.Projects); len(ents) != 0 {
				t.Errorf("left in the new folder: %v", ents)
			}
		})
	}
}

func TestMoveProjectsWhenTheOldFolderStays(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	e, a, _, root := moveEnv(t)
	moved := e.paths.WithDataFolder(filepath.Join(root, "other"))
	rename = crossRename
	removeAll = func(p string) error {
		if strings.HasPrefix(p, e.paths.Projects) {
			return errors.New("in use")
		}
		return os.RemoveAll(p)
	}
	defer func() { rename, removeAll = os.Rename, os.RemoveAll }()
	r, err := MoveProjects(ctx, moved, e.reg, log)
	if err != nil || r.Moved != 2 || len(r.Failed) != 0 {
		t.Fatalf("%+v, %v", r, err)
	}
	old := filepath.Join(e.paths.Projects, string(a))
	if got, want := folderOf(t, e, a), filepath.Join(moved.Projects, string(a)); got != want || !exists(old) {
		t.Fatalf("folder %s, want %s; old one there: %v", got, want, exists(old))
	}
	// A start that still sees the old data folder removes the old copy.
	removeAll = os.RemoveAll
	if err := os.WriteFile(e.paths.LastData, []byte(e.paths.DataFolder+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = MoveProjects(ctx, moved, e.reg, log)
	if err != nil || r.Moved != 0 || len(r.Failed) != 0 {
		t.Fatalf("%+v, %v", r, err)
	}
	if exists(old) {
		t.Error("the old copy is still there")
	}
	if got, want := folderOf(t, e, a), filepath.Join(moved.Projects, string(a)); got != want {
		t.Errorf("folder %s, want %s", got, want)
	}
}

func TestMoveProjectsAfterAPartialMove(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	e, a, b, root := moveEnv(t)
	// A to B: a moves, b's place is taken.
	pb := e.paths.WithDataFolder(filepath.Join(root, "b"))
	if err := os.MkdirAll(filepath.Join(pb.Projects, string(b)), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := MoveProjects(ctx, pb, e.reg, log)
	if err != nil || r.Moved != 1 || len(r.Failed) != 1 {
		t.Fatalf("A to B: %+v, %v", r, err)
	}
	// On to C, and back to A: both follow each time.
	pc := e.paths.WithDataFolder(filepath.Join(root, "c"))
	for _, to := range []config.Paths{pc, e.paths} {
		r, err := MoveProjects(ctx, to, e.reg, log)
		if err != nil || r.Moved != 2 || len(r.Failed) != 0 {
			t.Fatalf("to %s: %+v, %v", to.DataFolder, r, err)
		}
		for _, pid := range []id.Project{a, b} {
			if got, want := folderOf(t, e, pid), filepath.Join(to.Projects, string(pid)); got != want || !exists(filepath.Join(want, "project.db")) {
				t.Errorf("to %s: %s is at %s", to.DataFolder, pid, got)
			}
		}
	}
	if exists(filepath.Join(pb.Projects, string(a))) {
		t.Error("a was left in B")
	}
	if r, err := MoveProjects(ctx, e.paths, e.reg, log); r != nil || err != nil {
		t.Fatalf("next start: %+v, %v", r, err)
	}
}

func TestMoveProjectsRefusesNestedFolders(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	t.Run("new inside old", func(t *testing.T) {
		e, a, _, _ := moveEnv(t)
		moved := e.paths.WithDataFolder(filepath.Join(e.paths.Projects, string(a)))
		r, err := MoveProjects(ctx, moved, e.reg, log)
		if !errors.Is(err, ErrNested) || r == nil || r.Moved != 0 || len(r.Failed) != 2 {
			t.Fatalf("%+v, %v", r, err)
		}
		if exists(moved.Projects) {
			t.Error("the new projects folder was made inside a project")
		}
		if got, want := folderOf(t, e, a), filepath.Join(e.paths.Projects, string(a)); got != want {
			t.Errorf("folder %s, want %s", got, want)
		}
	})
	t.Run("old inside new", func(t *testing.T) {
		root := t.TempDir()
		e := newEnv(t, root)
		defer e.stop(t)
		e.paths = e.paths.WithDataFolder(filepath.Join(root, "outer", "projects", "inner"))
		e.m = NewManager(Deps{Paths: e.paths, Registry: e.reg})
		if _, err := MoveProjects(ctx, e.paths, e.reg, log); err != nil {
			t.Fatal(err)
		}
		create(t, e.m, "Coins")
		moved := e.paths.WithDataFolder(filepath.Join(root, "outer"))
		if r, err := MoveProjects(ctx, moved, e.reg, log); !errors.Is(err, ErrNested) || r == nil || len(r.Failed) != 1 {
			t.Fatalf("%+v, %v", r, err)
		}
	})
}

func TestSamePath(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "Data")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if !samePath(a, a+string(filepath.Separator)) || samePath(a, root) {
		t.Error("plain paths")
	}
	if !samePath(filepath.Join(root, "Gone"), filepath.Join(root, "x", "..", "Gone")) {
		t.Error("missing paths")
	}
	if caseless(runtime.GOOS) && !samePath(a, strings.ToUpper(a)) {
		t.Error("case")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(a, link); err != nil {
		t.Logf("no symlink here: %v", err)
	} else if !samePath(a, link) {
		t.Error("a link to the folder")
	}
	if !inside(filepath.Join(a, "projects", "x"), a) || inside(a, filepath.Join(a, "projects")) || inside(a, a) {
		t.Error("inside")
	}
}
