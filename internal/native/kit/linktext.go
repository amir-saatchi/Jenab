package kit

import "github.com/egoist/mygo/ui"

// TextPart is a run of text, or a link in it, as LinkText shows it.
type TextPart struct {
	Text string
	Link bool
}

// LinkText is a paragraph of parts, its links in the accent color, as a
// jenab:// link in the user's message (SPEC 4.5): a click on a link, or
// Enter while it has the focus, calls open with its text. Style the rest
// of the paragraph on the element, as with FontSize and TextColor. The
// paragraph takes the direction of its first strong character.
func LinkText(c *Context, parts []TextPart, open func(link string)) Element {
	k := TokensOf(c)
	return ui.RichText(c).PreserveSpaces().Children(func() {
		for i, p := range parts {
			if !p.Link {
				ui.Text(c, p.Text)
				continue
			}
			c.Key(i)
			l := ui.Link(c, p.Text, "").TextColor(k.Primary).Underline()
			if l.Clicked() && open != nil {
				open(p.Text)
			}
		}
	})
}

// LinkBubble is Bubble with links in its text, as LinkText's.
func LinkBubble(c *Context, parts []TextPart, outline bool, open func(link string)) Element {
	k := TokensOf(c)
	b := ui.Box(c).Padding(10, 14).Radius(Radius * 1.6).MaxWidthPercent(80)
	if outline {
		b.Border(1, k.Border).BorderStyle(ui.BorderDashed)
	} else {
		b.Background(k.Secondary).TextColor(k.SecondaryForeground)
	}
	return b.Children(func() { LinkText(c, parts, open) })
}
