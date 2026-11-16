package kit

import "github.com/egoist/mygo/ui"

// LinkButton is a button that looks like a link, as shadcn's Button with
// variant="link" and no padding: text in the primary color, underlined
// while the pointer is on it. label names it for assistive technology when
// its text alone says too little, as a request's number; "" leaves the
// text as its name. Clicked on it reports a click, or Enter or Space.
func LinkButton(c *Context, text, label string) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Radius(Radius * 0.4).AlignItems(ui.Center).FontWeight(500).TextColor(k.Primary)
	if label != "" {
		b.Label(label)
	}
	return b.Children(func() {
		t := ui.Text(c, text).SingleLine().Shrink(1)
		if b.Hovered() {
			t.Underline()
		}
	})
}
