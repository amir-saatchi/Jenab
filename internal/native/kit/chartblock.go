package kit

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// ChartType is what a chart block draws (SPEC 5.4).
type ChartType string

const (
	ChartLine    ChartType = "line"
	ChartBar     ChartType = "bar"
	ChartScatter ChartType = "scatter"
	ChartPie     ChartType = "pie"
)

// ChartAxis is an axis of a chart block: its title, and how its values
// are written on the ticks and in the readout. A nil Format writes plain
// numbers. Time, on the x axis, takes the values as Unix seconds: the
// ticks fall on round times, and the readout writes the date. As a
// table shows times (TimeOf), values all at midnight UTC are days, the
// same in every zone, and any others are instants, shown in the
// computer's zone (time.Local).
type ChartAxis struct {
	Label  string
	Format func(float64) string
	Time   bool
}

// ChartSeries is a series of a chart block: its name in the legend and
// its values, NaN for none. A line or a scatter series may have an x of
// its own for each value, as the series a series_by field makes; nil X
// takes the chart's X, or its Categories.
type ChartSeries struct {
	Name string
	X    []float64
	Y    []float64
}

// ChartData is what a chart block shows, in plain Go: the type, the x
// (Categories for bars and pie slices, and for a line along text; X for
// the numbers or times of a line or a scatter plot), the axes and the
// series. A pie takes the first series' values, one for each category.
// A notice about the data, as "Showing the first 10,000 rows", is the
// block's BlockState.Notice, as for the other data blocks.
// The series take the tones in their fixed order (Tones).
//
// The chart keeps what it works out from the data while it is given the
// same *ChartData: give it a new one when the data changes.
type ChartData struct {
	Type         ChartType
	Categories   []string
	X            []float64
	XAxis, YAxis ChartAxis
	Series       []ChartSeries
}

// The room a chart block's plot takes, which its empty state keeps.
const chartHeight = 220

// PieSlices is the most slices a pie draws, "Other" among them (SPEC
// 5.4): with more values than that, the largest PieSlices-1 are drawn and
// the rest are summed into "Other".
const PieSlices = 12

// ChartBlock is a chart block of a page (SPEC 5.4) drawing data on
// MyGo's ui/chart: a line, bars grouped by category, a scatter plot or a
// pie, with grid lines at the value ticks and the legend below. Its plot
// takes the focus, where the arrows, Home and End choose a point, a bar
// or a slice, whose values show below it and are read out; Escape clears
// the choice, as does the pointer leaving it. st shows the block loading,
// failed or with a notice above it (BlockBody); data with no values
// shows "No data", and data a chart can't show, as a pie of negative
// values, an error. label names it. A chart is laid out left to right in
// a right-to-left language too, as charts are.
func ChartBlock(c *Context, label string, data *ChartData, st BlockState) Element {
	col := ui.Column(c).AlignItems(ui.Stretch)
	m := ui.Local(col, "chart", func() chartModel { return chartModel{} })
	sel := ui.Local(col, "chart.selected", func() int { return -1 })
	col.Children(func() {
		BlockBody(c, label, st, func() { m.show(c, label, data, sel) })
	})
	return col
}

// show builds the chart of data, or why there is none.
func (m *chartModel) show(c *Context, label string, data *ChartData, sel *int) {
	if data == nil {
		chartEmpty(c)
		return
	}
	m.update(data, formatLocale(c))
	if m.err != "" {
		Alert(c, Red, "Can't show "+label, m.err)
		return
	}
	ui.Column(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
		if m.points == 0 {
			chartEmpty(c)
			return
		}
		if *sel >= m.choices() {
			*sel = -1
		}
		switch data.Type {
		case ChartPie:
			m.pie(c, label, sel)
		case ChartBar:
			m.bars(c, label, sel)
		default:
			m.xy(c, label, sel)
		}
		m.legend(c)
		if *sel >= 0 {
			m.showReadout(c, *sel)
		}
	})
}

func chartEmpty(c *Context) {
	ui.Column(c).Height(chartHeight).AlignItems(ui.Stretch).Children(func() {
		Empty(c, IconActivity, "No data", "The query returned no rows.", nil)
	})
}

