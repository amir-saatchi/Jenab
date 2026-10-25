package app

import (
	"context"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The settings save memory as the user's, return a conflict with the
// section as it is now, and the chip's undo reverts one edit and puts it
// back (SPEC 3.3).
func TestMemoryServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Memory")
	if err != nil {
		t.Fatal(err)
	}

	s, err := e.svc.Settings.SaveUserMemory(ctx, MemoryEdit{Section: "preferences", Content: "  Short answers.\n"})
	if err != nil || s.Conflict != nil || len(s.Memory.Sections) != 1 || s.Memory.Sections[0].Content != "Short answers." ||
		s.Memory.Sections[0].Source != id.SourceUser || s.Memory.Cap != 1000 || s.Memory.Tokens == 0 {
		t.Fatalf("save %+v, %v", s, err)
	}
	s, err = e.svc.Settings.SaveUserMemory(ctx, MemoryEdit{Section: "preferences", Content: "Long answers."})
	if err != nil || s.Conflict == nil || s.Conflict.Revision != 1 || s.Conflict.Content != "Short answers." {
		t.Fatalf("stale save %+v, %v", s, err)
	}
	_, err = e.svc.Settings.SaveUserMemory(ctx, MemoryEdit{Section: "a/b", Content: "x"})
	uiErr(t, err, KindInvalid)

	// A chat's edits of both memories, then their chips' undo.
	src := id.SourceOf("message", id.New())
	var userChange, projChange string
	err = open(ctx, e.pm, op.ID, func(p *project.Project) error {
		w, err := p.Registry.UpdateUserMemory(ctx, store.MemoryEdit{Section: "preferences", Content: "In Persian.", Expected: 1, Source: src}, p.ID)
		if err != nil {
			return err
		}
		userChange = w.Change
		w, err = p.DB.UpdateMemory(ctx, store.MemoryEdit{Section: "goals", Content: "Track gold.", Source: src})
		projChange = w.Change
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.svc.Chat.UndoMemory(ctx, op.ID, MemoryUndoRequest{Target: "memory:user/preferences", Change: userChange})
	if err != nil || !u.Done {
		t.Fatalf("undo %+v, %v", u, err)
	}
	if v, err := e.svc.Settings.UserMemory(ctx); err != nil || v.Sections[0].Content != "Short answers." || v.Sections[0].Revision != 3 {
		t.Errorf("user memory after undo %+v, %v", v, err)
	}
	// A chip mounted again, after a reload: it reads the edit as undone,
	// and a second undo is no error but says so.
	userReq := MemoryUndoRequest{Target: "memory:user/preferences", Change: userChange}
	if undone, err := e.svc.Chat.MemoryUndone(ctx, op.ID, userReq); err != nil || !undone {
		t.Errorf("undone %v, %v", undone, err)
	}
	if u, err := e.svc.Chat.UndoMemory(ctx, op.ID, userReq); err != nil || u.Done || !u.Undone {
		t.Errorf("undo again %+v, %v", u, err)
	}
	u, err = e.svc.Chat.UndoMemory(ctx, op.ID, MemoryUndoRequest{Target: "memory:project/goals", Change: projChange})
	if err != nil || !u.Done {
		t.Fatalf("undo %+v, %v", u, err)
	}
	if v, err := e.svc.Project.Memory(ctx, op.ID); err != nil || len(v.Sections) != 0 || v.Cap != 2000 {
		t.Errorf("project memory after undo %+v, %v", v, err)
	}
	if u, err = e.svc.Chat.UndoMemory(ctx, op.ID, MemoryUndoRequest{Target: "memory:project/goals", Change: projChange, Redo: true}); err != nil || !u.Done || u.Undone {
		t.Fatalf("redo %+v, %v", u, err)
	}
	if undone, err := e.svc.Chat.MemoryUndone(ctx, op.ID, MemoryUndoRequest{Target: "memory:project/goals", Change: projChange}); err != nil || undone {
		t.Errorf("undone after redo %v, %v", undone, err)
	}

	// Edited since: the undo is refused and says so.
	if _, err := e.svc.Project.SaveMemory(ctx, op.ID, MemoryEdit{Section: "goals", Content: "Track silver.", Revision: 3}); err != nil {
		t.Fatal(err)
	}
	if u, err = e.svc.Chat.UndoMemory(ctx, op.ID, MemoryUndoRequest{Target: "memory:project/goals", Change: projChange}); err != nil || u.Done || !u.Changed {
		t.Errorf("undo over a later edit %+v, %v", u, err)
	}
	if v, err := e.svc.Project.Memory(ctx, op.ID); err != nil || len(v.Sections) != 1 || v.Sections[0].Content != "Track silver." {
		t.Errorf("project memory %+v, %v", v, err)
	}
}
