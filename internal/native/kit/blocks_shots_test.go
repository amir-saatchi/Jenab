package kit

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestBlockShots saves the table, stat and filter blocks, light and dark,
// as blocks-<name>-<theme>.png in the folder JENAB_SHOTS names, for a
// look outside the repo; without it, it is skipped.
func TestBlockShots(t *testing.T) {
	dir := os.Getenv("JENAB_SHOTS")
	if dir == "" {
		t.Skip("JENAB_SHOTS is not set")
	}
	save := func(t *testing.T, tt *ui.Tester, name string) {
		t.Helper()
		theme := "light"
		if darkOf(tt) {
			theme = "dark"
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, tt.Image()); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "blocks-"+name+"-"+theme+".png"), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A page: filters, stats, and a table with its actions.
	var st TableState
	spec := formatsSpec()
	spec.Actions = []RowAction{{Label: "Edit"}, {Label: "Delete", Bulk: true, Destructive: true}}
	spec.State.Notice = "Showing the first 10,000 rows. Aggregate in the query to see them all."
	spec.Total = ManyRows
	filters := []FilterSpec{
		{Label: "Keyword", Control: FilterText},
		{Label: "Min price", Control: FilterNumber},
		{Label: "From", Control: FilterDate},
		{Label: "Coin", Control: FilterSelect, Options: []string{"BTC", "ETH"}},
		{Label: "Only open", Control: FilterCheckbox},
	}
	values := []any{nil, int64(100), "2026-08-22", "BTC", true}
	page := func(c *Context) {
		Column(c).Padding(16).Gap(16).AlignItems(Stretch).Grow(1).Children(func() {
			FilterBar(c, "Filters", func() {
				for i, f := range filters {
					Filter(c, f, &values[i])
				}
			})
			Row(c).Gap(16).Children(func() {
				for _, s := range []StatSpec{
					{Label: "Bitcoin", Value: 91842.5, Format: FormatCurrency, Options: FormatOptions{Currency: "USD"},
						Change: 0.025, ChangeFormat: FormatPercent, Caption: "Against yesterday"},
					{Label: "Open tickets", Value: 42, Format: FormatNumber, Change: -3, ChangeFormat: FormatNumber, Caption: "This week"},
					{Label: "قیمت طلا", Value: 1234, Format: FormatNumber, Change: 0, ChangeFormat: FormatNumber, Caption: "در برابر دیروز"},
				} {
					Column(c).Grow(1).Padding(16).Border(1, TokensOf(c).Border).Radius(Radius).AlignItems(Stretch).Children(func() {
						StatBlock(c, s)
					})
				}
			})
			Column(c).Height(300).AlignItems(Stretch).Children(func() { TableBlock(c, &st, spec) })
		})
	}
	// A Persian table, a stat loading and one that failed, and a date
	// filter's calendar open.
	var fa TableState
	persianSpec := TableSpec{
		Label:   "قیمت‌ها",
		Columns: []BlockColumn{{Field: "name", Label: "نام"}, {Field: "price", Label: "قیمت", Format: FormatNumber}, {Field: "day", Label: "روز", Format: FormatDate}},
		Rows:    [][]any{{"طلا", 1200, "2026-11-20"}, {"نقره", 30.5, "2026-11-19"}, {"Platinum", 950, nil}},
		Total:   3,
	}
	var date any = "2026-11-20"
	others := func(c *Context) {
		Column(c).Padding(16).Gap(16).AlignItems(Stretch).Grow(1).Children(func() {
			Row(c).Gap(16).Children(func() {
				Column(c).Grow(1).Padding(16).Border(1, TokensOf(c).Border).Radius(Radius).AlignItems(Stretch).Children(func() {
					StatBlock(c, StatSpec{Label: "Bitcoin", State: BlockState{Loading: true}})
				})
				Column(c).Grow(1).Padding(16).Border(1, TokensOf(c).Border).Radius(Radius).AlignItems(Stretch).Children(func() {
					StatBlock(c, StatSpec{Label: "Bitcoin", State: BlockState{Error: "The query returned 3 rows; a stat needs one."}})
				})
			})
			Column(c).Height(200).AlignItems(Stretch).Children(func() { TableBlock(c, &fa, persianSpec) })
			FilterBar(c, "Filters", func() { Filter(c, FilterSpec{Label: "From", Control: FilterDate}, &date) })
		})
	}
	themes(t, page, func(t *testing.T, tt *ui.Tester) {
		st = TableState{}
		tt.SetSize(1000, 720)
		tt.Frame()
		tt.Click("Choose row 1")
		tt.Frame()
		tt.Move(400, 700)
		tt.Frame()
		save(t, tt, "page")
	})
	themes(t, others, func(t *testing.T, tt *ui.Tester) {
		tt.SetSize(1000, 720)
		tt.Frame()
		from, ok := tt.AccessByRole(ui.RoleButton, "From")
		if !ok {
			t.Fatal("no date filter")
		}
		tt.ClickAt(from.Bounds.X+from.Bounds.W/2, from.Bounds.Y+from.Bounds.H/2)
		tt.Frame()
		save(t, tt, "states")
	})
}
