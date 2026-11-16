package kit

import "github.com/egoist/mygo/ui"

// Tooltip gives e a tip that shows as the pointer rests on it or the
// keyboard focus comes to it, in the theme's inverse colors as shadcn's
// tooltip; assistive technology reads it as e's description.
func Tooltip(e Element, tip string) Element { return e.Tooltip(tip) }

// MenuButton is an outline button showing label that opens the menu build
// fills, below it in the window, as shadcn's DropdownMenu.
func MenuButton(c *Context, label string, build func(m *Menu)) Element {
	k := TokensOf(c)
	return ui.PopoverMenuButton(c, label, build).Height(36).Radius(Radius*0.8).Border(1, k.Input).FontSize(14)
}

// MoreButton is a ghost button showing "…" that opens the menu build
// fills: label names it and shows as its tooltip.
func MoreButton(c *Context, label string, build func(m *Menu)) Element {
	b := IconButton(c, IconMore, label, Ghost)
	return b.PopoverMenuButton(build)
}

// Dialog shows a dialog over the dimmed window while *open, as shadcn's
// Dialog: title as its heading, which names it, and fn's content below.
// Escape or a click outside closes it; Dismissed reports that frame.
func Dialog(c *Context, open *bool, title string, fn func()) Element {
	k := TokensOf(c)
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.Oklch(0, 0, 0).Alpha(0.5))
		panel.Width(512).MaxWidthPercent(90).Padding(24).Gap(16).Radius(Radius*1.4).
			Background(k.Background).Border(1, k.Border).Shadow(0, 8, 24, 0, ui.Oklch(0, 0, 0).Alpha(0.15)).
			Label(title).TextColor(k.Foreground)
		Heading(c, title)
		fn()
	})
}

// Heading is a dialog's or a section's title, as shadcn's DialogTitle:
// text-lg, semibold, a heading to assistive technology.
func Heading(c *Context, title string) Element {
	return ui.Text(c, title).FontSize(18).FontWeight(600).Role(ui.RoleHeading).Level(2)
}

// AlertDialog asks about something important while *open, as shadcn's
// AlertDialog: a title, a message and buttons, the last the default,
// which Enter clicks. It returns the index of the button clicked in the
// frame it is, AlertDismissed in the frame Escape closes it, and -1
// otherwise.
func AlertDialog(c *Context, open *bool, title, message string, buttons ...string) int {
	return ui.AlertDialog(c, open, title, message, buttons...)
}

// AlertDismissed is what AlertDialog returns as Escape closes it.
const AlertDismissed = ui.AlertDismissed
