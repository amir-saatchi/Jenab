package settings

import (
	"strconv"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
)

// usagePage is Settings › Usage (SPEC 3.9): tokens per day, per model and
// per chat, with the cost from the catalog prices.
func (s *Screen) usagePage(c *kit.Context) {
	pages := s.st.Pages
	r := pages.Usage
	kit.SettingsSection(c, "Usage", "Tokens across all projects. Costs come from the built-in prices; your provider's bill is what counts.", func() {
		kit.Row(c).Gap(12).AlignItems(kit.Center).Children(func() {
			if r != nil {
				kit.Row(c).Gap(4).AlignItems(kit.Baseline).Children(func() {
					kit.Text(c, logic.Tokens(float64(logic.Total(r.Total)))).FontSize(14).FontWeight(500).FontFeatures("tnum")
					kit.Text(c, "tokens ·").FontSize(14)
					kit.Text(c, logic.Cost(r.Total)).FontSize(14).FontWeight(500).FontFeatures("tnum")
				})
			}
			kit.Spacer(c)
			if pages.UsageBusy {
				kit.Spinner(c, "Reading the usage")
			}
			days := strconv.Itoa(pages.UsageDays)
			choices := make([]kit.Choice, len(logic.UsageRanges))
			for i, d := range logic.UsageRanges {
				choices[i] = kit.Choice{Value: strconv.Itoa(d), Label: strconv.Itoa(d) + " days"}
			}
			if kit.Segmented(c, &days, choices, "Days").Changed() {
				if n, err := strconv.Atoi(days); err == nil {
					pages.LoadUsage(n)
				}
			}
		})
		if r == nil {
			kit.Spinner(c, "Reading the usage")
			return
		}
		usageChart(c, r.Days)
		if len(r.Skipped) > 0 {
			kit.Alert(c, kit.Amber, "Not counted, their chats couldn't be read: "+strings.Join(r.Skipped, ", "), "")
		}
		usageTable(c, "By model", r.Models, true, func(l app.UsageLine) string { return l.Model + " · " + l.Provider })
		usageTable(c, "By chat", r.Chats, false, func(l app.UsageLine) string {
			chat := l.Chat
			if chat == "" {
				chat = "Untitled chat"
			}
			return chat + " · " + l.Project
		})
		if r.Total.Unpriced > 0 {
			kit.Hint(c, "A + means some tokens are from models without a price; they count as tokens only.")
		}
	})
}

// usageChart is tokens per day, input (cache reads included) and output
// stacked (usage-chart.tsx).
func usageChart(c *kit.Context, days []app.UsageLine) {
	bars := make([]string, len(days))
	in := make([]float64, len(days))
	out := make([]float64, len(days))
	for i, d := range days {
		bars[i] = dayLabel(c, d.Day)
		in[i] = float64(d.Input + d.CacheRead + d.CacheWrite)
		out[i] = float64(d.Output)
	}
	kit.BarChart(c, "Tokens per day", bars, []kit.Series{
		{Name: "Input", Tone: kit.Blue, Values: in},
		{Name: "Output", Tone: kit.Purple, Values: out},
	}, logic.Tokens)
}

// dayLabel is a day as the chart names it, "Jan 2"; a day it can't read
// stays as it is.
func dayLabel(c *kit.Context, day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("Jan 2")
}

func usageTable(c *kit.Context, title string, lines []app.UsageLine, split bool, name func(app.UsageLine) string) {
	kit.Column(c).Gap(8).AlignItems(kit.Stretch).Children(func() {
		kit.Subheading(c, title)
		if len(lines) == 0 {
			kit.Muted(c, "No tokens used in these days.")
			return
		}
		cols := []kit.TableColumn{{Title: "Chat"}, {Title: "Tokens", Numeric: true}, {Title: "Cost", Numeric: true}}
		if split {
			cols = []kit.TableColumn{{Title: "Model"}, {Title: "Input", Numeric: true}, {Title: "Cached", Numeric: true},
				{Title: "Output", Numeric: true}, {Title: "Cost", Numeric: true}}
		}
		rows := make([][]string, len(lines))
		for i, l := range lines {
			tok := func(n int) string { return logic.Tokens(float64(n)) }
			if split {
				rows[i] = []string{name(l), tok(l.Input + l.CacheWrite), tok(l.CacheRead), tok(l.Output), logic.Cost(l)}
			} else {
				rows[i] = []string{name(l), tok(logic.Total(l)), logic.Cost(l)}
			}
		}
		kit.Table(c, title, cols, rows)
	})
}
