package kit

import "github.com/egoist/mygo/ui"

// ItemRow is a row of a list on a page, as shadcn's Item variant="outline"
// size="xs": a border, rounded corners, what fn builds in a row, with its
// actions last. Assistive technology finds it as a group named label.
func ItemRow(c *Context, label string, fn func()) Element {
	k := TokensOf(c)
	row := ui.Row(c).Gap(8).MinHeight(36).Padding(4, 4, 4, 12).Radius(Radius*0.8).Border(1, k.Border).
		AlignItems(ui.Center).FontSize(14).Role(ui.RoleGroup).Label(label)
	return row.Children(fn)
}
