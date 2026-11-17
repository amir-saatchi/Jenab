package kit

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Links from content open only safe addresses (TASK-064).

func TestSafeLink(t *testing.T) {
	for url, want := range map[string]bool{
		"https://example.com/a?b=1#c":             true,
		"HTTPS://EXAMPLE.COM":                     true,
		"http://127.0.0.1:8099/":                  true,
		"mailto:someone@example.com":              true,
		"file:///C:/Windows/notepad.exe":          false,
		"javascript:alert(1)":                     false,
		"calc":                                    false,
		"calc.exe":                                false,
		"docs/a.md":                               false,
		"./a.md":                                  false,
		`\\host\share\x.exe`:                      false,
		"//host/share/x.exe":                      false,
		`https:\\host\x`:                          false,
		"https://a\x01b.example":                  false,
		"https://a\nb.example":                    false,
		"https://a\x7fb.example":                  false,
		" https://example.com":                    false,
		"ms-settings:network":                     false,
		"jenab://01JABCDEFGHJKMNPQRSTVWXYZ0/a.md": false,
		"": false,
	} {
		if got := SafeLink(url); got != want {
			t.Errorf("SafeLink(%q) = %v, want %v", url, got, want)
		}
	}
}

// linkDoc has a link of each kind. The ones Markdown can't write as a
// destination, with a backslash or a control character, come from inline
// code through CodeLink, as a `jenab://` link does.
const linkDoc = "[web](https://example.com/) [mail](mailto:someone@example.com) " +
	"[app](jenab://01JABCDEFGHJKMNPQRSTVWXYZ0/a.md) [prog](calc.exe) [bare](calc) " +
	"[file](file:///C:/Windows/notepad.exe) [js](javascript:alert(1)) [rel](docs/a.md) " +
	"[net](//host/share/x.exe) `unc` `ctl` " + persian

var linkCodes = map[string]string{"unc": `\\host\share\x.exe`, "ctl": "https://a\x01b.example/"}

var linkLabels = []string{"web", "mail", "app", "prog", "bare", "file", "js", "rel", "net", "unc", "ctl"}

func TestMarkdownOpensOnlySafeLinks(t *testing.T) {
	var owned, handled []string
	own := func(u string) bool {
		if strings.HasPrefix(u, "jenab://") {
			owned = append(owned, u)
			return true
		}
		return false
	}
	code := func(c string) (string, bool) { d, ok := linkCodes[c]; return d, ok }
	cases := []struct {
		name          string
		md            *Markdown
		owned, opened int // links owned, links the system opened
		handled       int // links OnLink got
	}{
		{"default", &Markdown{CodeLink: code}, 0, 2, 0},
		{"own", &Markdown{Own: own, CodeLink: code}, 1, 2, 0},
		{"onlink", &Markdown{Own: own, CodeLink: code, OnLink: func(u string) { handled = append(handled, u) }}, 1, 0, 2},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			view := func(c *Context) { Column(c).Width(760).Children(func() { k.md.View(c, linkDoc) }) }
			themes(t, view, func(t *testing.T, tt *ui.Tester) {
				owned, handled = nil, nil
				for _, label := range linkLabels {
					n, ok := tt.AccessByRole(ui.RoleLink, label)
					if !ok {
						t.Fatalf("no link %q in %q", label, tt.Texts())
					}
					tt.ClickAt(n.Bounds.X+n.Bounds.W/2, n.Bounds.Y+n.Bounds.H/2)
					tt.Frame()
				}
				safe := []string{"https://example.com/", "mailto:someone@example.com"}
				if got := tt.OpenedURLs(); len(got) != k.opened || k.opened > 0 && !slices.Equal(got, safe) {
					t.Errorf("the system opened %q", got)
				}
				if len(owned) != k.owned || k.owned > 0 && owned[0] != "jenab://01JABCDEFGHJKMNPQRSTVWXYZ0/a.md" {
					t.Errorf("owned %q", owned)
				}
				if len(handled) != k.handled || k.handled > 0 && !slices.Equal(handled, safe) {
					t.Errorf("OnLink got %q", handled)
				}
			})
		})
	}
}

