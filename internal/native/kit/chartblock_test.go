package kit

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// chartView builds a chart block between two buttons, 600 wide.
func chartView(label string, data *ChartData, st *BlockState) func(c *Context) {
	return func(c *Context) {
		between(c, func() {
			Column(c).Width(600).Children(func() {
				ChartBlock(c, label, data, *st)
			})
		})
	}
}

// tokensOf is the tokens of the theme tt's window is in.
func tokensOf(tt *ui.Tester) *Tokens {
	if near(tt.Image().RGBAAt(799, 599), Dark.Background) {
		return &Dark
	}
	return &Light
}

// paintsSeries fails unless the window has a pixel of each of the first
// n series' colors.
func paintsSeries(t *testing.T, tt *ui.Tester, n int) {
	t.Helper()
	k := tokensOf(tt)
	for s := range n {
		if !paints(tt, seriesColor(k, s)) {
			t.Errorf("nothing in the color of series %d", s)
		}
	}
}

// readsOut checks the chart's readout shows want and was read out.
func readsOut(t *testing.T, tt *ui.Tester, want string) {
	t.Helper()
	shows(t, tt, want)
	if a := tt.Announcements(); !slices.Contains(a, want) {
		t.Errorf("%q was not read out: %q", want, a)
	}
}

// focusable is what assistive technology gets of the element named label
// that takes the focus, as a chart's plot or a form's control, not the
// block or the field around it.
func focusable(tt *ui.Tester, label string) AccessInfo {
	for _, a := range tt.AccessAll() {
		if a.Name == label && a.Focusable {
			return a
		}
	}
	return AccessInfo{}
}

func day(y int, m time.Month, d int) float64 {
	return float64(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix())
}

