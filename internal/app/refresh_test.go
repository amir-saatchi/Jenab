package app

import (
	"context"
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Refresh memory from the header: nothing to review in a new chat, an
// estimate once it has a turn, and the chip's undo and redo of a batch in
// both memories.
func TestRefreshServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Chat.RefreshMemory(ctx, op.ID, op.Mother, false); err == nil {
		t.Error("a refresh of an empty chat")
	} else {
		uiErr(t, err, KindBusy)
	}
	src := id.SourceOf("refresh", string(op.Mother))
	var edits []chat.MemoryDiff
	err = open(ctx, e.pm, op.ID, func(p *project.Project) error {
		if _, _, err := p.Chats.AppendMessage(ctx, chat.Message{Chat: op.Mother, Turn: 1, Role: chat.RoleUser,
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "weekly reports"}}}}); err != nil {
			return err
		}
		w, err := p.DB.UpdateMemory(ctx, store.MemoryEdit{Section: "goals", Content: "Weekly.", Source: src})
		if err != nil {
			return err
		}
		edits = append(edits, chat.MemoryDiff{Target: w.Section.Target(), After: "Weekly.", Change: w.Change})
		u, err := p.Registry.UpdateUserMemory(ctx, store.MemoryEdit{Section: "preferences", Content: "Short.", Source: src}, p.ID)
		edits = append(edits, chat.MemoryDiff{Target: u.Section.Target(), After: "Short.", Change: u.Change})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	est, err := e.svc.Chat.RefreshEstimate(ctx, op.ID, op.Mother, false)
	if err != nil || est.Turns != 1 || est.Chunks != 1 || est.Calls != 2 || est.Long {
		t.Errorf("estimate %+v, %v", est, err)
	}

	res, err := e.svc.Chat.UndoRefresh(ctx, op.ID, RefreshUndoRequest{Edits: edits})
	if err != nil || !res.Done || !res.Undone || res.Changed {
		t.Fatalf("undo %+v, %v", res, err)
	}
	mem, _ := e.svc.Project.Memory(ctx, op.ID)
	user, _ := e.svc.Settings.UserMemory(ctx)
	if len(mem.Sections) != 0 || len(user.Sections) != 0 {
		t.Errorf("after the undo: %+v, %+v", mem, user)
	}
	if undone, err := e.svc.Chat.RefreshUndone(ctx, op.ID, edits); err != nil || !undone {
		t.Errorf("undone %v, %v", undone, err)
	}
	if res, err = e.svc.Chat.UndoRefresh(ctx, op.ID, RefreshUndoRequest{Edits: edits, Redo: true}); err != nil || !res.Done || res.Undone {
		t.Fatalf("redo %+v, %v", res, err)
	}
	if user, _ = e.svc.Settings.UserMemory(ctx); len(user.Sections) != 1 {
		t.Errorf("after the redo: %+v", user)
	}

	// A section changed since: nothing changes, and Changed says so.
	if _, err := e.svc.Project.SaveMemory(ctx, op.ID, MemoryEdit{Section: "goals", Content: "Monthly.", Revision: 3}); err != nil {
		t.Fatal(err)
	}
	if res, err = e.svc.Chat.UndoRefresh(ctx, op.ID, RefreshUndoRequest{Edits: edits}); err != nil || res.Done || !res.Changed ||
		!slices.Equal(res.Sections, []string{"project memory, section goals"}) {
		t.Errorf("undo after a change %+v, %v", res, err)
	}
	if user, _ = e.svc.Settings.UserMemory(ctx); len(user.Sections) != 1 {
		t.Errorf("user memory changed anyway: %+v", user)
	}
}

// The batch's undo is all or nothing: a user memory section the user
// edited after the refresh keeps project memory as it is too, every time.
func TestUndoRefreshUserChanged(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Refresh")
	if err != nil {
		t.Fatal(err)
	}
	src := id.SourceOf("refresh", string(op.Mother))
	var edits []chat.MemoryDiff
	err = open(ctx, e.pm, op.ID, func(p *project.Project) error {
		w, err := p.DB.UpdateMemory(ctx, store.MemoryEdit{Section: "goals", Content: "Weekly.", Source: src})
		if err != nil {
			return err
		}
		edits = append(edits, chat.MemoryDiff{Target: w.Section.Target(), After: "Weekly.", Change: w.Change})
		u, err := p.Registry.UpdateUserMemory(ctx, store.MemoryEdit{Section: "preferences", Content: "Short.", Source: src}, p.ID)
		edits = append(edits, chat.MemoryDiff{Target: u.Section.Target(), After: "Short.", Change: u.Change})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Settings.SaveUserMemory(ctx, MemoryEdit{Section: "preferences", Content: "Very short.", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		res, err := e.svc.Chat.UndoRefresh(ctx, op.ID, RefreshUndoRequest{Edits: edits})
		if err != nil || res.Done || !res.Changed || res.Undone || !slices.Equal(res.Sections, []string{"user memory, section preferences"}) {
			t.Fatalf("undo %+v, %v", res, err)
		}
		mem, _ := e.svc.Project.Memory(ctx, op.ID)
		if len(mem.Sections) != 1 || mem.Sections[0].Content != "Weekly." {
			t.Fatalf("project memory was undone alone: %+v", mem)
		}
	}
}
