package kit

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The window's own title bar (TASK-063). On Windows the window has no
// system title bar (mygo.TitleBarHidden): Windows' minimize, maximize and
// close buttons stay at its top end corner, over the view. The top row of
// each column is a Bar, all as high, so they line up as one bar; their
// empty parts move the window, and a double click maximizes it. The bar
// that reaches the window's end edge ends with ControlsRoom, so nothing
// of it is under the buttons. Where the system keeps its title bar, as on
// macOS and Linux until TASK-060, ControlsRoom takes no room.

// The right sidebar's tabs in the bar, Lucide's as the others; Files and
// Links take IconFolder and IconLink.
var (
	IconPagesTab  = icon("layout-dashboard")
	IconPipelines = icon("workflow")
)

// BarHeight is the height of every column's top bar, in DIPs, with the
// rule under it, where the system keeps its own title bar (macOS and
// Linux until TASK-060). Under Windows' buttons the bar is as high as
// they are, 32 DIPs on Windows 11, with the rule under them (barHeight).
const BarHeight = 40

// BarButton is the size of a bar's buttons, in DIPs: 28, so that they and
// their focus ring fit in Windows' 32 DIPs. Their centers line up with
// the window buttons'.
const BarButton = 28

// barHeight is the window buttons' height and the rule under it, so that
// the bar's row is as high as the buttons and centered with them; at
// least a bar button and its focus ring. BarHeight with no buttons.
func barHeight(c *Context) float32 {
	if h := c.TitleBar().Height; h > 0 {
		return max(h, BarButton+4) + 1
	}
	return BarHeight
}

// BarIconButton is an icon button of a bar, as the chat's ⋯ menu or the
// dock's buttons: a ghost IconButton BarButton square, label its name and
// tooltip. Its focus ring is on its edge, so it fits in the bar.
func BarIconButton(c *Context, icon *SVG, label string) Element {
	return IconButton(c, icon, label, Ghost).Size(BarButton, BarButton).Shrink(0).FocusRingOffset(0)
}

// BarMenuButton is a bar icon button opening the menu build fills, lined
// up with the button's end, as a button near the window's end has it.
// *open is whether the menu shows, so what the menu lists can be fetched
// as it opens rather than each frame; label names the button and is its
// tooltip. Disabled, it opens none.
func BarMenuButton(c *Context, icon *SVG, label string, open *bool, off bool, build func(m *Menu)) Element {
	b := Disable(BarIconButton(c, icon, label), off)
	return b.PopoverMenuButton(build, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Open: open})
}

// barPadding is a bar's padding on its start and end edges.
const barPadding = 12

// ControlsRoom keeps the end of the bar that reaches the window's end edge
// free for the window's buttons: an empty part of the bar, which moves the
// window as the rest does, and 8 DIPs between the bar's last button and
// the window's first. It is nothing where the system draws its own title
// bar. Put it last in the bar.
func ControlsRoom(c *Context) Element {
	w := c.TitleBar().Right
	if w <= 0 {
		return ui.Box(c).Size(0, 0)
	}
	// The bar's end padding is part of the room, and its gap between
	// children the 8 DIPs before the room.
	return ui.Box(c).Width(max(0, w-barPadding)).Height(1).Shrink(0)
}

// BarToggle is an icon button of a bar that stays pressed while on, as
// the right sidebar's tabs: BarButton square, filled with the accent while
// on. label names it for assistive technology, which gets a toggle button
// pressed while on, and shows as its tooltip. Changed reports a click, or
// Space or Enter while it has the focus.
func BarToggle(c *Context, icon *SVG, label string, on bool) Element {
	k := TokensOf(c)
	b := ui.ToggleBase(c, &on).Size(BarButton, BarButton).Shrink(0).FocusRingOffset(0).Radius(Radius * 0.8).AlignItems(ui.Center).Justify(ui.Center).
		Label(label).Tooltip(label).TextColor(k.MutedForeground)
	switch {
	case on:
		b.Background(k.Accent).TextColor(k.AccentForeground)
	case b.Hovered():
		b.Background(k.Accent).TextColor(k.Foreground)
	}
	return b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
}

// BarGroup is a row of related buttons in a bar, as the right sidebar's
// tabs: label names the group for assistive technology.
func BarGroup(c *Context, label string, fn func()) Element {
	return ui.Row(c).Shrink(0).AlignItems(ui.Center).Role(ui.RoleGroup).Label(label).Children(fn)
}

// PanelHead is a side panel's top bar, in the sidebar's colors: as high
// as Bar, with a rule under it, so that it lines up with the bars beside
// it, and it moves the window as they do. fn builds its row.
func PanelHead(c *Context, fn func()) Element {
	k := TokensOf(c)
	// Its end padding is a Bar's, so that ControlsRoom keeps the same room.
	return ui.Row(c).Height(barHeight(c)).Shrink(0).AlignItems(ui.Center).Gap(8).PaddingStart(8).PaddingEnd(barPadding).
		BorderColor(k.SidebarBorder).BorderWidth(0, 0, 1, 0).DragWindow().Children(fn)
}

// PanelTitle is a small heading at the top of a panel, naming what it
// shows, as the right sidebar's tab.
func PanelTitle(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(12).FontWeight(500).TextColor(TokensOf(c).SidebarForeground.Alpha(0.7)).
		Padding(12, 16, 4, 16).SingleLine().Role(ui.RoleHeading).Level(2)
}

// ThemeSourceOf is the app's theme setting as MyGo's theme source: light,
// dark, and System for anything else (SPEC 5.11).
func ThemeSourceOf(s string) mygo.ThemeSource {
	switch s {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}
