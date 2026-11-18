package kit

import "github.com/egoist/mygo/ui"

// Split lays main and side out side by side, as an open file and the chat
// docked next to it (SPEC 5.12), with a line between them that the user
// drags, or moves with the arrows once it has the keyboard focus, to
// resize side. *width is side's width in DIPs, which a move sets: 0 starts
// it at a third of the room. side stays at least min wide and leaves main
// at least as much; with less room side keeps min. start puts side on the
// start edge, the left, and otherwise on the end. label names the panes'
// group for assistive technology; MyGo names the line itself, "Divider",
// with no setting for it (still so in v0.10.0).
func Split(c *Context, width *float32, min float32, start bool, label string, main, side func()) Element {
	row := ui.Row(c).Grow(1).Shrink(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label(label)
	// The room as the last frame laid it out, 0 in the first.
	total := row.Bounds().W
	w := *width
	if w <= 0 {
		w = total / 3
	}
	w = fitSide(w, total, min)
	shown := w
	// MyGo's Split sizes its first pane: side, put on the end by laying
	// the split out right to left, its panes back left to right (see
	// Direction in kit.go). The split takes the direction as it is built,
	// for its arrows and drags, so the row around it sets it.
	if !start {
		row.Direction(ui.RightToLeft)
	}
	return row.Children(func() {
		pane := func(fn func()) func() {
			return func() {
				ui.Column(c).Direction(ui.LeftToRight).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(fn)
			}
		}
		ui.Split(c, &w, pane(side), pane(main)).Grow(1).Shrink(1).MinWidth(0)
		if w != shown {
			*width = fitSide(w, total, min)
		}
	})
}

// fitSide keeps side's width w at least min, and main at least min in
// total, the room less the line; with no room known, or too little, it is
// min.
func fitSide(w, total, min float32) float32 {
	if total > 0 {
		w = minf(w, total-1-min)
	}
	return maxf(w, min)
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
