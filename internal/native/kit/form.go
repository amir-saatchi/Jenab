package kit

import "github.com/egoist/mygo/ui"

// Field is a control with its label above it, as shadcn's Field: label
// names the control fn builds, and a click on it focuses the control.
// Below the control, problem shows in red and marks the control not
// valid, which assistive technology reads with it; while problem is "",
// description shows there in the muted color ("" for none).
func Field(c *Context, label, description, problem string, fn func()) Element {
	k := TokensOf(c)
	f := ui.Field(c, label, fn).Gap(8).FontSize(14).AlignItems(ui.Stretch)
	if problem != "" {
		return f.Error(problem)
	}
	if description != "" {
		f.Description(description)
	}
	return f.TextColor(k.Foreground)
}

// CheckField is a check box toggling *checked, with label beside it and
// description under the label in the muted color, as shadcn's horizontal
// Field with a Checkbox; assistive technology reads the description with
// the box.
func CheckField(c *Context, checked *bool, label, description string) Element {
	k := TokensOf(c)
	var box Element
	ui.Column(c).Gap(4).AlignItems(ui.Stretch).Children(func() {
		box = ui.Checkbox(c, checked, label).FontSize(14).FontWeight(500)
		if description != "" {
			box.Description(description)
			ui.Text(c, description).FontSize(13).TextColor(k.MutedForeground).MarginStart(24)
		}
	})
	return box
}
