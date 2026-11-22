package viewrun

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/amir-saatchi/jenab/internal/view"
)

// Chart is what a chart view draws (SPEC 5.4). It maps onto the kit's
// ChartData field for field; an axis's Format is the view's format name,
// which P3-09 turns into the kit's func(float64) string with FormatValue
// (and uses to write Categories that are dates).
type Chart struct {
	Type string // line, bar, scatter or pie
	// Categories are the x values as text, for bars, pie slices and a
	// line along text, in the order the query gives them.
	Categories []string
	// X are the x values of a line or a scatter plot along numbers or
	// times (Unix seconds), sorted; nil with Categories, or when each
	// series has its own (series_by).
	X            []float64
	XAxis, YAxis Axis
	Series       []ChartSeries
}

// Axis is a chart's axis, as the kit's ChartAxis.
type Axis struct {
	Label  string
	Format string // the view's format for x; "" for plain numbers
	// Time: the values are Unix seconds. Days are at midnight UTC, so
	// that an axis of days stays in UTC, as the kit's TimeOf rule has it.
	Time bool
}

// ChartSeries is one series, as the kit's ChartSeries: a name and a y for
// each x or category, NaN for none. X is the series' own x, sorted, for a
// line or scatter split by series_by.
type ChartSeries struct {
	Name string
	X    []float64
	Y    []float64
}

// The size rules (SPEC 5.4, P3-08).
const (
	// MaxChartRows is the most rows a chart shows; the query is cut at
	// MaxChartRows+1 to tell there are more.
	MaxChartRows = 10_000
	// MaxBars is the most bars a bar chart shows: categories times
	// series.
	MaxBars = 1_000
)

// Notices for charts.
const (
	NoticeRows = "Showing the first 10,000 rows. Aggregate in the query to see them all."
	NoticeBars = "Showing the first 1,000 bars. Aggregate in the query to see them all."
	noticeFull = "Showing the rows that fit in 8 MiB. Aggregate in the query to see them all."
)

// chart runs a chart view: SELECT * FROM (<query>) LIMIT 10001, then the
// rows made into series.
func (r *Runner) chart(ctx context.Context, v *view.View, c *view.Chart, args []any) (*Chart, string, error) {
	inner, err := wrapped(v.Query)
	if err != nil {
		return nil, "", err
	}
	q := fmt.Sprintf("SELECT * FROM %s LIMIT %d", inner, MaxChartRows+1)
	// One more than the limit, so the worker doesn't run the query again
	// to count it.
	rows, err := r.Source.ViewQuery(ctx, q, args, MaxChartRows+2)
	if err != nil {
		return nil, "", err
	}
	var notices []string
	vals := rows.Values
	if len(vals) > MaxChartRows {
		vals = vals[:MaxChartRows]
		notices = append(notices, NoticeRows)
	} else if rows.Full {
		notices = append(notices, noticeFull)
	}
	fields := []string{c.X.Field}
	for _, y := range c.Y {
		fields = append(fields, y.Field)
	}
	if c.SeriesBy != "" {
		fields = append(fields, c.SeriesBy)
	}
	idx, err := columnIndex(rows.Columns, fields...)
	if err != nil {
		return nil, "", err
	}
	out := &Chart{Type: c.ChartType, XAxis: Axis{Label: c.X.Label, Format: c.X.Format}}
	if len(c.Y) == 1 {
		out.YAxis.Label = c.Y[0].Label
	}
	pts := make([]point, len(vals))
	for i, row := range vals {
		p := point{x: row[idx[0]], ys: make([]float64, len(c.Y))}
		for j := range c.Y {
			p.ys[j] = floatOf(row[idx[1+j]])
		}
		if c.SeriesBy != "" {
			p.series = text(row[idx[len(idx)-1]])
		}
		pts[i] = p
	}
	switch c.ChartType {
	case "bar", "pie":
		out.categories(c, pts)
	default:
		kind := xKind(pts)
		if kind == xText {
			if c.ChartType == "scatter" {
				return nil, "", fmt.Errorf("a scatter chart needs numbers or dates for x; %s has text", c.X.Field)
			}
			out.categories(c, pts)
			break
		}
		out.along(c, pts, kind)
	}
	if c.ChartType == "pie" && len(out.Series) > 0 {
		for i, y := range out.Series[0].Y {
			if y < 0 {
				return nil, "", fmt.Errorf("a pie can't show negative values: %s is %s", out.Categories[i], text(y))
			}
		}
	}
	if c.ChartType == "bar" && len(out.Categories)*len(out.Series) > MaxBars {
		keep := max(MaxBars/max(len(out.Series), 1), 1)
		out.Categories = out.Categories[:keep]
		for i := range out.Series {
			out.Series[i].Y = out.Series[i].Y[:keep]
		}
		notices = append(notices, NoticeBars)
	}
	notice := ""
	for i, n := range notices {
		if i > 0 {
			notice += " "
		}
		notice += n
	}
	return out, notice, nil
}

