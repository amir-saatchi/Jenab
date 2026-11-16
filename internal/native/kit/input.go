package kit

import "github.com/egoist/mygo/ui"

// TextInput is a one-line field editing *value, as shadcn's Input: h-9,
// a border in the input color, rounded-md. label names it for assistive
// technology; give it a Placeholder for what to type.
func TextInput(c *Context, value *string, label string) Element {
	return field(c, ui.TextInput(c, value)).Height(36).Label(label)
}

// TextArea is a field of lines editing *value, as shadcn's Textarea,
// growing with its text: give it Lines(min, max). label names it.
func TextArea(c *Context, value *string, label string) Element {
	return field(c, ui.TextArea(c, value)).PaddingY(8).Label(label)
}

func field(c *Context, e Element) Element {
	k := TokensOf(c)
	return e.PaddingX(12).Radius(Radius*0.8).Border(1, k.Input).Background(c.Theme().Surface).
		FontSize(14).PlaceholderColor(k.MutedForeground)
}