// seriesTones is the order series take the tones in (SPEC 5.11): the
// colors first, so a chart of one series is blue, and neutral last.
var seriesTones = []Tone{Blue, Green, Amber, Red, Purple, Neutral}

// seriesColor is the color of the s-th series, or slice: the tones' text
// colors in seriesTones' order, and past the last tone the same again,
// halfway to the background, so neighbors still differ.
func seriesColor(k *Tokens, s int) Color {
	col := k.Tone(seriesTones[s%len(seriesTones)]).Text
	if s >= len(seriesTones) {
		col = col.Mix(k.Background, 0.45)
	}
	return col
}

// chartModel is what a chart block works out from its data, kept while
// the data stays the same, and the buffers its drawing reuses.
type chartModel struct {
	data   *ChartData
	locale string // the numbers' format
	// zone is where a time axis's ticks fall and its readout's times are
	// written: UTC for days, time.Local for instants.
	zone   *time.Location
	err    string
	points int // the values drawn; none shows "No data"
	names  []string

	// Lines and scatter plots: each series' points, sorted by x, as x and
	// y values and as fractions of the plot.
	xv, yv         [][]float64
	xf, yf         [][]float32
	xTicks, yTicks []chart.Tick
	// The x values the selection moves among, ascending: those of every
	// series, and where they are along the plot. A scatter plot's
	// selection moves among its points, each the selS-th series' selJ-th,
	// at selY up the plot.
	selV       []float64
	selX, selY []float32
	selS, selJ []int
	text       bool // x is Categories

	// Bars: a value for each category in each series.
	band chart.Band
	yLin chart.Linear
	zero float32 // where 0 is up the plot

	// Pies: the slices, largest first, the rest summed into "Other".
	slices    []chart.Slice
	sliceName []string
	sliceVal  []float64
	total     float64

	// The drawing's buffers, reused each frame.
	bx, by []float32
	pts    [][2]float32
	seen   []uint64
	// The pointer's last place over a pie, to tell when it moves.
	pointer [2]float32
	over    bool
}

