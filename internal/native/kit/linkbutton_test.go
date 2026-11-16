package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLinkButton(t *testing.T) {
	clicks := map[string]int{}
	view := func(c *Context) {
		between(c, func() {
			if LinkButton(c, "2", "Request 2").Clicked() {
				clicks["2"]++
			}
			if LinkButton(c, persian, "").Clicked() {
				clicks[persian]++
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clear(clicks)
		shows(t, tt, "2", persian)
		// It is named by its label, or by its text without one.
		if _, ok := tt.AccessByRole(ui.RoleButton, "Request 2"); !ok {
			t.Error("no button named Request 2")
		}
		if _, ok := tt.AccessByRole(ui.RoleButton, persian); !ok {
			t.Error("no button named by its Persian text")
		}
		if err := tt.Click(persian); err != nil {
			t.Fatal(err)
		}
		tabTo(t, tt, "Request 2")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if clicks["2"] != 1 || clicks[persian] != 1 {
			t.Errorf("clicks %v", clicks)
		}
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not go on past the links")
		}
	})
}
