package kit

import "github.com/egoist/mygo/ui"

// Path is a path or a host as the user types it, as the frontend's
// font-mono dir="ltr" spans: the code font at 12 px, laid out left to
// right whatever its letters, so a Persian folder name keeps the path's
// order. It wraps as text does and can be selected.
func Path(c *Context, s string) Element {
	return ui.Text(c, s).Font(FontMono).FontSize(12).Direction(ui.LeftToRight).Selectable()
}

// PathInput is a TextInput for a path or a host, in the code font and
// laid out left to right, as the frontend's font-mono dir="ltr" inputs.
func PathInput(c *Context, value *string, label string) Element {
	return TextInput(c, value, label).Font(FontMono).FontSize(12).Direction(ui.LeftToRight)
}
