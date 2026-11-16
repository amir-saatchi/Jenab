package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The data folder can't be a relative path, the projects folder or inside
// it, or inside a project's folder; CheckFolder says so, and Save refuses
// it instead of using the default (TASK-068).
func TestTheDataFolderIsRefusedWhereProjectsCantMove(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	e := newEnv(t, root)
	ss := e.svc.Settings
	ss.started.DataFolder = e.paths.DataFolder
	pid, err := e.pm.Create(ctx, "Ledger")
	if err != nil {
		t.Fatal(err)
	}
	inProject := filepath.Join(e.paths.Projects, string(pid), "data")
	elsewhere := filepath.Join(root, "Coins")
	if err := e.reg.SaveProject(ctx, store.ProjectEntry{ID: id.Project(id.New()), Name: "Coins", Folder: elsewhere, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ dir, want string }{
		{"Jenab", "not a full path"},
		{e.paths.Projects, "inside the projects folder"},
		{inProject, "inside the projects folder"},
		{filepath.Join(elsewhere, "x"), "inside the folder of the project Coins"},
	} {
		var ue *UIError
		if _, err := ss.CheckFolder(ctx, c.dir); !errors.As(err, &ue) || ue.Kind != KindInvalid || !strings.Contains(ue.Message, c.want) {
			t.Errorf("CheckFolder(%q) = %v", c.dir, err)
		}
		next := e.settings.Get()
		next.DataFolder = c.dir
		if _, err := ss.Save(ctx, next); !errors.As(err, &ue) || !strings.Contains(ue.Message, c.want) {
			t.Errorf("Save(%q) = %v", c.dir, err)
		}
		if got := e.settings.Get().DataFolder; got != "" {
			t.Fatalf("after Save(%q) the data folder is %q", c.dir, got)
		}
	}
	good := filepath.Join(root, "New")
	if w, err := ss.CheckFolder(ctx, good); w != nil || err != nil {
		t.Errorf("CheckFolder(good) = %v, %v", w, err)
	}
	next := e.settings.Get()
	next.DataFolder = good
	if _, err := ss.Save(ctx, next); err != nil || e.settings.Get().DataFolder != good {
		t.Errorf("Save(good) = %v; data folder %q", err, e.settings.Get().DataFolder)
	}
	// A save that keeps the folder isn't checked again.
	next = e.settings.Get()
	next.DevTools = !next.DevTools
	if _, err := ss.Save(ctx, next); err != nil {
		t.Errorf("Save(same folder) = %v", err)
	}
}
