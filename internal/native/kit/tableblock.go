package kit

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// BlockColumn is a column of a TableBlock (SPEC 5.3's columns): the field
// of the rows it shows, its heading, its format and the format's options,
// and where its cells line up: 0 by the format, numbers at the end and
// the rest at the start.
type BlockColumn struct {
	Field, Label string
	Format       CellFormat
	Options      FormatOptions
	Align        Align
}

// RowAction is a button on each row of a TableBlock (SPEC 5.3's
// row_actions), as Edit. Bulk also puts it over the table, for the rows
// chosen in its check box column. Destructive shows it in red, as Delete.
type RowAction struct {
	Label       string
	Bulk        bool
	Destructive bool
}

// ManyRows is a TableBlock's Total when the count of its rows took too
// long: it shows "many rows" instead (SPEC 5.3).
const ManyRows = -1

// MaxBulkRows is how many rows a bulk action takes at most (SPEC 5.3).
const MaxBulkRows = 500

// TableSpec is what a TableBlock shows: plain data, as a view's rows
// reach a page (SPEC 5.3, column names plus array rows).
type TableSpec struct {
	// Label names the table for assistive technology, as its view's title.
	Label   string
	Columns []BlockColumn
	// Rows is the page loaded, each a row's values in the order of
	// Columns: a string, a number, a bool, a time.Time, []byte, or nil
	// for NULL.
	Rows [][]any
	// Total is the number of rows of the whole query, or ManyRows.
	Total int
	// PageSize is the number of rows a page holds, 50 for 0.
	PageSize int
	Actions  []RowAction
	State    BlockState
	// Thumbnail returns the bytes of the picture of an image cell's
	// bucket key, ok false while they are on their way; nil shows an
	// icon for each.
	Thumbnail func(key string) (data []byte, ok bool)
	// OpenFile opens a file cell's or an image cell's bucket key in the
	// file view; nil leaves them unclickable.
	OpenFile func(key string)
	// OpenLink opens a url cell's link, which SafeLink allows; nil opens
	// it in the system.
	OpenLink func(url string)
}

// TableSort is how a TableBlock's rows are sorted: by the column of
// Field, ascending unless Descending.
type TableSort struct {
	Field      string
	Descending bool
}

// TableState is what a TableBlock keeps between frames: the sort and the
// page, which the user changes and the query follows, the place of the
// rows, and the rows chosen for a bulk action. Keep one per table; start
// it with the view's default sort.
type TableState struct {
	Sort TableSort
	// Page is the page shown, from 0.
	Page int

	rows   ui.ListState
	cursor int
	ready  bool
	chosen map[int]bool
	thumbs map[string]ui.ImageSource
}

// Chosen is the rows of the loaded page chosen in the check box column,
// in order.
func (s *TableState) Chosen() []int {
	out := make([]int, 0, len(s.chosen))
	for i, on := range s.chosen {
		if on {
			out = append(out, i)
		}
	}
	slices.Sort(out)
	return out
}

// ClearChosen chooses no row, as after the rows were loaded again.
func (s *TableState) ClearChosen() { clear(s.chosen) }

// reset goes to the top of the rows with none chosen, as a new page or
// sort shows.
func (s *TableState) reset() {
	s.ClearChosen()
	s.cursor = -1
	s.rows.ScrollTo(0, ui.Start)
}

// TableResult is a TableBlock as built in this frame, and what the user
// did with it.
type TableResult struct {
	Element
	// Sorted is set in the frame the user sorted by another column or
	// order, Sort then; Page goes back to 0. Query again.
	Sorted bool
	// Paged is set in the frame the user went to another page, Page then.
	Paged bool
	// Action is the index in Actions of the action clicked in this frame,
	// -1 for none, and Rows the rows of the loaded page it acts on: the
	// row's own, or the rows chosen for a bulk action.
	Action int
	Rows   []int
}

// The IDs of the check box column and the actions column, which no field
// has.
const (
	selectColumn  = "\x00select"
	actionsColumn = "\x00actions"
)

