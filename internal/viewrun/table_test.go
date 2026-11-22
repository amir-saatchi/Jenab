package viewrun

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

const bigTable = `version: 1
id: big_table
title: Big
type: table
query: |
  SELECT id, name, amount, share, day, at, ok, status, link, file FROM big
  WHERE (:status IS NULL OR status = :status) -- a final comment
filters:
  - { param: status, control: select, label: Status, options: { query: "SELECT DISTINCT status FROM big ORDER BY status" } }
default_sort: { field: amount, direction: desc }
rows_from: { table: big, key: id }
columns:
  - { field: name, label: Name }
  - { field: amount, label: Amount, format: currency, format_options: { currency: USD, decimals: 2 } }
  - { field: share, format: percent }
  - { field: day, format: date }
  - { field: at, format: datetime }
  - { field: ok, format: boolean }
  - { field: status, format: badge, format_options: { tones: { new: blue, failed: red } } }
  - { field: link, format: url }
  - { field: file, format: image }
`

// A 10,000-row table: the first page by the default sort, typed for the
// formats, the count, other sorts and pages, a filter, the bucket check.
func TestTable(t *testing.T) {
	ctx := context.Background()
	r := runner()
	v := parse(t, bigTable)

	begin := time.Now()
	b := r.View(ctx, v, ViewRequest{})
	took := time.Since(begin)
	noError(t, b)
	tb := b.Table
	t.Logf("10,000-row table, first page by amount desc, with its count: %v (block %v)", took, b.Took)
	if tb.Total != bigRows || len(tb.Rows) != 50 || tb.PageSize != 50 || tb.Page != 0 || tb.Sort != (Sort{"amount", true}) {
		t.Fatalf("total %d, %d rows, page size %d, page %d, sort %+v", tb.Total, len(tb.Rows), tb.PageSize, tb.Page, tb.Sort)
	}
	if b.Notice != "" || len(b.Params) != 1 || b.Params[0] != "status" {
		t.Errorf("notice %q, params %v", b.Notice, b.Params)
	}
	if len(tb.Columns) != 9 || !reflect.DeepEqual(tb.Columns[0], Column{Field: "name", Label: "Name", Format: FormatText}) ||
		tb.Columns[1].Options.Currency != "USD" || *tb.Columns[1].Options.Decimals != 2 || tb.Columns[2].Label != "share" ||
		tb.Columns[6].Options.Tones["failed"] != "red" {
		t.Errorf("columns %+v", tb.Columns)
	}
	// Row 1 has the largest amount; it has the file.
	row := tb.Rows[0]
	want := []any{"row 00001", 19999.5, 0.0001, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC), true, "read", "https://example.test/1", "images/a.png"}
	for i, w := range want {
		if got := row[i]; got != w {
			if gt, ok := got.(time.Time); !ok || !gt.Equal(w.(time.Time)) {
				t.Errorf("column %s: %#v, want %#v", tb.Columns[i].Field, got, w)
			}
		}
	}
	if tb.Keys[0] != int64(1) || tb.Keys[49] != int64(50) || len(tb.Keys) != 50 {
		t.Errorf("keys %v … %v", tb.Keys[0], tb.Keys[49])
	}
	// A key with no file, and a key that is no key, are blank.
	if tb.Rows[1][8] != nil || tb.Rows[2][8] != nil || tb.Rows[3][8] != nil {
		t.Errorf("files %v %v %v", tb.Rows[1][8], tb.Rows[2][8], tb.Rows[3][8])
	}
	if f := b.Filters[0]; f.Param != "status" || f.Value != nil || strings.Join(texts(f.Options), ",") != "failed,kept,new,read" {
		t.Errorf("filter %+v", f)
	}

	// Sorted by name ascending, page 3; the count runs again.
	b = r.View(ctx, v, ViewRequest{Table: TableRequest{Sort: &Sort{Field: "name"}, Page: 3}})
	noError(t, b)
	if tb := b.Table; tb.Total != bigRows || tb.Rows[0][0] != "row 00151" || tb.Rows[49][0] != "row 00200" || tb.Sort != (Sort{Field: "name"}) {
		t.Errorf("page 3 by name: total %d, %v … %v", tb.Total, tb.Rows[0][0], tb.Rows[49][0])
	}
	// The last page; one past it, and far past it, are the last.
	for _, page := range []int{199, 200, 5000} {
		b = r.View(ctx, v, ViewRequest{Table: TableRequest{Page: page}})
		noError(t, b)
		if tb := b.Table; len(tb.Rows) != 50 || tb.Total != bigRows || tb.Page != 199 || tb.Rows[49][0] != "row 10000" {
			t.Errorf("page %d: page %d, %d rows of %d", page, tb.Page, len(tb.Rows), tb.Total)
		}
	}
	// A column the table doesn't show, or a picture, can't sort it.
	for _, f := range []string{"id", "file", "nope", "Name"} {
		b = r.View(ctx, v, ViewRequest{Table: TableRequest{Sort: &Sort{Field: f}}})
		noError(t, b)
		if b.Table.Sort != (Sort{"amount", true}) {
			t.Errorf("sort by %s: %+v", f, b.Table.Sort)
		}
	}
	// A filter: 2,000 rows are failed; the select's text binds its option.
	b = r.View(ctx, v, ViewRequest{Values: Values{"status": "failed"}, Table: TableRequest{Page: 39}})
	noError(t, b)
	if b.Table.Total != 2000 || len(b.Table.Rows) != 50 || b.Table.Rows[0][6] != "failed" || b.Filters[0].Value != "failed" {
		t.Errorf("failed: %d of %d, %v", len(b.Table.Rows), b.Table.Total, b.Filters[0].Value)
	}
}

