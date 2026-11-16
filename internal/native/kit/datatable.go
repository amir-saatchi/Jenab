package kit

import "github.com/egoist/mygo/ui"

// DataColumn is a column of a DataTable: its title, and its width in
// pixels, or for 0 a share of the room left (Grow, 1 if 0). End aligns
// its cells to the end, as numbers are.
type DataColumn struct {
	Title string
	Width float32
	Grow  float32
	End   bool
}

// DataTable is a small table, as shadcn's Table: a header row of the
// columns' titles over the rows body builds with DataRow, DataSpan and
// DataFoot. It builds every row, so it is for tens of rows; a long list
// takes List. label names it for assistive technology. Numbers in it take
// digits of one width, as tabular-nums.
func DataTable(c *Context, label string, cols []DataColumn, body func()) Element {
	k := TokensOf(c)
	t := ui.Column(c).AlignItems(ui.Stretch).FontSize(14).FontFeatures("tnum").Role(ui.RoleTable).Label(label)
	return t.Children(func() {
		head := dataRow(c, cols, func(i int) {
			ui.Text(c, cols[i].Title).FontWeight(500).SingleLine().Shrink(1)
		}, ui.RoleColumnHeader)
		head.MinHeight(40).BorderColor(k.Border).BorderWidth(0, 0, 1, 0)
		body()
	})
}

// DataRow is a row of a DataTable: cell builds the cell of each column.
// A selected row is filled, as data-state="selected"; any row is tinted
// while the pointer is on it. Clicked on it reports a click on the row.
func DataRow(c *Context, cols []DataColumn, selected bool, cell func(col int)) Element {
	k := TokensOf(c)
	r := dataRow(c, cols, cell, ui.RoleCell).MinHeight(36).BorderColor(k.Border).BorderWidth(0, 0, 1, 0)
	switch {
	case selected:
		r.Background(k.Muted).AccessSelected(true)
	case r.Hovered():
		r.Background(k.Muted.Alpha(0.5))
	}
	return r
}

// DataSpan is a row of a DataTable across all its columns, as a row's
// error under it: fn builds what it shows.
func DataSpan(c *Context, fn func()) Element {
	k := TokensOf(c)
	return ui.Row(c).Role(ui.RoleRow).Padding(8).AlignItems(ui.Center).BorderColor(k.Border).BorderWidth(0, 0, 1, 0).
		Children(func() { ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Role(ui.RoleCell).Children(fn) })
}

// DataFoot is a DataTable's footer row, as a total: medium weight on a
// muted fill, with a rule above it.
func DataFoot(c *Context, cols []DataColumn, cell func(col int)) Element {
	k := TokensOf(c)
	return dataRow(c, cols, cell, ui.RoleCell).MinHeight(36).FontWeight(500).Background(k.Muted.Alpha(0.5)).
		BorderColor(k.Border).BorderWidth(1, 0, 0, 0)
}

// dataRow lays out one cell per column, each as wide as its column in
// every row.
func dataRow(c *Context, cols []DataColumn, cell func(col int), role ui.Role) Element {
	return ui.Row(c).Role(ui.RoleRow).AlignItems(ui.Center).Children(func() {
		for i, col := range cols {
			b := ui.Row(c).Padding(0, 8).Gap(6).AlignItems(ui.Center).MinWidth(0).Role(role)
			switch {
			case col.Width > 0:
				b.Width(col.Width).Shrink(0)
			case col.Grow > 0:
				b.Grow(col.Grow).Basis(0)
			default:
				b.Grow(1).Basis(0)
			}
			if col.End {
				b.Justify(ui.End)
			}
			b.Key(i).Children(func() { cell(i) })
		}
	})
}
