package viewrun

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/view"
)

// A 10,001-row line chart shows the first 10,000 rows with the notice,
// sorted by x.
func TestChartTenThousandRows(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: points_line
title: Points
type: chart
chart_type: line
query: SELECT x, y FROM points
x: { field: x, label: X }
y:
  - { field: y, label: Y }
`)
	begin := time.Now()
	b := runner().View(ctx, v, ViewRequest{})
	took := time.Since(begin)
	noError(t, b)
	t.Logf("10,001-row line chart: %v (block %v)", took, b.Took)
	c := b.Chart
	if b.Notice != NoticeRows || c.Type != "line" || len(c.X) != MaxChartRows || len(c.Series) != 1 || c.XAxis.Time || c.Categories != nil {
		t.Fatalf("notice %q, %d x, %d series, %+v", b.Notice, len(c.X), len(c.Series), c.XAxis)
	}
	// The first 10,000 rows in the query's order are ids 1 to 10,000, x
	// 10,000 down to 1: sorted, x = 1 is id 10,000, y 20,000.
	if c.X[0] != 1 || c.X[MaxChartRows-1] != MaxChartRows || c.Series[0].Y[0] != 20000 || c.Series[0].Y[MaxChartRows-1] != 2 ||
		c.Series[0].Name != "Y" || c.XAxis.Label != "X" || c.YAxis.Label != "Y" {
		t.Errorf("x %v…%v, y %v…%v, %+v", c.X[0], c.X[MaxChartRows-1], c.Series[0].Y[0], c.Series[0].Y[MaxChartRows-1], c.Series[0].Name)
	}

	// Exactly 10,000 rows: no notice.
	v.Query = "SELECT x, y FROM points WHERE id <= 10000"
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if b.Notice != "" || len(b.Chart.X) != MaxChartRows {
		t.Errorf("10,000 rows: notice %q, %d", b.Notice, len(b.Chart.X))
	}

	// Scatter, split by coin: each series has its own x, sorted.
	v = parse(t, `version: 1
id: points_scatter
title: Points
type: chart
chart_type: scatter
query: SELECT x, y, coin FROM points WHERE id <= 1000
x: { field: x }
y:
  - { field: y }
series_by: coin
`)
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	c = b.Chart
	if len(c.Series) != 2 || c.X != nil || c.Series[0].Name != "ETH" || len(c.Series[0].X) != 500 || len(c.Series[1].X) != 500 {
		t.Fatalf("%d series, first %s", len(c.Series), c.Series[0].Name)
	}
	for _, s := range c.Series {
		for i := 1; i < len(s.X); i++ {
			if s.X[i] < s.X[i-1] {
				t.Fatalf("%s not sorted at %d", s.Name, i)
			}
		}
	}
}

// Times on x: days at midnight UTC on a time axis, sorted; series_by on a
// line; a time outside the years 1 to 9999 is missing, as NULL is.
func TestChartTimes(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: price_line
title: Prices
type: chart
chart_type: line
query: SELECT day, coin, price FROM prices ORDER BY day DESC
x: { field: day, label: Day, format: date }
y:
  - { field: price, label: Price }
series_by: coin
`)
	b := runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	c := b.Chart
	day := func(s string) float64 {
		d, _ := time.Parse(time.DateOnly, s)
		return float64(d.Unix())
	}
	if !c.XAxis.Time || c.XAxis.Format != "date" || len(c.Series) != 2 {
		t.Fatalf("%+v", c)
	}
	btc, eth := c.Series[0], c.Series[1]
	if btc.Name != "BTC" || len(btc.X) != 4 || btc.X[0] != day("2026-11-18") || btc.X[3] != day("2026-11-21") || btc.Y[3] != 93000 ||
		eth.Name != "ETH" || len(eth.X) != 2 || eth.Y[1] != 3100 {
		t.Errorf("btc %+v, eth %+v", btc, eth)
	}

	// Instants keep their time; one with a zone is that instant.
	v.Query = "SELECT '2026-11-20 08:00:00' AS day, 'BTC' AS coin, 1 AS price UNION ALL SELECT '2026-11-20T10:00:00+03:30', 'BTC', 2"
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	at := time.Date(2026, 11, 20, 6, 30, 0, 0, time.UTC)
	if x := b.Chart.Series[0].X; len(x) != 2 || x[0] != float64(at.Unix()) || x[1] != float64(at.Unix()+5400) {
		t.Errorf("instants %v", x)
	}

	v.Query = "SELECT '0000-01-01' AS day, 'BTC' AS coin, 1 AS price UNION ALL SELECT '2026-11-20', 'BTC', 2"
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if s := b.Chart.Series; len(s) != 1 || len(s[0].X) != 1 || s[0].X[0] != day("2026-11-20") || s[0].Y[0] != 2 || !b.Chart.XAxis.Time {
		t.Errorf("year 0: %+v", b.Chart)
	}
}

