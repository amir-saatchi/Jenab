package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/desktop"
	"github.com/amir-saatchi/jenab/internal/store"
)

// DesktopUndoRequest names a desktop save's notice (SPEC 8.11): Change
// is the notice's Change.
type DesktopUndoRequest struct {
	Change string `json:"change"`
	Redo   bool   `json:"redo"`
}

func (r DesktopUndoRequest) change() (int64, error) {
	n, err := strconv.ParseInt(r.Change, 10, 64)
	if err != nil || n <= 0 {
		return 0, invalid(fmt.Sprintf("%q is not a desktop save.", r.Change))
	}
	return n, nil
}

// DesktopSaveUndone reports whether a desktop save is undone now, so its
// chip offers Redo after a reload.
func (s *ChatService) DesktopSaveUndone(ctx context.Context, r DesktopUndoRequest) (undone bool, err error) {
	defer s.guard("chat.desktop_save_undone", &err)
	n, err := r.change()
	if err != nil {
		return false, err
	}
	return s.registry.DesktopSaveUndone(ctx, n)
}

// UndoDesktopSave reverts an agent's macro or recipe save from its chip,
// or puts it back. One changed since is left as it is: Changed says so.
func (s *ChatService) UndoDesktopSave(ctx context.Context, r DesktopUndoRequest) (u store.DesktopUndo, err error) {
	defer s.guard("chat.undo_desktop_save", &err)
	n, err := r.change()
	if err != nil {
		return u, err
	}
	return s.registry.UndoDesktopSave(ctx, n, r.Redo)
}

// DesktopMacroItem is a macro in Settings › Desktop: its steps as a list
// to read and as JSON to edit, and its last runs.
type DesktopMacroItem struct {
	Macro   store.DesktopMacro `json:"macro"`
	Listing string             `json:"listing"`
	Steps   string             `json:"steps"`
	Runs    []store.DesktopRun `json:"runs"`
}

// DesktopAppItem is one app in Settings › Desktop.
type DesktopAppItem struct {
	App     string                `json:"app"`
	Macros  []DesktopMacroItem    `json:"macros"`
	Recipes []store.DesktopRecipe `json:"recipes"`
}

// DesktopList is Settings › Desktop: each app's macros and recipes, and
// what was removed in the last 30 days.
type DesktopList struct {
	Apps           []DesktopAppItem      `json:"apps"`
	RemovedMacros  []store.DesktopMacro  `json:"removed_macros"`
	RemovedRecipes []store.DesktopRecipe `json:"removed_recipes"`
}

// Desktop lists the saved macros and recipes, after the removal rules.
func (s *SettingsService) Desktop(ctx context.Context) (l DesktopList, err error) {
	defer s.guard("settings.desktop", &err)
	now := s.now()
	if err := s.registry.PruneDesktop(ctx, now); err != nil {
		return l, err
	}
	ms, err := s.registry.DesktopMacros(ctx, "")
	if err != nil {
		return l, err
	}
	rs, err := s.registry.DesktopRecipes(ctx)
	if err != nil {
		return l, err
	}
	byApp := map[string]*DesktopAppItem{}
	item := func(app string) *DesktopAppItem {
		if byApp[app] == nil {
			byApp[app] = &DesktopAppItem{App: app, Macros: []DesktopMacroItem{}, Recipes: []store.DesktopRecipe{}}
		}
		return byApp[app]
	}
	for _, m := range ms {
		runs, err := s.registry.DesktopRuns(ctx, m.ID)
		if err != nil {
			return l, err
		}
		var pretty bytes.Buffer
		if json.Indent(&pretty, m.Steps, "", "  ") != nil {
			pretty.Reset()
			pretty.Write(m.Steps)
		}
		a := item(m.App)
		a.Macros = append(a.Macros, DesktopMacroItem{Macro: m, Listing: desktop.Listing(m.Steps), Steps: pretty.String(), Runs: runs})
	}
	for _, r := range rs {
		a := item(r.App)
		a.Recipes = append(a.Recipes, r)
	}
	l.Apps = []DesktopAppItem{}
	for _, a := range byApp {
		l.Apps = append(l.Apps, *a)
	}
	slices.SortFunc(l.Apps, func(a, b DesktopAppItem) int { return strings.Compare(a.App, b.App) })
	l.RemovedMacros, l.RemovedRecipes, err = s.registry.DesktopRemoved(ctx, now.Add(-store.KeepRemoved))
	return l, err
}

