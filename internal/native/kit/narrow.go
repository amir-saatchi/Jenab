package kit

import "github.com/egoist/mygo/ui"

// The narrow window (SPEC 5.12, TASK-062): below NarrowWidth the rail and
// both sidebars open as overlays and a docked chat as a drawer, as
// the React app's hooks/use-mobile.ts and shadcn's Sheet do.

// NarrowWidth is the window's width below which it is narrow, in DIPs.
const NarrowWidth = 900

// The narrow window's icons.
var (
	IconMenu  = icon("menu")
	IconPages = icon("panel-right") // the right sidebar: Pages, Pipelines, Files, Links
)

// Narrow reports whether the window is narrower than NarrowWidth. A view
// that reads it is built again as the window's size changes.
func Narrow(c *Context) bool {
	w, _ := c.Size()
	return w > 0 && w < NarrowWidth
}

// Sheet shows fn in a panel along the window's start edge, or its end
// edge when end, over the dimmed window while *open, as shadcn's Sheet:
// the panel runs the window's full height and is as wide as fn's content,
// at most the window. title names it for assistive technology. Escape or
// a click outside closes it. As MyGo's dialogs, it gives the focus to the
// first control in it as it opens.
func Sheet(c *Context, open *bool, end bool, title string, fn func()) Element {
	k := TokensOf(c)
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.Oklch(0, 0, 0).Alpha(0.5)).Row().AlignItems(ui.Stretch).Justify(ui.Start)
		panel.MaxWidthPercent(100).MinHeight(0).AlignItems(ui.Stretch).Background(k.Background).BorderColor(k.Border).
			Shadow(0, 8, 24, 0, ui.Oklch(0, 0, 0).Alpha(0.15)).Label(title).TextColor(k.Foreground)
		if end {
			backdrop.Justify(ui.End)
			panel.BorderStartWidth(1)
		} else {
			panel.BorderEndWidth(1)
		}
		fn()
	})
}
