package viewrun

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// A view's 2 s start when a worker takes its query: a block that waits
// behind slow ones for longer than 2 s still gets its rows and its count.
// The four slow blocks hold both workers for 2 s, twice over; the table's
// queries come 200 ms later, so they queue behind them.
func TestWaitForWorker(t *testing.T) {
	ctx := context.Background()
	p := parsePage(t, `version: 1
id: busy
title: Busy
rows:
  - [ { view: slow }, { view: slow } ]
  - [ { view: slow }, { view: slow } ]
  - [ { view: big_table } ]
`)
	r := hookedRunner(hooked{query: func(ctx context.Context, q string, args []any, max int) (store.Rows, error) {
		if strings.Contains(q, "FROM big") {
			time.Sleep(200 * time.Millisecond)
		}
		return fixture.ViewQuery(ctx, q, args, max)
	}})
	c := newCollect()
	if err := r.Page(ctx, p, views(t, "slow", "big_table"), PageRequest{}, c.out); err != nil {
		t.Fatal(err)
	}
	for _, pl := range []string{"0.0", "0.1", "1.0", "1.1"} {
		if b := c.block(t, pl); !strings.HasPrefix(b.Error, "The query took longer than 2s") {
			t.Errorf("slow block %s: %q", pl, b.Error)
		}
	}
	b := c.block(t, "2.0")
	noError(t, b)
	if b.Table.Total != bigRows || len(b.Table.Rows) != 50 || len(b.Filters[0].Options) != 4 {
		t.Errorf("table: %d rows of %d, options %v", len(b.Table.Rows), b.Table.Total, b.Filters[0].Options)
	}
	if c.at["2.0"] < 3*time.Second {
		t.Errorf("the table answered at %v, so it didn't wait behind the slow blocks", c.at["2.0"])
	}
	t.Logf("slow blocks at %v %v %v %v; the table, which waited for a worker, at %v",
		c.at["0.0"], c.at["0.1"], c.at["1.0"], c.at["1.1"], c.at["2.0"])
}

// A page the 8 MiB cut short is not the last: the count runs, with max 2,
// and the notice says why the page is short. page_size is at most 500.
func TestTableFullPage(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var countMax []int
	r := hookedRunner(hooked{query: func(ctx context.Context, q string, args []any, max int) (store.Rows, error) {
		rows, err := fixture.ViewQuery(ctx, q, args, max)
		switch {
		case strings.HasPrefix(q, "SELECT count(*)"):
			mu.Lock()
			countMax = append(countMax, max)
			mu.Unlock()
		case strings.HasPrefix(q, "SELECT * FROM") && err == nil && len(rows.Values) > 10:
			rows.Values, rows.Full = rows.Values[:10], true
		}
		return rows, err
	}})
	v := parse(t, bigTable)
	for _, page := range []int{0, 199} {
		b := r.View(ctx, v, ViewRequest{Table: TableRequest{Page: page}})
		noError(t, b)
		if tb := b.Table; len(tb.Rows) != 10 || tb.Total != bigRows || tb.Page != page || b.Notice != NoticeFullPage {
			t.Errorf("page %d: page %d, %d rows of %d, notice %q", page, tb.Page, len(tb.Rows), tb.Total, b.Notice)
		}
	}
	if !slices.Equal(countMax, []int{2, 2}) {
		t.Errorf("count queries with max %v, want 2 each", countMax)
	}

	// The schema refuses a page_size over 500 (P3-02); ParseView doesn't
	// check, so the runtime clamps.
	big := parse(t, strings.Replace(bigTable, "default_sort:", "page_size: 1000\ndefault_sort:", 1))
	if big.Spec.(*view.Table).PageSize != 1000 {
		t.Fatal("page_size didn't parse")
	}
	b := runner().View(ctx, big, ViewRequest{})
	noError(t, b)
	if tb := b.Table; tb.PageSize != MaxPageSize || len(tb.Rows) != MaxPageSize || tb.Total != bigRows {
		t.Errorf("page_size 1000: size %d, %d rows of %d", tb.PageSize, len(tb.Rows), tb.Total)
	}
}

