package shell

import (
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// The window's title bar (TASK-063): on Windows the window has no system
// title bar, and the top bar of each column lines up in its place. The
// right sidebar's tabs are icons at the end of the bar, before the
// window's buttons.

// barEnd is what ends a bar: nothing, for a bar inside the window; the
// room of the window's buttons, for a bar at the window's end edge; or
// the right sidebar's tabs and that room, for the bar at the window's
// end edge while a chat is open.
type barEnd int

const (
	endNone barEnd = iota
	endRoom
	endTabs
)

// barEnd builds what end puts at the end of a bar, in a row of its own,
// so that its icons end as far from the window's buttons whatever the
// bar's gaps.
func (s *Shell) barEnd(c *kit.Context, end barEnd) {
	if end == endNone {
		return
	}
	kit.Row(c).Shrink(0).Gap(8).AlignItems(kit.Center).Children(func() {
		if end == endTabs {
			s.tabIcons(c)
		}
		kit.ControlsRoom(c)
	})
}

// rightTab is one of the right sidebar's tabs, as its icon in the bar.
type rightTab struct {
	tab   state.RightTab
	label string
	icon  *kit.SVG
}

// rightTabs are the right sidebar's tabs, in order.
var rightTabs = []rightTab{
	{state.TabPages, "Pages", kit.IconPagesTab},
	{state.TabPipelines, "Pipelines", kit.IconPipelines},
	{state.TabFiles, "Files", kit.IconFolder},
	{state.TabLinks, "Links", kit.IconLink},
}

// tabName is the name of the right sidebar's tab t.
func tabName(t state.RightTab) string {
	for _, r := range rightTabs {
		if r.tab == t {
			return r.label
		}
	}
	return rightTabs[0].label
}

// tabIcons are the right sidebar's tabs as icons: a click opens the
// sidebar on the tab, or hides it when it shows that tab already; the
// tab it shows is pressed. In a narrow window the sidebar is the overlay
// from the end edge.
func (s *Shell) tabIcons(c *kit.Context) {
	sh := s.Stores.Shell
	narrow := kit.Narrow(c)
	kit.BarGroup(c, "Project sidebar", func() {
		for _, t := range rightTabs {
			if kit.BarToggle(c, t.icon, t.label, sh.RightOpen(t.tab, narrow)).Changed() {
				sh.ToggleRight(t.tab, narrow)
			}
		}
	})
}

// chatEnds is what ends the bars of an open chat, of its file and of the
// chat beside it, in a wide window: the bar at the window's end edge gets
// the tabs, unless the right sidebar shows, whose bar has them then.
func (s *Shell) chatEnds(ref state.ChatRef, docked bool) (file, chat barEnd) {
	sh := s.Stores.Shell
	switch {
	case !sh.RightHidden:
		return endNone, endNone
	case !docked:
		return endNone, endTabs
	case sh.ChatHidden[ref] || sh.Side[ref.Project] == state.DockLeft:
		return endTabs, endNone
	}
	return endNone, endTabs
}