// update works out the model from data, its numbers written in locale,
// unless it already has.
func (m *chartModel) update(data *ChartData, locale string) {
	if m.data == data && m.locale == locale {
		return
	}
	*m = chartModel{data: data, locale: locale, bx: m.bx, by: m.by, pts: m.pts, seen: m.seen}
	m.names = make([]string, len(data.Series))
	for i, s := range data.Series {
		m.names[i] = s.Name
	}
	switch data.Type {
	case ChartPie:
		m.updatePie(data)
	case ChartBar:
		m.updateBars(data)
	default:
		m.updateXY(data)
	}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// span widens an empty stretch, as of a single value, so its scale has
// room.
func span(lo, hi float64) (float64, float64) {
	if lo < hi {
		return lo, hi
	}
	if lo == 0 {
		return -1, 1
	}
	d := math.Abs(lo) / 10
	return lo - d, hi + d
}

// The times a time axis can show, in Unix seconds: the years 1 to 9999.
const minUnix, maxUnix = -62135596800, 253402300799

func (m *chartModel) updateXY(data *ChartData) {
	m.text = data.Categories != nil
	scatter := data.Type == ChartScatter
	xlo, xhi := math.Inf(1), math.Inf(-1)
	ylo, yhi := math.Inf(1), math.Inf(-1)
	for _, s := range data.Series {
		xs := s.X
		if xs == nil {
			xs = data.X
		}
		n := len(s.Y)
		if m.text {
			n = min(n, len(data.Categories))
		} else {
			n = min(n, len(xs))
		}
		type pt struct{ x, y float64 }
		pts := make([]pt, 0, n)
		for i := range n {
			x, y := float64(i), s.Y[i]
			if !m.text {
				x = xs[i]
			}
			// A point with no place along x is left out. A line keeps a
			// point with no value as a gap, where it breaks, rather than
			// joining its neighbors across it.
			if !finite(x) || (scatter && !finite(y)) {
				continue
			}
			if !finite(y) {
				y = math.NaN()
			} else {
				xlo, xhi = min(xlo, x), max(xhi, x)
				ylo, yhi = min(ylo, y), max(yhi, y)
				m.points++
			}
			pts = append(pts, pt{x, y})
		}
		if !slices.IsSortedFunc(pts, func(a, b pt) int { return cmp.Compare(a.x, b.x) }) {
			slices.SortStableFunc(pts, func(a, b pt) int { return cmp.Compare(a.x, b.x) })
		}
		xv, yv := make([]float64, len(pts)), make([]float64, len(pts))
		for i, p := range pts {
			xv[i], yv[i] = p.x, p.y
		}
		m.xv, m.yv = append(m.xv, xv), append(m.yv, yv)
	}
	if m.points == 0 {
		return
	}
	ylo, yhi = span(ylo, yhi)
	y := chart.NewLinear(ylo, yhi).Nice(5)
	m.yTicks = m.labelTicks(y.Ticks(5), data.YAxis.Format)
	var xAt func(float64) float32
	switch {
	case m.text:
		m.band = chart.NewBand(data.Categories...)
		xAt = func(v float64) float32 { return m.band.At(int(v)) }
		m.xTicks = fewTicks(m.band.Ticks(), 8)
	case data.XAxis.Time:
		if xlo < minUnix || xhi > maxUnix {
			v := xlo
			if xhi > maxUnix {
				v = xhi
			}
			m.err = "The x value " + chartValue(m.locale, nil, v) + " is not a time in seconds from the years 1 to 9999."
			return
		}
		m.zone = timeZone(m.xv)
		if xlo == xhi {
			xlo, xhi = xlo-43200, xhi+43200
		}
		t := chart.NewTime(m.timeAt(xlo), m.timeAt(xhi))
		xAt = func(v float64) float32 { return t.At(unixTime(v)) }
		m.xTicks = t.Ticks(6)
		if f := data.XAxis.Format; f != nil {
			for i := range m.xTicks {
				m.xTicks[i].Label = f(m.xTicks[i].Value / 1000)
			}
		}
	default:
		xlo, xhi = span(xlo, xhi)
		x := chart.NewLinear(xlo, xhi)
		if scatter {
			x = x.Nice(6)
		}
		xAt = func(v float64) float32 { return x.At(v) }
		m.xTicks = m.labelTicks(x.Ticks(6), data.XAxis.Format)
	}
	nan := float32(math.NaN())
	for s := range m.xv {
		xf, yf := make([]float32, len(m.xv[s])), make([]float32, len(m.yv[s]))
		for i := range xf {
			xf[i], yf[i] = xAt(m.xv[s][i]), nan
			if v := m.yv[s][i]; finite(v) {
				yf[i] = y.At(v)
			}
		}
		m.xf, m.yf = append(m.xf, xf), append(m.yf, yf)
	}
	if scatter {
		// The selection moves among every point, by x and then by y, so
		// points at the same x are each reached.
		type ref struct{ s, j int }
		var refs []ref
		for s := range m.xv {
			for j := range m.xv[s] {
				refs = append(refs, ref{s, j})
			}
		}
		slices.SortFunc(refs, func(a, b ref) int {
			return cmp.Or(cmp.Compare(m.xv[a.s][a.j], m.xv[b.s][b.j]), cmp.Compare(m.yv[a.s][a.j], m.yv[b.s][b.j]), cmp.Compare(a.s, b.s))
		})
		for _, r := range refs {
			m.selS, m.selJ = append(m.selS, r.s), append(m.selJ, r.j)
			m.selV = append(m.selV, m.xv[r.s][r.j])
			m.selX, m.selY = append(m.selX, m.xf[r.s][r.j]), append(m.selY, m.yf[r.s][r.j])
		}
		return
	}
	// A line's selection moves among the x values of every series
	// together, where one of them has a value.
	for s := range m.xv {
		for j, v := range m.yv[s] {
			if finite(v) {
				m.selV = append(m.selV, m.xv[s][j])
			}
		}
	}
	slices.Sort(m.selV)
	m.selV = slices.Compact(m.selV)
	m.selX = make([]float32, len(m.selV))
	for i, v := range m.selV {
		m.selX[i] = xAt(v)
	}
}

func (m *chartModel) updateBars(data *ChartData) {
	m.text = true
	m.band = chart.NewBand(data.Categories...).Padding(0.2, 0.1)
	lo, hi := 0.0, 0.0
	for _, s := range data.Series {
		for i, v := range s.Y {
			if i < len(data.Categories) && finite(v) {
				lo, hi = min(lo, v), max(hi, v)
				m.points++
			}
		}
	}
	if m.points == 0 {
		return
	}
	lo, hi = span(lo, hi)
	m.yLin = chart.NewLinear(lo, hi).Nice(4)
	m.yTicks = m.labelTicks(m.yLin.Ticks(4), data.YAxis.Format)
	m.xTicks = fewTicks(m.band.Ticks(), 8)
	m.zero = m.yLin.At(0)
	m.selX = make([]float32, len(data.Categories))
	for i := range data.Categories {
		m.selX[i] = m.band.At(i)
	}
}

func (m *chartModel) updatePie(data *ChartData) {
	if len(data.Series) == 0 {
		return
	}
	type slice struct {
		name string
		v    float64
	}
	var all []slice
	for i, v := range data.Series[0].Y {
		if i >= len(data.Categories) || !finite(v) || v == 0 {
			continue
		}
		if v < 0 {
			m.err = "A pie can't show negative values: " + data.Categories[i] + " is " + chartValue(m.locale, data.YAxis.Format, v) + "."
			return
		}
		all = append(all, slice{data.Categories[i], v})
	}
	slices.SortStableFunc(all, func(a, b slice) int { return cmp.Compare(b.v, a.v) })
	if len(all) > PieSlices {
		other := 0.0
		for _, s := range all[PieSlices-1:] {
			other += s.v
		}
		all = append(all[:PieSlices-1], slice{"Other", other})
	}
	for _, s := range all {
		m.sliceName = append(m.sliceName, s.name)
		m.sliceVal = append(m.sliceVal, s.v)
		m.total += s.v
	}
	m.points = len(all)
	m.slices = chart.Pie(m.sliceVal, 0.012)
	m.names = m.sliceName
	m.selX = make([]float32, len(all))
	for i := range all {
		m.selX[i] = (float32(i) + 0.5) / float32(len(all))
	}
}

// choices is how many points, bars or slices the selection moves among.
func (m *chartModel) choices() int { return len(m.selX) }

// timeZone is where a time axis of xs, in Unix seconds, is read: UTC
// when every one is at midnight UTC, a day as a driver reads a DATE
// column, so no day moves to the one before; otherwise the computer's
// zone, where a table shows the same instants (TimeOf).
func timeZone(xs [][]float64) *time.Location {
	for _, s := range xs {
		for _, v := range s {
			if math.Mod(v, 86400) != 0 {
				return time.Local
			}
		}
	}
	return time.UTC
}

// timeAt is the time s seconds after 1970 began in the time axis's zone.
func (m *chartModel) timeAt(s float64) time.Time {
	if m.zone == nil {
		return unixTime(s)
	}
	return unixTime(s).In(m.zone)
}

// unixTime is the time s seconds after 1970 began, in UTC, held to the
// years 1 to 9999, which a time axis checks its values are within.
func unixTime(s float64) time.Time {
	sec, frac := math.Modf(max(minUnix, min(maxUnix, s)))
	return time.Unix(int64(sec), int64(frac*1e9)).UTC()
}

// labelTicks writes the ticks' labels with format, or as numbers in the
// locale's format, with as many decimals as the scale gave them.
func (m *chartModel) labelTicks(ticks []chart.Tick, format func(float64) string) []chart.Tick {
	if format != nil {
		for i := range ticks {
			ticks[i].Label = format(ticks[i].Value)
		}
		return ticks
	}
	decimals := 0
	for _, t := range ticks {
		if i := strings.IndexByte(t.Label, '.'); i >= 0 {
			decimals = max(decimals, len(t.Label)-i-1)
		}
	}
	o := i18n.NumberOptions{Integer: decimals == 0, MinFractionDigits: decimals, MaxFractionDigits: decimals}
	for i, t := range ticks {
		v := t.Value
		if v == 0 {
			v = 0 // no -0
		}
		ticks[i].Label = i18n.FormatNumber(m.locale, v, o)
	}
	return ticks
}

// chartValue writes v with format, or as a number in the locale's format:
// with up to two decimals, or three significant digits below 1.
func chartValue(locale string, format func(float64) string, v float64) string {
	if format != nil {
		return format(v)
	}
	o := i18n.NumberOptions{MaxFractionDigits: 2}
	if a := math.Abs(v); a == 0 {
		v = 0 // no -0
	} else if a < 1 {
		o.MaxFractionDigits = 2 - int(math.Floor(math.Log10(a)))
	}
	return i18n.FormatNumber(locale, v, o)
}

// formatX writes an x value of a line or a scatter plot, as the readout
// shows it.
func (m *chartModel) formatX(c *Context, v float64) string {
	d := m.data
	switch {
	case m.text:
		if i := int(v); i >= 0 && i < len(d.Categories) {
			return d.Categories[i]
		}
		return ""
	case d.XAxis.Format != nil:
		return d.XAxis.Format(v)
	case d.XAxis.Time:
		t := m.timeAt(v)
		if h, mi, s := t.Clock(); h != 0 || mi != 0 || s != 0 {
			return MediumDate(c, t) + " " + ShortTime(c, t)
		}
		return MediumDate(c, t)
	}
	return chartValue(m.locale, nil, v)
}

// describe tells what the i-th choice shows: "Nov 3: Input 12K, Output
// 3K", or a slice's value and share.
func (m *chartModel) describe(c *Context, i int) string {
	head, parts := m.readout(c, i)
	return head + ": " + strings.Join(parts, ", ")
}

// readout is what the i-th choice shows: the x, the category or the
// slice, and a part for each series with a value there.
func (m *chartModel) readout(c *Context, i int) (head string, parts []string) {
	d := m.data
	y := d.YAxis.Format
	switch d.Type {
	case ChartPie:
		pct := formatValue(m.locale, m.sliceVal[i]/m.total, FormatPercent, FormatOptions{}, false)
		return m.sliceName[i], []string{chartValue(m.locale, y, m.sliceVal[i]) + " (" + pct + ")"}
	case ChartBar:
		for _, s := range d.Series {
			if i < len(s.Y) && finite(s.Y[i]) {
				parts = append(parts, s.Name+" "+chartValue(m.locale, y, s.Y[i]))
			}
		}
		return d.Categories[i], parts
	case ChartScatter:
		s, j := m.selS[i], m.selJ[i]
		return m.formatX(c, m.xv[s][j]), []string{d.Series[s].Name + " " + chartValue(m.locale, y, m.yv[s][j])}
	}
	v := m.selV[i]
	for s := range m.xv {
		if j, ok := slices.BinarySearch(m.xv[s], v); ok && finite(m.yv[s][j]) {
			parts = append(parts, d.Series[s].Name+" "+chartValue(m.locale, y, m.yv[s][j]))
		}
	}
	return m.formatX(c, v), parts
}

// showReadout shows the i-th choice below the plot: each part a text of
// its own, so a part in Persian reads right to left, its value after
// its name, next to English ones. Assistive technology reads it whole.
func (m *chartModel) showReadout(c *Context, i int) {
	head, parts := m.readout(c, i)
	row := ui.Row(c).Wrap().GapX(4).Direction(ui.LeftToRight).Role(ui.RoleStatus).Label(m.describe(c, i))
	row.Children(func() {
		for j, p := range append([]string{head}, parts...) {
			switch {
			case j == 0 && len(parts) > 0:
				p += ":"
			case j > 0 && j < len(parts):
				p += ","
			}
			Text(c, p).FontSize(12).FontFeatures("tnum").NoWrap()
		}
	})
}

// axisLabel writes a tick's label in the muted color, in tabular figures.
func axisLabel(c *Context) func(chart.Side, chart.Tick) {
	k := TokensOf(c)
	return func(_ chart.Side, t chart.Tick) {
		Text(c, t.Label).NoWrap().FontSize(11).TextColor(k.MutedForeground).FontFeatures("tnum")
	}
}

// grid draws a line along the plot at each of ticks.
func grid(p *ui.Painter, r ui.Rect, ticks []chart.Tick, col Color) {
	for _, t := range ticks {
		p.Line(r.X, chart.Y(r, t.At), r.X+r.W, chart.Y(r, t.At), 1, col)
	}
}

// xy draws a line chart or a scatter plot.
func (m *chartModel) xy(c *Context, label string, sel *int) {
	k := TokensOf(c)
	scatter := m.data.Type == ChartScatter
	chart.Plot(c, chart.Axes{Left: m.yTicks, Bottom: m.xTicks, Label: axisLabel(c)}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			grid(p, r, m.yTicks, k.Border)
			for s := range m.xf {
				if scatter {
					m.drawDots(p, r, s, seriesColor(k, s))
				} else {
					m.drawLine(p, r, s, seriesColor(k, s))
				}
			}
		})
		area.Label(label)
		describe := func(i int) string { return m.describe(c, i) }
		chart.Select(c, area, sel, m.selX, describe)
		if scatter {
			m.pointAt(area, sel, describe)
		}
		if i := *sel; i >= 0 {
			chart.RuleX(c, m.selX[i], 1).Background(k.MutedForeground)
			mark := func(s, j int) {
				chart.Point(c, m.xf[s][j], m.yf[s][j], 10, 10).Radius(5).
					Background(seriesColor(k, s)).Border(2, k.Background)
			}
			if scatter {
				mark(m.selS[i], m.selJ[i])
			} else {
				for s := range m.xv {
					if j, ok := slices.BinarySearch(m.xv[s], m.selV[i]); ok && finite(m.yv[s][j]) {
						mark(s, j)
					}
				}
			}
		}
	}).Label(label).Height(chartHeight).Gap(6).Direction(ui.LeftToRight)
	m.axisTitle(c)
}

