package dev

import (
	"fmt"
	"strconv"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/kit"
)

// The runtime panel (SPEC 8.4): the chats in a turn, the background work,
// each database writer, the LLM-call slots, and anything that hasn't moved
// for longer than its limit. It reads again every second while it shows.

func (s *Screen) runtime(c *kit.Context) {
	d := s.dev
	d.PollRuntime()
	if d.RuntimeError != "" {
		kit.Alert(c, kit.Red, "Can't read the runtime", d.RuntimeError)
	}
	rt := d.Runtime
	if rt == nil {
		if d.RuntimeError == "" {
			kit.Spinner(c, "Loading").AlignSelf(kit.Start)
		}
		return
	}
	if rt.Stuck > 0 {
		kit.Alert(c, kit.Amber, StuckTitle(rt.Stuck), "They are marked Stuck below.")
	}
	calls(c, rt)
	if len(rt.Projects) == 0 {
		kit.Text(c, "No project is open.").FontSize(14).TextColor(kit.TokensOf(c).MutedForeground)
		return
	}
	for i := range rt.Projects {
		s.project(c, &rt.Projects[i])
	}
}

func stuck(c *kit.Context) { kit.Badge(c, "Stuck", kit.Amber) }

var providerColumns = []kit.DataColumn{
	{Title: "Provider", Grow: 1.2}, {Title: "Running", Width: 80, End: true}, {Title: "Waiting", Width: 80, End: true},
	{Title: "Limit", Width: 80, End: true}, {Title: "State", Grow: 2},
}

// calls is the LLM-call slots and each provider's limit.
func calls(c *kit.Context, rt *app.Runtime) {
	description := fmt.Sprintf("%d of %d slots in use, %d waiting. Chat calls take a slot but never wait for one, so they can go past the limit.",
		rt.Calls.InUse, rt.Calls.Size, rt.Calls.Waiting)
	var body func()
	if len(rt.Providers) > 0 {
		body = func() {
			kit.DataTable(c, "Providers", providerColumns, func() {
				for i, p := range rt.Providers {
					kit.DataRow(c, providerColumns, false, func(col int) {
						switch col {
						case 0:
							kit.Text(c, p.Name).Shrink(1)
						case 1:
							kit.Text(c, strconv.Itoa(p.Running))
						case 2:
							kit.Text(c, strconv.Itoa(p.Waiting))
						case 3:
							kit.Text(c, ProviderLimit(p))
						case 4:
							kit.Text(c, ProviderState(p)).Shrink(1)
						}
					}).Key(i)
				}
			})
		}
	}
	kit.TitledCard(c, "LLM calls", description, body)
}

var workColumns = []kit.DataColumn{
	{Title: "Work", Grow: 2}, {Title: "State", Grow: 1.2}, {Title: "Running for", Width: 104, End: true},
	{Title: "Last moved", Width: 104, End: true}, {Title: "Limit", Width: 72, End: true}, {Title: "", Width: 72, End: true},
}

var writerColumns = []kit.DataColumn{
	{Title: "Writer", Grow: 1.4}, {Title: "Queue", Width: 72, End: true}, {Title: "Now", Grow: 1.2},
	{Title: "Done", Width: 72, End: true}, {Title: "Last error", Grow: 1.4}, {Title: "", Width: 72, End: true},
}

// project is an open project's work and its database writers.
func (s *Screen) project(c *kit.Context, p *app.ProjectRuntime) {
	name := p.Name
	if name == "" {
		name = string(p.Project)
	}
	now := s.now()
	kit.TitledCard(c, name, ProjectLine(len(p.Work), p.Leases), func() {
		k := kit.TokensOf(c)
		red := k.Tone(kit.Red).Text
		if len(p.Work) > 0 {
			kit.DataTable(c, "Work", workColumns, func() {
				for _, w := range p.Work {
					kit.DataRow(c, workColumns, false, func(col int) {
						switch col {
						case 0:
							kit.Column(c).Gap(2).PaddingY(6).Shrink(1).MinWidth(0).Children(func() {
								kit.Row(c).Gap(6).AlignItems(kit.Baseline).Wrap().Children(func() {
									title := w.Title
									if title == "" {
										title = w.ID
									}
									kit.Text(c, title).Shrink(1)
									kit.Text(c, w.Kind).FontSize(12).TextColor(k.MutedForeground)
								})
								if w.Err != "" {
									kit.Text(c, w.Err).FontSize(12).TextColor(red)
								}
							})
						case 1:
							kit.Text(c, w.State)
							if w.Progress != "" {
								kit.Text(c, "· "+w.Progress).TextColor(k.MutedForeground).Shrink(1)
							}
						case 2:
							kit.Text(c, Duration(now.Sub(w.Started).Milliseconds()))
						case 3:
							kit.Text(c, Duration(w.IdleMS)+" ago")
						case 4:
							limit := "–"
							if w.LimitMS > 0 {
								limit = Duration(w.LimitMS)
							}
							kit.Text(c, limit)
						case 5:
							if w.Stuck {
								stuck(c)
							}
						}
					}).Key(w.ID)
				}
			})
		}
		kit.DataTable(c, "Writers", writerColumns, func() {
			for _, w := range p.Writers {
				kit.DataRow(c, writerColumns, false, func(col int) {
					switch col {
					case 0:
						kit.Mono(c, w.File).Shrink(1)
					case 1:
						kit.Text(c, fmt.Sprintf("%d · %d", w.Interactive, w.Background))
					case 2:
						if w.Busy {
							kit.Text(c, "Writing for "+Duration(w.RunningMS))
						} else {
							kit.Text(c, "Idle").TextColor(k.MutedForeground)
						}
					case 3:
						kit.Text(c, Num(w.Done))
					case 4:
						if w.LastError != "" {
							kit.Tooltip(kit.Text(c, w.LastError).TextColor(red).SingleLine().Shrink(1), w.LastError)
						}
					case 5:
						if w.Stuck {
							stuck(c)
						}
					}
				}).Key(w.File)
			}
		})
		kit.Text(c, "Queue: interactive · background requests waiting.").FontSize(12).TextColor(k.MutedForeground)
	})
}