func TestChartBlockLine(t *testing.T) {
	usd := func(v float64) string { return fmt.Sprintf("$%gK", v/1000) }
	data := &ChartData{
		Type:  ChartLine,
		X:     []float64{day(2026, 7, 1), day(2026, 7, 2), day(2026, 7, 3)},
		XAxis: ChartAxis{Label: "Date", Time: true},
		YAxis: ChartAxis{Format: usd},
		Series: []ChartSeries{
			{Name: "BTC", Y: []float64{60000, 62000, 61000}},
			// A series of its own x, as series_by makes: no point on Jul 2.
			{Name: persian, X: []float64{day(2026, 7, 3), day(2026, 7, 1)}, Y: []float64{30000, 20000}},
		},
	}
	st := &BlockState{}
	themes(t, chartView("Price", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		shows(t, tt, "BTC", persian, "Date", "Jul 2", "$60K", "$20K")
		paintsSeries(t, tt, 2)

		tabTo(t, tt, "Price")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		readsOut(t, tt, "Jul 1, 2026: BTC $60K, "+persian+" $20K")
		tt.Key(0, ui.KeyRight)
		tt.Frame()
		readsOut(t, tt, "Jul 2, 2026: BTC $62K")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		readsOut(t, tt, "Jul 3, 2026: BTC $61K, "+persian+" $30K")
		if a := focusable(tt, "Price"); !strings.HasPrefix(a.Value, "Jul 3") {
			t.Errorf("the plot's value is %q", a.Value)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if Shows(tt, "Jul 3, 2026: BTC") {
			t.Error("Escape kept the choice")
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not leave the chart")
		}
	})
}

// A line along text, as a month's name, keeps the categories' order.
func TestChartBlockLineOfCategories(t *testing.T) {
	data := &ChartData{
		Type:       ChartLine,
		Categories: []string{"فروردین", "اردیبهشت", "خرداد"},
		Series:     []ChartSeries{{Name: "Sales", Y: []float64{3, math.NaN(), 7}}},
	}
	st := &BlockState{}
	themes(t, chartView("Sales by month", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		shows(t, tt, "فروردین", "خرداد")
		paintsSeries(t, tt, 1)
		if Shows(tt, "Sales") && !Shows(tt, "Sales by month") {
			t.Error("a chart of one series shows a legend")
		}
		tabTo(t, tt, "Sales by month")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		readsOut(t, tt, "خرداد: Sales 7")
		// The month with no value is no stop of the arrows.
		tt.Key(0, ui.KeyLeft)
		tt.Frame()
		readsOut(t, tt, "فروردین: Sales 3")
	})
}

func TestChartBlockBar(t *testing.T) {
	data := &ChartData{
		Type:       ChartBar,
		Categories: []string{"Mon", persian, "Wed"},
		YAxis:      ChartAxis{Label: "Calls"},
		Series: []ChartSeries{
			{Name: "In", Y: []float64{3, 7, -2}},
			{Name: "Out", Y: []float64{1, 2, 4}},
			{Name: "Lost", Y: []float64{0, 1, 1}},
		},
	}
	st := &BlockState{}
	themes(t, chartView("Calls per day", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		shows(t, tt, "Mon", persian, "In", "Out", "Lost", "-2")
		paintsSeries(t, tt, 3)
		tabTo(t, tt, "Calls per day")
		tt.Key(0, ui.KeyRight)
		tt.Frame()
		readsOut(t, tt, "Mon: In 3, Out 1, Lost 0")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		readsOut(t, tt, "Wed: In -2, Out 4, Lost 1")
		tt.Key(0, ui.KeyLeft)
		tt.Frame()
		readsOut(t, tt, persian+": In 7, Out 2, Lost 1")
	})
}

func TestChartBlockScatter(t *testing.T) {
	kg := func(v float64) string { return fmt.Sprintf("%g kg", v) }
	data := &ChartData{
		Type:  ChartScatter,
		XAxis: ChartAxis{Label: "Weight", Format: kg},
		Series: []ChartSeries{
			{Name: "Cats", X: []float64{4, 3, 5}, Y: []float64{30, 25, 33}},
			{Name: persian, X: []float64{20, 30}, Y: []float64{55, 70}},
		},
	}
	st := &BlockState{}
	themes(t, chartView("Height by weight", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		shows(t, tt, "Cats", persian, "Weight", "10 kg")
		paintsSeries(t, tt, 2)
		tabTo(t, tt, "Height by weight")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		readsOut(t, tt, "3 kg: Cats 25")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		readsOut(t, tt, "30 kg: "+persian+" 70")
	})
}

func TestChartBlockPie(t *testing.T) {
	var cats []string
	var values []float64
	for i := range 14 {
		cats = append(cats, fmt.Sprintf("C%d", i))
		values = append(values, float64(14-i))
	}
	cats[0] = persian
	data := &ChartData{Type: ChartPie, Categories: cats, Series: []ChartSeries{{Name: "Count", Y: values}}}
	st := &BlockState{}
	themes(t, chartView("Shares", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		// 12 slices: the 11 largest, and the 3 smallest summed into Other.
		shows(t, tt, persian, "C10", "Other")
		if Shows(tt, "C11") || Shows(tt, "C12") || Shows(tt, "C13") {
			t.Error("a pie shows more than 12 slices")
		}
		paintsSeries(t, tt, 6)
		tabTo(t, tt, "Shares")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		readsOut(t, tt, persian+": 14 (13.3%)")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		readsOut(t, tt, "Other: 6 (5.7%)")
	})
}

func TestChartBlockPiePointer(t *testing.T) {
	data := &ChartData{Type: ChartPie, Categories: []string{"A", "B"}, Series: []ChartSeries{{Y: []float64{3, 1}}}}
	st := &BlockState{}
	view := func(c *Context) {
		Column(c).Width(600).Children(func() { ChartBlock(c, "Split", data, *st) })
	}
	tt := window(t, false, view)
	ring := focusable(tt, "Split")
	if ring.Bounds.W == 0 {
		t.Fatal("no pie to point at")
	}
	cx, cy := ring.Bounds.X+ring.Bounds.W/2, ring.Bounds.Y+ring.Bounds.H/2
	// B, the quarter from nine to twelve o'clock, is up and left.
	tt.Move(cx-40, cy-40)
	tt.Frame()
	shows(t, tt, "B: 1 (25%)")
	tt.Move(cx+40, cy+40)
	tt.Frame()
	shows(t, tt, "A: 3 (75%)")
	tt.Move(cx+200, cy)
	tt.Frame()
	if Shows(tt, "A: 3") {
		t.Error("the choice stayed as the pointer left the pie")
	}
}

func TestChartBlockStates(t *testing.T) {
	data := &ChartData{
		Type:       ChartBar,
		Categories: []string{"A"},
		Series:     []ChartSeries{{Name: "N", Y: []float64{1}}},
	}
	st := &BlockState{}
	cur := data
	view := func(c *Context) {
		between(c, func() {
			Column(c).Width(600).Children(func() { ChartBlock(c, "Totals", cur, *st) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		*st, cur = BlockState{Loading: true}, data
		tt.Frame()
		shows(t, tt, "Loading Totals")
		passesOver(t, tt)

		*st = BlockState{Error: "no such column: " + persian}
		tt.Frame()
		shows(t, tt, "Can't show Totals", "no such column: "+persian)
		if !HasRole(tt, "status", "Can't show Totals") {
			t.Error("the error is not read out")
		}

		*st = BlockState{Notice: "Showing the first 1,000 bars. " + persian}
		tt.Frame()
		shows(t, tt, "Showing the first 1,000 bars. "+persian)
		if !HasRole(tt, "status", "Showing the first 1,000 bars. "+persian) {
			t.Error("the notice is not read out")
		}
		tabTo(t, tt, "Totals")

		*st = BlockState{Notice: "Nothing yet"}
		cur = &ChartData{Type: ChartLine}
		tt.Frame()
		shows(t, tt, "No data", "Nothing yet")
		passesOver(t, tt)
		*st = BlockState{}

		cur = &ChartData{Type: ChartPie, Categories: []string{persian}, Series: []ChartSeries{{Y: []float64{-4}}}}
		tt.Frame()
		shows(t, tt, "A pie can't show negative values: "+persian+" is -4.")
		// Times in milliseconds, given as seconds, are past the year 9999.
		cur = &ChartData{Type: ChartLine, X: []float64{1.7e12}, XAxis: ChartAxis{Time: true}, Series: []ChartSeries{{Name: "N", Y: []float64{1}}}}
		tt.Frame()
		shows(t, tt, "Can't show Totals", "The x value 1,700,000,000,000 is not a time in seconds from the years 1 to 9999.")

		cur = nil
		tt.Frame()
		shows(t, tt, "No data")
	})
}

// A chart keeps its choice's index in range as new data has fewer points.
func TestChartBlockFewerPoints(t *testing.T) {
	many := &ChartData{Type: ChartBar, Categories: []string{"A", "B", "C"}, Series: []ChartSeries{{Name: "N", Y: []float64{1, 2, 3}}}}
	few := &ChartData{Type: ChartBar, Categories: []string{"A"}, Series: []ChartSeries{{Name: "N", Y: []float64{1}}}}
	cur, st := many, &BlockState{}
	tt := window(t, false, func(c *Context) {
		Column(c).Width(600).Children(func() { ChartBlock(c, "N", cur, *st) })
	})
	tabTo(t, tt, "N")
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	shows(t, tt, "C: N 3")
	cur = few
	tt.Frame()
	tt.Frame()
	if Shows(tt, "C: N 3") {
		t.Error("the choice outlived its bar")
	}
}

func TestChartValue(t *testing.T) {
	for _, tc := range []struct {
		locale string
		v      float64
		want   string
	}{
		{"en", 0.1 + 0.2, "0.3"},
		{"en", 62000, "62,000"},
		{"en", -2.5, "-2.5"},
		{"en", 1e-7, "0.0000001"},
		{"en", 60353.0864, "60,353.09"},
		{"en", 0.012345, "0.0123"},
		{"en", 0, "0"},
		{"en", math.Copysign(0, -1), "0"},
		{"de", 1234.5, "1.234,5"},
		{"fa", 1234.5, "۱٬۲۳۴٫۵"},
	} {
		if got := chartValue(tc.locale, nil, tc.v); got != tc.want {
			t.Errorf("chartValue(%s, %v) = %q, want %q", tc.locale, tc.v, got, tc.want)
		}
	}
	if got := chartValue("en", func(v float64) string { return "x" }, 1); got != "x" {
		t.Errorf("a format was not used: %q", got)
	}
}

// A time far outside the years a time axis shows is held to them, not
// wrapped around.
// Series take blue, green, amber, red, purple and neutral, then the same
// again faded toward the background, in both themes.
func TestSeriesColor(t *testing.T) {
	for _, k := range []*Tokens{&Light, &Dark} {
		for s, tone := range []Tone{Blue, Green, Amber, Red, Purple, Neutral} {
			if got := seriesColor(k, s); got != k.Tone(tone).Text {
				t.Errorf("series %d is %v, want %s's %v", s, got, tone, k.Tone(tone).Text)
			}
			if got, want := seriesColor(k, s+6), k.Tone(tone).Text.Mix(k.Background, 0.45); got != want {
				t.Errorf("series %d is %v, want %s faded, %v", s+6, got, tone, want)
			}
		}
	}
}

func TestUnixTime(t *testing.T) {
	for v, year := range map[float64]int{1e300: 9999, -1e300: 1, 1.7e12: 9999, day(2026, 7, 1): 2026} {
		if got := unixTime(v).Year(); got != year {
			t.Errorf("unixTime(%g) is in %d, want %d", v, got, year)
		}
	}
}

// Points at one x in a scatter plot are each reached, by the arrows and
// by the pointer.
func TestChartBlockScatterSameX(t *testing.T) {
	data := &ChartData{
		Type: ChartScatter,
		Series: []ChartSeries{
			{Name: "A", X: []float64{10, 0, 10}, Y: []float64{100, 50, 0}},
			{Name: persian, X: []float64{10}, Y: []float64{60}},
		},
	}
	st := &BlockState{}
	themes(t, chartView("Spread", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		tabTo(t, tt, "Spread")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		readsOut(t, tt, "0: A 50")
		for _, want := range []string{"10: A 0", "10: " + persian + " 60", "10: A 100"} {
			tt.Key(0, ui.KeyRight)
			tt.Frame()
			readsOut(t, tt, want)
		}
		// The pointer chooses the point it is at, up or down the plot.
		b := focusable(tt, "Spread").Bounds
		tt.Move(b.X+b.W-2, b.Y+2)
		tt.Frame()
		shows(t, tt, "10: A 100")
		tt.Move(b.X+b.W-2, b.Y+b.H-2)
		tt.Frame()
		shows(t, tt, "10: A 0")
	})
}

// A line breaks where a value is missing or infinite, rather than joining
// its neighbors across the gap, and the readout passes over it.
func TestChartBlockLineGaps(t *testing.T) {
	data := &ChartData{
		Type:   ChartLine,
		X:      []float64{0, 1, 2, 3, 4, 5},
		Series: []ChartSeries{{Name: "L", Y: []float64{1, math.NaN(), 3, math.Inf(1), 5, 6}}},
	}
	st := &BlockState{}
	themes(t, chartView("Gaps", data, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		col := seriesColor(tokensOf(tt), 0)
		b := focusable(tt, "Gaps").Bounds
		y := chart.NewLinear(1, 6).Nice(5)
		// paintsNear tells whether the line passes within 2 px of the
		// point at x, y in the data.
		paintsNear := func(x, v float64) bool {
			img := tt.Image()
			px, py := int(b.X+float32(x/5)*b.W), int(b.Y+(1-y.At(v))*b.H)
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if near(img.RGBAAt(px+dx, py+dy), col) {
						return true
					}
				}
			}
			return false
		}
		if !paintsNear(4.5, 5.5) {
			t.Error("no line from 4 to 5")
		}
		if paintsNear(1, 2) {
			t.Error("the line bridges the gap at 1")
		}
		if paintsNear(3, 4) {
			t.Error("the line bridges the gap at 3")
		}
		if !paintsNear(2, 3) {
			t.Error("no dot at 2, between two gaps")
		}
		tabTo(t, tt, "Gaps")
		for _, want := range []string{"0: L 1", "2: L 3", "4: L 5"} {
			tt.Key(0, ui.KeyRight)
			tt.Frame()
			readsOut(t, tt, want)
		}
	})
}

// Charts of little or odd data draw, and their readout works.
func TestChartBlockEdgeCases(t *testing.T) {
	cats := make([]string, 10000)
	many := make([]float64, len(cats))
	for i := range cats {
		cats[i] = fmt.Sprintf("D%d", i)
		many[i] = float64(i % 7)
	}
	for _, tc := range []struct {
		name  string
		data  *ChartData
		shows []string
		home  string
	}{
		{"one point", &ChartData{Type: ChartLine, X: []float64{day(2026, 7, 1)}, XAxis: ChartAxis{Time: true}, Series: []ChartSeries{{Name: "A", Y: []float64{5}}}}, nil, "Jul 1, 2026: A 5"},
		{"one scatter point", &ChartData{Type: ChartScatter, X: []float64{3}, Series: []ChartSeries{{Name: "A", Y: []float64{5}}}}, nil, "3: A 5"},
		{"equal values", &ChartData{Type: ChartLine, X: []float64{1, 2, 3}, Series: []ChartSeries{{Name: "A", Y: []float64{12000, 12000, 12000}}}}, []string{"12,000"}, "1: A 12,000"},
		{"negative bars", &ChartData{Type: ChartBar, Categories: []string{"X", persian}, Series: []ChartSeries{{Name: "N", Y: []float64{-3, -5}}}}, []string{"-5", persian}, "X: N -3"},
		{"10,000 bars", &ChartData{Type: ChartBar, Categories: cats, Series: []ChartSeries{{Name: "N", Y: many}}}, []string{"D9999"}, "D0: N 0"},
		{"pie of zeros", &ChartData{Type: ChartPie, Categories: []string{"A", "B"}, Series: []ChartSeries{{Y: []float64{0, 0}}}}, []string{"No data"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &BlockState{}
			themes(t, chartView("Edge", tc.data, st), func(t *testing.T, tt *ui.Tester) {
				tt.UseScreenReader()
				shows(t, tt, tc.shows...)
				if Shows(tt, "Can't show Edge") {
					t.Fatal("the chart shows an error")
				}
				if tc.home == "" {
					passesOver(t, tt)
					return
				}
				paintsSeries(t, tt, 1)
				tabTo(t, tt, "Edge")
				tt.Key(0, ui.KeyHome)
				tt.Frame()
				readsOut(t, tt, tc.home)
			})
		})
	}
}

// A time axis follows the rule a table's times do (TimeOf): days, all at
// midnight UTC, stay the same day in every zone; instants show in the
// computer's zone, here pinned to UTC-5, ticks and readout alike.
func TestChartBlockTimeZones(t *testing.T) {
	local := time.Local
	defer func() { time.Local = local }()
	time.Local = time.FixedZone("Test", -5*3600)

	days := &ChartData{
		Type:   ChartLine,
		X:      []float64{day(2026, 7, 1), day(2026, 7, 2), day(2026, 7, 3)},
		XAxis:  ChartAxis{Time: true},
		Series: []ChartSeries{{Name: "N", Y: []float64{1, 2, 3}}},
	}
	// 03:30 UTC on Jul 2 is 22:30 on Jul 1 in UTC-5, just before midnight.
	at := func(h, m int) float64 { return float64(time.Date(2026, 7, 2, h, m, 0, 0, time.UTC).Unix()) }
	instants := &ChartData{
		Type:   ChartLine,
		X:      []float64{at(3, 30), at(8, 0), at(12, 0)},
		XAxis:  ChartAxis{Time: true},
		Series: []ChartSeries{{Name: "N", Y: []float64{1, 2, 3}}},
	}
	st := &BlockState{}
	themes(t, chartView("Days", days, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		tabTo(t, tt, "Days")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		readsOut(t, tt, "Jul 1, 2026: N 1")
	})
	themes(t, chartView("Instants", instants, st), func(t *testing.T, tt *ui.Tester) {
		tt.UseScreenReader()
		// Local midnight falls between the first two points: a tick of its
		// own, which the same times in UTC wouldn't have.
		shows(t, tt, "Jul 2")
		tabTo(t, tt, "Instants")
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		readsOut(t, tt, "Jul 1, 2026 10:30 PM: N 1")
		tt.Key(0, ui.KeyEnd)
		tt.Frame()
		readsOut(t, tt, "Jul 2, 2026 7:00 AM: N 3")
	})
}

func TestTimeZone(t *testing.T) {
	if z := timeZone([][]float64{{day(2026, 7, 1)}, {day(1960, 1, 1)}}); z != time.UTC {
		t.Errorf("days, one before 1970, read in %v", z)
	}
	if z := timeZone([][]float64{{day(2026, 7, 1)}, {day(2026, 7, 1) + 1}}); z != time.Local {
		t.Errorf("an instant among days read in %v", z)
	}
}
