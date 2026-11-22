package viewrun

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// Table is one page of a table view's rows (SPEC 5.3). It maps onto the
// kit's TableSpec: Columns onto BlockColumn (Format as CellFormat, Align
// as kit.Align, Options with their tones as kit.Tone), Rows, Total (with
// ManyRows the kit's own) and PageSize as they are.
type Table struct {
	Columns []Column
	// Rows are the page's rows, each with a value for each column, in
	// Columns' order, as its format needs it (value): an int64, float64,
	// string, bool, time.Time, []byte, or nil for NULL. An image or file
	// cell holds its bucket key only when the bucket has a file there;
	// any other key is nil.
	Rows [][]any
	// Keys are each row's key, from rows_from's key column, for row
	// actions; nil without rows_from.
	Keys []any
	// Total is the query's rows in all, or ManyRows when counting them
	// took longer than a view's 2 s.
	Total    int
	Page     int // the page shown, from 0
	PageSize int
	Sort     Sort // the sort the rows are in
}

// ManyRows is a Table's Total when its count took too long: it shows
// "many rows" instead (the kit's ManyRows).
const ManyRows = -1

// Column is a table's column, as the kit's BlockColumn.
type Column struct {
	Field   string
	Label   string // the field when the config has none
	Format  string // text when the config has none
	Options FormatOptions
	Align   string // start, center or end; "" for the format's own
}

// sortable reports whether the table can be sorted by f: a column it
// shows, as written, that is not a picture or a file.
func sortable(t *view.Table, f string) bool {
	for _, c := range t.Columns {
		if c.Field == f {
			return c.Format != FormatImage && c.Format != FormatFile
		}
	}
	return false
}

// MaxPageSize is the most rows a table page holds. The schema refuses a
// larger page_size (P3-02); a view that reaches the runtime without it is
// clamped.
const MaxPageSize = 500

// table runs a table view's page: the query wrapped with the sort and the
// page (5.3), then, when the page doesn't tell the total, the count. A
// page past the last is the last.
func (r *Runner) table(ctx context.Context, v *view.View, t *view.Table, args []any, req TableRequest) (*Table, string, error) {
	size := t.PageSize
	if size <= 0 {
		size = view.DefaultPageSize
	}
	size = min(size, MaxPageSize)
	sort := Sort{Field: t.DefaultSort.Field, Desc: t.DefaultSort.Direction == "desc"}
	if req.Sort != nil && sortable(t, req.Sort.Field) {
		sort = *req.Sort
	}
	page := max(req.Page, 0)
	inner, err := wrapped(v.Query)
	if err != nil {
		return nil, "", err
	}
	order := quoteName(sort.Field)
	if sort.Desc {
		order += " DESC"
	}
	if t.RowsFrom != nil && t.RowsFrom.Key != sort.Field {
		// Rows with the same value keep one order from page to page.
		order += ", " + quoteName(t.RowsFrom.Key)
	}
	fetch := func(page int) (store.Rows, error) {
		q := "SELECT * FROM " + inner + " ORDER BY " + order +
			" LIMIT " + strconv.Itoa(size) + " OFFSET " + strconv.Itoa(page*size)
		// One row more than the page, so a full page doesn't make the
		// worker count the wrapped query again.
		return r.Source.ViewQuery(ctx, q, args, size+1)
	}
	rows, err := fetch(page)
	if err != nil {
		return nil, "", err
	}
	total := -2 // not counted yet
	if len(rows.Values) == 0 && page > 0 {
		// Past the last page: count, and show the last.
		if total, err = r.count(ctx, inner, args); err != nil {
			return nil, "", err
		}
		if total != ManyRows {
			page = max((total-1)/size, 0)
			if rows, err = fetch(page); err != nil {
				return nil, "", err
			}
		}
	}
	fields := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		fields[i] = c.Field
	}
	if t.RowsFrom != nil {
		fields = append(fields, t.RowsFrom.Key)
	}
	idx, err := columnIndex(rows.Columns, fields...)
	if err != nil {
		return nil, "", err
	}
	out := &Table{Page: page, PageSize: size, Sort: sort, Columns: make([]Column, len(t.Columns))}
	for i, c := range t.Columns {
		out.Columns[i] = Column{Field: c.Field, Label: cmpOr(c.Label, c.Field), Format: cmpOr(c.Format, FormatText),
			Options: formatOptions(c.FormatOptions), Align: c.Align}
	}
	out.Rows = make([][]any, len(rows.Values))
	for i, row := range rows.Values {
		cells := make([]any, len(t.Columns))
		for j, c := range out.Columns {
			cells[j] = value(c.Format, row[idx[j]])
		}
		out.Rows[i] = cells
		if t.RowsFrom != nil {
			out.Keys = append(out.Keys, row[idx[len(t.Columns)]])
		}
	}
	var notices []string
	if rows.Full {
		notices = append(notices, NoticeFullPage)
	}
	if problem := r.checkKeys(ctx, out.Columns, out.Rows); problem != "" {
		notices = append(notices, problem)
	}
	n := len(out.Rows)
	switch {
	case total != -2:
		out.Total = total
	case !rows.Full && (n > 0 && n < size || n == 0 && page == 0):
		// The last page tells the total; one the 8 MiB cut short doesn't.
		out.Total = page*size + n
	default:
		out.Total, err = r.count(ctx, inner, args)
		if err != nil {
			return nil, "", err
		}
	}
	return out, strings.Join(notices, " "), nil
}

