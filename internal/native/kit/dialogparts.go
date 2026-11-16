package kit

import "github.com/egoist/mygo/ui"

// The parts of a Dialog under its title, as shadcn's DialogDescription and
// DialogFooter.

// DialogText is a dialog's description under its title: text-sm in the
// muted color, wrapping, in the direction of its words. Assistive
// technology reads it as the dialog's description.
func DialogText(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(14).TextColor(TokensOf(c).MutedForeground).Role(ui.RoleText)
}

// DialogFooter is the row of a dialog's buttons at its end, the main
// action last, as fn builds them.
func DialogFooter(c *Context, fn func()) Element {
	return ui.Row(c).Gap(8).Justify(ui.End).AlignItems(ui.Center).MarginY(4).Children(fn)
}

// Locked is a Dialog's open while busy: a dialog that may not close yet,
// as an export that runs, takes it in place of its own open, so Escape
// and a click outside leave it open. Otherwise it is open itself.
func Locked(open *bool, busy bool) *bool {
	if !busy {
		return open
	}
	stay := true
	return &stay
}
