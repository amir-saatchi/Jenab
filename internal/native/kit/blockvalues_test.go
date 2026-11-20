package kit

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestFormatValueEdges: a percent shows up to one decimal; NaN and the
// infinities are a dash; a zero time is blank; a signed number has its
// plus.
func TestFormatValueEdges(t *testing.T) {
	zero := 0
	for _, tc := range []struct {
		v    any
		f    CellFormat
		o    FormatOptions
		want string
	}{
		{0.004, FormatPercent, FormatOptions{}, "0.4%"},
		{0.256, FormatPercent, FormatOptions{}, "25.6%"},
		{0.25, FormatPercent, FormatOptions{}, "25%"},
		{0.256, FormatPercent, FormatOptions{Decimals: &zero}, "26%"},
		{math.NaN(), FormatNumber, FormatOptions{}, NoNumber},
		{math.Inf(1), FormatCurrency, FormatOptions{Currency: "USD"}, NoNumber},
		{float32(math.Inf(-1)), FormatPercent, FormatOptions{}, NoNumber},
		{math.NaN(), FormatText, FormatOptions{}, NoNumber},
		{time.Time{}, FormatDate, FormatOptions{}, ""},
		{time.Time{}, FormatDateTime, FormatOptions{}, ""},
		{time.Time{}, FormatText, FormatOptions{}, ""},
	} {
		if got := formatValue("en", tc.v, tc.f, tc.o, false); got != tc.want {
			t.Errorf("%v as %q is %q, want %q", tc.v, tc.f, got, tc.want)
		}
	}
	if got := formatValue("en", 5, FormatNumber, FormatOptions{}, true); got != "+5" {
		t.Errorf("a signed 5 is %q", got)
	}
}

// TestTimeOf: a time.Time and text with a zone are the same instant,
// shown in the computer's zone; text without one is UTC; a day, as a
// date alone or midnight in its own zone, stays the day it is.
func TestTimeOf(t *testing.T) {
	local := time.Local
	defer func() { time.Local = local }()
	time.Local = time.FixedZone("Test", -5*3600)

	tehran := time.FixedZone("Tehran", 3*3600+1800)
	at := time.Date(2026, 11, 20, 8, 0, 0, 0, tehran)
	for _, v := range []any{at, "2026-11-20T08:00:00+03:30", "2026-11-20 08:00:00+03:30", []byte("2026-11-20T04:30:00Z")} {
		got, ok := TimeOf(v)
		if !ok || !got.Equal(at) || got.Location() != time.Local {
			t.Errorf("%v read as %v, %v", v, got, ok)
		}
		if s := formatValue("en", v, FormatDateTime, FormatOptions{}, false); s != "Nov 19, 2026, 11:30 PM" {
			t.Errorf("%v shows as %q", v, s)
		}
	}
	// No zone: UTC, as datetime('now') writes it.
	got, ok := TimeOf("2026-11-20 08:00:00")
	if want := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC); !ok || !got.Equal(want) || got.Location() != time.Local {
		t.Errorf("a zoneless time read as %v", got)
	}
	// A day is the same day in every zone, with no time of day.
	for _, v := range []any{"2026-11-19", time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC), "2026-11-19T00:00:00Z", "2026-11-19 00:00:00"} {
		for _, f := range []CellFormat{FormatDate, FormatDateTime} {
			if s := formatValue("en", v, f, FormatOptions{}, false); s != "Nov 19, 2026" {
				t.Errorf("the day %v as %q shows as %q", v, f, s)
			}
		}
	}
	for _, v := range []any{time.Time{}, "soon", "", nil, 42} {
		if _, ok := TimeOf(v); ok {
			t.Errorf("%#v read as a time", v)
		}
	}
}

