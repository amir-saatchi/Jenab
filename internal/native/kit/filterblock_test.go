package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// filterView builds the filters of specs in a FilterBar, between two
// buttons, each editing its value, and counts the changes.
func filterView(specs []FilterSpec, values []any, changes *int) func(c *Context) {
	return func(c *Context) {
		Column(c).Padding(16).Gap(12).AlignItems(Stretch).Children(func() {
			between(c, func() {
				FilterBar(c, "Filters", func() {
					for i, s := range specs {
						if Filter(c, s, &values[i]).Changed {
							*changes++
						}
					}
				})
			})
		})
	}
}

func TestFilterText(t *testing.T) {
	specs := []FilterSpec{
		{Label: "Keyword", Control: FilterText},
		{Label: "Owner", Control: FilterText, Required: true},
	}
	values := make([]any, 2)
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		values[0], values[1], changes = nil, "ops", 0
		tt.Frame()
		if _, ok := tt.AccessByRole(ui.RoleSearch, "Filters"); !ok {
			t.Error("the filters are no search landmark")
		}
		if a, _ := tt.AccessByRole(ui.RoleTextField, "Keyword"); a.Placeholder != "Any" {
			t.Errorf("the optional field's placeholder is %q", a.Placeholder)
		}
		// Persian text commits on Enter.
		tabTo(t, tt, "Keyword")
		tt.Type(persian)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if values[0] != persian || changes != 1 {
			t.Errorf("Enter bound %#v, %d changes", values[0], changes)
		}
		// Emptied and left, an optional filter binds NULL.
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if values[0] != nil || changes != 2 {
			t.Errorf("an emptied filter bound %#v, %d changes", values[0], changes)
		}
		// A required one goes back, marked.
		if !tt.Focused("Owner") {
			t.Fatal("Tab did not go on to Owner")
		}
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		a, _ := tt.AccessByRole(ui.RoleTextField, "Owner")
		if values[1] != "ops" || a.Value != "ops" || !a.Invalid || a.Description != "Required" {
			t.Errorf("an emptied required filter bound %#v, reads %+v", values[1], a)
		}
		// Escape puts back what was typed.
		tt.Type("x")
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if a, _ := tt.AccessByRole(ui.RoleTextField, "Owner"); a.Value != "ops" {
			t.Errorf("Escape left %q", a.Value)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") || changes != 2 {
			t.Errorf("Tab did not leave the filters, or %d changes", changes)
		}
	})
}

func TestFilterNumber(t *testing.T) {
	specs := []FilterSpec{{Label: "Min price", Control: FilterNumber}}
	values := make([]any, 1)
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		values[0], changes = int64(5), 0
		tt.Frame()
		shows(t, tt, "Min price")
		for _, tc := range []struct {
			typed string
			want  any
		}{
			{"1,250", int64(1250)},
			{"۱۲٫۵", 12.5},
			{"-3", int64(-3)},
			{"−3", int64(-3)},
			{"", nil},
		} {
			tabTo(t, tt, "Min price")
			tt.Key(ui.Ctrl, ui.KeyA)
			tt.Key(0, ui.KeyBackspace)
			tt.Type(tc.typed)
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			if values[0] != tc.want {
				t.Errorf("%q bound %#v", tc.typed, values[0])
			}
		}
		// A word goes back, marked.
		values[0] = int64(7)
		tt.Frame()
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("lots")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		a, _ := tt.AccessByRole(ui.RoleTextField, "Min price")
		if values[0] != int64(7) || a.Value != "7" || a.Description != "Not a number; write it as 1,234.5" {
			t.Errorf("a word bound %#v, reads %+v", values[0], a)
		}
	})
}

