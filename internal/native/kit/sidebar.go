package kit

import "github.com/egoist/mygo/ui"

// Sidebar is the list of chats and projects down the window's side: items
// in sections, choosing the ID of one into *selected, in the sidebar's
// colors. Give it a width. Up, Down, Home and End choose while it has the
// focus, and Tab leaves it.
//
// MyGo's sidebar shows the item chosen in the accent color while it has
// the focus, as AppKit's, and in SurfacePressed without it: the kit makes
// that the sidebar-accent gray of shadcn's, so only the focused look
// differs from the web app's.
func Sidebar(c *Context, selected *string, fn func()) Element {
	k := TokensOf(c)
	t := *c.Theme()
	t.Background, t.Surface = k.Sidebar, k.Sidebar
	t.SurfaceHover, t.SurfacePressed = k.SidebarAccent, k.SidebarAccent
	t.Text = k.SidebarForeground
	t.Accent, t.AccentText = k.SidebarPrimary, k.SidebarPrimaryFg
	t.Focus = k.SidebarRing.Alpha(0.5)
	box := ui.Themed(c, &t).Background(k.Sidebar).BorderColor(k.SidebarBorder).BorderEndWidth(1)
	return box.Children(func() { ui.Sidebar(c, selected, fn).Grow(1).Padding(8) })
}

// SidebarSection holds the items fn builds under title while *open; a
// click on the title shows and hides them. A nil open keeps it open.
func SidebarSection(c *Context, title string, open *bool, fn func()) Element {
	return ui.SidebarSection(c, title, open, fn)
}

// SidebarItem is an item of the sidebar choosing id, showing label after
// icon (nil for none); add a badge to it with Children.
func SidebarItem(c *Context, id string, icon *SVG, label string) Element {
	return ui.SidebarItem(c, id, icon, label).FontSize(14)
}
