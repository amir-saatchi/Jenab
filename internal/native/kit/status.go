package kit

import "github.com/egoist/mygo/ui"

// Badge is a small label in a tone's soft fill, as shadcn's Badge.
func Badge(c *Context, text string, tone Tone) Element {
	t := TokensOf(c).Tone(tone)
	return ui.Badge(c, text).Background(t.Soft).TextColor(t.Text).Radius(Radius * 0.8).FontSize(12).FontWeight(500)
}

// Spinner shows that something is under way; label tells assistive
// technology what.
func Spinner(c *Context, label string) Element {
	return ui.Spinner(c).Label(label).TextColor(TokensOf(c).MutedForeground)
}

// Alert is a box telling something about the page, as shadcn's Alert: an
// icon and the title in the tone, the text below. Red and Amber alerts are
// errors and warnings, read out as they show.
func Alert(c *Context, tone Tone, title, text string) Element {
	k := TokensOf(c)
	tc := k.Tone(tone)
	icon := IconInfo
	switch tone {
	case Red:
		icon = IconError
	case Amber:
		icon = IconWarning
	case Green:
		icon = IconSuccess
	}
	box := ui.Row(c).Gap(12).Padding(12, 16).Radius(Radius).Border(1, k.Border).Background(k.Card).
		AlignItems(ui.Start).Role(ui.RoleStatus).Label(title)
	return box.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(tc.Text).MarginY(2)
		ui.Column(c).Gap(4).Grow(1).Children(func() {
			ui.Text(c, title).FontWeight(500).TextColor(tc.Text)
			if text != "" {
				Text(c, text).TextColor(k.MutedForeground)
			}
		})
	})
}

// Empty is what a list or a page shows when it has nothing, as shadcn's
// Empty: an icon in a muted square, a title, a line of text and actions,
// centered. actions may be nil.
func Empty(c *Context, icon *SVG, title, text string, actions func()) Element {
	k := TokensOf(c)
	col := ui.Column(c).Gap(8).Padding(24).AlignItems(ui.Center).Justify(ui.Center).Grow(1)
	return col.Children(func() {
		if icon != nil {
			ui.Box(c).Size(40, 40).Radius(Radius).Background(k.Muted).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, icon).Size(20, 20).TextColor(k.Foreground)
			})
		}
		ui.Text(c, title).FontSize(16).FontWeight(500).TextAlign(ui.Center).Role(ui.RoleHeading).Level(2)
		if text != "" {
			Text(c, text).TextColor(k.MutedForeground).TextAlign(ui.Center).MaxWidth(360)
		}
		if actions != nil {
			ui.Row(c).Gap(8).MarginY(8).Children(actions)
		}
	})
}