// TableBlock is the table block (SPEC 5.3): the rows of one page in a
// virtual table (MyGo's ui.Table, a List's rows) that builds only those
// in view, under its column headings, with the pages under it.
//
//   - A click on a heading, or Enter or Space while it has the focus,
//     sorts by its column, and again reverses the order; the query sorts.
//   - Tab reaches the table, whose arrows, Page Up and Down, Home and End
//     move through the rows, then the headings, then what the rows hold.
//   - Each cell shows its value in its column's format: a badge in its
//     tone, a url as a link if SafeLink allows it, an image as a
//     thumbnail and a file as a link to the file view. A cell takes the
//     direction of its first strong character, so Persian text starts on
//     the right; numbers stay left to right at the end of the column. A
//     cut cell shows its text whole in its tooltip.
//   - A table whose headings read right to left runs its columns right to
//     left (SPEC 5.8).
//   - With a Bulk action, a check box column chooses rows, and a bar over
//     the table acts on them.
//   - Loading, an error, a notice and no rows show by the state.
//
// The table grows in its parent; give the block a height.
func TableBlock(c *Context, st *TableState, spec TableSpec) TableResult {
	res := TableResult{Action: -1}
	if st.chosen == nil {
		st.chosen = map[int]bool{}
	}
	if !st.ready {
		st.ready, st.cursor = true, -1
	}
	for i := range st.chosen {
		if i >= len(spec.Rows) {
			delete(st.chosen, i)
		}
	}
	size := spec.PageSize
	if size <= 0 {
		size = 50
	}
	res.Element = BlockBody(c, spec.Label, spec.State, func() {
		bulk := slices.ContainsFunc(spec.Actions, func(a RowAction) bool { return a.Bulk })
		if len(spec.Rows) == 0 {
			Empty(c, IconInbox, "No rows", "", nil)
		} else {
			if bulk {
				bulkBar(c, st, spec, &res)
			}
			rowsTable(c, st, spec, bulk, size, &res)
		}
		if len(spec.Rows) > 0 || st.Page > 0 {
			pager(c, st, spec, size, &res)
		}
	})
	if res.Sorted || res.Paged {
		st.reset()
	}
	return res
}

// rowsTable builds the table of the rows.
func rowsTable(c *Context, st *TableState, spec TableSpec, bulk bool, size int, res *TableResult) {
	k := TokensOf(c)
	page := c.Theme()
	cols := tableColumns(c, spec, bulk)
	st.rows.Selected, st.rows.RowHeight = &st.cursor, 36
	st.rows.Label = func(i int) string { return rowLabel(c, spec, i) }
	st.rows.HeaderCell = func(_ ui.Element, col int) {
		switch id := cols[col].ID; id {
		case selectColumn:
			ui.Themed(c, page).Children(func() { chooseAll(c, st, len(spec.Rows)) })
		case actionsColumn:
		default:
			heading(c, st, spec.Columns[col-b2i(bulk)], cols[col].Align, res)
		}
	}
	// The row chosen by the keys shows muted, as shadcn's selected row,
	// not in the primary color.
	t := *page
	t.Accent, t.InactiveAccent, t.AccentText = k.Muted, k.Muted, k.Foreground
	labels := make([]string, len(spec.Columns))
	for i, col := range spec.Columns {
		labels[i] = col.Label
	}
	dir := ui.LeftToRight
	if RightToLeft(strings.Join(labels, " ")) {
		dir = ui.RightToLeft
	}
	aligns := make([]ui.Align, len(cols))
	for i, col := range cols {
		f := FormatText
		if j := i - b2i(bulk); col.ID != selectColumn && col.ID != actionsColumn {
			f = spec.Columns[j].Format
		}
		aligns[i] = cellAlign(col.Align, f, dir)
	}
	ui.Themed(c, &t).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		table := ui.Table(c, &st.rows, cols, len(spec.Rows), func(row, col int) {
			// The page's theme again, for the check boxes, without its
			// background, which would hide the row's. Each cell is laid
			// out left to right, its text in its own direction, and lines
			// up at its column's side of the table.
			ui.Themed(c, page).Background(ui.Transparent).Direction(ui.LeftToRight).Grow(1).Shrink(1).MinWidth(0).
				AlignItems(aligns[col]).
				Children(func() { tableCell(c, st, spec, cols[col].ID, row, col, bulk, res) })
		})
		table.Label(spec.Label).Direction(dir).Grow(1).MinHeight(144).Border(1, k.Border).Radius(Radius * 0.8)
	})
}

// cellAlign is how a cell, laid out left to right, places what is in it
// for its column's alignment in a table of direction dir. Text fills the
// cell, to start at the side its own direction gives it; anything else,
// as a date or a badge, lines up with its heading: at the table's start,
// or at its end for numbers, the right of a table left to right and the
// left of one right to left.
func cellAlign(a Align, f CellFormat, dir ui.Direction) ui.Align {
	rtl, text := dir == ui.RightToLeft, f == FormatText || f == ""
	switch {
	case a == ui.Center:
		return ui.Center
	case a == ui.End && rtl, a != ui.End && !rtl && !text:
		return ui.Start
	case a == ui.End, !text:
		return ui.End
	}
	return ui.Stretch
}

