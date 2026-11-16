package kit

import "github.com/egoist/mygo/ui"

// StatusBar is the bar along the window's bottom (SPEC 5.12), 24 px high in
// the sidebar's colors with a rule over it and small muted text: fn builds
// its items, a Spacer between those on the start and those on the end.
func StatusBar(c *Context, fn func()) Element {
	k := TokensOf(c)
	return ui.Row(c).Height(24).Shrink(0).AlignItems(ui.Center).Gap(16).PaddingX(12).Background(k.Sidebar).
		BorderColor(k.Border).BorderWidth(1, 0, 0, 0).FontSize(12).TextColor(k.MutedForeground).
		Role(ui.RoleContentInfo).Label("Status").Children(fn)
}

// StatusText is an item of the status bar: text in tone, muted for
// Neutral, medium weight for Red; tip, if not "", shows as the pointer
// rests on it.
func StatusText(c *Context, text string, tone Tone, tip string) Element {
	k := TokensOf(c)
	t := ui.Text(c, text).SingleLine().FontFeatures("tnum")
	switch tone {
	case Neutral:
	case Red:
		t.TextColor(k.Tone(tone).Text).FontWeight(500)
	default:
		t.TextColor(k.Tone(tone).Text)
	}
	if tip != "" {
		t.Tooltip(tip)
	}
	return t
}

// StatusButton is an item of the status bar that does something, as the
// count of chats at work opening one: Clicked reports a click, or Enter.
func StatusButton(c *Context, text string) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Label(text).Radius(4).PaddingX(4).AlignItems(ui.Center)
	if b.Hovered() {
		b.TextColor(k.Foreground)
	}
	return b.Children(func() { ui.Text(c, text).SingleLine() })
}