// pointAt makes the pointer choose the scatter plot's point nearest it,
// across as well as along, so of points at one x it chooses the one it is
// at. It chooses as the pointer comes and moves, as chart.Select does.
func (m *chartModel) pointAt(area ui.Element, sel *int, describe func(int) string) {
	x, y, over := area.PointerPosition()
	if !over {
		m.over = false
		return
	}
	if moved := !m.over || m.pointer != [2]float32{x, y}; moved {
		b := area.Bounds()
		best, far := -1, float32(math.Inf(1))
		for i := range m.selX {
			dx, dy := m.selX[i]*b.W-x, (1-m.selY[i])*b.H-y
			if d := dx*dx + dy*dy; d < far {
				best, far = i, d
			}
		}
		if *sel = best; best >= 0 {
			area.Value(describe(best))
		}
	}
	m.over, m.pointer = true, [2]float32{x, y}
}

// axisTitle writes the x axis's title below its ticks.
func (m *chartModel) axisTitle(c *Context) {
	if t := m.data.XAxis.Label; t != "" {
		Text(c, t).FontSize(11).TextColor(TokensOf(c).MutedForeground).TextAlign(ui.Center)
	}
}

// drawLine strokes the s-th series, reduced to the lowest and highest
// value of each pixel column, so a line of thousands of points draws as
// fast as one of a few hundred and looks the same. A line sparse enough
// gets a dot at each point.
func (m *chartModel) drawLine(p *ui.Painter, r ui.Rect, s int, col Color) {
	xf, yf := m.xf[s], m.yf[s]
	w := int(math.Ceil(float64(r.W * p.Scale())))
	m.bx, m.by = chart.MinMax(m.bx, m.by, xf, yf, w)
	m.pts = m.pts[:0]
	for i, x := range m.bx {
		m.pts = append(m.pts, [2]float32{chart.X(r, x), chart.Y(r, m.by[i])})
	}
	p.StrokePolyline(m.pts, 2, col)
	// A point with gaps on both sides is no line: it gets its dot however
	// many points there are.
	dots := float32(len(xf))*12 <= r.W
	defined := func(i int) bool { return i >= 0 && i < len(yf) && !math.IsNaN(float64(yf[i])) }
	for i := range xf {
		if defined(i) && (dots || !defined(i-1) && !defined(i+1)) {
			x, y := chart.X(r, xf[i]), chart.Y(r, yf[i])
			p.Fill(ui.Rect{X: x - 3, Y: y - 3, W: 6, H: 6}, col, 3)
		}
	}
}