// A page that isn't full tells the total without a count; an empty first
// page is 0.
func TestTableShort(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: few
title: Few
type: table
query: SELECT sym, price, rank FROM coins WHERE rank <= :most
params: { most: 2 }
default_sort: { field: price, direction: asc }
page_size: 5
columns:
  - { field: sym }
  - { field: price, format: number }
`)
	b := runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if tb := b.Table; tb.Total != 2 || len(tb.Rows) != 2 || tb.Rows[0][0] != "ETH" || tb.Rows[1][1] != 92500.0 || tb.Keys != nil {
		t.Errorf("%+v", tb)
	}
	v.Params["most"] = int64(0)
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if b.Table.Total != 0 || len(b.Table.Rows) != 0 {
		t.Errorf("%+v", b.Table)
	}
}

// When the count runs out of its 2 s, the rows still come, with "many
// rows" for the total. The rows walk the table in key order and stop
// after a page; the count has to read all 10,000 rows, each with a
// subquery over 40,000.
func TestTableManyRows(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: slow_count
title: Slow count
type: table
query: |
  SELECT id, name FROM big
  WHERE (SELECT count(*) FROM big b2 JOIN big b3 ON b3.id <= 4 WHERE b2.amount >= big.amount - 1e12) > 0
default_sort: { field: id, direction: asc }
columns:
  - { field: id, format: number }
  - { field: name }
`)
	begin := time.Now()
	b := runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	t.Logf("rows and a count that timed out: %v", time.Since(begin))
	if b.Table.Total != ManyRows || len(b.Table.Rows) != 50 || b.Table.Rows[0][0] != int64(1) {
		t.Errorf("total %d, %d rows", b.Table.Total, len(b.Table.Rows))
	}
}

// A result column that is gone, as after a migration, is an error on the
// block that names the columns there are; names match as written.
func TestTableMissingColumn(t *testing.T) {
	v := parse(t, `version: 1
id: gone
title: Gone
type: table
query: SELECT sym AS Sym, price FROM coins
default_sort: { field: price, direction: asc }
columns:
  - { field: sym }
`)
	b := runner().View(context.Background(), v, ViewRequest{})
	if b.Error != "'sym' is not a result column of the query; it has Sym, price; names are matched as written." {
		t.Errorf("error %q", b.Error)
	}
	if b.Table != nil {
		t.Error("a table with an error")
	}
}

func texts(vs []any) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = text(v)
	}
	return out
}
