package kit

import "github.com/egoist/mygo/ui"

// TextLink is a button that reads as a link in a line of text, as the
// welcome page's "change in Settings": underlined, in the muted color,
// the foreground while hovered. Clicked reports a click, or Enter or
// Space.
func TextLink(c *Context, label string) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Label(label).TextColor(k.MutedForeground).Radius(4)
	if b.Hovered() {
		b.TextColor(k.Foreground)
	}
	return b.Children(func() { ui.Text(c, label).Underline().SingleLine() })
}
