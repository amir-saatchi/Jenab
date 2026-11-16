package kit

import "github.com/egoist/mygo/ui"

// Item is a row in an outline box, as shadcn's Item: a title, a badge
// after it in tone if badge is not "", and a muted line under it, as a
// version of a file.
func Item(c *Context, title, badge string, tone Tone, sub string) Element {
	k := TokensOf(c)
	box := ui.Column(c).Gap(2).Padding(10, 14).Radius(Radius).Border(1, k.Border).AlignItems(ui.Stretch)
	return box.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(500).SingleLine()
			if badge != "" {
				Badge(c, badge, tone)
			}
		})
		if sub != "" {
			ui.Text(c, sub).FontSize(13).TextColor(k.MutedForeground)
		}
	})
}
