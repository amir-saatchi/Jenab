package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMeter(t *testing.T) {
	f := 0.5
	view := func(c *Context) {
		Column(c).Width(300).Gap(8).Children(func() {
			Text(c, persian)
			Meter(c, f, "Memory used")
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, persian, "Memory used")
		k := &Light
		if near(tt.Image().RGBAAt(799, 599), Dark.Background) {
			k = &Dark
		}
		if !paints(tt, k.Primary) {
			t.Error("a half-full meter is not in the primary color")
		}
		f = 0.9
		tt.Frame()
		if !paints(tt, k.Tone(Amber).Text) {
			t.Error("a nearly full meter is not amber")
		}
		f = 0.5
	})
}
