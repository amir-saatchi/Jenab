package kit

import "github.com/egoist/mygo/ui"

// LinesView shows a long text in the code font as a virtual list of its
// lines, so a request body of any length opens at once: it builds only
// the lines in view. Each line wraps, keeps its spaces, can be selected,
// and reads in the direction of its first strong character. s keeps the
// list's place; label names it. Give it a height, or grow it.
//
// It takes the keyboard focus, where Up and Down scroll a line, Page Up
// and Page Down a screen, and Home and End go to the ends: MyGo's List
// moves by the keyboard only with a row chosen, which a text has not.
func LinesView(c *Context, s *ListState, lines []string, label string) Element {
	k := TokensOf(c)
	box := ui.Column(c).Radius(Radius*0.8).Border(1, k.Border).AlignItems(ui.Stretch).ClipY()
	return box.Children(func() {
		l := ui.List(c, s, len(lines), func(i int) {
			line := lines[i]
			if line == "" {
				line = " " // an empty line keeps its height
			}
			ui.Text(c, line).Font(FontMono).FontSize(12).PreserveSpaces().TabSize(CodeTabSize).Selectable().Padding(0, 12)
		}).Grow(1).PaddingY(8).Label(label).Focusable()
		if l.FocusVisible() {
			box.BorderColor(k.Ring)
		}
		scroll := func(by func(first, last int) int) func() {
			return func() {
				first, last := s.Visible()
				s.ScrollTo(min(max(by(first, last), 0), max(len(lines)-1, 0)), ui.Start)
			}
		}
		l.OnShortcut(0, ui.KeyDown, scroll(func(f, _ int) int { return f + 1 }))
		l.OnShortcut(0, ui.KeyUp, scroll(func(f, _ int) int { return f - 1 }))
		l.OnShortcut(0, ui.KeyPageDown, scroll(func(_, l int) int { return l }))
		l.OnShortcut(0, ui.KeyPageUp, scroll(func(f, l int) int { return f - (l - f) }))
		l.OnShortcut(0, ui.KeyHome, scroll(func(int, int) int { return 0 }))
		l.OnShortcut(0, ui.KeyEnd, s.ScrollToEnd)
	})
}