// drawDots draws the s-th series' points as dots, one for each pixel
// they cover: points drawn over one another show as one.
func (m *chartModel) drawDots(p *ui.Painter, r ui.Rect, s int, col Color) {
	xf, yf := m.xf[s], m.yf[s]
	const d = 5
	w, h := int(r.W)+1, int(r.H)+1
	words := (w*h + 63) / 64
	if cap(m.seen) < words {
		m.seen = make([]uint64, words)
	}
	m.seen = m.seen[:words]
	clear(m.seen)
	for i := range xf {
		x, y := chart.X(r, xf[i]), chart.Y(r, yf[i])
		cell := int(y-r.Y)*w + int(x-r.X)
		if cell < 0 || cell >= w*h || m.seen[cell/64]&(1<<(cell%64)) != 0 {
			continue
		}
		m.seen[cell/64] |= 1 << (cell % 64)
		p.Fill(ui.Rect{X: x - d/2, Y: y - d/2, W: d, H: d}, col, d/2)
	}
}

// bars draws the bars, a group in each category, a bar for each series.
func (m *chartModel) bars(c *Context, label string, sel *int) {
	k := TokensOf(c)
	d := m.data
	n := len(d.Series)
	chart.Plot(c, chart.Axes{Left: m.yTicks, Bottom: m.xTicks, Label: axisLabel(c)}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			if i := *sel; i >= 0 {
				b := m.band.Band(i)
				p.Fill(ui.Rect{X: chart.X(r, b.Lo) - 2, Y: r.Y, W: (b.Hi-b.Lo)*r.W + 4, H: r.H}, k.Muted, 3)
			}
			grid(p, r, m.yTicks, k.Border)
			bw := m.band.Width() * r.W / float32(n)
			gap := min(bw*0.15, 2)
			radius := min(3, (bw-gap)/4)
			zero := chart.Y(r, m.zero)
			for s, sr := range d.Series {
				col := seriesColor(k, s)
				for i, v := range sr.Y {
					if i >= len(d.Categories) || !finite(v) || v == 0 {
						continue
					}
					x := chart.X(r, m.band.Band(i).Lo) + float32(s)*bw
					y := chart.Y(r, m.yLin.At(v))
					top, bottom := min(y, zero), max(y, zero)
					p.Fill(ui.Rect{X: x + gap/2, Y: top, W: max(bw-gap, 1), H: max(bottom-top, 1)}, col, radius)
				}
			}
		})
		area.Label(label)
		chart.Select(c, area, sel, m.selX, func(i int) string { return m.describe(c, i) })
	}).Label(label).Height(chartHeight).Gap(6).Direction(ui.LeftToRight)
	m.axisTitle(c)
}

