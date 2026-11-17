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
// leave. Text is cut to one line; a cell whose text is cut, in any
// column, shows it whole in its tooltip. label names it. For a long list,
// use List.
func Table(c *Context, label string, cols []TableColumn, rows [][]string) Element {
	k := TokensOf(c)
	cell := func(i int, s string, head bool) {
		t := ui.Text(c, s).FontSize(14).SingleLine().Role(ui.RoleCell)
		if head {
			t.Role(ui.RoleColumnHeader).FontWeight(500).TextColor(k.MutedForeground)
		}
		if i == 0 {
			t.Grow(1).Shrink(1).MinWidth(0)
		} else {
			t.Width(96).Shrink(0)
		}
		if !head && s != "" && cut(c, t, s) {
			t.Tooltip(s)
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

// cut reports whether t, a one-line text of s at 14 px, was too narrow
// for it in the last frame. As the width changes, the frame is built
// again with the new answer.
func cut(c *Context, t Element, s string) bool {
	w, _ := c.MeasureText(0, ui.Span{Text: s, Size: 14})
	short := func(box float32) bool { return box > 0 && w > box+0.5 }
	was := short(t.LayoutBounds().W)
	t.Draw(func(_ *ui.Painter, r ui.Rect) {
		if short(r.W) != was {
			c.Invalidate()
		}
	})
	return was
}
