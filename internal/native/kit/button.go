package kit

import "github.com/egoist/mygo/ui"

// Variant is how a button looks, as shadcn/ui's button variants.
type Variant int

const (
	Default     Variant = iota // the main action: primary
	Secondary                  // a gray fill
	Outline                    // a border, the background
	Ghost                      // no face until hovered
	Destructive                // what deletes or cannot be undone
)

// Button is a button showing label, h-9 with rounded-md corners as
// shadcn's. Clicked on it reports a click, or Enter or Space.
func Button(c *Context, label string, v Variant) Element {
	b := button(c, v).PaddingX(16)
	return b.Children(func() { ui.Text(c, label).SingleLine() })
}

// IconButton is a square button showing only icon: label names it for
// assistive technology and shows as its tooltip.
func IconButton(c *Context, icon *SVG, label string, v Variant) Element {
	b := button(c, v).Width(36).Label(label).Tooltip(label)
	return b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
}

// button is a button's box in the colors of v, with what is in it to come.
func button(c *Context, v Variant) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Height(36).Gap(8).Radius(Radius * 0.8).AlignItems(ui.Center).Justify(ui.Center).
		FontSize(14).FontWeight(500)
	var bg, fg, hover Color
	switch v {
	case Default:
		bg, fg, hover = k.Primary, k.PrimaryForeground, k.Primary.Alpha(0.9)
	case Secondary:
		bg, fg, hover = k.Secondary, k.SecondaryForeground, k.Secondary.Alpha(0.8)
	case Outline:
		t := c.Theme()
		bg, fg, hover = t.Surface, k.Foreground, t.SurfaceHover
		b.Border(1, k.Input)
	case Ghost:
		fg, hover = k.Foreground, k.Accent
	case Destructive:
		bg, fg, hover = k.Destructive, ui.Oklch(1, 0, 0), k.Destructive.Alpha(0.9)
		if c.Theme().Dark {
			bg, hover = k.Destructive.Alpha(0.6), k.Destructive.Alpha(0.5)
		}
	}
	if b.Hovered() {
		bg = hover
	}
	if v != Ghost || b.Hovered() {
		b.Background(bg)
	}
	return b.TextColor(fg)
}

// Disable makes e take neither clicks nor the focus while d, grayed out as
// shadcn's disabled controls are (opacity-50).
func Disable(e Element, d bool) Element {
	if d {
		e.Opacity(0.5)
	}
	return e.Disabled(d)
}
