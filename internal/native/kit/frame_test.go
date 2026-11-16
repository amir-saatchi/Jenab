package kit

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRail(t *testing.T) {
	clicked := ""
	view := func(c *Context) {
		Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			Rail(c, "Projects", func() {
				if RailTile(c, persian, true, func() { TileFace(c, "س", true, true) }).Clicked() {
					clicked = persian
				}
				Separator(c, 24, false)
				RailTile(c, "Mother", false, func() { IconFace(c, IconSparkles) })
				Spacer(c)
				RailTile(c, "Settings", false, func() { IconFace(c, IconSettings) })
			})
			Main(c, func() {
				Bar(c, func() { Title(c, persian); Muted(c, "Project home") })
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clicked = ""
		shows(t, tt, "Projects", persian, "Mother", "Settings", "Project home")
		tabTo(t, tt, persian)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if clicked != persian {
			t.Error("Enter on a tile did not click it")
		}
		tabTo(t, tt, "Settings")
		top, _ := tt.Find("Mother")
		bottom, _ := tt.Find("Settings")
		if bottom.Y < 500 || top.Y > 200 {
			t.Errorf("the spacer did not push Settings down: Mother at %v, Settings at %v", top, bottom)
		}
	})
}

// A list of 10,000 rows builds only those in view.
func TestList(t *testing.T) {
	var s ListState
	built := 0
	view := func(c *Context) {
		built = 0
		List(c, &s, 10000, func(i int) {
			built++
			Text(c, fmt.Sprintf("%s %d", persian, i)).Height(24)
		}).Grow(1).Label("Messages")
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		s = ListState{}
		tt.Frame()
		shows(t, tt, "Messages", persian+" 0")
		if built > 100 {
			t.Errorf("built %d rows of 10,000", built)
		}
		s.ScrollToEnd()
		tt.Frame()
		tt.Frame()
		shows(t, tt, persian+" 9999")
	})
}