// "" and blanks are cleared, as nil is: an optional filter binds NULL, a
// required one its default or first option, and says it needs a value; a
// required checkbox is unchecked.
func TestFiltersCleared(t *testing.T) {
	v := parse(t, filterView)
	b := runner().View(context.Background(), v, ViewRequest{Values: Values{
		"day": "", "since": "", "coin": " ", "rank": nil, "min": "", "sym": "\t ", "must": "", "word": "  ",
	}})
	noError(t, b)
	got := map[string]any{}
	for i, c := range b.Table.Columns {
		got[c.Field] = b.Table.Rows[0][i]
	}
	want := map[string]any{"day": nil, "since": "2026-10-21", "coin": nil, "rank": int64(3), "min": nil,
		"sym": "BTC", "mine": nil, "must": int64(0), "word": nil}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %#v, want %#v", k, got[k], w)
		}
	}
	for _, f := range b.Filters {
		want := ""
		if f.Param == "since" || f.Param == "rank" || f.Param == "sym" {
			want = NeedsValue
		}
		if f.Error != want {
			t.Errorf("%s: error %q, want %q", f.Param, f.Error, want)
		}
	}
}

// NaN and infinities: a filter refuses them; a chart has a gap; a stat
// shows nothing.
func TestNotANumber(t *testing.T) {
	ctx := context.Background()
	v := parse(t, filterView)
	for given, problem := range map[any]string{
		"NaN": "NaN is not a number", "Inf": "Inf is not a number", "-infinity": "-infinity is not a number",
		math.NaN(): "NaN is not a number", math.Inf(1): "+Inf is not a number",
	} {
		b := runner().View(ctx, v, ViewRequest{Values: Values{"min": given}})
		noError(t, b)
		if f := b.Filters[4]; f.Error != problem || f.Value != int64(5) {
			t.Errorf("min %v: %+v", given, f)
		}
	}

	c := parse(t, `version: 1
id: inf
title: Inf
type: chart
chart_type: line
query: SELECT 1 AS x, 9e999 AS y UNION ALL SELECT 2, -9e999 UNION ALL SELECT 3, 'NaN' UNION ALL SELECT 4, 7
x: { field: x }
y:
  - { field: y }
`)
	b := runner().View(ctx, c, ViewRequest{})
	noError(t, b)
	y := b.Chart.Series[0].Y
	if len(y) != 4 || !math.IsNaN(y[0]) || !math.IsNaN(y[1]) || !math.IsNaN(y[2]) || y[3] != 7 {
		t.Errorf("y %v", y)
	}

	s := parse(t, `version: 1
id: inf_stat
title: Inf
type: stat
query: SELECT 9e999 AS v, 'NaN' AS c
value: { field: v }
change: { field: c }
`)
	b = runner().View(ctx, s, ViewRequest{})
	noError(t, b)
	if b.Stat.Value != nil || b.Stat.Change != nil {
		t.Errorf("stat %+v", b.Stat)
	}
	s.Query = "SELECT 'Infinity' AS v, -9e999 AS c"
	b = runner().View(ctx, s, ViewRequest{})
	noError(t, b)
	if b.Stat.Value != nil || b.Stat.Change != nil {
		t.Errorf("stat %+v", b.Stat)
	}
}

// When the bucket can't be read, the image and file cells are blank and a
// notice says why; the rows still show. The check has a view's 2 s.
func TestBucketCheckFails(t *testing.T) {
	ctx := context.Background()
	v := parse(t, bigTable)
	for fault, why := range map[error]string{
		errors.New("disk I/O error"): "",
		context.DeadlineExceeded:     "the check took longer than 2s.",
	} {
		var deadline time.Duration
		r := hookedRunner(hooked{exists: func(ctx context.Context, keys []string) (map[string]bool, error) {
			if d, ok := ctx.Deadline(); ok {
				deadline = time.Until(d)
			}
			return nil, fault
		}})
		b := r.View(ctx, v, ViewRequest{})
		noError(t, b)
		if deadline <= 0 || deadline > store.ViewTimeout {
			t.Errorf("%v: the check's deadline was %v away", fault, deadline)
		}
		if !strings.HasPrefix(b.Notice, NoticeNoFiles+" ") || !strings.HasSuffix(b.Notice, why) {
			t.Errorf("%v: notice %q", fault, b.Notice)
		}
		t.Logf("%v: %s", fault, b.Notice)
		if b.Table.Total != bigRows || len(b.Table.Rows) != 50 {
			t.Errorf("%v: %d rows of %d", fault, len(b.Table.Rows), b.Table.Total)
		}
		for i, row := range b.Table.Rows {
			if row[8] != nil {
				t.Errorf("%v: row %d has file %v", fault, i, row[8])
			}
		}
	}
}

