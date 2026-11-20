package kit

import "github.com/egoist/mygo/ui"

// BlockState is what a data block shows besides its data (SPEC 5.7):
// that its query is on its way, that it can't show it, or a notice about
// it.
type BlockState struct {
	// Loading shows that the block's rows are on their way.
	Loading bool
	// Error is why the block can't show its data, as a failed query or a
	// view that no longer validates; "" for none. It is shown in place of
	// the data.
	Error string
	// Notice is a line above the data, as "Showing the first 10,000 rows.
	// Aggregate in the query to see them all."; "" for none.
	Notice string
}

// BlockBody builds a data block's body by its state: a spinner while it
// loads, the error in place of the data, else the notice, if any, over
// what fn builds. label is the block's name, as its view's title, which
// the spinner and the error name for assistive technology.
func BlockBody(c *Context, label string, st BlockState, fn func()) Element {
	return ui.Column(c).Gap(8).AlignItems(ui.Stretch).Grow(1).MinHeight(0).Children(func() {
		switch {
		case st.Error != "":
			Alert(c, Red, "Can't show "+label, st.Error)
		case st.Loading:
			ui.Row(c).Grow(1).MinHeight(96).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				Spinner(c, "Loading "+label)
			})
		default:
			if st.Notice != "" {
				BlockNotice(c, st.Notice)
			}
			fn()
		}
	})
}

// BlockNotice is a line about a block's data, as the "10,000 rows"
// notice: an info icon and the text, muted, read out as it shows. Its
// text takes the direction of its first strong character.
func BlockNotice(c *Context, text string) Element {
	k := TokensOf(c)
	row := ui.Row(c).Gap(6).AlignItems(ui.Center).Role(ui.RoleStatus).Label(text).TextColor(k.MutedForeground)
	return row.Children(func() {
		ui.Icon(c, IconInfo).Size(14, 14).Shrink(0)
		ui.Text(c, text).FontSize(12).Grow(1).Shrink(1).MinWidth(0)
	})
}