// point is one row of a chart: its x as the query gives it, its y values,
// and its series_by value.
type point struct {
	x      any
	ys     []float64
	series string
}

type xkind int

const (
	xNumber xkind = iota
	xTime
	xText
)

// xKind tells what the x values are: numbers when every one that isn't
// NULL is, times when every one reads as a date or time, else text. A
// time outside the years 1 to 9999 counts as missing.
func xKind(pts []point) xkind {
	numbers, times := true, true
	for _, p := range pts {
		if p.x == nil || outOfYears(p.x) {
			continue
		}
		switch number(p.x).(type) {
		case int64, float64:
		default:
			numbers = false
		}
		if _, _, ok := timeOf(p.x); !ok {
			times = false
		}
		if !numbers && !times {
			return xText
		}
	}
	if numbers {
		return xNumber
	}
	return xTime
}

// outOfYears reports whether x is a time outside the years 1 to 9999,
// which a time axis can't place: it is missing, as NULL is (SPEC 5.4).
func outOfYears(x any) bool {
	t, _, ok := timeOf(x)
	return ok && (t.Year() < 1 || t.Year() > 9999)
}

// xValue is p's x as a number: Unix seconds for a time, a day at midnight
// UTC; NaN for none.
func xValue(x any, kind xkind) float64 {
	if kind == xNumber {
		return floatOf(x)
	}
	t, day, ok := timeOf(x)
	if !ok || t.Year() < 1 || t.Year() > 9999 {
		return math.NaN()
	}
	if day {
		y, m, d := t.Date()
		t = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	return float64(t.Unix()) + float64(t.Nanosecond())/1e9
}

// along fills a line or scatter plot along numbers or times, sorted by x.
// A row with no x is left out. With series_by, the series come in the
// order the query first gives them.
func (ch *Chart) along(c *view.Chart, pts []point, kind xkind) {
	ch.XAxis.Time = kind == xTime
	type xp struct {
		x float64
		p *point
	}
	xs := make([]xp, 0, len(pts))
	for i := range pts {
		if x := xValue(pts[i].x, kind); !math.IsNaN(x) {
			xs = append(xs, xp{x, &pts[i]})
		}
	}
	slices.SortStableFunc(xs, func(a, b xp) int {
		switch {
		case a.x < b.x:
			return -1
		case a.x > b.x:
			return 1
		}
		return 0
	})
	if c.SeriesBy == "" {
		ch.X = make([]float64, len(xs))
		ch.Series = make([]ChartSeries, len(c.Y))
		for j, y := range c.Y {
			ch.Series[j] = ChartSeries{Name: cmpOr(y.Label, y.Field), Y: make([]float64, len(xs))}
		}
		for i, p := range xs {
			ch.X[i] = p.x
			for j := range c.Y {
				ch.Series[j].Y[i] = p.p.ys[j]
			}
		}
		return
	}
	// The series in the order the query gives them first.
	at := map[string]int{}
	for _, p := range pts {
		if _, ok := at[p.series]; !ok {
			at[p.series] = len(ch.Series)
			ch.Series = append(ch.Series, ChartSeries{Name: p.series})
		}
	}
	for _, p := range xs {
		i := at[p.p.series]
		ch.Series[i].X = append(ch.Series[i].X, p.x)
		ch.Series[i].Y = append(ch.Series[i].Y, p.p.ys[0])
	}
}

// categories fills a chart along text: bars, pie slices, or a line along
// text. A row with no x is left out. Categories keep the order the query gives them; with series_by,
// each series has a y for each category, NaN where it has no row, and a
// later row for the same category and series takes its place.
func (ch *Chart) categories(c *view.Chart, all []point) {
	pts := make([]point, 0, len(all))
	for _, p := range all {
		if p.x != nil {
			pts = append(pts, p)
		}
	}
	at := map[string]int{}
	for _, p := range pts {
		k := text(p.x)
		if _, ok := at[k]; !ok {
			at[k] = len(ch.Categories)
			ch.Categories = append(ch.Categories, k)
		}
	}
	if c.SeriesBy == "" {
		ch.Series = make([]ChartSeries, len(c.Y))
		for j, y := range c.Y {
			ch.Series[j] = ChartSeries{Name: cmpOr(y.Label, y.Field), Y: nans(len(ch.Categories))}
		}
		for _, p := range pts {
			i := at[text(p.x)]
			for j := range c.Y {
				ch.Series[j].Y[i] = p.ys[j]
			}
		}
		return
	}
	series := map[string]int{}
	for _, p := range pts {
		s, ok := series[p.series]
		if !ok {
			s = len(ch.Series)
			series[p.series] = s
			ch.Series = append(ch.Series, ChartSeries{Name: p.series, Y: nans(len(ch.Categories))})
		}
		ch.Series[s].Y[at[text(p.x)]] = p.ys[0]
	}
}

func nans(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}
