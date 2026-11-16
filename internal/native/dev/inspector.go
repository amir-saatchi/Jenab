package dev

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// The turn inspector (SPEC 8.4): what the context builder sent for a turn,
// and what came back. It opens at a turn, from *Inspect* in the turn
// footer, or at the newest turn recorded.

func (s *Screen) inspector(c *kit.Context) {
	d := s.dev
	d.PollTurn()
	view, err := d.Shown()
	switch {
	case d.Turns != nil && len(d.Turns) == 0:
		framed(c, func() {
			kit.Empty(c, kit.IconSearch, "No turns recorded yet",
				"Turns are recorded from the start of the app while the developer tools are on, in memory only: the last 100.", nil)
		})
	case err != "":
		framed(c, func() { kit.Empty(c, nil, "Can't show this turn", err, nil) })
	case view != nil:
		s.turn(c, view)
	default:
		kit.Spinner(c, "Loading").AlignSelf(kit.Start)
	}
}

// pickers choose the chat, then its turn. A choice made in the last frame
// is in the pickers' values, which otherwise follow the pick.
func (s *Screen) pickers(c *kit.Context) {
	d := s.dev
	if len(d.Turns) == 0 {
		return
	}
	if s.chatValue != s.chatShown {
		for _, t := range d.Turns { // newest first: the chat's newest turn
			if ChatKey(t.Project, t.Chat) == s.chatValue {
				d.Choose(state.TurnRef{Project: t.Project, Chat: t.Chat, Turn: t.Turn})
				break
			}
		}
	} else if s.turnValue != s.turnShown && d.Pick != nil {
		if n, err := strconv.Atoi(s.turnValue); err == nil {
			d.Choose(state.TurnRef{Project: d.Pick.Project, Chat: d.Pick.Chat, Turn: n})
		}
	}
	s.chatValue, s.turnValue = "", ""
	if p := d.Pick; p != nil {
		s.chatValue, s.turnValue = ChatKey(p.Project, p.Chat), strconv.Itoa(p.Turn)
	}
	s.chatShown, s.turnShown = s.chatValue, s.turnValue

	var chats, turns []kit.PickerOption
	for _, o := range ChatOptions(d.Turns) {
		chats = append(chats, kit.PickerOption{Value: o.Key, Label: o.Label})
	}
	for _, t := range d.Turns {
		if ChatKey(t.Project, t.Chat) == s.chatValue {
			label := TurnLabel(kit.Clock(c, t.Started.Local()), t)
			turns = append(turns, kit.PickerOption{Value: strconv.Itoa(t.Turn), Label: label})
		}
	}
	kit.Row(c).Gap(8).AlignItems(kit.Center).Wrap().Shrink(1).Children(func() {
		kit.Picker(c, "Chat", &s.chatValue, chats).MaxWidth(256).Shrink(1)
		kit.Picker(c, "Turn", &s.turnValue, turns).Shrink(0)
	})
}

// turn shows the turn read: its timeline, its requests and tool calls, and
// the context blocks of the request chosen.
func (s *Screen) turn(c *kit.Context, v *app.TurnView) {
	ref := state.TurnRef{Project: v.Project, Chat: v.Chat, Turn: v.Turn}
	chosen := 0
	if n := len(v.Requests); n > 0 {
		chosen = min(s.chosen[ref.Key()], n-1)
	}
	timeline(c, v)
	kit.Row(c).Gap(16).AlignItems(kit.Start).Wrap().Children(func() {
		kit.Column(c).Gap(16).Grow(1).Basis(420).MinWidth(0).AlignItems(kit.Stretch).Children(func() {
			if i, ok := requests(c, v, chosen); ok {
				s.chosen[ref.Key()] = i
			}
			if len(v.Tools) > 0 {
				tools(c, v.Tools)
			}
		})
		if len(v.Requests) > 0 {
			kit.Column(c).Grow(1).Basis(420).MinWidth(0).AlignItems(kit.Stretch).Children(func() {
				s.blocks(c, v, chosen)
			})
		}
	})
}