// Bars keep the query's order, as categories; more than 1,000 bars are
// cut with the notice, counting each series' bars.
func TestChartBars(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: bars
title: Bars
type: chart
chart_type: bar
query: SELECT name, amount, share FROM big WHERE id <= :n
x: { field: name }
y:
  - { field: amount, label: Amount }
  - { field: share, label: Share }
`)
	for _, tc := range []struct {
		n, want int
		notice  string
	}{{500, 500, ""}, {501, 500, NoticeBars}, {1500, 500, NoticeBars}} {
		v.Params = map[string]any{"n": int64(tc.n)}
		b := runner().View(ctx, v, ViewRequest{})
		noError(t, b)
		c := b.Chart
		if len(c.Categories) != tc.want || len(c.Series) != 2 || len(c.Series[1].Y) != tc.want || b.Notice != tc.notice || c.X != nil {
			t.Errorf("%d rows: %d bars, notice %q", tc.n, len(c.Categories), b.Notice)
		}
		if c.Categories[0] != "row 00001" || c.Series[0].Y[0] != 19999.5 || c.Series[1].Name != "Share" {
			t.Errorf("first bar %s %v", c.Categories[0], c.Series[0].Y[0])
		}
	}
	// One series: 1,000 bars, then the notice.
	ch := v.Spec.(*view.Chart)
	ch.Y = ch.Y[:1]
	v.Params = map[string]any{"n": int64(1001)}
	b := runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if len(b.Chart.Categories) != MaxBars || b.Notice != NoticeBars {
		t.Errorf("one series: %d bars, %q", len(b.Chart.Categories), b.Notice)
	}

	// Split by coin: a y for each category in each series, NaN where a
	// coin has no row.
	v = parse(t, `version: 1
id: coin_bars
title: Bars
type: chart
chart_type: bar
query: SELECT day, coin, price FROM prices
x: { field: day, format: date }
y:
  - { field: price }
series_by: coin
`)
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	c := b.Chart
	if strings.Join(c.Categories, ",") != "2026-11-18,2026-11-19,2026-11-20,2026-11-21" || len(c.Series) != 2 {
		t.Fatalf("%+v", c)
	}
	if eth := c.Series[1]; eth.Name != "ETH" || eth.Y[0] != 3000 || !math.IsNaN(eth.Y[1]) || eth.Y[2] != 3100 || !math.IsNaN(eth.Y[3]) {
		t.Errorf("eth %+v", eth)
	}
}

// A pie passes its rows (the kit folds the slices past 11 into "Other");
// a negative value is an error naming the slice. A scatter needs numbers
// or times on x; a line along text has categories.
func TestChartPieAndText(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: pie
title: Pie
type: chart
chart_type: pie
query: SELECT name, amount FROM big WHERE id <= 20
x: { field: name }
y:
  - { field: amount }
`)
	b := runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if len(b.Chart.Categories) != 20 || len(b.Chart.Series) != 1 || len(b.Chart.Series[0].Y) != 20 {
		t.Errorf("pie %+v", b.Chart)
	}
	v.Query = "SELECT sym, change FROM coins"
	ch := v.Spec.(*view.Chart)
	ch.X.Field, ch.Y[0].Field = "sym", "change"
	b = runner().View(ctx, v, ViewRequest{})
	if b.Error != "A pie can't show negative values: ETH is -0.01." || b.Chart != nil {
		t.Errorf("negative: %q", b.Error)
	}

	ch.ChartType = "scatter"
	b = runner().View(ctx, v, ViewRequest{})
	if !strings.Contains(b.Error, "scatter chart needs numbers or dates for x; sym has text") {
		t.Errorf("scatter of text: %q", b.Error)
	}
	ch.ChartType = "line"
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if c := b.Chart; strings.Join(c.Categories, ",") != "BTC,ETH,XRP" || c.X != nil || !math.IsNaN(c.Series[0].Y[2]) {
		t.Errorf("line of text %+v", c)
	}
}

// Rows of a line with no x are left out; a y that is NULL or no number is
// a gap.
func TestChartGaps(t *testing.T) {
	v := parse(t, `version: 1
id: gaps
title: Gaps
type: chart
chart_type: line
query: SELECT 3 AS x, 'n/a' AS y UNION ALL SELECT NULL, 1 UNION ALL SELECT 1, NULL UNION ALL SELECT 2, '5'
x: { field: x }
y:
  - { field: y }
`)
	b := runner().View(context.Background(), v, ViewRequest{})
	noError(t, b)
	c := b.Chart
	if got := strconv.Itoa(len(c.X)); got != "3" || c.X[0] != 1 || !math.IsNaN(c.Series[0].Y[0]) || c.Series[0].Y[1] != 5 || !math.IsNaN(c.Series[0].Y[2]) {
		t.Errorf("x %v, y %v", c.X, c.Series[0].Y)
	}
}