// tableColumns are the columns ui.Table gets: the check box column, the
// data's, and the actions'.
func tableColumns(c *Context, spec TableSpec, bulk bool) []ui.TableColumn {
	var cols []ui.TableColumn
	if bulk {
		cols = append(cols, ui.TableColumn{ID: selectColumn, Width: 44, Fixed: true, Align: ui.Center})
	}
	for _, col := range spec.Columns {
		// The heading is the kit's (heading): MyGo's sorts on a click alone.
		tc := ui.TableColumn{ID: col.Field, Align: col.Align, MinWidth: 72}
		if tc.Align == 0 && col.Format.Numeric() {
			tc.Align = ui.End
		}
		switch col.Format {
		case FormatImage:
			tc.Width = 72
		case FormatBoolean:
			tc.MinWidth = 56
		}
		cols = append(cols, tc)
	}
	if len(spec.Actions) > 0 {
		// As wide as its buttons: each its label's width and padding.
		w := float32(20)
		for _, a := range spec.Actions {
			tw, _ := c.MeasureText(0, ui.Span{Text: a.Label, Size: 13, Weight: 500})
			w += tw + 20 + 4
		}
		cols = append(cols, ui.TableColumn{ID: actionsColumn, Width: w, Fixed: true, Align: ui.End})
	}
	return cols
}

// heading is a column's heading. A sortable column's is a button: a
// click, or Enter or Space while it has the focus, sorts by it, ascending,
// and again reverses the order, which an arrow shows and assistive
// technology reads. MyGo v0.10.0's own sortable headings take only a
// click, and Tab never reaches them (egoist/mygo#2146).
func heading(c *Context, st *TableState, col BlockColumn, align Align, res *TableResult) {
	k := TokensOf(c)
	title := func() {
		t := ui.Text(c, col.Label).SingleLine().FontWeight(500).TextColor(k.MutedForeground).Shrink(1).MinWidth(0)
		if cut(c, t, col.Label, false) {
			t.Tooltip(col.Label)
		}
	}
	if col.Format == FormatImage || col.Format == FormatFile {
		title()
		return
	}
	b := ui.ButtonBase(c).Label(col.Label).Grow(1).Shrink(1).MinWidth(0).Height(28).Margin(0, -6).PaddingX(6).Gap(4).
		Radius(4).AlignItems(ui.Center)
	switch align {
	case ui.End:
		b.Justify(ui.End)
	case ui.Center:
		b.Justify(ui.Center)
	default:
		b.Justify(ui.Start)
	}
	if b.Hovered() {
		b.Background(k.Accent)
	}
	on := st.Sort.Field == col.Field
	switch {
	case !on:
		b.Sort(ui.SortNone)
	case st.Sort.Descending:
		b.Sort(ui.SortDescending)
	default:
		b.Sort(ui.SortAscending)
	}
	b.Children(func() {
		title()
		if on {
			icon := IconArrowUp
			if st.Sort.Descending {
				icon = IconArrowDown
			}
			ui.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(k.Foreground)
		}
	})
	if pressed(b) {
		st.Sort = TableSort{Field: col.Field, Descending: on && !st.Sort.Descending}
		st.Page, res.Sorted = 0, true
	}
}

