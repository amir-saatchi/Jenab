package kit

import "github.com/egoist/mygo/ui"

// ShareBar is a small bar filled to share, 0 to 1, in the primary color
// over the muted track, as a block's share of the largest: label tells
// assistive technology what it shows.
func ShareBar(c *Context, share float64, label string) Element {
	k := TokensOf(c)
	share = min(max(share, 0), 1)
	track := ui.Box(c).Width(96).Height(6).Radius(3).Background(k.Muted).Shrink(0).Role(ui.RoleMeter).Label(label)
	return track.Children(func() {
		ui.Box(c).WidthPercent(float32(share * 100)).Height(6).Radius(3).Background(k.Primary)
	})
}
