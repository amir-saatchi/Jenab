package kit

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The charts SPEC 5.4's size rules are about (P3-08): lines of 2,000 and
// 10,000 points, a scatter plot of 10,000 and 1,000 bars, each drawn
// headless, on the CPU, in a window 1,000 by 600. "first" is a new
// ChartData's first frame, with the model worked out; "frame" a frame as
// the keyboard moves the choice, built, laid out and painted again;
// "resize" a frame at a new width.

func benchLine(n int, typ ChartType) *ChartData {
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range xs {
		xs[i] = day(2020, 1, 1) + float64(i)*3600
		ys[i] = 60000 + 5000*math.Sin(float64(i)/50) + 800*math.Sin(float64(i)*1.7)
	}
	if typ == ChartScatter {
		for i := range xs {
			xs[i] = 50 + 20*math.Sin(float64(i)*0.37)
		}
	}
	return &ChartData{Type: typ, X: xs, XAxis: ChartAxis{Time: typ == ChartLine}, Series: []ChartSeries{{Name: "Price", Y: ys}}}
}

func benchBars(n int) *ChartData {
	cats, ys := make([]string, n), make([]float64, n)
	for i := range cats {
		cats[i] = fmt.Sprintf("D%d", i)
		ys[i] = 100 + 80*math.Sin(float64(i)/9)
	}
	return &ChartData{Type: ChartBar, Categories: cats, Series: []ChartSeries{{Name: "Count", Y: ys}}}
}

var benchCharts = []struct {
	name string
	data func() *ChartData
}{
	{"line2000", func() *ChartData { return benchLine(2000, ChartLine) }},
	{"line10000", func() *ChartData { return benchLine(10000, ChartLine) }},
	{"scatter10000", func() *ChartData { return benchLine(10000, ChartScatter) }},
	{"bar1000", func() *ChartData { return benchBars(1000) }},
	// Over SPEC 5.4's cap, which the runtime keeps: what the kit costs without it.
	{"bar10000", func() *ChartData { return benchBars(10000) }},
}

func BenchmarkChartBlock(b *testing.B) {
	if err := RegisterFonts(); err != nil {
		b.Fatal(err)
	}
	for _, bc := range benchCharts {
		data := bc.data()
		cur := data
		tt := ui.NewTester(func(c *ui.Context) {
			Root(c, func() {
				Column(c).Padding(16).AlignItems(Stretch).Children(func() { ChartBlock(c, "Chart", cur, BlockState{}) })
			})
		}, 1000, 600)
		tt.Frame()
		b.Run(bc.name+"/first", func(b *testing.B) {
			for b.Loop() {
				cur = bc.data()
				tt.Frame()
			}
		})
		cur = data
		tt.Frame()
		for range 8 {
			if tt.Key(0, ui.KeyTab); tt.Focused("Chart") {
				break
			}
		}
		b.Run(bc.name+"/frame", func(b *testing.B) {
			i := 0
			for b.Loop() {
				if i++; i%2 == 0 {
					tt.Key(0, ui.KeyRight)
				} else {
					tt.Key(0, ui.KeyLeft)
				}
				tt.Frame()
			}
		})
		b.Run(bc.name+"/resize", func(b *testing.B) {
			i := 0
			for b.Loop() {
				i++
				tt.SetSize(1000-i%2*200, 600)
				tt.Frame()
			}
		})
	}
	// The kit's first chart, its bars elements of their own, for the
	// difference drawing them makes.
	bars := benchBars(1000)
	values := bars.Series[0].Y
	tt := ui.NewTester(func(c *ui.Context) {
		Root(c, func() {
			BarChart(c, "Chart", bars.Categories, []Series{{Name: "Count", Tone: Blue, Values: values}}, nil)
		})
	}, 1000, 600)
	tt.Frame()
	b.Run("BarChart1000elements/resize", func(b *testing.B) {
		i := 0
		for b.Loop() {
			i++
			tt.SetSize(1000-i%2*200, 600)
			tt.Frame()
		}
	})
}
