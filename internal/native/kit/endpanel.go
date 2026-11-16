package kit

import "github.com/egoist/mygo/ui"

// EndPanel is the right sidebar (SPEC 5.12), 288 px wide on the window's
// end edge in the sidebar's colors, a rule on its start edge: label names
// it, a landmark for assistive technology, and fn builds it top to bottom.
func EndPanel(c *Context, label string, fn func()) Element {
	k := TokensOf(c)
	box := ui.Themed(c, railTheme(c)).Width(288).Shrink(0).Background(k.Sidebar).BorderColor(k.SidebarBorder).
		BorderStartWidth(1)
	return box.Children(func() {
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Role(ui.RoleComplementary).Label(label).Children(fn)
	})
}
