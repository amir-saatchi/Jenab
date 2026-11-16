package kit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const goCode = "```go\nfunc f() {\n\treturn 1\n}\n```\n"

// MyGo v0.7.4 drew a tab as nothing; v0.8.1 and later draw it (#1217).
// Without a tab size it advances as the system's text stack sets it; the
// kit's Root puts stops every 4 columns, its code blocks every 2.
func TestMyGoDrawsTabs(t *testing.T) {
	draw := func(src string) []byte {
		var o markdown.Options
		return ui.NewTester(func(c *ui.Context) { markdown.View(c, src, &o) }, 600, 200).Image().Pix
	}
	if bytes.Equal(draw("```\nx\n```\n"), draw("```\n\tx\n```\n")) {
		t.Error("MyGo draws tabs as nothing again")
	}
}

// Text in Root draws a tab to the next stop of 4 columns.
func TestTextTabs(t *testing.T) {
	draw := func(s string) []byte {
		return window(t, false, func(c *Context) { Text(c, s).Font(FontMono) }).Image().Pix
	}
	if !bytes.Equal(draw("ab\tx"), draw("ab  x")) || bytes.Equal(draw("\tx"), draw("x")) {
		t.Error("a tab in text does not draw to the next stop of 4 columns")
	}
}

func TestMarkdownKeepsTabs(t *testing.T) {
	draw := func(src string) []byte {
		var m Markdown
		return window(t, false, func(c *Context) { m.View(c, src) }).Image().Pix
	}
	tab, spaces, none := draw(goCode), draw(strings.Replace(goCode, "\t", "  ", 1)), draw(strings.Replace(goCode, "\t", "", 1))
	if !bytes.Equal(tab, spaces) || bytes.Equal(tab, none) {
		t.Error("a tab in a code block does not draw as 2 spaces, as the frontend's")
	}
}

func TestMarkdown(t *testing.T) {
	var m Markdown
	src := "# " + persian + "\n\n" + persian + " **bold** and `code`.\n\n" + goCode
	view := func(c *Context) { m.View(c, src) }
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "Code, go", "Copy code", "code")
		if !tt.HasText(persian) {
			t.Errorf("no Persian text in %q", tt.Texts())
		}
		tabTo(t, tt, "Copy code")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if got := tt.Clipboard(); got != "func f() {\n\treturn 1\n}\n" {
			t.Errorf("copied %q, want the code with its tab", got)
		}
		shows(t, tt, "Copied")
	})
}

// Stream shows a reply as it grows, tabs and all.
func TestMarkdownStream(t *testing.T) {
	var m Markdown
	src := ""
	tt := window(t, true, func(c *Context) { m.Stream(c, src) })
	for _, part := range []string{"Here:\n\n```go\nfunc f() {\n", "\treturn 1\n", "}\n```\n"} {
		src += part
		tt.Frame()
		tt.Advance(2e9)
	}
	shows(t, tt, "Copy code")
	if !tt.HasText("\treturn 1") {
		t.Errorf("texts %q", tt.Texts())
	}
	if err := tt.Click("Copy code"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "func f() {\n\treturn 1\n}\n" {
		t.Errorf("copied %q", got)
	}
}

// bounds finds the element a screen reader gets as name, in tt.
func bounds(t *testing.T, tt *ui.Tester, name string) ui.Rect {
	t.Helper()
	for _, a := range tt.AccessAll() {
		if a.Name == name {
			return a.Bounds
		}
	}
	var names []string
	for _, a := range tt.AccessAll() {
		names = append(names, a.Name)
	}
	t.Fatalf("no %q in %q", name, names)
	return ui.Rect{}
}

// A table runs in the direction of its first strong character, as the
// frontend's <table dir>: a Persian one's first column is on the right.
// The window stays left to right.
func TestMarkdownTablesTakeTheirDirection(t *testing.T) {
	src := "| نام | سن | شهر |\n|---|---|---|\n| علی | ۳۲ | تهران |\n\n| Name | City |\n|---|---|\n| Sara | Berlin |\n"
	var m Markdown
	themes(t, func(c *Context) { m.View(c, src) }, func(t *testing.T, tt *ui.Tester) {
		if name, city := bounds(t, tt, "نام"), bounds(t, tt, "شهر"); name.X <= city.X {
			t.Errorf("the Persian table runs left to right: نام at %v, شهر at %v", name.X, city.X)
		}
		if name, city := bounds(t, tt, "Name"), bounds(t, tt, "City"); name.X >= city.X {
			t.Errorf("the English table runs right to left: Name at %v, City at %v", name.X, city.X)
		}
	})
}

// A list item has its marker on its start side, as the frontend's
// <li dir>: on the right of a Persian item, on the left of an English
// one, also in one list, which keeps its numbers.
func TestMarkdownListItemsTakeTheirDirection(t *testing.T) {
	src := "- خرید نان\n- پرداخت قبض\n\n1. Open the project\n2. پروژه را باز کنید\n3. Run the tests\n"
	var m Markdown
	themes(t, func(c *Context) { m.View(c, src) }, func(t *testing.T, tt *ui.Tester) {
		const width = 800
		for _, s := range []string{"خرید نان", "پروژه را باز کنید"} {
			if r := bounds(t, tt, s); r.X+r.W > width-20 || r.X+r.W < width/2 {
				t.Errorf("%q ends at %v: its marker is not on the right", s, r.X+r.W)
			}
		}
		for _, s := range []string{"Open the project", "Run the tests"} {
			if r := bounds(t, tt, s); r.X < 15 || r.X > width/2 {
				t.Errorf("%q starts at %v: its marker is not on the left", s, r.X)
			}
		}
		for _, n := range []string{"1.", "2.", "3."} {
			if !tt.HasText(n) {
				t.Errorf("no number %q in %q", n, tt.Texts())
			}
		}
	})
}
