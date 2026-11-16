package kit

import "github.com/egoist/mygo/ui"

// Tabs is a row of tabs showing labels, as shadcn's Tabs: *selected is the
// index of the one chosen, in the background's color on the muted row.
// label names the row. A click chooses a tab, as do the arrows, Home and
// End while one has the focus, which follows the choice; Changed reports
// a new choice.
func Tabs(c *Context, selected *int, label string, labels ...string) Element {
	k := TokensOf(c)
	tabs := ui.TabsBase(c, selected, len(labels))
	return tabs.List.Label(label).Height(36).Padding(3).Radius(Radius).Background(k.Muted).AlignItems(ui.Stretch).
		Children(func() {
			for i, name := range labels {
				tab := tabs.Tab(i).Grow(1).Shrink(1).MinWidth(0).Padding(4, 8).Radius(Radius * 0.8).
					AlignItems(ui.Center).Justify(ui.Center).FontSize(14).FontWeight(500).TextColor(k.MutedForeground)
				if i == *selected {
					tab.Background(k.Background).TextColor(k.Foreground).Shadow(0, 1, 2, 0, ui.Oklch(0, 0, 0).Alpha(0.05))
				}
				tab.Children(func() { ui.Text(c, name).SingleLine() })
			}
		})
}