func TestFilterDate(t *testing.T) {
	specs := []FilterSpec{
		{Label: "From", Control: FilterDate},
		{Label: "To", Control: FilterDate, Required: true},
	}
	values := make([]any, 2)
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		// The relative default is shown as the date it stands for.
		from, _ := RelativeDate("today-90d", time.Date(2026, 11, 20, 15, 0, 0, 0, time.Local))
		values[0], values[1], changes = from.Format(time.DateOnly), "2026-11-20", 0
		tt.Frame()
		shows(t, tt, "Aug 22, 2026", "Nov 20, 2026")
		if a, _ := tt.AccessByRole(ui.RoleButton, "From"); a.Value != "Aug 22, 2026" {
			t.Errorf("the date reads %q", a.Value)
		}
		// Enter opens the calendar on the date; the arrows move and Enter
		// chooses the day.
		tabTo(t, tt, "From")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if a, _ := tt.AccessByRole(ui.RoleButton, "From"); !a.Expanded {
			t.Fatal("Enter did not open the calendar")
		}
		tt.Key(0, ui.KeyRight)
		tt.Frame()
		if values[0] != "2026-08-22" {
			t.Errorf("an arrow chose %#v", values[0])
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if values[0] != "2026-08-23" || changes != 1 {
			t.Errorf("Enter chose %#v, %d changes", values[0], changes)
		}
		if a, _ := tt.AccessByRole(ui.RoleButton, "From"); a.Expanded {
			t.Error("the calendar stayed open")
		}
		// An optional date clears to NULL; a required one has no clear.
		if err := tt.Click("Clear From"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if values[0] != nil {
			t.Errorf("Clear bound %#v", values[0])
		}
		shows(t, tt, "Any date")
		if _, ok := tt.AccessByRole(ui.RoleButton, "Clear To"); ok {
			t.Error("a required date can be cleared")
		}
		// Escape closes the calendar without choosing.
		if err := tt.Click("Nov 20, 2026"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Key(0, ui.KeyLeft)
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if values[1] != "2026-11-20" {
			t.Errorf("Escape bound %#v", values[1])
		}
	})
}

func TestFilterSelectAndCheckbox(t *testing.T) {
	specs := []FilterSpec{
		{Label: "Coin", Control: FilterSelect, Options: []string{"BTC", "ETH", "طلا"}},
		{Label: "Status", Control: FilterSelect, Options: []string{"open", "closed"}, Required: true},
		{Label: "Only mine", Control: FilterCheckbox},
		{Label: "Archived", Control: FilterCheckbox, Required: true},
	}
	values := make([]any, 4)
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		values[0], values[1], values[2], values[3], changes = nil, "open", nil, false, 0
		tt.Frame()
		shows(t, tt, "Any", "open")
		// The keyboard chooses: Down opens at Any, Down twice to ETH.
		tabTo(t, tt, "Coin")
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		shows(t, tt, "BTC", "ETH", "طلا")
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if values[0] != "ETH" || changes != 1 {
			t.Errorf("chose %#v, %d changes", values[0], changes)
		}
		// Any binds NULL.
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyHome)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if values[0] != nil {
			t.Errorf("Any bound %#v", values[0])
		}
		// A required select offers no Any.
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		if tt.HasText("Any") && countText(tt, "Any") > 1 {
			t.Error("the required select offers Any")
		}
		tt.Key(0, ui.KeyEscape)
		// Check boxes: Space checks; unchecked, an optional one binds
		// NULL and a required one false.
		tabTo(t, tt, "Only mine")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if values[2] != true {
			t.Errorf("checked bound %#v", values[2])
		}
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if values[2] != nil {
			t.Errorf("unchecked optional bound %#v", values[2])
		}
		tabTo(t, tt, "Archived")
		tt.Key(0, ui.KeySpace)
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if values[3] != false {
			t.Errorf("unchecked required bound %#v", values[3])
		}
	})
}

// countText counts the window's texts that are s.
func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, t := range tt.Texts() {
		if t == s {
			n++
		}
	}
	return n
}

// TestFilterPersian: a filter labelled in Persian lines up from the
// right, its field too.
func TestFilterPersian(t *testing.T) {
	specs := []FilterSpec{
		{Label: "سکه", Control: FilterSelect, Options: []string{"طلا", "نقره"}},
		{Label: "از تاریخ", Control: FilterDate},
	}
	values := []any{"طلا", nil}
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		tt.Frame()
		shows(t, tt, "سکه", "طلا", "از تاریخ", "Any date")
		label, _ := tt.Find("سکه")
		field, _ := tt.AccessByRole(ui.RolePopUpButton, "سکه")
		// The label sits at the field's right.
		if label.X+label.W < field.Bounds.X+field.Bounds.W-2 {
			t.Errorf("the label is not at the right: %v, field %v", label, field.Bounds)
		}
		tabTo(t, tt, "از تاریخ")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if a, _ := tt.AccessByRole(ui.RoleButton, "از تاریخ"); !a.Expanded {
			t.Error("Space did not open the calendar")
		}
	})
}

func TestRelativeDate(t *testing.T) {
	today := time.Date(2026, 3, 31, 17, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		expr, want string
	}{
		{"today", "2026-03-31"},
		{"today-7d", "2026-03-24"},
		{"today-90d", "2025-12-31"},
		{"today-1m", "2026-03-03"}, // as SQLite: Feb 31 runs on
		{"today-3m", "2025-12-31"},
		{"today-1y", "2025-03-31"},
		{"start_of_month", "2026-03-01"},
		{"start_of_year", "2026-01-01"},
		{"2026-11-20", "2026-11-20"},
		{" today ", "2026-03-31"},
	} {
		got, ok := RelativeDate(tc.expr, today)
		if !ok || got.Format(time.DateOnly) != tc.want || got.Hour() != 0 {
			t.Errorf("RelativeDate(%q) = %v, %v; want %s", tc.expr, got, ok, tc.want)
		}
	}
	for _, bad := range []string{"", "yesterday", "today-7", "today-xd", "today+7d", "today-+7d", "today-7w", "2026-13-01"} {
		if _, ok := RelativeDate(bad, today); ok {
			t.Errorf("RelativeDate(%q) read", bad)
		}
	}
}
