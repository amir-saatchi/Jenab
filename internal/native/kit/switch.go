package kit

import "github.com/egoist/mygo/ui"

// Switch turns *on on and off, as shadcn's Switch, for a setting that
// applies at once: a click turns it over, as does Space while it has the
// focus. It shows no text: label names it. Changed reports a click.
func Switch(c *Context, on *bool, label string) Element {
	k := TokensOf(c)
	t := *c.Theme()
	t.Accent, t.Border = k.Primary, k.Input
	var e Element
	ui.Themed(c, &t).Children(func() { e = ui.Switch(c, on).Label(label) })
	return e
}