// pressed reports a click on a button in a table's rows or headings, or
// Enter while it has the focus. A table whose rows the keys move through
// takes Enter from the buttons in it (MyGo v0.10.0, egoist/mygo#2145), so
// they ask for it.
func pressed(b Element) bool {
	enter := b.Shortcut(0, ui.KeyEnter)
	return b.Clicked() || enter
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// chooseAll is the check box of the check box column's heading, checked
// while every row of the page is chosen and mixed while some are. With
// none chosen, it chooses the page's rows, at most MaxBulkRows; with any
// chosen, it chooses none, so a page of more than MaxBulkRows rows can
// always be cleared.
func chooseAll(c *Context, st *TableState, n int) {
	on := len(st.Chosen())
	state := ui.CheckOff
	switch {
	case on > 0 && on == n:
		state = ui.CheckOn
	case on > 0:
		state = ui.CheckMixed
	}
	if ui.CheckboxState(c, &state, "").Label("Choose all rows").Changed() {
		clear(st.chosen)
		if on == 0 {
			for i := range min(n, MaxBulkRows) {
				st.chosen[i] = true
			}
		}
	}
}

// tableCell builds the cell of row and col, of the column id.
func tableCell(c *Context, st *TableState, spec TableSpec, id string, row, col int, bulk bool, res *TableResult) {
	switch id {
	case selectColumn:
		on := st.chosen[row]
		if ui.Checkbox(c, &on, "").Label("Choose row " + itoa(st.Page*pageSize(spec)+row+1)).Changed() {
			st.chosen[row] = on
		}
		return
	case actionsColumn:
		k := TokensOf(c)
		ui.Row(c).Gap(4).Justify(ui.End).Grow(1).Children(func() {
			for i, a := range spec.Actions {
				b := button(c, Ghost).Height(28).PaddingX(10).FontSize(13).Key(i)
				if a.Destructive {
					b.TextColor(k.Tone(Red).Text)
				}
				b.Children(func() { ui.Text(c, a.Label).SingleLine() })
				if pressed(b) {
					res.Action, res.Rows = i, []int{row}
				}
			}
		})
		return
	}
	if bulk {
		col--
	}
	if col < 0 || col >= len(spec.Columns) || col >= len(spec.Rows[row]) {
		return
	}
	cellValue(c, st, spec, spec.Columns[col], spec.Rows[row][col])
}

func pageSize(spec TableSpec) int {
	if spec.PageSize <= 0 {
		return 50
	}
	return spec.PageSize
}

// cellValue shows v in the format of col.
func cellValue(c *Context, st *TableState, spec TableSpec, col BlockColumn, v any) {
	k := TokensOf(c)
	s := FormatValue(c, v, col.Format, col.Options)
	switch col.Format {
	case FormatBadge:
		if s != "" {
			tone, ok := col.Options.Tones[s]
			if !ok {
				tone = Neutral
			}
			b := Badge(c, s, tone).Shrink(1).MinWidth(0)
			if cut(c, b, s, false) {
				b.Tooltip(s)
			}
		}
		return
	case FormatURL:
		if SafeLink(s) {
			l := ui.Link(c, s, "").TextColor(k.Primary).Underline().SingleLine().Direction(ui.LeftToRight).
				Shrink(1).MinWidth(0)
			if cut(c, l, s, false) {
				l.Tooltip(s)
			}
			if pressed(l) {
				if spec.OpenLink != nil {
					spec.OpenLink(s)
				} else {
					c.OpenURL(s)
				}
			}
			return
		}
	case FormatImage:
		if s != "" {
			thumbnail(c, st, spec, s)
		}
		return
	case FormatFile:
		if s != "" {
			fileLink(c, spec, s)
		}
		return
	}
	cellText(c, s, col.Format.Numeric(), col.Format == FormatBoolean && s == "No")
}

// cellText is a cell's text, in the direction of its first strong
// character, cut to one line with its tooltip then; a number is left to
// right in figures of one width. muted shows it in the muted color, as
// No.
func cellText(c *Context, s string, numeric, muted bool) {
	t := ui.Text(c, s).SingleLine().Shrink(1).MinWidth(0)
	switch {
	case numeric:
		t.FontFeatures("tnum").Direction(ui.LeftToRight)
	case RightToLeft(s):
		t.Grow(1).Direction(ui.RightToLeft)
	default:
		t.Grow(1).Direction(ui.LeftToRight)
	}
	if muted {
		t.TextColor(TokensOf(c).MutedForeground)
	}
	if s != "" && cut(c, t, s, numeric) {
		t.Tooltip(s)
	}
}

// thumbnail is an image cell: the picture of its bucket key, 28 px high,
// or an icon while it is on its way or can't be shown. A click opens it
// in the file view.
func thumbnail(c *Context, st *TableState, spec TableSpec, key string) {
	k := TokensOf(c)
	name := path.Base(key)
	b := ui.ButtonBase(c).Label(name).Tooltip(key).Radius(4).Disabled(spec.OpenFile == nil)
	b.Children(func() {
		if src := thumbOf(st, spec, key); src != nil {
			w, h := ui.ImageSize(src)
			if h > 0 {
				w = min(w*28/h, 56)
			}
			ui.Image(c, src).Size(max(w, 1), 28).Radius(4).Border(1, k.Border)
			return
		}
		ui.Box(c).Size(28, 28).Radius(4).Background(k.Muted).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, IconFileImage).Size(16, 16).TextColor(k.MutedForeground)
		})
	})
	if pressed(b) && spec.OpenFile != nil {
		spec.OpenFile(key)
	}
}

