package kit

import "github.com/egoist/mygo/ui"

// TitledCard is a card on a page, as shadcn's Card with a CardHeader: a
// title, a heading for assistive technology that names the card, a muted
// description under it if any, and fn's content below.
func TitledCard(c *Context, title, description string, fn func()) Element {
	k := TokensOf(c)
	card := ui.Column(c).Gap(16).Padding(20).Radius(Radius*1.4).Border(1, k.Border).Background(k.Card).
		TextColor(k.CardForeground).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label(title)
	return card.Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, title).FontWeight(600).Role(ui.RoleHeading).Level(2)
			if description != "" {
				ui.Text(c, description).FontSize(14).TextColor(k.MutedForeground)
			}
		})
		if fn != nil {
			fn()
		}
	})
}
