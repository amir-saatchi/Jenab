package kit

import "github.com/egoist/mygo/ui"

// A settings page's pieces (SPEC 5.12), as frontend/src/shell's
// settings-layout: the title, the sections and their rows.

// PageTitle is a page's title, as SettingsTitle: text-xl semibold, the
// page's heading, in the direction of its words, with description ("" for
// none) under it in the muted color.
func PageTitle(c *Context, title, description string) Element {
	k := TokensOf(c)
	return ui.Column(c).Gap(4).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, title).FontSize(20).FontWeight(600).Role(ui.RoleHeading).Level(1)
		if description != "" {
			ui.Text(c, description).FontSize(14).TextColor(k.MutedForeground)
		}
	})
}

// Section is a part of a page, as SettingsSection: title as its heading,
// description ("" for none) under it in the muted color, then what fn
// builds. Assistive technology finds it as a group named title.
func Section(c *Context, title, description string, fn func()) Element {
	k := TokensOf(c)
	col := ui.Column(c).Gap(12).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label(title)
	return col.Children(func() {
		ui.Column(c).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(500).Role(ui.RoleHeading).Level(2)
			if description != "" {
				ui.Text(c, description).FontSize(14).TextColor(k.MutedForeground)
			}
		})
		fn()
	})
}

// Reveal scrolls the page so e's start is at the top, gliding there, as a
// page list's click on a section does. Call it in the frame it should
// come into view only.
func Reveal(e Element) Element {
	return e.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollStart, Animation: &ui.Smooth})
}

// DetailRow is a row of a label and what fn builds beside it, as settings-layout.tsx's FactRow:
// the label in the muted color in a column 144 px wide.
func DetailRow(c *Context, label string, fn func()) Element {
	k := TokensOf(c)
	row := ui.Row(c).Gap(16).AlignItems(ui.Center).FontSize(14)
	return row.Children(func() {
		ui.Text(c, label).Width(144).Shrink(0).TextColor(k.MutedForeground)
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Gap(8).Children(fn)
	})
}

// Rule is a line across a page between its sections, as SettingsDivider.
func Rule(c *Context) Element {
	return ui.Box(c).Height(1).Shrink(0).Background(TokensOf(c).Border).Role(ui.RoleSeparator)
}
