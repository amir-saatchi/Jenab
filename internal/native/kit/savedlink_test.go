package kit

import (
	"image"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// savedLinks are a link of each kind the Links tab shows (P3-11).
var savedLinks = []struct {
	url string
	f   SavedLinkFace
}{
	{"https://example.com/a", SavedLinkFace{Title: "Bitcoin news", Host: "example.com", Note: "Daily prices",
		Tags: []string{"btc", "خبر"}, Saved: "Saved by you · Nov 16, 2026"}},
	{"https://news.example.ir/x", SavedLinkFace{Title: persian, Host: "news.example.ir", Note: "یادداشتی کوتاه درباره‌ی بازار",
		Saved: "Saved by the agent · Nov 16, 2026"}},
	{"javascript:alert(1)", SavedLinkFace{Host: "javascript:", Blocked: true}},
}

// inkSides counts the pixels unlike bg in the first and the last third of
// r in img.
func inkSides(img *image.RGBA, r ui.Rect, bg Color) (start, end int) {
	x0, x1 := int(r.X), int(r.X+r.W)
	third := (x1 - x0) / 3
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := x0; x < x1; x++ {
			if near(img.RGBAAt(x, y), bg) {
				continue
			}
			switch {
			case x < x0+third:
				start++
			case x >= x1-third:
				end++
			}
		}
	}
	return start, end
}

// A saved link shows its title, host, note, tags and who saved it; the
// rows are reached by the keyboard and found by their titles; only a safe
// link opens; Persian titles and notes start on the right.
func TestSavedLink(t *testing.T) {
	var opened []bool
	view := func(c *Context) {
		between(c, func() {
			Tree(c, "Links", func(rows TreeRows) {
				for _, l := range savedLinks {
					label := l.f.Title
					if label == "" {
						label = l.f.Host
					}
					r := rows.Item(l.url, label, nil, false, func() { SavedLink(c, l.f) }, nil)
					if r.Clicked() {
						opened = append(opened, OpenLink(c, l.url))
					}
				}
			}).Width(420)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		opened = nil
		shows(t, tt, "Bitcoin news", "example.com", "Daily prices", "btc", "خبر", "Saved by you · Nov 16, 2026",
			persian, "news.example.ir", "یادداشتی کوتاه درباره‌ی بازار", "javascript:", "Doesn't open")
		if _, ok := tt.AccessByRole(ui.RoleTree, "Links"); !ok {
			t.Fatal("no tree named Links")
		}
		for _, name := range []string{"Bitcoin news", persian, "javascript:"} {
			if _, ok := tt.AccessByRole(ui.RoleTreeItem, name); !ok {
				t.Errorf("no row named %q", name)
			}
		}
		// The keyboard opens the first; the unsafe one opens nothing.
		tabTo(t, tt, "Bitcoin news")
		tt.Key(0, ui.KeyEnter)
		tt.Key(0, ui.KeyEnd)
		if !tt.Focused("javascript:") {
			t.Fatal("End did not move to the last row")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if !slices.Equal(opened, []bool{true, false}) {
			t.Errorf("opened %v", opened)
		}
		if got := tt.OpenedURLs(); !slices.Equal(got, []string{"https://example.com/a"}) {
			t.Errorf("the system opened %q", got)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab stopped inside the list")
		}
		// Direction: the Persian title and note on the right, the English
		// ones on the left.
		bg := Light.Background
		if near(tt.Image().RGBAAt(799, 599), Dark.Background) {
			bg = Dark.Background
		}
		img := tt.Image()
		// A title names its row, so Find gives the row's box: its first
		// line, the title's, is what counts.
		for _, s := range []struct {
			text  string
			rtl   bool
			title bool
		}{{"Bitcoin news", false, true}, {"Daily prices", false, false}, {persian, true, true}, {"یادداشتی کوتاه درباره‌ی بازار", true, false}} {
			r, ok := tt.Find(s.text) // a title names the row
			if !ok {
				r, ok = tt.Find(isolate(s.text)) // the text is drawn isolated
			}
			if !ok {
				t.Fatalf("no %q", s.text)
			}
			if s.title {
				r.H = 24
			}
			start, end := inkSides(img, r, bg)
			if s.rtl && (end == 0 || start > end/4) || !s.rtl && (start == 0 || end > start/4) {
				t.Errorf("%q: ink %d at the start, %d at the end", s.text, start, end)
			}
		}
	})
}
