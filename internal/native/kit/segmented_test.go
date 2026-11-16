package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSegmented(t *testing.T) {
	value := "standard"
	choices := []Choice{{"strict", "Strict"}, {"standard", "Standard"}, {"auto", persian}}
	view := func(c *Context) {
		between(c, func() { Segmented(c, &value, choices, "Approval level") })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		value = "standard"
		tt.Frame()
		shows(t, tt, "Strict", "Standard", persian)
		// One stop of Tab, at the segment chosen.
		tabTo(t, tt, "Standard")
		tt.Key(0, ui.KeyRight)
		tt.Frame()
		if value != "auto" {
			t.Errorf("Right chose %q", value)
		}
		if err := tt.Click("Strict"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if value != "strict" {
			t.Errorf("a click chose %q", value)
		}
	})
}
