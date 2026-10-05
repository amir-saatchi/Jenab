package project

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
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
	rename = func(string, string) error { return errors.New("not the same device") }
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
