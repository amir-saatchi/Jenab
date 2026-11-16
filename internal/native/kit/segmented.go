package kit

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// Choice is a value a segmented control or a picker offers, and the text
// it shows.
type Choice struct{ Value, Label string }

// Segmented chooses one of choices into *value, as shadcn's ToggleGroup
// type="single" variant="outline": a click chooses a segment, as do the
// arrows while it has the focus. It is one stop of Tab. A value none of
// the choices has leaves every segment unchosen. label names it; Changed
// reports a new choice.
func Segmented(c *Context, value *string, choices []Choice, label string) Element {
	i := slices.IndexFunc(choices, func(ch Choice) bool { return ch.Value == *value })
	labels := make([]string, len(choices))
	for j, ch := range choices {
		labels[j] = ch.Label
	}
	k := TokensOf(c)
	t := *c.Theme()
	t.Accent, t.AccentText = k.Accent, k.AccentForeground
	var e Element
	ui.Themed(c, &t).Children(func() {
		e = ui.Segmented(c, &i, labels...).Label(label).FontSize(14)
	})
	if e.Changed() && i >= 0 && i < len(choices) {
		*value = choices[i].Value
	}
	return e
}
