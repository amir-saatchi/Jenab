package kit

import "github.com/egoist/mygo/ui"

// Progress is a bar filled to value between 0 and 1, as shadcn's
// Progress: 8 px high, the primary color over a track of it, filled from
// the window's start edge. label names it; assistive technology reads the
// value with it.
func Progress(c *Context, value float64, label string) Element {
	k := TokensOf(c)
	return ui.Progress(c, max(0, min(value, 1))).Height(8).Radius(4).TextColor(k.Primary).
		Background(k.Primary.Alpha(0.2)).Label(label)
}
