package kit

import (
	"image"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLinesView(t *testing.T) {
	lines := []string{"── system · turn 3", persian, "", "\tindented"}
	for i := range 20_000 {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	var s ListState
	view := func(c *Context) {
		Column(c).Width(600).Height(400).Gap(8).Children(func() {
			Button(c, "Before", Outline)
			LinesView(c, &s, lines, "Block text").Grow(1)
			Button(c, "After", Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		s = ListState{}
		tt.Frame()
		shows(t, tt, "── system · turn 3", persian, "line 0")
		// Only the lines in view are built.
		if tt.HasText("line 19999") || tt.HasText("line 500") {
			t.Error("lines far out of view were built")
		}
		if _, ok := tt.AccessByRole(ui.RoleList, "Block text"); !ok {
			t.Error("no list named Block text")
		}
		// A Persian line reads right to left: its ink is on the right of
		// the box, an English line's on the left.
		if l, r := lineInk(tt, persian); r < 560 || l < 100 {
			t.Errorf("the Persian line's ink runs from %d to %d", l, r)
		}
		if l, r := lineInk(tt, "line 0"); l > 30 || r > 300 {
			t.Errorf("an English line's ink runs from %d to %d", l, r)
		}
		// The keyboard reaches the list, and End goes to its last line.
		tabTo(t, tt, "Block text")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		tt.Frame()
		shows(t, tt, "line 19999")
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not leave the list")
		}
	})
}

// lineInk is where the ink of the text s runs across the window, left and
// right.
func lineInk(tt *ui.Tester, s string) (int, int) {
	r, ok := tt.Find(s)
	if !ok {
		return -1, -1
	}
	img := tt.Image().SubImage(image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H))).(*image.RGBA)
	b, _ := ink(img)
	return b.Min.X, b.Max.X
}