// DesktopMacroEdit is the user's edit of a macro: Steps is the JSON list
// Settings shows.
type DesktopMacroEdit struct {
	ID          int64    `json:"id"`
	Description string   `json:"description"`
	Params      []string `json:"params"`
	Steps       string   `json:"steps"`
}

// EditDesktopMacro saves the user's edit of a macro as a new version,
// after the checks of a save (SPEC 8.11).
func (s *SettingsService) EditDesktopMacro(ctx context.Context, e DesktopMacroEdit) (m store.DesktopMacro, err error) {
	defer s.guard("settings.edit_desktop_macro", &err)
	e.Description = strings.TrimSpace(e.Description)
	if e.Description == "" || utf8.RuneCountInString(e.Description) > 300 {
		return m, invalid("A description has 1 to 300 characters.")
	}
	params := []string{}
	for _, p := range e.Params {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(params, p) {
			params = append(params, p)
		}
	}
	slices.Sort(params)
	steps, err := desktop.ParseSteps([]byte(e.Steps), params)
	if err != nil {
		return m, invalid("Not saved: " + err.Error() + ".")
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return m, err
	}
	return s.registry.EditDesktopMacro(ctx, e.ID, e.Description, params, raw)
}

// DeleteDesktopMacro removes a macro; Settings lists it for 30 days.
func (s *SettingsService) DeleteDesktopMacro(ctx context.Context, mid int64) (err error) {
	defer s.guard("settings.delete_desktop_macro", &err)
	return s.registry.RemoveDesktopMacro(ctx, mid, "deleted by the user", s.now())
}

// DesktopRecipeWrite is the user's recipe: ID 0 for a new one, with its
// app, site and name; else an edit of ID's text.
type DesktopRecipeWrite struct {
	ID   int64  `json:"id"`
	App  string `json:"app"`
	Site string `json:"site"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// WriteDesktopRecipe saves the user's recipe or edit. The user's recipes
// are never removed by the rules (SPEC 8.11).
func (s *SettingsService) WriteDesktopRecipe(ctx context.Context, w DesktopRecipeWrite) (d store.DesktopRecipe, err error) {
	defer s.guard("settings.write_desktop_recipe", &err)
	w.Text = strings.TrimSpace(w.Text)
	if w.Text == "" || utf8.RuneCountInString(w.Text) > 1200 {
		return d, invalid("A recipe has 1 to 1,200 characters.")
	}
	d = store.DesktopRecipe{Text: w.Text}
	if w.ID == 0 {
		d.App = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(w.App)), ".exe")
		d.Site, d.Name = strings.TrimSpace(w.Site), strings.TrimSpace(w.Name)
		if d.App == "" || d.Name == "" {
			return d, invalid("A new recipe needs its app and a name.")
		}
	}
	return s.registry.WriteDesktopRecipe(ctx, w.ID, d, s.now())
}

// invalid is an input the user can fix, with its message.
func invalid(msg string) error { return &UIError{Kind: KindInvalid, Message: msg} }

// DeleteDesktopRecipe removes a recipe; Settings lists it for 30 days.
func (s *SettingsService) DeleteDesktopRecipe(ctx context.Context, rid int64) (err error) {
	defer s.guard("settings.delete_desktop_recipe", &err)
	return s.registry.RemoveDesktopRecipe(ctx, rid, "deleted by the user", s.now())
}
