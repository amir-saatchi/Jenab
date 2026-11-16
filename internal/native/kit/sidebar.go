package kit

import "github.com/egoist/mygo/ui"

// Sidebar is the list of chats and projects down the window's side: items
// in sections, choosing the ID of one into *selected, in the sidebar's
// colors. Give it a width. Up, Down, Home and End choose while it has the
// focus, and Tab leaves it.
//
// The item chosen is in shadcn's sidebar-accent gray, with or without the
// focus, as in the web app. MyGo's sidebar fills it with Theme.Accent while
// it has the focus, as AppKit's, and has no setting for it (still so in
// v0.9.1): SidebarItem paints it again, and AccentText, which MyGo gives
// its icon, is the sidebar-accent foreground.
func Sidebar(c *Context, selected *string, fn func()) Element {
	k := TokensOf(c)
	t := *c.Theme()
	t.Background, t.Surface = k.Sidebar, k.Sidebar
	t.SurfaceHover, t.SurfacePressed = k.SidebarAccent, k.SidebarAccent
	t.Text = k.SidebarForeground
	t.Accent, t.AccentText = k.SidebarPrimary, k.SidebarAccentFg
	t.Focus = k.SidebarRing.Alpha(0.5)
	box := ui.Themed(c, &t).Background(k.Sidebar).BorderColor(k.SidebarBorder).BorderEndWidth(1)
	return box.Children(func() {
		sidebars = append(sidebars, selected)
		defer func() { sidebars = sidebars[:len(sidebars)-1] }()
		ui.Sidebar(c, selected, fn).Grow(1).Padding(8)
	})
}

// sidebars are the choices of the Sidebars being built, the innermost
// last, for SidebarItem to find the item chosen. Frames are built on the
// UI thread.
var sidebars []*string

// SidebarSection holds the items fn builds under title while *open; a
// click on the title shows and hides them. A nil open keeps it open.
func SidebarSection(c *Context, title string, open *bool, fn func()) Element {
	return ui.SidebarSection(c, title, open, fn)
}

// SidebarItem is an item of the sidebar choosing id, showing label after
// icon (nil for none); add a badge to it with Children.
func SidebarItem(c *Context, id string, icon *SVG, label string) Element {
	e := ui.SidebarItem(c, id, icon, label).FontSize(14)
	if n := len(sidebars); n > 0 && sidebars[n-1] != nil && *sidebars[n-1] == id {
		k := TokensOf(c)
		e.Background(k.SidebarAccent).TextColor(k.SidebarAccentFg)
	}
	return e
}