// timeline lays the turn's requests, tool calls and the parts that came
// back on one time axis, from the turn's start.
func timeline(c *kit.Context, v *app.TurnView) {
	sc := NewScale(v.TookMS)
	at := func(ms int64) float64 { return float64(ms) / float64(sc.Span) }
	model := v.Model
	if model == "" {
		model = "No model yet"
	}
	description := model + " · " + Duration(v.TookMS)
	if v.Running {
		description += " so far, still running"
	}
	kit.TitledCard(c, "Timeline", description, func() {
		kit.Column(c).Gap(8).AlignItems(kit.Stretch).Children(func() {
			var ticks []kit.AxisTick
			for _, t := range sc.Ticks {
				ticks = append(ticks, kit.AxisTick{At: at(t), Label: Tick(t, sc.Step)})
			}
			kit.TimeAxis(c, ticks)
			kit.Lane(c, "Model", func() {
				for i, r := range v.Requests {
					tone := kit.Blue
					if r.Err != "" {
						tone = kit.Red
					}
					kit.LaneBar(c, at(r.StartMS), at(r.TookMS), tone, fmt.Sprintf("req %d", i+1), "").Key(i)
				}
			})
			kit.Lane(c, "Tools", func() {
				for i, t := range v.Tools {
					tone := kit.Green
					if t.Error {
						tone = kit.Red
					}
					kit.LaneBar(c, at(t.StartMS), at(t.TookMS), tone, t.Name, t.Name+" · "+Duration(t.TookMS)).Key(i)
				}
			})
			kit.Lane(c, "Parts", func() {
				for i, r := range v.Requests {
					kit.LaneBar(c, at(r.StartMS), at(r.TookMS), kit.Neutral, Parts(r.Parts), "").Key(i)
				}
			})
			if len(v.Tools) > 0 {
				k := kit.TokensOf(c)
				kit.Row(c).Wrap().AlignItems(kit.Baseline).MarginStart(84).FontSize(12).TextColor(k.MutedForeground).Children(func() {
					kit.Text(c, "Tool calls: ").PreserveSpaces()
					for i, t := range v.Tools {
						kit.Mono(c, t.Name)
						end := ", "
						if i == len(v.Tools)-1 {
							end = "."
						}
						kit.Text(c, " "+Duration(t.TookMS)+end).PreserveSpaces()
					}
				})
			}
		})
	})
}

var requestColumns = []kit.DataColumn{
	{Title: "#", Width: 48}, {Title: "Prompt", End: true}, {Title: "Cached", End: true},
	{Title: "Output", End: true}, {Title: "Time", End: true}, {Title: "Tools", Grow: 1.4},
}

// requests is the table of the turn's requests; a click on one chooses
// it, which requests reports.
func requests(c *kit.Context, v *app.TurnView, chosen int) (int, bool) {
	picked, ok := 0, false
	kit.TitledCard(c, "Requests", "Token counts from the provider. Pick a request to see its context blocks.", func() {
		kit.DataTable(c, "Requests", requestColumns, func() {
			for i, r := range v.Requests {
				var names []string
				for _, t := range v.Tools {
					if t.Request == i {
						names = append(names, t.Name)
					}
				}
				row := kit.DataRow(c, requestColumns, i == chosen, func(col int) {
					switch col {
					case 0:
						if kit.LinkButton(c, strconv.Itoa(i+1), fmt.Sprintf("Request %d", i+1)).Clicked() {
							picked, ok = i, true
						}
					case 1:
						kit.Text(c, Counted(r, Prompt(r)))
					case 2:
						kit.Text(c, Counted(r, int64(r.Usage.CacheRead)))
					case 3:
						kit.Text(c, Counted(r, int64(r.Usage.Output)))
					case 4:
						kit.Text(c, Duration(r.TookMS))
					case 5:
						if len(names) == 0 {
							kit.Text(c, "–")
						} else {
							kit.Mono(c, strings.Join(names, ", ")).Shrink(1)
						}
					}
				}).Key(i)
				if row.Clicked() {
					picked, ok = i, true
				}
				if r.Err != "" {
					kit.DataSpan(c, func() {
						kit.Text(c, r.Err).TextColor(kit.TokensOf(c).Tone(kit.Red).Text)
					}).Key(fmt.Sprintf("err %d", i))
				}
			}
		})
	})
	return picked, ok
}

var toolColumns = []kit.DataColumn{
	{Title: "Tool", Grow: 1.2}, {Title: "Time", Width: 88, End: true}, {Title: "Result", Width: 120}, {Title: "Ref", Grow: 1.4},
}

