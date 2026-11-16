package kit

import "github.com/egoist/mygo/ui"

// PathBar is the way back up a path, as the files panel's "Files › docs ›
// 2026" (SPEC 5.12): labels from the top down, the last the place shown.
// A click on another, or Enter on it, sets *chosen to its index, which
// Changed reports. Each label reads in the direction of its own words.
func PathBar(c *Context, labels []string, chosen *int) Element {
	return ui.Breadcrumbs(c, labels, chosen).Label("Path").FontSize(12).Shrink(1).MinWidth(0)
}
