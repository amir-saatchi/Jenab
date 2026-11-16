package kit

import "github.com/egoist/mygo/ui"

// NavRow is a NavItem with a ⋯ menu at its end, as a chat's row in the
// chat list (SPEC 5.12): the button shows while the pointer is on the row
// or the focus is in it, in place of the row's badge, and Tab reaches it
// after the row. key tells the rows apart; label names the row and
// menuLabel the button; face builds what the row shows, told whether the
// menu's button shows, so it can leave its badge out; build fills the
// menu. It reports a click on the row.
func NavRow(c *Context, key, label string, active bool, menuLabel string, face func(menu bool), build func(m *Menu)) (clicked bool) {
	box := ui.Box(c).Key(key).AlignItems(ui.Stretch)
	show := box.Hovered() || box.FocusWithin()
	box.Children(func() {
		clicked = NavItem(c, label, active, func() { face(show) }).Clicked()
		b := IconButton(c, IconMore, menuLabel, Ghost).Size(28, 28).Absolute().End(6).Top(10)
		if !show {
			b.Opacity(0)
		}
		b.PopoverMenuButton(build)
	})
	return clicked
}

// MenuTitle is a panel's title that opens a menu, as the project's name
// at the top of the chat list (SPEC 5.12): title over sub (in the code
// font, left to right, as a folder) with a chevron at its end, in the
// sidebar's colors. label names it; build fills the menu.
func MenuTitle(c *Context, title, sub, label string, build func(m *Menu)) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Label(label).MinHeight(48).Padding(6, 8).Gap(8).Radius(Radius * 0.8).AlignItems(ui.Center).
		TextColor(k.SidebarForeground)
	if b.Hovered() {
		b.Background(k.SidebarAccent)
	}
	b.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(500).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(12).SingleLine().TextColor(k.MutedForeground).Direction(ui.LeftToRight)
			}
		})
		ui.Icon(c, IconChevronDown).Size(16, 16).Shrink(0)
	})
	return b.PopoverMenuButton(build)
}

// TileSpinner is a small spinner on the bottom end corner of a rail tile,
// as a chat that works.
func TileSpinner(c *Context, label string) Element {
	k := TokensOf(c)
	box := ui.Box(c).Absolute().Bottom(0).End(8).Size(16, 16).Radius(8).Background(k.Sidebar).
		AlignItems(ui.Center).Justify(ui.Center)
	return box.Children(func() {
		ui.Spinner(c).Size(12, 12).Label(label).TextColor(k.MutedForeground)
	})
}