// tools is the table of the turn's tool calls.
func tools(c *kit.Context, ts []app.ToolView) {
	kit.TitledCard(c, "Tool calls", "", func() {
		kit.DataTable(c, "Tool calls", toolColumns, func() {
			for i, t := range ts {
				kit.DataRow(c, toolColumns, false, func(col int) {
					k := kit.TokensOf(c)
					switch col {
					case 0:
						kit.Mono(c, t.Name).Shrink(1)
					case 1:
						kit.Text(c, Duration(t.TookMS))
					case 2:
						if t.Error {
							kit.Text(c, Size(t.Bytes)+" · error").TextColor(k.Tone(kit.Red).Text)
						} else {
							kit.Text(c, Size(t.Bytes))
						}
					case 3:
						ref := t.Ref
						if ref == "" {
							ref = "–"
						}
						kit.Mono(c, ref).TextColor(k.MutedForeground).Shrink(1)
					}
				}).Key(i)
			}
		})
	})
}

var cacheTones = map[string]kit.Tone{"hit": kit.Green, "part": kit.Amber, "miss": kit.Neutral}

var blockColumns = []kit.DataColumn{
	{Title: "#", Width: 32}, {Title: "Block", Grow: 2}, {Title: "", Width: 88},
	{Title: "Tokens", Width: 72, End: true}, {Title: "Cache", Width: 136, End: true},
}

// blocks is the table of a request's context blocks; a block's name opens
// its text.
func (s *Screen) blocks(c *kit.Context, v *app.TurnView, index int) {
	r := v.Requests[index]
	most, sum := 1, int64(0)
	for _, b := range r.Blocks {
		most = max(most, b.Tokens)
		sum += int64(b.Tokens)
	}
	description := "Fixed order, so the prefix stays cached. Tokens are estimates."
	if r.Dropped {
		description += " The texts were dropped to save memory; the numbers stay."
	}
	kit.TitledCard(c, fmt.Sprintf("Context blocks, request %d", index+1), description, func() {
		kit.DataTable(c, "Context blocks", blockColumns, func() {
			k := kit.TokensOf(c)
			for i, b := range r.Blocks {
				kit.DataRow(c, blockColumns, false, func(col int) {
					switch col {
					case 0:
						kit.Text(c, strconv.Itoa(i+1)).TextColor(k.MutedForeground)
					case 1:
						// The cache point goes under the name when the two don't fit.
						kit.Row(c).Wrap().Gap(6).AlignItems(kit.Baseline).PaddingY(6).Shrink(1).MinWidth(0).Children(func() {
							if r.Dropped {
								kit.Text(c, b.Name).Shrink(1)
							} else if kit.LinkButton(c, b.Name, "").Shrink(1).Clicked() {
								s.dev.OpenBlock(v, index, i)
								s.blockOpen, s.lines = true, kit.ListState{}
							}
							if b.Point {
								kit.Text(c, "· cache point").FontSize(12).TextColor(k.MutedForeground).SingleLine()
							}
						})
					case 2:
						kit.ShareBar(c, float64(b.Tokens)/float64(most), fmt.Sprintf("%d %% of the largest block", b.Tokens*100/most)).Width(72)
					case 3:
						kit.Text(c, Num(int64(b.Tokens)))
					case 4:
						if b.Cache != "" {
							kit.Badge(c, b.Cache, cacheTones[b.Cache])
						}
					}
				}).Key(i)
			}
			kit.DataFoot(c, blockColumns, func(col int) {
				switch col {
				case 1:
					kit.Text(c, "Total")
				case 3:
					kit.Text(c, Num(sum))
				case 4:
					kit.Text(c, Share(int64(r.Usage.CacheRead), Prompt(r))).FontSize(12).TextAlign(kit.End).Shrink(1)
				}
			})
		})
	})
}

// blockDialog shows the text of the block open, as the provider got it.
func (s *Screen) blockDialog(c *kit.Context) {
	b := s.dev.Block
	if b == nil {
		return
	}
	if !s.blockOpen {
		s.dev.CloseBlock()
		return
	}
	kit.Dialog(c, &s.blockOpen, b.Name, func() {
		k := kit.TokensOf(c)
		kit.Text(c, fmt.Sprintf("Request %d · about %s tokens · secrets redacted", b.Request+1, Num(int64(b.Tokens)))).
			FontSize(14).TextColor(k.MutedForeground)
		switch {
		case b.Err != "":
			kit.Text(c, b.Err).TextColor(k.Tone(kit.Red).Text)
		case b.Lines == nil:
			kit.Spinner(c, "Loading").AlignSelf(kit.Start)
		default:
			lines := b.Lines
			if len(lines) == 1 && lines[0] == "" {
				lines = []string{"(empty)"}
			}
			kit.LinesView(c, &s.lines, lines, b.Name).Height(440)
		}
	}).Width(768)
}