// TestFilterReadsNumbers: a number is read as the locale writes it, its group
// separator only where its grouping puts it, with either minus.
func TestFilterReadsNumbers(t *testing.T) {
	nnbsp := string(rune(0x202f))
	for _, tc := range []struct {
		locale, s string
		want      any // nil: refused
	}{
		{"en", "1,250", int64(1250)},
		{"en", "1,234,567.5", 1234567.5},
		{"en", "1234567", int64(1234567)},
		{"en", "1,5", nil},
		{"en", "1,25", nil},
		{"en", "1,2345", nil},
		{"en", "12,34,567", nil},
		{"en", ",250", nil},
		{"en", "1.5", 1.5},
		{"en", "-3", int64(-3)},
		{"en", "−3", int64(-3)},
		{"en", "−1,250.5", -1250.5},
		{"en", "3-", nil},
		{"en", "--3", nil},
		{"en", "1.", nil},
		{"en", "1e5", nil},
		{"en", "NaN", nil},
		{"en", "1 250", nil},
		{"en", "۱۲٫۵", 12.5},
		{"en", "۱٬۲۵۰", int64(1250)},
		{"en", "99999999999999999999", 1e20},
		{"de", "1.234,5", 1234.5},
		{"de", "1,5", 1.5},
		{"de", "1.234", int64(1234)},
		{"de", "1.5", nil},
		{"de", "−1.234", int64(-1234)},
		{"fa", "۱٬۲۳۴٫۵", 1234.5},
		{"fa", "1,234.5", 1234.5},
		{"fa", "۱,۵", nil},
		{"fa", "‎−۱۲", int64(-12)},
		{"fr", "1 234,5", 1234.5},
		{"fr", "1" + nnbsp + "234", int64(1234)},
		{"fr", "1,5", 1.5},
		{"fr", "1 23", nil},
		{"en-IN", "12,34,567", int64(1234567)},
		{"en-IN", "1,234", int64(1234)},
		{"en-IN", "1,234,567", nil},
		{"de-CH", "1'234.5", 1234.5},
		{"de-CH", "1’234", int64(1234)},
		{"de-CH", "1,234", int64(1234)},
	} {
		got, ok := readNumber(tc.s, tc.locale)
		if tc.want == nil {
			if ok {
				t.Errorf("%s: %q read as %#v", tc.locale, tc.s, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("%s: %q read as %#v, %v; want %#v", tc.locale, tc.s, got, ok, tc.want)
		}
	}
}

// TestFilterRequiredFallsBack: a required select with no value takes its
// first option, and a required check box false, each once.
func TestFilterRequiredFallsBack(t *testing.T) {
	specs := []FilterSpec{
		{Label: "Status", Control: FilterSelect, Options: []string{"open", "closed"}, Required: true},
		{Label: "Archived", Control: FilterCheckbox, Required: true},
		{Label: "Kind", Control: FilterSelect, Required: true},
	}
	values := make([]any, 3)
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		values[0], values[1], values[2], changes = nil, nil, nil, 0
		tt.Frame()
		if values[0] != "open" || values[1] != false || values[2] != nil || changes != 2 {
			t.Errorf("required filters bound %#v, %d changes", values, changes)
		}
		tt.Frame()
		if changes != 2 {
			t.Errorf("%d changes in the next frame", changes)
		}
		if a, _ := tt.AccessByRole(ui.RolePopUpButton, "Status"); a.Value != "open" {
			t.Errorf("the select reads %q", a.Value)
		}
	})
}

