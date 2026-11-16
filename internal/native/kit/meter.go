package kit

import "github.com/egoist/mygo/ui"

// Meter is a bar filled to fraction, from 0 to 1, as how much of a
// memory's room is used: in the primary color, amber from 80 percent and
// red when full. It stretches across its column; label names it.
func Meter(c *Context, fraction float64, label string) Element {
	k := TokensOf(c)
	return ui.Meter(c, fraction, 0, 1, &ui.MeterLevels{
		Warning: 0.8, Critical: 1,
		NormalColor: k.Primary, WarningColor: k.Tone(Amber).Text, CriticalColor: k.Tone(Red).Text,
	}).Label(label).Height(8).Background(k.Muted)
}
