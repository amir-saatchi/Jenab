package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Tester drives a view with no window, for the screens' tests: it
// clicks, types and reads what is shown and what a screen reader gets.
type Tester = ui.Tester

// The keys and modifiers the screens' tests press.
type (
	Key       = ui.Key
	Modifiers = ui.Modifiers
)

const (
	KeyEnter  = ui.KeyEnter
	KeyEscape = ui.KeyEscape
	KeyTab    = ui.KeyTab
	KeyHome   = ui.KeyHome
	KeyC      = ui.KeyC
	Ctrl      = ui.Ctrl
	Shift     = ui.Shift
)

// NewTester builds view in Root, in a window of w by h, light or dark.
func NewTester(view func(c *Context), w, h int, dark bool) (*Tester, error) {
	if err := RegisterFonts(); err != nil {
		return nil, err
	}
	tt := ui.NewTester(view, w, h)
	tt.SetDark(dark)
	tt.Frame()
	return tt, nil
}

// Shows reports whether the window shows s: a text containing it, an
// element named it, or a text field whose text or placeholder contains it.
// MyGo's Tester.Texts, Find and HasText have only the texts drawn as
// labels, not what a text field holds (still so in v0.9.1); a screen
// reader gets that as the field's Value and Placeholder, which is where
// Shows reads it.
func Shows(tt *Tester, s string) bool {
	if _, ok := tt.Find(s); ok || tt.HasText(s) {
		return true
	}
	for _, a := range tt.AccessAll() {
		if strings.Contains(a.Value, s) || strings.Contains(a.Placeholder, s) {
			return true
		}
	}
	return false
}

// HasRole reports whether a screen reader gets an element named name in
// one of the roles the kit's frame gives: "navigation", "main", "article",
// "status", "heading", "list", and "complementary", "contentinfo", "tree",
// "group", "image", and "dialog".
func HasRole(tt *Tester, role, name string) bool {
	r, ok := roles[role]
	if !ok {
		return false
	}
	_, found := tt.AccessByRole(r, name)
	return found
}

var roles = map[string]ui.Role{
	"navigation": ui.RoleNavigation,
	"main":       ui.RoleMain,
	"article":    ui.RoleArticle,
	"status":     ui.RoleStatus,
	"heading":    ui.RoleHeading,
	"list":       ui.RoleList,
	// TASK-056: the right sidebar, the bottom bar, the files and pictures.
	"complementary": ui.RoleComplementary,
	"contentinfo":   ui.RoleContentInfo,
	"tree":          ui.RoleTree,
	"group":         ui.RoleGroup,
	"image":         ui.RoleImage,
	// TASK-062: the narrow window's overlays and drawer.
	"dialog": ui.RoleDialog,
}
