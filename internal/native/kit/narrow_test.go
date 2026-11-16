package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TASK-062: the narrow window's pieces.

// A sheet runs along its edge, the window's full height, as wide as what
// it holds; the focus goes into it, and Escape closes it.
func TestSheet(t *testing.T) {
	var start, end bool
	view := func(c *Context) {
		Root(c, func() {
			Sheet(c, &start, false, "Start", func() {
				Column(c).Width(240).Grow(1).Padding(8).Children(func() { Button(c, "One", Ghost) })
			})
			Sheet(c, &end, true, "End", func() {
				Column(c).Width(320).Grow(1).Padding(8).Children(func() { Button(c, "Two", Ghost) })
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		for _, c := range []struct {
			open  *bool
			name  string
			x     float32
			w     float32
			first string
		}{{&end, "End", 800 - 321, 321, "Two"}, {&start, "Start", 0, 241, "One"}} {
			*c.open = true
			tt.Frame()
			tt.Frame()
			a, ok := tt.AccessByRole(ui.RoleDialog, c.name)
			if !ok {
				t.Fatalf("no sheet %q", c.name)
			}
			// The width holds the 1 px border on the side facing the window.
			if a.Bounds.X != c.x || a.Bounds.W != c.w || a.Bounds.Y != 0 || a.Bounds.H != 600 {
				t.Errorf("%s is at %+v", c.name, a.Bounds)
			}
			if !tt.Focused(c.first) {
				t.Errorf("the focus did not go into %s", c.name)
			}
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if *c.open {
				t.Errorf("Escape did not close %s", c.name)
			}
		}
	})
}

// Narrow is the window under 900 px.
func TestNarrow(t *testing.T) {
	var narrow bool
	tt := ui.NewTester(func(c *Context) { narrow = Narrow(c) }, 1280, 800)
	tt.Frame()
	if narrow {
		t.Error("1280 px is narrow")
	}
	for w, want := range map[int]bool{900: false, 899: true, 800: true} {
		tt.SetSize(w, 800)
		tt.Frame()
		if narrow != want {
			t.Errorf("%d px: narrow %v", w, narrow)
		}
	}
}
