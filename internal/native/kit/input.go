package kit

import "github.com/egoist/mygo/ui"

// TextInput is a one-line field editing *value, as shadcn's Input: h-9,
// a border in the input color, rounded-md. label names it for assistive
// technology; give it a Placeholder for what to type. Its text takes the
// direction of its first strong letter, as dir="auto" does: Persian text
// starts at the right. Direction overrides it, as a path's LeftToRight.
func TextInput(c *Context, value *string, label string) Element {
	return autoDirection(field(c, ui.TextInput(c, value)), *value).Height(36).Label(label)
}

// TextArea is a field of lines editing *value, as shadcn's Textarea,
// growing with its text: give it Lines(min, max). label names it. Its
// direction is that of its first strong letter, as TextInput's.
func TextArea(c *Context, value *string, label string) Element {
	return autoDirection(field(c, ui.TextArea(c, value)), *value).PaddingY(8).Label(label)
}

func field(c *Context, e Element) Element {
	k := TokensOf(c)
	return e.PaddingX(12).Radius(Radius*0.8).Border(1, k.Input).Background(c.Theme().Surface).
		FontSize(14).PlaceholderColor(k.MutedForeground)
}

// autoDirection lays a text field out in the direction of its text's
// first strong letter, left to right when it has none, as dir="auto" on
// an input: a field of Persian text starts at the right, its caret and
// placeholder too, and only the field is turned, never the layout around
// it. The text is read as the view is built, so the field turns in the
// frame after the letter that decides it is typed.
func autoDirection(e Element, text string) Element {
	if RightToLeft(text) {
		return e.Direction(ui.RightToLeft)
	}
	return e.Direction(ui.LeftToRight)
}