func TestAHeadingLinkScrolls(t *testing.T) {
	var b strings.Builder
	b.WriteString("[Go to the end](#the-end) and [nowhere](#no-such-heading)\n\n")
	for range 60 {
		b.WriteString("A paragraph that only takes room.\n\n")
	}
	b.WriteString("## The end\n\nThe last line.\n")
	src := b.String()
	md := &Markdown{Own: func(string) bool { return false }}
	// A second view of the same Markdown, with the heading too, stays put.
	other := "## The end\n\nAnother text."
	view := func(c *Context) {
		Row(c).Gap(8).Children(func() {
			Scroll(c).Height(400).Width(380).Children(func() { md.View(c, src) })
			Scroll(c).Height(400).Width(380).Children(func() { md.View(c, other) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		top := func() float32 {
			r, ok := tt.Find("The last line.")
			if !ok {
				return 1e9 // not built: below the view
			}
			return r.Y
		}
		otherAt, _ := tt.Find("Another text.")
		if y := top(); y < 400 {
			t.Fatalf("the end shows before the click, at %v", y)
		}
		if err := tt.Click("nowhere"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Frame()
		if err := tt.Click("Go to the end"); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			tt.Frame()
		}
		// MyGo scrolls the heading just into view, at the bottom edge: the
		// line under it, one line further down, is just past it.
		if y := top(); y < 0 || y > 460 {
			t.Errorf("the end is at %v after the click", y)
		}
		if r, _ := tt.Find("Another text."); r.Y != otherAt.Y {
			t.Errorf("the other view moved from %v to %v", otherAt.Y, r.Y)
		}
		if len(tt.OpenedURLs()) != 0 {
			t.Errorf("a heading link opened %q", tt.OpenedURLs())
		}
	})
}

// TASK-075: two views of one Markdown with the same text, as two answers
// alike in the thread: a heading link scrolls only the view it was clicked
// in.
func TestAHeadingLinkScrollsItsOwnView(t *testing.T) {
	var b strings.Builder
	b.WriteString("[Go to the end](#the-end)\n\n")
	for range 60 {
		b.WriteString("A paragraph that only takes room.\n\n")
	}
	b.WriteString("## The end\n\nThe last line.\n")
	src := b.String()
	md := &Markdown{}
	view := func(c *Context) {
		Row(c).Gap(8).Children(func() {
			Scroll(c).Height(400).Width(380).Children(func() { md.ViewAs(c, "first", src) })
			Scroll(c).Height(400).Width(380).Children(func() { md.ViewAs(c, "second", src) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		// shown is the views, "first" or "second", whose heading shows.
		shown := func() (out []string) {
			for _, n := range tt.AccessAll() {
				if n.Role == ui.RoleHeading && n.Name == "The end" && n.Bounds.H > 0 && n.Bounds.Y < 460 {
					out = append(out, map[bool]string{false: "first", true: "second"}[n.Bounds.X > 300])
				}
			}
			return out
		}
		if got := shown(); len(got) != 0 {
			t.Fatalf("the end shows in %q before the click", got)
		}
		var second ui.AccessInfo
		for _, n := range tt.AccessAll() {
			if n.Role == ui.RoleLink && n.Name == "Go to the end" && n.Bounds.X > 300 {
				second = n
			}
		}
		if second.Bounds.W == 0 {
			t.Fatalf("no link in the second view: %q", Links(tt))
		}
		tt.ClickAt(second.Bounds.X+second.Bounds.W/2, second.Bounds.Y+second.Bounds.H/2)
		for range 3 {
			tt.Frame()
		}
		if got := shown(); !slices.Equal(got, []string{"second"}) {
			t.Errorf("the end shows in %q after a click in the second view", got)
		}
	})
}
