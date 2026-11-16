// Package kit is Jenab's components on MyGo's ui (SPIKE-048): the theme
// (SPEC 5.11), the fonts, the right-to-left defaults, and the fixes for
// MyGo's bugs, each done once. Screens use the kit and the stores
// (internal/native/state) and never import github.com/egoist/mygo/ui
// themselves; a test checks that. So a change in MyGo's API before its v1
// touches only the kit.
//
// Every component has a headless test (ui.NewTester): light and dark,
// keyboard focus, an accessible label, and right-to-left text in it.
//
// # Direction
//
// Jenab supports right-to-left text, not a mirrored layout (SPEC 5.8, 11).
// A MyGo window takes the direction of the app's language, so Root lays
// the window out left to right whatever it is; a paragraph still reads in
// the direction of its first strong character, so Persian text starts on
// the right.
package kit

import "github.com/egoist/mygo/ui"

// The ui types screens name. They are aliases, so the kit's components
// and MyGo's take the same values.
type (
	Context = ui.Context
	Element = ui.Element
	Color   = ui.Color
	SVG     = ui.SVG
	Menu    = ui.Menu
	Align   = ui.Align
)

// The alignments of rows and columns.
const (
	Start        = ui.Start
	Center       = ui.Center
	End          = ui.End
	Stretch      = ui.Stretch
	SpaceBetween = ui.SpaceBetween
	Baseline     = ui.Baseline
)

// Row, Column, Box, Scroll and Text are MyGo's layout elements, for
// screens to lay the kit's components out with.
func Row(c *Context) Element                            { return ui.Row(c) }
func Column(c *Context) Element                         { return ui.Column(c) }
func Box(c *Context) Element                            { return ui.Box(c) }
func Scroll(c *Context) Element                         { return ui.Scroll(c) }
func Text(c *Context, s string) Element                 { return ui.Text(c, s) }
func Icon(c *Context, icon *SVG) Element                { return ui.Icon(c, icon) }
func Local[T any](e Element, key any, init func() T) *T { return ui.Local(e, key, init) }

// Content is what a window shows; give View's to mygo.WindowOptions.
type Content = ui.Content

// View is a window's content that view builds, each frame.
func View(view func(c *Context)) *Content { return ui.View(view) }

// TabSize is where text puts its tab stops, every 4 columns, as the
// frontend's code blocks (tab-size: 4).
const TabSize = 4

// Root builds the window: Jenab's theme for the window's appearance, laid
// out left to right (see Direction above), with tab stops every TabSize
// columns, and fn's content filling it.
func Root(c *Context, fn func()) Element {
	t := Theme(c.Theme().Dark)
	c.SetTheme(t)
	return ui.Column(c).Direction(ui.LeftToRight).Grow(1).AlignItems(ui.Stretch).TabSize(TabSize).Children(fn)
}

// Tokens is the tokens of the theme the window builds with now.
func TokensOf(c *Context) *Tokens {
	if c.Theme().Dark {
		return &Dark
	}
	return &Light
}
