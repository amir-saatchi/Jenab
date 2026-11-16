package kit

import "github.com/egoist/mygo/ui"

// PickerOption is an option of a Picker: the value it chooses and the
// label it shows.
type PickerOption struct {
	Value string
	Label string
}

// Picker is a drop-down choosing one of options into *selected, as
// shadcn's Select with size="sm": the trigger shows the label of the
// option chosen, or label while none is, and opens the options below it.
// The arrows, Home and End move among them, Enter chooses one and Escape
// closes them; Changed on the trigger reports a new choice. label names
// it for assistive technology. Many options scroll.
func Picker(c *Context, label string, selected *string, options []PickerOption) Element {
	k := TokensOf(c)
	sel := ui.SelectBase(c, selected)
	b := sel.Trigger.Height(32).Padding(0, 8, 0, 12).Gap(8).Radius(Radius*0.8).Border(1, k.Input).
		AlignItems(ui.Center).Justify(ui.SpaceBetween).FontSize(14).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(k.Accent)
	}
	shown, muted := label, true
	for _, o := range options {
		if o.Value == *selected {
			shown, muted = o.Label, false
		}
	}
	b.Children(func() {
		t := ui.Text(c, shown).SingleLine().Shrink(1)
		if muted {
			t.TextColor(k.MutedForeground)
		}
		ui.Icon(c, IconChevronDown).Size(16, 16).Shrink(0).TextColor(k.MutedForeground)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(Radius*0.8).Background(k.Popover).TextColor(k.PopoverForeground).
			Border(1, k.Border).Shadow(0, 6, 20, 0, ui.Oklch(0, 0, 0).Alpha(0.15)).MinWidth(160).MaxWidth(420)
		ui.Scroll(c).MaxHeight(320).Children(func() {
			ui.Column(c).AlignItems(ui.Stretch).Children(func() {
				for _, o := range options {
					item := sel.Item(o.Value).Padding(6, 8).Gap(8).Radius(Radius * 0.6).AlignItems(ui.Center).FontSize(14)
					if item.Highlighted() {
						item.Background(k.Accent).TextColor(k.AccentForeground)
					}
					item.Children(func() {
						ui.Text(c, o.Label).SingleLine().Grow(1).Shrink(1)
						check := ui.Icon(c, IconCheck).Size(16, 16).Shrink(0)
						if o.Value != *selected {
							check.Opacity(0)
						}
					})
				}
			})
		})
	})
	return b
}
