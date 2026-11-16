package kit

import "github.com/egoist/mygo/ui"

// More for the screens' tests (TASK-057): the key that deletes back, and
// whether a page's part is in view.

const KeyBackspace = ui.KeyBackspace

// InView reports whether a screen reader gets an element named name in
// role, as HasRole names the roles, or "group" for a page's Section, and
// finds it in the window's view rather than scrolled away.
func InView(tt *Tester, role, name string) bool {
	r, ok := roles[role]
	if role == "group" {
		r, ok = ui.RoleGroup, true
	}
	if !ok {
		return false
	}
	a, found := tt.AccessByRole(r, name)
	return found && !a.Offscreen
}
