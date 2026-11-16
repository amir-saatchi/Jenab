package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// Series is a series of a bar chart: its name in the legend, its tone and
// a value for each bar.
type Series struct {
	Name   string
	Tone   Tone
	Values []float64
}

// BarChart is a chart of stacked bars, as Settings › Usage shows tokens
// per day (usage-chart.tsx, on MyGo's ui/chart): a bar for each of bars,
// its series piled up from the bottom in their order, a grid line at each
// value tick, which format labels, and the legend below. Its area takes
// the focus, and the arrows, Home and End choose a bar, whose values show
// above the plot and are read out; Escape clears the choice. The chart
// is laid out left to right in a right-to-left language too, as charts
// are. label names it.
func BarChart(c *Context, label string, bars []string, series []Series, format func(float64) string) Element {
	k := TokensOf(c)
	col := Column(c).Gap(8)
	sel := ui.Local(col, "bar", func() int { return -1 })
	if *sel >= len(bars) {
		*sel = -1
	}
	values := make([][]float64, len(series))
	for s, sr := range series {
		values[s] = make([]float64, len(bars))
		copy(values[s], sr.Values)
	}
	describe := func(i int) string {
		parts := make([]string, len(series))
		for s, sr := range series {
			parts[s] = sr.Name + " " + format(values[s][i])
		}
		return bars[i] + ": " + strings.Join(parts, ", ")
	}
	col.Children(func() {
		x := chart.NewBand(bars...).Padding(0.25, 0.1)
		stacked := chart.Stack(values, chart.StackStandard)
		hi := 0.0
		for _, sr := range stacked {
			for _, seg := range sr {
				hi = max(hi, seg.Hi)
			}
		}
		if hi == 0 {
			hi = 1
		}
		y := chart.NewLinear(0, hi).Nice(4)
		ticks := y.Ticks(4)
		for i := range ticks {
			ticks[i].Label = format(ticks[i].Value)
		}
		muted := func(_ chart.Side, t chart.Tick) {
			Text(c, t.Label).NoWrap().FontSize(11).TextColor(k.MutedForeground).FontFeatures("tnum")
		}
		xs := make([]float32, len(bars))
		for i := range bars {
			xs[i] = x.At(i)
		}
		chart.Plot(c, chart.Axes{Left: ticks, Bottom: fewTicks(x.Ticks(), 8), Label: muted}, func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, t := range ticks {
					p.Line(r.X, chart.Y(r, t.At), r.X+r.W, chart.Y(r, t.At), 1, k.Border)
				}
			})
			if *sel >= 0 {
				chart.Rect(c, x.Band(*sel), chart.Span{Lo: 0, Hi: 1}).Background(k.Muted).Radius(3)
			}
			for s, sr := range stacked {
				tone := k.Tone(series[s].Tone).Text
				for i, seg := range sr {
					if seg.Hi <= seg.Lo {
						continue
					}
					bar := chart.Rect(c, x.Band(i), y.Span(seg.Lo, seg.Hi)).Background(tone)
					if s == len(stacked)-1 {
						bar.Radius(3)
					}
				}
			}
			area.Label(label)
			chart.Select(c, area, sel, xs, describe)
		}).Label(label).Height(192).Gap(6).Direction(ui.LeftToRight)
		Row(c).Gap(16).Justify(Center).Direction(ui.LeftToRight).Children(func() {
			for _, sr := range series {
				Row(c).Gap(6).AlignItems(Center).Children(func() {
					Box(c).Size(10, 10).Radius(2).Background(k.Tone(sr.Tone).Text)
					Text(c, sr.Name).FontSize(12).TextColor(k.MutedForeground)
				})
			}
		})
		if *sel >= 0 {
			Text(c, describe(*sel)).FontSize(12).FontFeatures("tnum").Direction(ui.LeftToRight).Role(ui.RoleStatus)
		}
	})
	return col
}

// fewTicks keeps at most n of ticks, evenly spaced and the last among
// them, so the day labels of 90 bars don't run into each other.
func fewTicks(ticks []chart.Tick, n int) []chart.Tick {
	if len(ticks) <= n {
		return ticks
	}
	step := (len(ticks) + n - 1) / n
	var out []chart.Tick
	for i := len(ticks) - 1; i >= 0; i -= step {
		out = append([]chart.Tick{ticks[i]}, out...)
	}
	return out
}
