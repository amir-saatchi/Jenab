package kit

import "github.com/egoist/mygo/ui"

// ActionButton is a Button with icon (nil for none) before label, as
// shadcn's Button with data-icon="inline-start"; while busy a spinner
// takes the icon's place and the button is disabled, as a form's submit
// button while its call runs. An icon that points, as an arrow, keeps
// its way in right-to-left text: the layout is not mirrored (SPEC 5.8).
func ActionButton(c *Context, icon *SVG, label string, v Variant, busy bool) Element {
	b := button(c, v).PaddingX(12).Label(label)
	b.Children(func() {
		switch {
		case busy:
			ui.Spinner(c).Size(16, 16).Label(label)
		case icon != nil:
			ui.Icon(c, icon).Size(16, 16)
		}
		ui.Text(c, label).SingleLine()
	})
	if busy {
		return Disable(b, true)
	}
	return b
}