// Table notices.
const (
	// NoticeFullPage is a page cut short by the 8 MiB a result holds.
	NoticeFullPage = "Showing the rows that fit in 8 MiB. Show fewer columns or rows a page."
	// NoticeNoFiles is a page whose image and file cells couldn't be
	// checked against the bucket, so they are blank.
	NoticeNoFiles = "Can't check the files in the bucket, so they are left out:"
)

// count runs SPEC 5.3's count query; one that runs out of its 2 s is
// ManyRows.
func (r *Runner) count(ctx context.Context, inner string, args []any) (int, error) {
	// Max 2: with 1, the worker would count the one row it keeps by running
	// the query again.
	rows, err := r.Source.ViewQuery(ctx, "SELECT count(*) FROM "+inner, args, 2)
	switch {
	case timedOut(ctx, err):
		return ManyRows, nil
	case err != nil:
		return 0, err
	case len(rows.Values) == 0 || len(rows.Values[0]) == 0:
		return ManyRows, nil
	}
	n, _ := rows.Values[0][0].(int64)
	return int(n), nil
}

// checkKeys clears the image and file cells whose key has no file in the
// bucket, in one read for the page, within a view's 2 s. When the bucket
// can't be read, it clears them all and returns the notice that says so,
// so the rows still show.
func (r *Runner) checkKeys(ctx context.Context, cols []Column, rows [][]any) string {
	var keys []string
	for j, c := range cols {
		if c.Format != FormatImage && c.Format != FormatFile {
			continue
		}
		for _, row := range rows {
			if s, ok := row[j].(string); ok {
				keys = append(keys, s)
			} else {
				row[j] = nil
			}
		}
	}
	if len(keys) == 0 {
		return ""
	}
	kctx, cancel := context.WithTimeout(ctx, store.ViewTimeout)
	defer cancel()
	have, err := r.Source.ObjectsExist(kctx, keys)
	problem := ""
	if err != nil {
		why := fmt.Sprintf("the check took longer than %v.", store.ViewTimeout)
		if !errors.Is(err, context.DeadlineExceeded) {
			why = lowerFirst(queryMessage(err))
		}
		have, problem = nil, NoticeNoFiles+" "+why
	}
	for j, c := range cols {
		if c.Format != FormatImage && c.Format != FormatFile {
			continue
		}
		for _, row := range rows {
			if s, ok := row[j].(string); ok && !have[s] {
				row[j] = nil
			}
		}
	}
	return problem
}

// lowerFirst starts s with a small letter, to go after a colon.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