// thumbOf is the picture of key, decoded once; nil while its bytes are on
// their way or for bytes that are no picture.
func thumbOf(st *TableState, spec TableSpec, key string) ui.ImageSource {
	if src, ok := st.thumbs[key]; ok {
		return src
	}
	if spec.Thumbnail == nil {
		return nil
	}
	data, ok := spec.Thumbnail(key)
	if !ok {
		return nil
	}
	src, err := ui.DecodeImage(data)
	if err != nil {
		src = nil
	}
	if st.thumbs == nil || len(st.thumbs) >= 512 {
		st.thumbs = map[string]ui.ImageSource{}
	}
	st.thumbs[key] = src
	return src
}

// fileLink is a file cell: the file's name, as a link to the file view,
// with its key as its tooltip.
func fileLink(c *Context, spec TableSpec, key string) {
	k := TokensOf(c)
	name := path.Base(key)
	b := ui.ButtonBase(c).Label(name).Tooltip(key).Radius(4).Gap(6).AlignItems(ui.Center).Shrink(1).MinWidth(0).
		TextColor(k.Primary).Disabled(spec.OpenFile == nil)
	b.Children(func() {
		ui.Icon(c, IconFile).Size(14, 14).Shrink(0)
		ui.Text(c, name).SingleLine().Underline().Shrink(1).MinWidth(0)
	})
	if pressed(b) && spec.OpenFile != nil {
		spec.OpenFile(key)
	}
}

// rowLabel is the text of row i for assistive technology and for typing
// to find a row: its first column's.
func rowLabel(c *Context, spec TableSpec, i int) string {
	if i < 0 || i >= len(spec.Rows) || len(spec.Columns) == 0 || len(spec.Rows[i]) == 0 {
		return ""
	}
	return FormatValue(c, spec.Rows[i][0], spec.Columns[0].Format, spec.Columns[0].Options)
}

// bulkBar is the bar over a table with bulk actions: how many rows are
// chosen, and the bulk actions, which act on them.
func bulkBar(c *Context, st *TableState, spec TableSpec, res *TableResult) {
	chosen := st.Chosen()
	over := len(chosen) > MaxBulkRows
	// Why the buttons are off, with too many rows chosen: shown, and read
	// with each button.
	why := "Choose at most " + count(c, MaxBulkRows) + " rows"
	ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleToolbar).Label("Chosen rows").Children(func() {
		Muted(c, count(c, len(chosen))+" chosen").FontSize(13).FontFeatures("tnum").Shrink(0)
		if over {
			Muted(c, why).FontSize(13).TextColor(TokensOf(c).Tone(Red).Text).Role(ui.RoleStatus).Label(why)
		}
		ui.Box(c).Grow(1)
		for i, a := range spec.Actions {
			if !a.Bulk {
				continue
			}
			v := Outline
			if a.Destructive {
				v = Destructive
			}
			b := button(c, v).Height(32).PaddingX(12).FontSize(13).Key(i).Label(a.Label + " chosen rows")
			b.Children(func() { ui.Text(c, a.Label).SingleLine() })
			Disable(b, len(chosen) == 0 || over)
			if over {
				b.Description(why)
			}
			if b.Clicked() {
				res.Action, res.Rows = i, chosen
			}
		}
	})
}

// pager is the line under a table: the rows shown of how many, and the
// buttons to the pages before and after.
func pager(c *Context, st *TableState, spec TableSpec, size int, res *TableResult) {
	first, last := st.Page*size+1, st.Page*size+len(spec.Rows)
	total, pages := "many", ""
	more := len(spec.Rows) >= size
	if spec.Total >= 0 {
		total = count(c, spec.Total)
		n := max(1, (spec.Total+size-1)/size)
		pages = "Page " + count(c, st.Page+1) + " of " + count(c, n)
		more = st.Page+1 < n
	}
	shown := "No rows"
	if len(spec.Rows) > 0 {
		shown = "Rows " + count(c, first) + "–" + count(c, last) + " of " + total
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleNavigation).Label("Pages of " + spec.Label).Children(func() {
		Muted(c, shown).FontSize(13).FontFeatures("tnum").Grow(1)
		if pages != "" {
			Muted(c, pages).FontSize(13).FontFeatures("tnum")
		}
		prev := IconButton(c, IconChevronLeft, "Previous page", Outline).Size(32, 32)
		Disable(prev, st.Page == 0)
		next := IconButton(c, IconChevronRight, "Next page", Outline).Size(32, 32)
		Disable(next, !more)
		switch {
		case prev.Clicked() && st.Page > 0:
			st.Page--
			res.Paged = true
		case next.Clicked() && more:
			st.Page++
			res.Paged = true
		}
	})
}

// count writes n with the separators of the user's regional formats.
func count(c *Context, n int) string {
	return i18n.FormatNumber(formatLocale(c), n, i18n.NumberOptions{})
}
