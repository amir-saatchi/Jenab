package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDataTable(t *testing.T) {
	chosen := 0
	cols := []DataColumn{{Title: "#", Width: 48}, {Title: "Tool"}, {Title: "Time", Width: 80, End: true}}
	view := func(c *Context) {
		Column(c).Width(500).Gap(8).Children(func() {
			Button(c, "Before", Outline)
			DataTable(c, "Tool calls", cols, func() {
				for i, name := range []string{"fetch_url", persian} {
					if DataRow(c, cols, i == chosen, func(col int) {
						switch col {
						case 0:
							if Button(c, itoa(i+1), Ghost).Label("Request " + itoa(i+1)).Clicked() {
								chosen = i
							}
						case 1:
							Mono(c, name)
						case 2:
							Text(c, []string{"640 ms", "38 ms"}[i])
						}
					}).Key(i).Clicked() {
						chosen = i
					}
				}
				DataSpan(c, func() { Text(c, "Can't reach the provider") })
				DataFoot(c, cols, func(col int) {
					if col == 1 {
						Text(c, "Total")
					}
				})
			})
			Button(c, "After", Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		chosen = 0
		shows(t, tt, "#", "Tool", "Time", "fetch_url", persian, "640 ms", "38 ms", "Can't reach the provider", "Total")
		if _, ok := tt.AccessByRole(ui.RoleTable, "Tool calls"); !ok {
			t.Error("no table named Tool calls")
		}
		// The cells of a column line up: the end column ends at one edge.
		a, _ := tt.Find("Time")
		var ends []float32
		for _, s := range []string{"640 ms", "38 ms"} {
			r, _ := tt.Find(s)
			ends = append(ends, r.X+r.W)
		}
		if len(ends) != 2 || ends[0] != ends[1] || ends[0] < a.X+a.W-1 || ends[0] > a.X+a.W+1 {
			t.Errorf("the end column's cells end at %v, its title at %v", ends, a.X+a.W)
		}
		tabTo(t, tt, "Request 2")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if chosen != 1 {
			t.Errorf("Enter on the row's button chose %d", chosen)
		}
		if err := tt.Click("fetch_url"); err != nil || chosen != 0 {
			t.Errorf("a click on the row: %v, chose %d", err, chosen)
		}
		tabTo(t, tt, "Request 2")
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not leave the table")
		}
	})
}
