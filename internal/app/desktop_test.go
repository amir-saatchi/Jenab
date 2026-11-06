package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/store"
)

// Settings › Desktop and the chip's Undo (SPEC 8.11).
func TestDesktopServices(t *testing.T) {
	e := newEnv(t, t.TempDir())
	ctx := context.Background()
	now := time.Now()
	s, err := e.reg.SaveDesktopMacro(ctx, store.DesktopMacro{App: "calculatorapp", Name: "add", Description: "Adds.", Params: []string{"a"},
		Steps: json.RawMessage(`[{"do":"click_each","role":"button","text":"{a}"},{"do":"click","role":"button","name":"Equals"}]`)}, now)
	if err != nil {
		t.Fatal(err)
	}
	e.reg.RecordDesktopRun(ctx, store.DesktopRun{MacroID: s.Macro.ID, Version: 1, At: now, FailedAt: 2, Reason: "no button"})
	if _, err := e.reg.SaveDesktopRecipe(ctx, store.DesktopRecipe{App: "calculatorapp", Name: "adding", Text: "Click."}, now); err != nil {
		t.Fatal(err)
	}

	l, err := e.svc.Settings.Desktop(ctx)
	if err != nil || len(l.Apps) != 1 || len(l.Apps[0].Macros) != 1 || len(l.Apps[0].Recipes) != 1 {
		t.Fatalf("list: %+v %v", l, err)
	}
	mi := l.Apps[0].Macros[0]
	if !strings.Contains(mi.Listing, `1 click_each button text "{a}"`) || len(mi.Runs) != 1 || mi.Runs[0].OK {
		t.Fatalf("macro item: %+v", mi)
	}

	// The user's edit is checked as a save is.
	if _, err := e.svc.Settings.EditDesktopMacro(ctx, DesktopMacroEdit{ID: s.Macro.ID, Description: "Adds.", Params: []string{"a", "b"}, Steps: mi.Steps}); err == nil ||
		!strings.Contains(err.Error(), `"b" is in no step`) {
		t.Fatalf("edit with an unused param: %v", err)
	}
	m, err := e.svc.Settings.EditDesktopMacro(ctx, DesktopMacroEdit{ID: s.Macro.ID, Description: "Adds a number.", Params: []string{"a"}, Steps: mi.Steps})
	if err != nil || m.Version != 2 || m.Description != "Adds a number." {
		t.Fatalf("edit: %+v %v", m, err)
	}
	// The chip's Undo finds the macro changed and leaves it.
	u, err := e.svc.Chat.UndoDesktopSave(ctx, DesktopUndoRequest{Change: fmt.Sprint(s.Change)})
	if err != nil || u.Done || !u.Changed {
		t.Fatalf("undo after an edit: %+v %v", u, err)
	}
	if _, err := e.svc.Chat.UndoDesktopSave(ctx, DesktopUndoRequest{Change: "x"}); err == nil {
		t.Fatal("a bad change must fail")
	}

	// Deleted: listed as removed.
	if err := e.svc.Settings.DeleteDesktopMacro(ctx, s.Macro.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Settings.DeleteDesktopRecipe(ctx, l.Apps[0].Recipes[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Settings.WriteDesktopRecipe(ctx, DesktopRecipeWrite{App: "Chrome.exe", Name: "search", Text: "Type, then Return."}); err != nil {
		t.Fatal(err)
	}
	l, _ = e.svc.Settings.Desktop(ctx)
	if len(l.RemovedMacros) != 1 || len(l.RemovedRecipes) != 1 || l.RemovedMacros[0].RemovedWhy != "deleted by the user" {
		t.Fatalf("removed: %+v", l)
	}
	if len(l.Apps) != 1 || l.Apps[0].App != "chrome" || l.Apps[0].Recipes[0].Source != store.RecipeUser {
		t.Fatalf("apps: %+v", l.Apps)
	}
}
