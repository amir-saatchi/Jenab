package kit

import "github.com/egoist/mygo/ui"

// Link is a button that reads as a link in running text: underlined, in
// the text's size and the foreground color, as the settings link to a
// project's settings. Clicked reports a click.
func Link(c *Context, label string) Element {
	b := ui.ButtonBase(c).Label(label).Radius(2)
	return b.Children(func() {
		ui.Text(c, label).FontSize(14).Underline().UnderlineOffset(3)
	})
}