// TestFilterProblemShows: why a filter went back is text under it, seen
// without hovering and read with the field.
func TestFilterProblemShows(t *testing.T) {
	specs := []FilterSpec{
		{Label: "Owner", Control: FilterText, Required: true},
		{Label: "Min price", Control: FilterNumber},
	}
	values := []any{"ops", int64(5)}
	changes := 0
	themes(t, filterView(specs, values, &changes), func(t *testing.T, tt *ui.Tester) {
		values[0], values[1], changes = "ops", int64(5), 0
		tt.Frame()
		tabTo(t, tt, "Owner")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		shows(t, tt, "Required")
		if a, _ := tt.AccessByRole(ui.RoleTextField, "Owner"); !a.Invalid || a.Description != "Required" {
			t.Errorf("the emptied field reads %+v", a)
		}
		// The next good commit takes the text away.
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("dev")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if values[0] != "dev" || Shows(tt, "Required") {
			t.Errorf("bound %#v, Required shows: %v", values[0], Shows(tt, "Required"))
		}
		// "1,5" is no number in English: refused, with an example.
		tabTo(t, tt, "Min price")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("1,5")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		why := "Not a number; write it as 1,234.5"
		shows(t, tt, why)
		if a, _ := tt.AccessByRole(ui.RoleTextField, "Min price"); values[1] != int64(5) || a.Value != "5" || !a.Invalid || a.Description != why {
			t.Errorf("1,5 bound %#v, reads %+v", values[1], a)
		}
		// Escape takes the problem away too.
		tt.Type("x")
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if Shows(tt, why) {
			t.Error("Escape left the problem")
		}
	})
}