// pie draws the slices, the one chosen drawn out a little.
func (m *chartModel) pie(c *Context, label string, sel *int) {
	k := TokensOf(c)
	const size = chartHeight
	ui.Row(c).Justify(ui.Center).Direction(ui.LeftToRight).Children(func() {
		ring := ui.Box(c).Size(size, size).Label(label)
		describe := func(i int) string { return m.describe(c, i) }
		chart.Select(c, ring, sel, m.selX, describe)
		// The pointer chooses the slice under it, not the one nearest its x.
		if x, y, over := ring.PointerPosition(); over {
			if !m.over || m.pointer != [2]float32{x, y} {
				*sel = -1
				if i, ok := chart.SliceAt(m.slices, x-size/2, y-size/2, 0, size/2); ok {
					*sel = i
				}
				if *sel >= 0 {
					ring.Value(describe(*sel))
				}
			}
			m.over, m.pointer = true, [2]float32{x, y}
		} else {
			m.over = false
		}
		ring.Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			outer := min(r.W, r.H)/2 - 6
			for i, s := range m.slices {
				o := outer
				if i == *sel {
					o += 5
				}
				p.FillPath(chart.Sector(cx, cy, 0, o, s), seriesColor(k, i))
			}
		})
	})
}

// legend names the series, or the slices, beside a swatch of their color.
func (m *chartModel) legend(c *Context) {
	if len(m.names) < 2 && m.data.Type != ChartPie {
		return
	}
	k := TokensOf(c)
	ui.Row(c).Wrap().Gap(16).Justify(ui.Center).Direction(ui.LeftToRight).Children(func() {
		for i, name := range m.names {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(10, 10).Radius(2).Shrink(0).Background(seriesColor(k, i))
				Text(c, name).FontSize(12).TextColor(k.MutedForeground)
			})
		}
	})
}
