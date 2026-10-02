package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

func TestRegistry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.db")
	r, err := OpenRegistry(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	a := ProjectEntry{ID: id.Project(id.New()), Name: "Bitcoin", Folder: `D:\Jenab\projects\a`, CreatedAt: day}
	b := ProjectEntry{ID: id.Project(id.New()), Name: "پروژه", Folder: "/b", CreatedAt: day.Add(time.Hour), LastOpened: day.Add(2 * time.Hour)}
	c := ProjectEntry{ID: id.Project(id.New()), Name: "Pinned", Folder: "/c", Pinned: true, CreatedAt: day}
	for _, p := range []ProjectEntry{a, b, c} {
		if err := r.SaveProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.SaveProject(ctx, ProjectEntry{ID: "not-a-ulid", Name: "x"}); err == nil {
		t.Error("saved a project with a bad ID")
	}
	order := func() []string {
		ps, err := r.Projects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range ps {
			names = append(names, p.Name)
		}
		return names
	}
	if got := order(); len(got) != 3 || got[0] != "Pinned" || got[1] != "پروژه" || got[2] != "Bitcoin" {
		t.Errorf("order = %v, want pinned, then last opened, then never opened", got)
	}
	if err := r.TouchProject(ctx, a.ID, day.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := order(); got[1] != "Bitcoin" {
		t.Errorf("order after opening Bitcoin = %v", got)
	}
	got, err := r.Project(ctx, b.ID)
	if err != nil || got.Name != b.Name || !got.LastOpened.Equal(b.LastOpened) || !got.CreatedAt.Equal(b.CreatedAt) {
		t.Errorf("Project = %+v, %v", got, err)
	}
	if err := r.DeleteProject(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Project(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted project: %v", err)
	}
	if err := r.TouchProject(ctx, b.ID, day); !errors.Is(err, ErrNotFound) {
		t.Errorf("touch of a deleted project: %v", err)
	}
	r.Close()

	r, err = OpenRegistry(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if got := order(); len(got) != 2 {
		t.Errorf("after reopen: %v", got)
	}
	var n int
	if err := r.c.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name IN ('user_memory', 'connections', 'mcp_servers')").Scan(&n); err != nil || n != 3 {
		t.Errorf("tables for later phases: %d, %v", n, err)
	}
}
