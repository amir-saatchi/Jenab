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
// kit's Root and code blocks put stops every 4 columns.
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
	tab, spaces, none := draw(goCode), draw(strings.Replace(goCode, "\t", "    ", 1)), draw(strings.Replace(goCode, "\t", "", 1))
	if !bytes.Equal(tab, spaces) || bytes.Equal(tab, none) {
		t.Error("a tab in a code block does not draw as 4 spaces")
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