// A panic in one block, in its views or its query, is an error on that
// block only.
func TestPanics(t *testing.T) {
	ctx := context.Background()
	p := parsePage(t, `version: 1
id: panics
title: Panics
rows:
  - [ { view: boom }, { view: price }, { view: since } ]
`)
	fine := views(t, "price", "since")
	vs := func(id string) (*view.View, error) {
		if id == "boom" {
			panic("no views today")
		}
		return fine(id)
	}
	r := hookedRunner(hooked{query: func(ctx context.Context, q string, args []any, max int) (store.Rows, error) {
		if strings.Contains(q, "FROM coins WHERE sym") {
			panic("bad driver")
		}
		return fixture.ViewQuery(ctx, q, args, max)
	}})
	c := newCollect()
	if err := r.Page(ctx, p, vs, PageRequest{}, c.out); err != nil {
		t.Fatal(err)
	}
	if b := c.block(t, "0.0"); !strings.HasPrefix(b.Error, "Something went wrong") || !strings.Contains(b.Error, "no views today") {
		t.Errorf("boom: %q", b.Error)
	}
	if b := c.block(t, "0.1"); !strings.HasPrefix(b.Error, "Something went wrong") || !strings.Contains(b.Error, "bad driver") || b.View != "price" {
		t.Errorf("price: %+v", b)
	}
	noError(t, c.block(t, "0.2"))
	if len(c.blocks) != 3 {
		t.Errorf("%d blocks", len(c.blocks))
	}
	b := r.View(ctx, fine.must(t, "price"), ViewRequest{})
	if !strings.Contains(b.Error, "bad driver") {
		t.Errorf("view on its own: %q", b.Error)
	}
}

// A view's options queries run at once: two that take 300 ms each are
// ready in about 300 ms.
func TestOptionsParallel(t *testing.T) {
	ctx := context.Background()
	v := parse(t, filterView)
	r := hookedRunner(hooked{query: func(ctx context.Context, q string, args []any, max int) (store.Rows, error) {
		if strings.Contains(q, "FROM coins ORDER BY") {
			time.Sleep(300 * time.Millisecond)
		}
		return fixture.ViewQuery(ctx, q, args, max)
	}})
	runner().View(ctx, v, ViewRequest{}) // warm the workers
	begin := time.Now()
	b := r.View(ctx, v, ViewRequest{})
	took := time.Since(begin)
	noError(t, b)
	if len(b.Filters[3].Options) != 3 || len(b.Filters[5].Options) != 3 {
		t.Errorf("options %v %v", b.Filters[3].Options, b.Filters[5].Options)
	}
	if took > 550*time.Millisecond {
		t.Errorf("two 300 ms options queries took %v", took)
	}
	t.Logf("two 300 ms options queries and the view: %v", took)
}

// A NULL x has no bar or slice: it is skipped.
func TestChartNullX(t *testing.T) {
	for _, kind := range []string{"bar", "pie"} {
		v := parse(t, `version: 1
id: null_x
title: Null x
type: chart
chart_type: `+kind+`
query: SELECT NULL AS k, 1 AS v UNION ALL SELECT 'a', 2 UNION ALL SELECT 'b', 3
x: { field: k }
y:
  - { field: v }
`)
		b := runner().View(context.Background(), v, ViewRequest{})
		noError(t, b)
		if c := b.Chart; !slices.Equal(c.Categories, []string{"a", "b"}) || !slices.Equal(c.Series[0].Y, []float64{2, 3}) {
			t.Errorf("%s: categories %v, y %v", kind, c.Categories, c.Series[0].Y)
		}
	}
}

// Two blocks of one view keep their own table state: Blocks is by place.
func TestSameViewTwice(t *testing.T) {
	p := parsePage(t, `version: 1
id: twice
title: Twice
rows:
  - [ { view: big_table }, { view: big_table } ]
`)
	c := newCollect()
	err := runner().Page(context.Background(), p, views(t, "big_table"), PageRequest{Blocks: map[string]ViewRequest{
		"0.0": {Table: TableRequest{Page: 1}},
		"0.1": {Values: Values{"status": "failed"}, Table: TableRequest{Page: 2, Sort: &Sort{Field: "name"}}},
	}}, c.out)
	if err != nil {
		t.Fatal(err)
	}
	a, b := c.block(t, "0.0"), c.block(t, "0.1")
	noError(t, a)
	noError(t, b)
	if a.Table.Page != 1 || a.Table.Total != bigRows || a.Filters[0].Value != nil || a.Table.Sort != (Sort{"amount", true}) {
		t.Errorf("0.0: page %d of %d, %v, %+v", a.Table.Page, a.Table.Total, a.Filters[0].Value, a.Table.Sort)
	}
	if b.Table.Page != 2 || b.Table.Total != 2000 || b.Filters[0].Value != "failed" || b.Table.Sort != (Sort{Field: "name"}) {
		t.Errorf("0.1: page %d of %d, %v, %+v", b.Table.Page, b.Table.Total, b.Filters[0].Value, b.Table.Sort)
	}
}
