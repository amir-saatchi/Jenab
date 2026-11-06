package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

var deskNow = time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

func macro(name, steps string, params ...string) DesktopMacro {
	if params == nil {
		params = []string{}
	}
	return DesktopMacro{App: "calculatorapp", Name: name, Description: "Adds.", Params: params, Steps: json.RawMessage(steps), MadeByModel: "m/x", Untrusted: true}
}

func TestDesktopMacroVersionsAndUndo(t *testing.T) {
	r := openTestRegistry(t)
	ctx := context.Background()
	s1, err := r.SaveDesktopMacro(ctx, macro("add", `[{"do":"click","role":"button","name":"Plus"}]`), deskNow)
	if err != nil || s1.Macro.Version != 1 {
		t.Fatalf("save: %+v %v", s1, err)
	}
	if _, err := r.RecordDesktopRun(ctx, DesktopRun{MacroID: s1.Macro.ID, Version: 1, At: deskNow.Add(time.Hour), OK: true}); err != nil {
		t.Fatal(err)
	}
	s2, err := r.SaveDesktopMacro(ctx, macro("add", `[{"do":"click","role":"button","name":["Addieren","Plus"]}]`), deskNow.Add(2*time.Hour))
	if err != nil || s2.Macro.ID != s1.Macro.ID || s2.Macro.Version != 2 || s2.Macro.Runs != 1 {
		t.Fatalf("second save: %+v %v", s2.Macro, err)
	}
	// Undo the second: the first version is back, with its run.
	u, err := r.UndoDesktopSave(ctx, s2.Change, false)
	if err != nil || !u.Done || !u.Undone {
		t.Fatalf("undo: %+v %v", u, err)
	}
	m, _ := r.DesktopMacro(ctx, s1.Macro.ID)
	if m.Version != 1 || string(m.Steps) != `[{"do":"click","role":"button","name":"Plus"}]` || m.Runs != 1 {
		t.Fatalf("after undo: %+v", m)
	}
	if undone, _ := r.DesktopSaveUndone(ctx, s2.Change); !undone {
		t.Fatal("the chip must read undone")
	}
	// Undo again is no change; redo puts version 2 back.
	if u, _ := r.UndoDesktopSave(ctx, s2.Change, false); u.Done || !u.Undone {
		t.Fatalf("undo twice: %+v", u)
	}
	if u, err := r.UndoDesktopSave(ctx, s2.Change, true); err != nil || !u.Done || u.Undone {
		t.Fatalf("redo: %+v %v", u, err)
	}
	// Changed since (the user's edit): the undo leaves it.
	if _, err := r.EditDesktopMacro(ctx, m.ID, "Adds two.", []string{}, json.RawMessage(`[{"do":"click","role":"button","name":"Equals"}]`)); err != nil {
		t.Fatal(err)
	}
	if u, err := r.UndoDesktopSave(ctx, s2.Change, false); err != nil || u.Done || !u.Changed {
		t.Fatalf("undo after an edit: %+v %v", u, err)
	}
	// Undo of a new macro deletes it.
	s3, _ := r.SaveDesktopMacro(ctx, macro("sub", `[{"do":"click","role":"button","name":"Minus"}]`), deskNow)
	if _, err := r.UndoDesktopSave(ctx, s3.Change, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DesktopMacro(ctx, s3.Macro.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("undone new macro: %v", err)
	}
}

func TestDesktopRunsAndRemoval(t *testing.T) {
	r := openTestRegistry(t)
	ctx := context.Background()
	s, _ := r.SaveDesktopMacro(ctx, macro("add", `[]`), deskNow)
	for i := range 25 {
		if _, err := r.RecordDesktopRun(ctx, DesktopRun{MacroID: s.Macro.ID, Version: 1, At: deskNow.Add(time.Duration(i) * time.Minute), OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ := r.DesktopRuns(ctx, s.Macro.ID)
	if len(runs) != keepRuns || !runs[0].At.Equal(deskNow.Add(24*time.Minute)) {
		t.Fatalf("%d runs, newest %v", len(runs), runs[0].At)
	}
	for i := range MacroFailLimit {
		removed, err := r.RecordDesktopRun(ctx, DesktopRun{MacroID: s.Macro.ID, Version: 1, At: deskNow.Add(time.Hour), FailedAt: 2, Reason: "no button"})
		if err != nil || removed != (i == MacroFailLimit-1) {
			t.Fatalf("failure %d: removed %v %v", i+1, removed, err)
		}
	}
	if ms, _ := r.DesktopMacros(ctx, ""); len(ms) != 0 {
		t.Fatalf("listed after 3 failures: %+v", ms)
	}
	gone, _, _ := r.DesktopRemoved(ctx, deskNow)
	if len(gone) != 1 || gone[0].RemovedWhy != "failed 3 replays in a row" {
		t.Fatalf("removed: %+v", gone)
	}
	// Saved again: a new macro, without the old runs.
	s2, _ := r.SaveDesktopMacro(ctx, macro("add", `[]`), deskNow.Add(2*time.Hour))
	if s2.Macro.ID != s.Macro.ID || s2.Macro.Version != 1 || s2.Macro.Runs != 0 {
		t.Fatalf("saved over a removed one: %+v", s2.Macro)
	}
	if runs, _ := r.DesktopRuns(ctx, s.Macro.ID); len(runs) != 0 {
		t.Fatalf("old runs kept: %d", len(runs))
	}
}

func TestDesktopPrune(t *testing.T) {
	r := openTestRegistry(t)
	ctx := context.Background()
	old, used := deskNow, deskNow.Add(80*24*time.Hour)
	sOld, _ := r.SaveDesktopMacro(ctx, macro("old", `[]`), old)
	sUsed, _ := r.SaveDesktopMacro(ctx, macro("used", `[]`), old)
	r.RecordDesktopRun(ctx, DesktopRun{MacroID: sUsed.Macro.ID, Version: 1, At: used, OK: true})
	agent, _ := r.SaveDesktopRecipe(ctx, DesktopRecipe{App: "calculatorapp", Name: "agents", Text: "t"}, old)
	mine, _ := r.WriteDesktopRecipe(ctx, 0, DesktopRecipe{App: "calculatorapp", Name: "mine", Text: "t"}, old)
	if err := r.ShipDesktopRecipes(ctx, []DesktopRecipe{{App: "calculatorapp", Name: "calculator", Text: "shipped"}}, "1", old); err != nil {
		t.Fatal(err)
	}

	// 91 days: the unused macro goes, the recipes stay.
	day91 := old.Add(91 * 24 * time.Hour)
	if err := r.PruneDesktop(ctx, day91); err != nil {
		t.Fatal(err)
	}
	ms, _ := r.DesktopMacros(ctx, "calculatorapp")
	if len(ms) != 1 || ms[0].ID != sUsed.Macro.ID {
		t.Fatalf("after 91 days: %+v", ms)
	}
	if rs, _ := r.DesktopRecipes(ctx, "calculatorapp"); len(rs) != 3 {
		t.Fatalf("recipes after 91 days: %d", len(rs))
	}
	// 30 days after its removal the row and its Undo rows are gone.
	if err := r.PruneDesktop(ctx, day91.Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DesktopMacro(ctx, sOld.Macro.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed macro kept: %v", err)
	}
	if _, err := r.DesktopSaveUndone(ctx, sOld.Change); !errors.Is(err, ErrNotFound) {
		t.Fatalf("undo row kept: %v", err)
	}
	// A year: the agent's recipe goes; the user's and the shipped stay.
	if err := r.PruneDesktop(ctx, old.Add(366*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rs, _ := r.DesktopRecipes(ctx, "calculatorapp")
	var names []string
	for _, d := range rs {
		names = append(names, d.Name+"/"+d.Source)
		if d.ID == agent.Recipe.ID {
			t.Fatal("the agent's recipe outlived a year unused")
		}
	}
	if len(rs) != 2 || rs[0].ID != mine.ID && rs[1].ID != mine.ID {
		t.Fatalf("after a year: %v", names)
	}
}

func TestDesktopRecipeSources(t *testing.T) {
	r := openTestRegistry(t)
	ctx := context.Background()
	ship := []DesktopRecipe{{App: "calculatorapp", Name: "calculator", Text: "v1"}}
	r.ShipDesktopRecipes(ctx, ship, "1", deskNow)
	r.ShipDesktopRecipes(ctx, ship, "1", deskNow) // the same text: no new row
	rs, _ := r.DesktopRecipes(ctx)
	if len(rs) != 1 || rs[0].Source != RecipeApp {
		t.Fatalf("shipped: %+v", rs)
	}
	// The user's edit of a shipped recipe is a copy of their own.
	mine, err := r.WriteDesktopRecipe(ctx, rs[0].ID, DesktopRecipe{Text: "my way"}, deskNow)
	if err != nil || mine.ID == rs[0].ID || mine.Source != RecipeUser || mine.Name != "calculator" {
		t.Fatalf("edit: %+v %v", mine, err)
	}
	// An agent's recipe of the same name: saved apart, Undo removes it.
	s, _ := r.SaveDesktopRecipe(ctx, DesktopRecipe{App: "calculatorapp", Name: "calculator", Text: "agent's", MadeByModel: "m/x"}, deskNow)
	if rs, _ := r.DesktopRecipes(ctx, "calculatorapp"); len(rs) != 3 {
		t.Fatalf("%d recipes, want shipped, the user's and the agent's", len(rs))
	}
	if u, err := r.UndoDesktopSave(ctx, s.Change, false); err != nil || !u.Done {
		t.Fatalf("undo: %+v %v", u, err)
	}
	// A new shipped text replaces the old, keeping its uses.
	r.UseDesktopRecipes(ctx, []int64{rs[0].ID}, deskNow)
	r.ShipDesktopRecipes(ctx, []DesktopRecipe{{App: "calculatorapp", Name: "calculator", Text: "v2"}}, "2", deskNow)
	rs, _ = r.DesktopRecipes(ctx, "calculatorapp")
	for _, d := range rs {
		if d.Source == RecipeApp && (d.Text != "v2" || d.Uses != 1 || d.AppVersion != "2") {
			t.Fatalf("shipped v2: %+v", d)
		}
	}
}
