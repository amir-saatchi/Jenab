package kit

import "github.com/egoist/mygo/ui"

// The pieces of a timeline, as the turn inspector's (SPEC 8.4): lanes of
// bars on one time axis. Places and widths are shares of the axis, 0 to
// 1, from its start.

// AxisTick is a tick of a TimeAxis: its place on the axis and its label.
type AxisTick struct {
	At    float64
	Label string
}

// laneLabel is the width of the lanes' labels, which the axis leaves free.
const laneLabel = 72

// TimeAxis is the row of tick labels above a timeline's lanes, muted: the
// first starts at its tick, the last ends at it, and the others are
// centered on theirs.
func TimeAxis(c *Context, ticks []AxisTick) Element {
	k := TokensOf(c)
	return ui.Row(c).Gap(12).Children(func() {
		ui.Box(c).Width(laneLabel).Shrink(0)
		ui.Box(c).Grow(1).Height(16).Children(func() {
			for i, t := range ticks {
				l := ui.Text(c, t.Label).FontSize(12).FontFeatures("tnum").TextColor(k.MutedForeground).SingleLine().
					Absolute().Top(0).StartPercent(float32(t.At * 100)).Key(i)
				switch {
				case i == len(ticks)-1 && i > 0:
					l.TranslatePercent(-100, 0)
				case i > 0:
					l.TranslatePercent(-50, 0)
				}
			}
		})
	})
}

// Lane is a timeline's row: label, muted, before a track that holds the
// bars fn builds with LaneBar.
func Lane(c *Context, label string, fn func()) Element {
	k := TokensOf(c)
	return ui.Row(c).Gap(12).AlignItems(ui.Center).Role(ui.RoleGroup).Label(label).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(k.MutedForeground).Width(laneLabel).Shrink(0).SingleLine()
		ui.Box(c).Grow(1).Height(24).Radius(Radius * 0.6).Background(k.Muted.Alpha(0.5)).Children(fn)
	})
}

// LaneBar is a bar on a lane from start for width, both shares of the
// axis, at least 4 px wide: text in the tone over its soft fill, cut to
// fit. tip, if any, shows as the pointer rests on it.
func LaneBar(c *Context, start, width float64, tone Tone, text, tip string) Element {
	tc := TokensOf(c).Tone(tone)
	b := ui.Box(c).Absolute().Top(0).Bottom(0).StartPercent(float32(start * 100)).WidthPercent(float32(width * 100)).MinWidth(4).
		Radius(Radius * 0.6).PaddingX(6).Background(tc.Soft).TextColor(tc.Text).Justify(ui.Center).ClipX().Label(text)
	if tip != "" {
		b.Tooltip(tip)
	}
	return b.Children(func() { ui.Text(c, text).FontSize(12).SingleLine() })
}
