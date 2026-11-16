package kit

import "github.com/egoist/mygo/ui"

// TableColumn is a column of a Table: its heading, and whether it holds
// numbers, which line up at its end in figures of one width.
type TableColumn struct {
	Title   string
	Numeric bool
}

// Table is a small table, as shadcn's Table: a row of headings, then the
// rows, a line under each. The first column takes the room the others
// leave; text cut to one line shows whole in its tooltip. label names
// it. For a long list, use List.
func Table(c *Context, label string, cols []TableColumn, rows [][]string) Element {
	k := TokensOf(c)
	cell := func(i int, s string, head bool) {
		t := ui.Text(c, s).FontSize(14).SingleLine().Role(ui.RoleCell)
		if head {
			t.Role(ui.RoleColumnHeader).FontWeight(500).TextColor(k.MutedForeground)
		}
		if i == 0 {
			t.Grow(1).Shrink(1).MinWidth(0)
			if !head && s != "" {
				t.Tooltip(s)
			}
		} else {
			t.Width(96).Shrink(0)
		}
		if i < len(cols) && cols[i].Numeric {
			t.TextAlign(ui.End).FontFeatures("tnum").Direction(ui.LeftToRight)
		}
	}
	row := func(cells []string, head bool) {
		Row(c).Gap(12).PaddingX(8).Height(40).AlignItems(Center).Role(ui.RoleRow).
			BorderColor(k.Border).BorderWidth(0, 0, 1, 0).Children(func() {
			for i, s := range cells {
				cell(i, s, head)
			}
		})
	}
	return ui.Column(c).Role(ui.RoleTable).Label(label).AlignItems(Stretch).Children(func() {
		heads := make([]string, len(cols))
		for i, col := range cols {
			heads[i] = col.Title
		}
		row(heads, true)
		for _, r := range rows {
			row(r, false)
		}
	})
}