// TestStatBlockOddValues: NaN and the infinities are a dash, as a value
// and as a change; a plain number change is signed; a change that is no
// number has no arrow and no word.
func TestStatBlockOddValues(t *testing.T) {
	spec := StatSpec{Label: "Rate", Format: FormatNumber}
	view := func(c *Context) {
		Column(c).Width(400).Padding(16).Children(func() { StatBlock(c, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		for _, v := range []any{math.NaN(), math.Inf(1), math.Inf(-1)} {
			spec.Value, spec.Change, spec.ChangeFormat = v, v, FormatPercent
			tt.Frame()
			if n := countText(tt, NoNumber); n < 2 {
				t.Errorf("%v shows %d dashes: %q", v, n, tt.Texts())
			}
			for _, w := range []string{"Up ", "Down ", "No change "} {
				if _, ok := accessPrefix(tt, w); ok {
					t.Errorf("a %v change is read as %q", v, w)
				}
			}
		}
		spec.Value, spec.Change, spec.ChangeFormat = 12, 5, ""
		tt.Frame()
		shows(t, tt, "12", "+5")
		if _, ok := accessPrefix(tt, "Up +5"); !ok {
			t.Error("a change of 5 is not read as Up +5")
		}
		spec.Change = "steady"
		tt.Frame()
		shows(t, tt, "steady")
		for _, w := range []string{"Up ", "Down ", "No change "} {
			if _, ok := accessPrefix(tt, w); ok {
				t.Errorf("a word change is read as %q", w)
			}
		}
	})
}

// TestTableBlockOddValues: NaN and the infinities are a dash in a cell,
// and a zero time is blank.
func TestTableBlockOddValues(t *testing.T) {
	var st TableState
	spec := TableSpec{
		Label: "Odd",
		Columns: []BlockColumn{
			{Field: "name", Label: "Name"},
			{Field: "n", Label: "N", Format: FormatNumber},
			{Field: "share", Label: "Share", Format: FormatPercent},
			{Field: "at", Label: "At", Format: FormatDateTime},
		},
		Rows: [][]any{
			{"nan", math.NaN(), math.Inf(1), time.Time{}},
			{"inf", math.Inf(-1), 0.004, time.Time{}},
		},
		Total: 2,
	}
	view := func(c *Context) {
		Column(c).Width(600).Height(300).Padding(8).AlignItems(Stretch).Children(func() { TableBlock(c, &st, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		tt.Frame()
		shows(t, tt, "nan", "inf", "0.4%")
		if n := countText(tt, NoNumber); n != 3 {
			t.Errorf("%d dashes: %q", n, tt.Texts())
		}
		for _, s := range tt.Texts() {
			if s == "NaN" || s == "+Inf" || s == "-Inf" || s == "∞" || s == "Jan 1, 1" {
				t.Errorf("a cell shows %q", s)
			}
		}
	})
}

// TestTableBlockTenThousandRowsPaged: 10,000 rows at the usual page size
// page by the keyboard, and on a page of 600, Choose all takes the first
// 500, a second click clears, and past 500 the bulk actions are off,
// with why shown.
func TestTableBlockTenThousandRowsPaged(t *testing.T) {
	var st TableState
	var acts int
	all := make([][]any, 10000)
	for i := range all {
		all[i] = []any{fmt.Sprintf("row %d", i+1), i}
	}
	spec := TableSpec{
		Label:   "Log",
		Columns: []BlockColumn{{Field: "line", Label: "Line"}, {Field: "n", Label: "N", Format: FormatNumber}},
		Total:   10000,
		Actions: []RowAction{{Label: "Delete", Bulk: true, Destructive: true}},
	}
	view := func(c *Context) {
		Column(c).Width(600).Height(400).Padding(8).AlignItems(Stretch).Children(func() {
			size := pageSize(spec)
			from := min(st.Page*size, len(all))
			spec.Rows = all[from:min(from+size, len(all))]
			if TableBlock(c, &st, spec).Action >= 0 {
				acts++
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		st, acts, spec.PageSize = TableState{}, 0, 0
		tt.Frame()
		shows(t, tt, "Rows 1–50 of 10,000", "Page 1 of 200", "row 1")
		// The pager is the last stop: Shift+Tab reaches it.
		for range 3 {
			tt.Key(ui.Shift, ui.KeyTab)
			tt.Frame()
			if a, _ := tt.AccessByRole(ui.RoleButton, "Next page"); a.Focused {
				break
			}
		}
		if a, _ := tt.AccessByRole(ui.RoleButton, "Next page"); !a.Focused {
			t.Fatal("Shift+Tab did not reach Next page")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		tt.Frame()
		shows(t, tt, "Rows 51–100 of 10,000", "Page 2 of 200", "row 51")

		// A page of 600: Choose all takes 500, and the box is mixed.
		st, spec.PageSize = TableState{}, 600
		tt.Frame()
		shows(t, tt, "Page 1 of 17")
		if err := tt.Click("Choose all rows"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if n := len(st.Chosen()); n != MaxBulkRows {
			t.Errorf("Choose all chose %d", n)
		}
		if a, _ := tt.AccessByRole(ui.RoleCheckBox, "Choose all rows"); !a.Mixed {
			t.Errorf("Choose all reads %+v", a)
		}
		shows(t, tt, "500 chosen")
		if a, _ := tt.AccessByRole(ui.RoleButton, "Delete chosen rows"); a.Disabled {
			t.Error("the bulk action is off with 500 chosen")
		}
		// A mixed box clears.
		tt.Click("Choose all rows")
		tt.Frame()
		if n := len(st.Chosen()); n != 0 {
			t.Errorf("a second click left %d", n)
		}
		// Past 500, the bulk action is off, and why shows.
		tt.Click("Choose all rows")
		tt.Frame()
		st.chosen[MaxBulkRows] = true
		tt.Frame()
		why := "Choose at most 500 rows"
		shows(t, tt, "501 chosen", why)
		a, _ := tt.AccessByRole(ui.RoleButton, "Delete chosen rows")
		if !a.Disabled || a.Description != why {
			t.Errorf("past 500 the bulk action reads %+v", a)
		}
		tt.Click("Delete chosen rows")
		tt.Frame()
		if acts != 0 {
			t.Errorf("the bulk action acted %d times past 500", acts)
		}
		// Back to 500, it is on again.
		delete(st.chosen, 0)
		tt.Frame()
		if a, _ := tt.AccessByRole(ui.RoleButton, "Delete chosen rows"); a.Disabled || Shows(tt, why) {
			t.Errorf("with 500 chosen the bulk action reads %+v", a)
		}
	})
}
