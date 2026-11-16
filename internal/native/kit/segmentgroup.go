package kit

import "github.com/egoist/mygo/ui"

// Segment is one of a SegmentGroup's segments: the value it chooses, its
// label, and an icon before the label (nil for none).
type Segment struct {
	Value string
	Label string
	Icon  *SVG
}

// SegmentGroup chooses one of choices into *selected, as shadcn's single
// ToggleGroup variant="outline" size="sm": joined segments with a border,
// the one chosen filled in the accent color. It is one Tab stop, among
// whose segments the arrows choose; label names it for assistive
// technology, which sees a radio group. Disable it with Disable.
func SegmentGroup(c *Context, selected *string, label string, choices []Segment) Element {
	k := TokensOf(c)
	i := -1
	for n, ch := range choices {
		if ch.Value == *selected {
			i = n
		}
	}
	before := i
	at := max(i, 0)
	parts := ui.SegmentedBase(c, &at, len(choices))
	if i < 0 {
		// Nothing chosen yet: SegmentedBase marks the first, so read a
		// change from it only once a segment is clicked.
		before = at
	}
	track := parts.Track.Height(32).Radius(Radius*0.8).Border(1, k.Input).Label(label).FontSize(13).FontWeight(500)
	track.Children(func() {
		for n, ch := range choices {
			s := parts.Segment(n).Gap(6).PaddingX(10).AlignItems(ui.Center).Justify(ui.Center).
				TextColor(k.Foreground).Label(ch.Label)
			if n > 0 {
				s.BorderColor(k.Input).BorderStartWidth(1)
			}
			switch {
			case n == i:
				s.Background(k.Accent).TextColor(k.AccentForeground)
			case s.Hovered():
				s.Background(k.Muted)
			}
			s.Children(func() {
				if ch.Icon != nil {
					ui.Icon(c, ch.Icon).Size(14, 14)
				}
				ui.Text(c, ch.Label).SingleLine()
			})
		}
	})
	if at != before && at < len(choices) {
		*selected = choices[at].Value
	}
	return track
}
