package kit

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func two() *int { n := 2; return &n }

// formatsSpec is a table with a column of each format (SPEC 5.3).
func formatsSpec() TableSpec {
	return TableSpec{
		Label: "Prices",
		Columns: []BlockColumn{
			{Field: "coin", Label: "Coin"},
			{Field: "price", Label: "Price", Format: FormatCurrency, Options: FormatOptions{Currency: "USD", Decimals: two()}},
			{Field: "change", Label: "Change", Format: FormatPercent},
			{Field: "day", Label: "Day", Format: FormatDate},
			{Field: "status", Label: "Status", Format: FormatBadge, Options: FormatOptions{Tones: map[string]Tone{"failed": Red, "new": Blue}}},
			{Field: "site", Label: "Site", Format: FormatURL},
			{Field: "ok", Label: "Read", Format: FormatBoolean},
		},
		Rows: [][]any{
			{"BTC", 91842.5, 0.25, "2026-11-20", "failed", "https://example.com/btc", int64(1)},
			{persian, "12", -0.031, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC), "new", "javascript:alert(1)", int64(0)},
			{"ETH", nil, nil, nil, "other", nil, nil},
		},
		Total: 3,
	}
}

func TestTableBlockFormats(t *testing.T) {
	var st TableState
	var opened []string
	spec := formatsSpec()
	spec.OpenLink = func(url string) { opened = append(opened, url) }
	view := func(c *Context) {
		Column(c).Width(780).Height(400).Padding(8).Gap(8).AlignItems(Stretch).Children(func() {
			Button(c, "Before", Outline)
			TableBlock(c, &st, spec)
			Button(c, "After", Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		opened = nil
		tt.Frame()
		shows(t, tt, "Coin", "Price", "BTC", persian, "$91,842.50", "$12.00", "25%", "-3.1%", "Nov 20, 2026", "Nov 19, 2026",
			"failed", "new", "other", "Yes", "No", "javascript:alert(1)", "Rows 1–3 of 3", "Page 1 of 1")
		if _, ok := tt.AccessByRole(ui.RoleTable, "Prices"); !ok {
			t.Error("no table named Prices")
		}
		// Only the safe link is a link, and it opens through OpenLink.
		if got := Links(tt); !slices.Equal(got, []string{"https://example.com/btc"}) {
			t.Errorf("the links are %q", got)
		}
		if err := tt.Click("https://example.com/btc"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !slices.Equal(opened, []string{"https://example.com/btc"}) || len(tt.OpenedURLs()) > 0 {
			t.Errorf("opened %q, the system %q", opened, tt.OpenedURLs())
		}
		// Numbers end at the end of their column, the text starts at its
		// start, and Persian text at its end, on the right.
		a, _ := tt.Find("$91,842.50")
		b, _ := tt.Find("$12.00")
		if d := (a.X + a.W) - (b.X + b.W); d > 1 || d < -1 {
			t.Errorf("the amounts end at %v and %v", a.X+a.W, b.X+b.W)
		}
		btc, _ := tt.Find("BTC")
		fa, _ := tt.Find(persian)
		if fa.X+fa.W < btc.X+60 {
			t.Errorf("the Persian cell starts on the left: %v, BTC %v", fa, btc)
		}
		// The badges take their tones' colors.
		img := tt.Image()
		k := Light
		if tt.Image() != nil && darkOf(tt) {
			k = Dark
		}
		r, _ := tt.Find("failed")
		if !hasColor(img, r, k.Tone(Red).Soft) {
			t.Error("the failed badge is not in the red tone")
		}
		// Tab reaches the table, its headings and its link, and leaves it.
		tabTo(t, tt, "Before")
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("Prices") {
			t.Error("Tab did not reach the table")
		}
		focusOn(t, tt, "Coin")
		focusOn(t, tt, "https://example.com/btc")
		tabTo(t, tt, "After")
	})
}

// darkOf reports whether the tester's window is dark, by its corner.
func darkOf(tt *ui.Tester) bool {
	return near(tt.Image().RGBAAt(799, 599), Dark.Background)
}

// hasColor reports whether r of img has a pixel of col.
func hasColor(img *image.RGBA, r ui.Rect, col Color) bool {
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X) - 4; x < int(r.X+r.W)+4; x++ {
			if near(img.RGBAAt(x, y), col) {
				return true
			}
		}
	}
	return false
}

// TestTableBlockSorts: a click on a heading sorts by it, Enter on a
// heading with the focus reverses it, and either goes to the first page.
func TestTableBlockSorts(t *testing.T) {
	var st TableState
	var res TableResult
	sorted := 0
	spec := formatsSpec()
	spec.Total = 300
	view := func(c *Context) {
		Column(c).Width(780).Height(400).Padding(8).AlignItems(Stretch).Children(func() {
			res = TableBlock(c, &st, spec)
			if res.Sorted {
				sorted++
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		st, sorted = TableState{Sort: TableSort{Field: "day", Descending: true}, Page: 2}, 0
		tt.Frame()
		if err := tt.Click("Price"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if st.Sort != (TableSort{Field: "price"}) || st.Page != 0 || sorted != 1 {
			t.Errorf("a click sorted by %+v, page %d, %d sorts", st.Sort, st.Page, sorted)
		}
		if a, ok := tt.AccessByRole(ui.RoleButton, "Price"); !ok || a.Sort != ui.SortAscending {
			t.Errorf("the heading reads sorted %v", a.Sort)
		}
		focusOn(t, tt, "Price")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if st.Sort != (TableSort{Field: "price", Descending: true}) || sorted != 2 {
			t.Errorf("Enter sorted by %+v, %d sorts", st.Sort, sorted)
		}
		tt.Key(0, ui.KeyTab) // to Change
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if st.Sort != (TableSort{Field: "change"}) {
			t.Errorf("Space sorted by %+v", st.Sort)
		}
	})
}

func TestTableBlockPages(t *testing.T) {
	var st TableState
	var paged int
	rows := make([][]any, 50)
	for i := range rows {
		rows[i] = []any{fmt.Sprintf("row %d", i)}
	}
	spec := TableSpec{Label: "Log", Columns: []BlockColumn{{Field: "line", Label: "Line"}}, Rows: rows, Total: 120}
	view := func(c *Context) {
		Column(c).Width(500).Height(400).Padding(8).AlignItems(Stretch).Children(func() {
			if TableBlock(c, &st, spec).Paged {
				paged++
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		st, paged, spec.Total, spec.Rows = TableState{}, 0, 120, rows
		tt.Frame()
		shows(t, tt, "Rows 1–50 of 120", "Page 1 of 3")
		tabTo(t, tt, "Next page")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if st.Page != 1 || paged != 1 {
			t.Fatalf("Next went to page %d, %d times", st.Page, paged)
		}
		shows(t, tt, "Rows 51–100 of 120", "Page 2 of 3")
		st.Page = 2
		spec.Rows = rows[:20]
		tt.Frame()
		shows(t, tt, "Rows 101–120 of 120")
		if a, _ := tt.AccessByRole(ui.RoleButton, "Next page"); !a.Disabled {
			t.Error("Next is not disabled on the last page")
		}
		// A count that took too long: many rows, and Next while a page is
		// full.
		st.Page, spec.Total, spec.Rows = 1, ManyRows, rows
		tt.Frame()
		shows(t, tt, "Rows 51–100 of many")
		if a, _ := tt.AccessByRole(ui.RoleButton, "Next page"); a.Disabled {
			t.Error("Next is disabled with a full page of many")
		}
		if err := tt.Click("Previous page"); err != nil || st.Page != 0 {
			t.Errorf("Previous: %v, page %d", err, st.Page)
		}
	})
}

// TestTableBlockScrollsTenThousandRows: a page of 10,000 rows of 12
// columns builds only the rows in view, and the keys scroll it.
func TestTableBlockScrollsTenThousandRows(t *testing.T) {
	var st TableState
	cols := make([]BlockColumn, 12)
	for i := range cols {
		cols[i] = BlockColumn{Field: fmt.Sprint("c", i), Label: fmt.Sprint("Col ", i), Format: FormatNumber}
	}
	cols[0].Format = FormatText
	rows := make([][]any, 10000)
	for i := range rows {
		rows[i] = make([]any, 12)
		rows[i][0] = fmt.Sprintf("%s %d", persian, i)
		for j := 1; j < 12; j++ {
			rows[i][j] = i * j
		}
	}
	built := 0
	spec := TableSpec{Label: "Big", Columns: cols, Rows: rows, Total: 10000, PageSize: 10000}
	spec.Thumbnail = func(string) ([]byte, bool) { built++; return nil, false }
	view := func(c *Context) {
		Column(c).Width(780).Height(500).Padding(8).AlignItems(Stretch).Children(func() {
			Button(c, "Before", Outline)
			TableBlock(c, &st, spec)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		st = TableState{}
		tt.Frame()
		shows(t, tt, persian+" 0", "Rows 1–10,000 of 10,000")
		if n := len(tt.AccessAll()); n > 1500 {
			t.Errorf("%d elements for 10,000 rows", n)
		}
		tabTo(t, tt, "Big")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		tt.Frame()
		shows(t, tt, persian+" 9999")
		if Shows(tt, persian+" 0") {
			t.Error("the first row still shows at the end")
		}
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		tt.Frame()
		shows(t, tt, persian+" 0")
		tt.Key(0, ui.KeyPageDown)
		tt.Frame()
		tt.Key(0, ui.KeyPageDown)
		tt.Frame()
		tt.Frame()
		if Shows(tt, persian+" 0") || st.cursor < 10 {
			t.Errorf("Page Down left the cursor at %d", st.cursor)
		}
	})
}

// TestTableBlockPersianColumns: a table whose headings are Persian runs
// its columns right to left; an English one left to right.
func TestTableBlockPersianColumns(t *testing.T) {
	var st TableState
	spec := TableSpec{
		Label:   "قیمت‌ها",
		Columns: []BlockColumn{{Field: "name", Label: "نام"}, {Field: "price", Label: "قیمت", Format: FormatNumber}},
		Rows:    [][]any{{"طلا", 1200}, {"Silver", 30}},
		Total:   2,
	}
	view := func(c *Context) {
		Column(c).Width(500).Height(300).Padding(8).AlignItems(Stretch).Children(func() { TableBlock(c, &st, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		name, _ := tt.Find("نام")
		price, _ := tt.Find("قیمت")
		if name.X < price.X {
			t.Errorf("the first column is left of the second: %v, %v", name, price)
		}
		gold, _ := tt.Find("طلا")
		silver, _ := tt.Find("Silver")
		// Each cell in its own direction: Persian at the cell's right,
		// English at its left.
		if gold.X+gold.W < silver.X+silver.W-1 || silver.X > gold.X+gold.W {
			t.Errorf("the cells don't take their own direction: %v, %v", gold, silver)
		}
		// A number lines up with its heading at the column's end: its
		// left, right to left.
		if n, _ := tt.Find("1,200"); n.X > price.X+12 || n.X < price.X-12 {
			t.Errorf("the number is not under its heading's end: %v, %v", n, price)
		}
		if _, ok := tt.AccessByRole(ui.RoleTable, "قیمت‌ها"); !ok {
			t.Error("no table named in Persian")
		}
	})
}

// thumbGreen is tinyPNG's colour, which no token has.
var thumbGreen = ui.Color{R: 30, G: 160, B: 90, A: 255}

func tinyPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{30, 160, 90, 255})
	}
	img.Set(0, 0, color.RGBA{A: 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// TestTableBlockActions: row actions, the check box column and its bulk
// bar, images and files.
func TestTableBlockActions(t *testing.T) {
	var st TableState
	var res TableResult
	var files []string
	spec := TableSpec{
		Label: "Keywords",
		Columns: []BlockColumn{
			{Field: "keyword", Label: "Keyword"},
			{Field: "pic", Label: "Picture", Format: FormatImage},
			{Field: "doc", Label: "Document", Format: FormatFile},
		},
		Rows: [][]any{
			{persian, "uploads/a.png", "uploads/report.pdf"},
			{"gold", "uploads/b.png", nil},
			{"silver", nil, "uploads/notes.md"},
		},
		Total:     3,
		Actions:   []RowAction{{Label: "Edit"}, {Label: "Delete", Bulk: true, Destructive: true}},
		Thumbnail: func(key string) ([]byte, bool) { return tinyPNG(), key == "uploads/a.png" },
		OpenFile:  func(key string) { files = append(files, key) },
	}
	var acts []TableResult
	view := func(c *Context) {
		Column(c).Width(780).Height(400).Padding(8).AlignItems(Stretch).Children(func() {
			res = TableBlock(c, &st, spec)
			if res.Action >= 0 {
				acts = append(acts, res)
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		st, acts, files = TableState{}, nil, nil
		tt.Frame()
		shows(t, tt, "0 chosen", "report.pdf", "notes.md")
		if a, _ := tt.AccessByRole(ui.RoleButton, "Delete chosen rows"); !a.Disabled {
			t.Error("the bulk action is enabled with no row chosen")
		}
		// The thumbnail is drawn in its button; the row without one shows
		// the placeholder.
		if r, ok := tt.Find("a.png"); !ok || !hasColor(tt.Image(), r, thumbGreen) {
			t.Error("the thumbnail is not drawn")
		}
		if r, ok := tt.Find("b.png"); !ok || hasColor(tt.Image(), r, thumbGreen) {
			t.Error("the picture with no bytes is drawn")
		}
		// Choose the second row by the keyboard.
		focusOn(t, tt, "Choose row 2")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		shows(t, tt, "1 chosen")
		if err := tt.Click("Delete chosen rows"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if len(acts) != 1 || acts[0].Action != 1 || !slices.Equal(acts[0].Rows, []int{1}) {
			t.Fatalf("the bulk delete acted as %+v", acts)
		}
		// With a row chosen, the box is mixed, and a click clears; then
		// it chooses all, and clears again.
		if a, _ := tt.AccessByRole(ui.RoleCheckBox, "Choose all rows"); !a.Mixed || len(st.Chosen()) != 1 {
			t.Errorf("with one row chosen, Choose all reads %+v", a)
		}
		if err := tt.Click("Choose all rows"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := st.Chosen(); len(got) != 0 {
			t.Errorf("a mixed Choose all left %v", got)
		}
		tt.Click("Choose all rows")
		tt.Frame()
		if got := st.Chosen(); !slices.Equal(got, []int{0, 1, 2}) {
			t.Errorf("Choose all chose %v", got)
		}
		if a, _ := tt.AccessByRole(ui.RoleCheckBox, "Choose all rows"); !a.Checked {
			t.Error("Choose all is not checked")
		}
		tt.Click("Choose all rows")
		tt.Frame()
		if got := st.Chosen(); len(got) != 0 {
			t.Errorf("Choose all again left %v", got)
		}
		// A row's own action, by the keyboard.
		acts = nil
		focusOn(t, tt, "Edit")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(acts) != 1 || acts[0].Action != 0 || !slices.Equal(acts[0].Rows, []int{0}) {
			t.Errorf("Edit acted as %+v", acts)
		}
		// The picture and the file open in the file view.
		if err := tt.Click("a.png"); err != nil {
			t.Fatal(err)
		}
		focusOn(t, tt, "report.pdf")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if !slices.Equal(files, []string{"uploads/a.png", "uploads/report.pdf"}) {
			t.Errorf("opened %q", files)
		}
		if d, _ := Description(tt, "report.pdf"); d != "uploads/report.pdf" {
			t.Errorf("the file's tooltip is %q", d)
		}
	})
}

func TestTableBlockStates(t *testing.T) {
	var st TableState
	spec := TableSpec{Label: "Prices", Columns: []BlockColumn{{Field: "coin", Label: "Coin"}}}
	view := func(c *Context) {
		Column(c).Width(500).Height(300).Padding(8).AlignItems(Stretch).Children(func() { TableBlock(c, &st, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		spec.State, spec.Rows = BlockState{Loading: true}, nil
		tt.Frame()
		shows(t, tt, "Loading Prices")
		spec.State = BlockState{Error: "no such column: coin"}
		tt.Frame()
		shows(t, tt, "Can't show Prices", "no such column: coin")
		if !HasRole(tt, "status", "Can't show Prices") {
			t.Error("the error is not read out")
		}
		spec.State = BlockState{}
		tt.Frame()
		shows(t, tt, "No rows")
		spec.State = BlockState{Notice: "Showing the first 10,000 rows. " + persian}
		spec.Rows = [][]any{{persian}}
		tt.Frame()
		shows(t, tt, "Showing the first 10,000 rows. "+persian, persian)
		if !HasRole(tt, "status", "Showing the first 10,000 rows. "+persian) {
			t.Error("the notice is not read out")
		}
	})
}

// TestTableBlockCutCells: a cut cell shows its text whole in its tooltip.
func TestTableBlockCutCells(t *testing.T) {
	var st TableState
	long := "gemini-3.5-flash-thinking-preview-with-a-very-long-name"
	spec := TableSpec{
		Label:   "Models",
		Columns: []BlockColumn{{Field: "a", Label: "A"}, {Field: "b", Label: "B"}, {Field: "c", Label: "C"}, {Field: "d", Label: "D"}},
		Rows:    [][]any{{long, "x", "y", "z"}},
		Total:   1,
	}
	view := func(c *Context) {
		Column(c).Width(420).Height(300).Padding(8).AlignItems(Stretch).Children(func() { TableBlock(c, &st, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		tt.Frame()
		if d, ok := cellTip(tt, long); !ok || d != long {
			t.Errorf("the cut cell's tooltip is %q (%v)", d, ok)
		}
		if d, _ := cellTip(tt, "x"); d != "" {
			t.Errorf("a cell that fits has the tooltip %q", d)
		}
	})
}

// cellTip is the tooltip of the cell's text s, not its row's, which the
// row's first cell names.
func cellTip(tt *ui.Tester, s string) (string, bool) {
	for _, a := range tt.AccessAll() {
		if a.Name == s && a.Role != ui.RoleRow {
			return a.Description, true
		}
	}
	return "", false
}

// focusOn presses Tab until the element named label holds the focus
// itself. tabTo stops on the table, which counts as focusing every
// control in its rows.
func focusOn(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for range 40 {
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		for _, a := range tt.AccessAll() {
			if a.Focused && a.Name == label {
				return
			}
		}
	}
	t.Fatalf("Tab never focused %q", label)
}
