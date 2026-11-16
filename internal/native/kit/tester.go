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
// "group", "image", "dialog" and "link".
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
	// TASK-064: the links in a text.
	"link": ui.RoleLink,
}

// WindowsButtons gives the tester's window the room of Windows 11's
// minimize, maximize and close buttons, 3 × 46 by 32 at its top end
// corner, as the app's window has with no system title bar (TASK-063).
func WindowsButtons(tt *Tester) { tt.SetTitleBar(windowsButtonsRoom) }

var windowsButtonsRoom = ui.TitleBar{Height: 32, Right: 138, Bottom: 32}

// Pressed reports whether the toggle button named name is pressed, and
// whether the window has one.
func Pressed(tt *Tester, name string) (on, ok bool) {
	a, ok := tt.AccessByRole(ui.RoleToggleButton, name)
	return a.Checked, ok
}

// OffTheWindowButtonsMiddle names the controls in the window's title bar,
// under the buttons WindowsButtons gives, whose middle is not the
// buttons' middle, as every control of the bar's should be. A control
// running past the bar, as the dock's divider, isn't in it.
func OffTheWindowButtonsMiddle(tt *Tester) []string {
	bar := windowsButtonsRoom
	var off []string
	for _, a := range tt.AccessAll() {
		r := a.Bounds
		if !a.Focusable || r.W <= 0 || r.H <= 0 || r.Y >= bar.Height || r.Y+r.H > bar.Height+1 {
			continue
		}
		if mid := r.Y + r.H/2; mid < bar.Height/2-0.5 || mid > bar.Height/2+0.5 {
			off = append(off, a.Name)
		}
	}
	return off
}

// AccessInfo is what assistive technology gets of an element (Tester.AccessAll).
type AccessInfo = ui.AccessInfo

// Description is what assistive technology reads after the name of the
// element named name, as its tooltip, and whether the window has one.
func Description(tt *Tester, name string) (string, bool) {
	for _, a := range tt.AccessAll() {
		if a.Name == name {
			return a.Description, true
		}
	}
	return "", false
}

// UnderWindowButtons names the controls that lie under the buttons
// WindowsButtons gives a window width wide, which none should.
func UnderWindowButtons(tt *Tester, width int) []string {
	w, bar := float32(width), windowsButtonsRoom
	var under []string
	for _, a := range tt.AccessAll() {
		r := a.Bounds
		if a.Focusable && r.W > 0 && r.Y < bar.Height && r.X+r.W > w-bar.Right {
			under = append(under, a.Name)
		}
	}
	return under
}

// Links returns the names of the links a screen reader gets, in order
// (TASK-064).
func Links(tt *Tester) []string {
	var out []string
	for _, n := range tt.AccessAll() {
		if n.Role == ui.RoleLink {
			out = append(out, n.Name)
		}
	}
	return out
}
