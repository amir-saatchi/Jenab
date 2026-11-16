package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTimeline(t *testing.T) {
	view := func(c *Context) {
		Column(c).Width(600).Gap(8).Children(func() {
			Button(c, "Before", Outline)
			TimeAxis(c, []AxisTick{{0, "0 s"}, {0.5, "2 s"}, {1, "4 s"}})
			Lane(c, "Model", func() {
				LaneBar(c, 0, 0.3, Blue, "req 1", "")
				LaneBar(c, 0.5, 0.4, Red, "req 2", "")
			})
			Lane(c, "Tools", func() { LaneBar(c, 0.32, 0.001, Green, persian, "fetch_url · 4 ms") })
			Button(c, "After", Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "0 s", "2 s", "4 s", "Model", "Tools", "req 1", "req 2")
		if _, ok := tt.Find(persian); !ok {
			t.Errorf("no bar named %q", persian)
		}
		// Bars take their place and width on the axis.
		r1, _ := tt.Find("req 1")
		r2, _ := tt.Find("req 2")
		model, _ := tt.Find("Model")
		track := 600 - 72 - 12
		start := model.X + 72 + 12
		if d := r2.X - start - 0.5*float32(track); d < -1 || d > 1 {
			t.Errorf("req 2 starts at %v, want %v", r2.X, start+0.5*float32(track))
		}
		if d := r1.W - 0.3*float32(track); d < -1 || d > 1 {
			t.Errorf("req 1 is %v wide, want %v", r1.W, 0.3*float32(track))
		}
		if b, _ := tt.Find(persian); b.W < 4 {
			t.Errorf("a short bar is %v wide, under 4 px", b.W)
		}
		// The last tick ends at the axis' end, the first starts at its
		// start.
		first, _ := tt.Find("0 s")
		last, _ := tt.Find("4 s")
		if first.X < start-1 || first.X > start+1 || last.X+last.W > start+float32(track)+1 || last.X+last.W < start+float32(track)-1 {
			t.Errorf("the ticks run from %v to %v, the axis from %v to %v", first.X, last.X+last.W, start, start+float32(track))
		}
		passesOver(t, tt)
		// The tip shows as the pointer rests on its bar.
		b, _ := tt.Find(persian)
		tt.Move(b.X+b.W/2, b.Y+b.H/2)
		tt.Advance(time.Second)
		tt.Frame()
		shows(t, tt, "fetch_url · 4 ms")
	})
}
