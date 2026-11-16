package kit

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestBarChart(t *testing.T) {
	days := []string{"Nov 1", "Nov 2", "Nov 3"}
	format := func(v float64) string { return fmt.Sprintf("%gK", v/1000) }
	view := func(c *Context) {
		between(c, func() {
			Column(c).Width(500).Children(func() {
				BarChart(c, "Tokens per day", days, []Series{
					{Name: "Input", Tone: Blue, Values: []float64{4000, 0, 12000}},
					{Name: persian, Tone: Purple, Values: []float64{1000, 0, 3000}},
				}, format)
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		shows(t, tt, "Input", persian, "Nov 1", "Nov 3", "0K", "15K")
		k := &Light
		if near(tt.Image().RGBAAt(799, 599), Dark.Background) {
			k = &Dark
		}
		for _, tone := range []Tone{Blue, Purple} {
			if !paints(tt, k.Tone(tone).Text) {
				t.Errorf("no bar in the %s tone", tone)
			}
		}

		tabTo(t, tt, "Tokens per day")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		first := "Nov 1: Input 4K, " + persian + " 1K"
		shows(t, tt, first)
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		last := "Nov 3: Input 12K, " + persian + " 3K"
		shows(t, tt, last)
		if a := tt.Announcements(); !slices.ContainsFunc(a, func(s string) bool { return strings.Contains(s, "Nov 3") }) {
			t.Errorf("the choice was not read out: %q", a)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if Shows(tt, last) {
			t.Error("Escape kept the choice")
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not leave the chart")
		}
	})
}

// paints tells whether the window has a pixel of col.
func paints(tt *ui.Tester, col Color) bool {
	img := tt.Image()
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			if near(img.RGBAAt(x, y), col) {
				return true
			}
		}
	}
	return false
}

func TestFewTicks(t *testing.T) {
	var ticks []chart.Tick
	for i := range 90 {
		ticks = append(ticks, chart.Tick{Label: fmt.Sprint(i)})
	}
	got := fewTicks(ticks, 8)
	if len(got) > 8 || got[len(got)-1].Label != "89" {
		t.Errorf("fewTicks kept %d, the last %q", len(got), got[len(got)-1].Label)
	}
	if len(fewTicks(ticks[:5], 8)) != 5 {
		t.Error("fewTicks dropped ticks it had room for")
	}
}
