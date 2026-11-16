package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSwitch(t *testing.T) {
	var on bool
	changes := 0
	view := func(c *Context) {
		between(c, func() {
			Text(c, persian)
			if Switch(c, &on, "Desktop control").Changed() {
				changes++
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		on, changes = false, 0
		tt.Frame()
		shows(t, tt, persian, "Desktop control")
		tabTo(t, tt, "Desktop control")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if !on || changes != 1 {
			t.Errorf("Space left it %v after %d changes", on, changes)
		}
		if err := tt.Click("Desktop control"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if on || changes != 2 {
			t.Errorf("a click left it %v after %d changes", on, changes)
		}
	})
}
