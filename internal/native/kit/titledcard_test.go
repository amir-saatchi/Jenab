package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTitledCardAndShareBar(t *testing.T) {
	view := func(c *Context) {
		Column(c).Width(500).Gap(8).Children(func() {
			Button(c, "Before", Outline)
			TitledCard(c, "Requests", persian, func() {
				ShareBar(c, 0.25, "A quarter")
				ShareBar(c, 2, "Over full")
			})
			TitledCard(c, persian, "", nil)
			Button(c, "After", Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "Requests", persian, "A quarter")
		if !HasRole(tt, "heading", "Requests") || !HasRole(tt, "heading", persian) {
			t.Error("a card's title is no heading")
		}
		if _, ok := tt.AccessByRole(ui.RoleGroup, "Requests"); !ok {
			t.Error("the card is not named by its title")
		}
		// The bar is filled to its share, from its start.
		k := &Light
		if near(tt.Image().RGBAAt(799, 599), Dark.Background) {
			k = &Dark
		}
		img := tt.Image()
		r, ok := tt.Find("A quarter")
		if !ok {
			t.Fatal("no bar")
		}
		y := int(r.Y + r.H/2)
		if got := img.RGBAAt(int(r.X+r.W*0.1), y); !near(got, k.Primary) {
			t.Errorf("the filled part is %v, want primary %v", got, k.Primary)
		}
		if got := img.RGBAAt(int(r.X+r.W*0.6), y); !near(got, k.Muted) {
			t.Errorf("the rest is %v, want muted %v", got, k.Muted)
		}
		over, _ := tt.Find("Over full")
		if got := img.RGBAAt(int(over.X+over.W-2), int(over.Y+over.H/2)); !near(got, k.Primary) {
			t.Errorf("a share over 1 is not full: %v", got)
		}
		passesOver(t, tt)
	})
}
