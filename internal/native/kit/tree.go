package kit

import "github.com/egoist/mygo/ui"

// Tree is a tree of rows, as the files panel's folders and files (SPEC
// 5.12), in the sidebar's colors: label names it, and fn builds its rows
// with TreeRows.Item. It is one stop of Tab, which goes back to the row
// that had the focus; while a row has it, Up and Down move to the rows
// around, Home and End to the first and the last, Right opens a branch or
// moves into it, Left closes it or moves to the row it is in (the other
// way round right to left), and typing a row's first letters moves to it.
// Enter and Space click the row that has the focus, as the pointer does;
// Shift+F10 opens its menu.
func Tree(c *Context, label string, fn func(rows TreeRows)) Element {
	p := ui.TreeBase(c)
	return p.Root.Label(label).Gap(2).AlignItems(ui.Stretch).Children(func() { fn(TreeRows{c: c, p: p}) })
}

// TreeRows builds a Tree's rows.
type TreeRows struct {
	c *Context
	p ui.TreeParts
}

// TreeRow is a row of a Tree: Clicked reports a click on it, Enter or
// Space, and Group builds the rows inside it while it is open.
type TreeRow struct {
	Element
	item ui.TreeItemParts
}

// Item adds a row to the tree, or, inside a row's Group, to that row. key
// keeps the focus on the row as rows above it come and go, as a file's
// key; label names it, and is what typing its first letters matches; a
// nil open makes a leaf, otherwise a branch whose rows show while *open.
// face builds what the row shows after its arrow, such as an icon and
// Lines. The row is filled while active, the file open, as NavItem; menu,
// if not nil, fills the menu that a right click, Shift+F10 or the "…" that
// shows on the hovered row opens.
func (t TreeRows) Item(key any, label string, open *bool, active bool, face func(), menu func(m *Menu)) TreeRow {
	c := t.c
	k := TokensOf(c)
	c.Key(key)
	it := t.p.Item(open)
	row := it.Row.Label(label).MinHeight(32).Padding(4, 8).PaddingStart(8 + 16*float32(it.Depth())).Gap(8).
		Radius(Radius * 0.8).AlignItems(ui.Center).TextColor(k.SidebarForeground).AccessSelected(active)
	hovered := row.Hovered()
	switch {
	case active:
		row.Background(k.SidebarAccent).TextColor(k.SidebarAccentFg)
	case hovered:
		row.Background(k.SidebarAccent)
	}
	if menu != nil {
		row.ContextMenu(menu)
	}
	row.Children(func() {
		if open != nil {
			chevron := IconChevronRight
			if *open {
				chevron = IconChevronDown
			}
			ui.Icon(c, chevron).Size(14, 14).Shrink(0).TextColor(k.MutedForeground)
		}
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(face)
		if menu != nil {
			// The pointer's way to the menu; the keyboard's is Shift+F10, so
			// Tab passes over it and the tree stays one stop.
			more := IconButton(c, IconMore, "More", Ghost).Size(24, 24).TabStop(false).PopoverMenuButton(menu)
			if !hovered && !active {
				more.Opacity(0)
			}
		}
	})
	return TreeRow{Element: row, item: it}
}

// Group builds the rows inside a branch, with the tree's Item, while it
// is open.
func (r TreeRow) Group(fn func()) { r.item.Group(fn) }
