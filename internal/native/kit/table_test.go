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
