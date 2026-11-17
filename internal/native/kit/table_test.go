package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTable(t *testing.T) {
	view := func(c *Context) {
		between(c, func() {
			Column(c).Width(500).Children(func() {
				Table(c, "By model", []TableColumn{{Title: "Model"}, {Title: "Tokens", Numeric: true}, {Title: "Cost", Numeric: true}},
					[][]string{{persian, "12.5K", "$0.04"}, {"gemini/flash", "950", "—"}})
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "Model", "Tokens", "Cost", persian, "12.5K", "$0.04", "gemini/flash", "—")
		if !HasRole(tt, "table", "By model") || !HasRole(tt, "columnheader", "Tokens") {
			t.Error("the table is not one for assistive technology")
		}
		passesOver(t, tt)
		// Numbers line up at the end of their column.
		a, _ := tt.Find("12.5K")
		b, _ := tt.Find("950")
		if d := (a.X + a.W) - (b.X + b.W); d > 1 || d < -1 {
			t.Errorf("the numbers end at %v and %v", a.X+a.W, b.X+b.W)
		}
	})
}

// TASK-069: a cut cell in any column shows its text whole in its tooltip;
// a cell that fits has none. TASK-075: a number is measured in its figures
// of one width: "11,111,111,111" takes about 60 px in the font's own
// figures and 101 in those, more than the column's 96.
func TestTableTooltips(t *testing.T) {
	big := "11,111,111,111"
	long := "gemini-3.5-flash-thinking-preview"
	path := `C:\Jenab\projects\demo\notes\reading-notes.md`
	view := func(c *Context) {
		between(c, func() {
			Column(c).Width(420).Children(func() {
				Table(c, "Files", []TableColumn{{Title: "File"}, {Title: "Model"}, {Title: "Tokens", Numeric: true}},
					[][]string{{path, long, "12.5K"}, {"a.md", "mini", "950"}, {"b.md", "mini", big}})
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		tt.Frame()
		for name, want := range map[string]string{path: path, long: long, "12.5K": "", "a.md": "", "mini": "", "950": "", big: big} {
			if d, ok := Description(tt, name); !ok || d != want {
				t.Errorf("the cell %q has the tooltip %q (found %v), want %q", name, d, ok, want)
			}
		}
	})
}
