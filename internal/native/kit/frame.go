package kit

import "github.com/egoist/mygo/ui"

// The window's frame (SPEC 5.12): the rail, the bars and the lists that
// hold the screens.

// The icons the frame and the chat take, Lucide's as the others.
var (
	IconBrain        = icon("brain")
	IconReply        = icon("reply")
	IconPencil       = icon("pencil")
	IconUndo         = icon("undo-2")
	IconSend         = icon("send")
	IconStop         = icon("square")
	IconPaperclip    = icon("paperclip")
	IconSparkles     = icon("sparkles")
	IconShieldCheck  = icon("shield-check")
	IconFile         = icon("file")
	IconFileText     = icon("file-text")
	IconImage        = icon("image")
	IconDiamond      = icon("diamond")
	IconPin          = icon("pin")
	IconFolderCog    = icon("folder-cog")
	IconRefresh      = icon("refresh-cw")
	IconBell         = icon("bell")
	IconCircleSlash  = icon("circle-slash")
	IconAppWindow    = icon("app-window")
	IconSidebarClose = icon("panel-left-close")
	IconSidebarOpen  = icon("panel-left-open")
	IconSchedules    = icon("calendar-clock")
	IconPackage      = icon("package")
	IconArrowDown    = icon("arrow-down")
)

// Rail is the rail down the window's start edge, 56 px wide in the
// sidebar's colors (SPEC 5.12): fn builds its tiles and buttons, top to
// bottom; a Spacer pushes those after it to the bottom. label names it.
// Under the window's own title bar its top is an empty strip as high as
// the bar, which moves the window as the bar does (TASK-063).
func Rail(c *Context, label string, fn func()) Element {
	k := TokensOf(c)
	t := railTheme(c)
	box := ui.Themed(c, t).Width(56).Shrink(0).Background(k.Sidebar).BorderColor(k.SidebarBorder).BorderEndWidth(1)
	return box.Children(func() {
		col := ui.Column(c).Grow(1).AlignItems(ui.Center).Gap(4).PaddingY(8).Role(ui.RoleNavigation).Label(label)
		if c.TitleBar().Height > 0 {
			col.Padding(0, 0, 8, 0)
		}
		col.Children(func() {
			if c.TitleBar().Height > 0 {
				ui.Box(c).Height(barHeight(c)).Shrink(0).AlignSelf(ui.Stretch).DragWindow()
			}
			fn()
		})
	})
}

func railTheme(c *Context) *ui.Theme {
	k := TokensOf(c)
	t := *c.Theme()
	t.Background, t.Surface = k.Sidebar, k.Sidebar
	t.SurfaceHover, t.SurfacePressed = k.SidebarAccent, k.SidebarAccent
	t.Text = k.SidebarForeground
	t.Focus = k.SidebarRing.Alpha(0.5)
	return &t
}

// Spacer takes the room left in its row or column.
func Spacer(c *Context) Element { return ui.Spacer(c) }

// RailTile is a rail button: label names it and shows as its tooltip,
// face builds what it shows, and a bar on the start edge marks it while
// active, as the open project or chat.
func RailTile(c *Context, label string, active bool, face func()) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Width(56).Height(40).AlignItems(ui.Center).Justify(ui.Center).
		Label(label).Tooltip(label).FocusRing(false)
	if active {
		b.Current(ui.CurrentPage)
	}
	hovered, focused := b.Hovered(), b.FocusVisible()
	return b.Children(func() {
		bar := ui.Box(c).Absolute().Start(0).Width(4).Radius(0, 4, 4, 0).Background(k.SidebarForeground)
		switch {
		case active:
			bar.Top(6).Bottom(6)
		case hovered:
			bar.Top(12).Bottom(12)
		default:
			bar.Top(12).Bottom(12).Opacity(0)
		}
		ring := ui.Box(c).Size(40, 40).Radius(14).AlignItems(ui.Center).Justify(ui.Center)
		if focused {
			ring.Border(2, k.SidebarRing)
		}
		ring.Children(face)
	})
}

// TileFace is a rail tile's square: text, such as a project's initial or
// a chat's initials, in the primary color for a project and in the
// sidebar's accent for a chat, the sidebar's primary while active.
func TileFace(c *Context, text string, project, active bool) Element {
	k := TokensOf(c)
	bg, fg := k.SidebarAccent, k.SidebarAccentFg
	switch {
	case project:
		bg, fg = k.Primary, k.PrimaryForeground
	case active:
		bg, fg = k.SidebarPrimary, k.SidebarPrimaryFg
	}
	box := ui.Box(c).Size(36, 36).Radius(12).Background(bg).AlignItems(ui.Center).Justify(ui.Center)
	return box.Children(func() {
		size := float32(12)
		if project {
			size = 14
		}
		ui.Text(c, text).FontSize(size).FontWeight(600).TextColor(fg).SingleLine()
	})
}

// IconFace is a rail tile's square showing icon, as Mother's tile: in the
// foreground's color, filled.
func IconFace(c *Context, icon *SVG) Element {
	k := TokensOf(c)
	box := ui.Box(c).Size(36, 36).Radius(12).Background(k.Foreground).AlignItems(ui.Center).Justify(ui.Center)
	return box.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(k.Background) })
}

// Dot is a small round mark in a tone, as the amber dot of a chat that
// waits; it sits on the top end corner of what holds it.
func Dot(c *Context, tone Tone) Element {
	k := TokensOf(c)
	return ui.Box(c).Absolute().Top(2).End(10).Size(10, 10).Radius(5).Background(k.Tone(tone).Text).
		Border(2, k.Sidebar)
}

// Separator is a short rule between groups, across a rail or along a
// row: vertical makes it run up and down.
func Separator(c *Context, length float32, vertical bool) Element {
	k := TokensOf(c)
	s := ui.Box(c).Background(k.Border).Role(ui.RoleSeparator).Shrink(0)
	if vertical {
		return s.Size(1, length)
	}
	return s.Size(length, 1).MarginY(6)
}

// Bar is a screen's top bar, one row as high as the window's buttons
// (barHeight) with a rule under it, as the
// chat's header: fn builds its row. It is part of the window's title bar
// (titlebar.go): its empty parts move the window and a double click on
// them maximizes it, while its buttons take their clicks. The bar at the
// window's end edge ends with ControlsRoom.
func Bar(c *Context, fn func()) Element {
	k := TokensOf(c)
	return ui.Row(c).Height(barHeight(c)).Shrink(0).AlignItems(ui.Center).Gap(8).PaddingX(barPadding).
		BorderColor(k.Border).BorderWidth(0, 0, 1, 0).Role(ui.RoleBanner).DragWindow().Children(fn)
}

// Title is a bar's or a row's main line: medium weight, one line cut with
// an ellipsis, in the direction of its words.
func Title(c *Context, s string) Element {
	return ui.Text(c, s).FontWeight(500).SingleLine().Shrink(1)
}

// Muted is a second line under a title, or any quiet text: small, in the
// muted color, one line.
func Muted(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(12).TextColor(TokensOf(c).MutedForeground).SingleLine().Shrink(1)
}

// Main is the window's main area, a landmark for assistive technology:
// fn builds what it shows.
func Main(c *Context, fn func()) Element {
	return ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Role(ui.RoleMain).Children(fn)
}

// ListState is a list's place and what follows it (MyGo's ui.ListState):
// keep one per list.
type ListState = ui.ListState

// List is a virtual list of n rows: it builds only the rows in view, and
// a few beyond, so a chat of any length opens as fast as a short one.
// Give it a size, or grow it in its parent.
func List(c *Context, s *ListState, n int, row func(i int)) Element {
	return ui.List(c, s, n, row)
}

// Glide makes a list that follows its end glide there, as chat apps
// scroll: set it as s.FollowAnimation.
var Glide = &ui.Smooth

// Announce tells assistive technology news that doesn't move the focus,
// as an answer that finished; it is read once.
func Announce(c *Context, text string) { c.Announce(text) }

// SidePanel is the chat list's panel beside the rail, 256 px wide in the
// sidebar's colors, as shadcn's Sidebar: fn builds it top to bottom.
func SidePanel(c *Context, label string, fn func()) Element {
	return HeadedSidePanel(c, label, nil, fn)
}

// HeadedSidePanel is SidePanel with head, when not nil, building a
// PanelHead over the rest, as the chat list's project name: it lines up
// with the bars beside it.
func HeadedSidePanel(c *Context, label string, head, fn func()) Element {
	k := TokensOf(c)
	box := ui.Themed(c, railTheme(c)).Width(256).Shrink(0).Background(k.Sidebar).BorderColor(k.SidebarBorder).BorderEndWidth(1)
	return box.Children(func() {
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Role(ui.RoleNavigation).Label(label).Children(func() {
			if head != nil {
				PanelHead(c, head)
			}
			ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(4).Padding(8).Children(fn)
		})
	})
}

// Group is a panel's small heading over the items under it, as "Chats".
func Group(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(12).FontWeight(500).TextColor(TokensOf(c).SidebarForeground.Alpha(0.7)).Padding(6, 8, 2, 8).
		Role(ui.RoleHeading).Level(2)
}

// NavItem is a row of a panel that opens something, as a chat of the
// chat list: label names it, fn builds what it shows (an icon, a title
// and a second line, a badge), and it is filled while active, the place
// shown.
func NavItem(c *Context, label string, active bool, fn func()) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Label(label).MinHeight(48).Padding(6, 8).Gap(8).Radius(Radius * 0.8).AlignItems(ui.Center).
		TextColor(k.SidebarForeground)
	switch {
	case active:
		b.Current(ui.CurrentPage).Background(k.SidebarAccent).TextColor(k.SidebarAccentFg)
	case b.Hovered():
		b.Background(k.SidebarAccent)
	}
	return b.Children(fn)
}

// Lines is a title over a second line, both cut to one line, as a chat's
// row: the second line in tone, or muted for Neutral.
func Lines(c *Context, title, sub string, tone Tone) Element {
	k := TokensOf(c)
	return ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
		ui.Text(c, title).FontSize(14).SingleLine()
		if sub != "" {
			s := ui.Text(c, sub).FontSize(12).SingleLine().TextColor(k.MutedForeground)
			if tone != Neutral {
				s.TextColor(k.Tone(tone).Text)
			}
		}
	})
}

// Count is a small count on the top end corner of what holds it, as the
// number of chats that wait, in tone.
func Count(c *Context, n int, tone Tone) Element {
	k := TokensOf(c)
	box := ui.Box(c).Absolute().Top(0).End(4).Height(16).MinWidth(16).PaddingX(4).Radius(8).
		Background(k.Tone(tone).Text).AlignItems(ui.Center).Justify(ui.Center)
	return box.Children(func() {
		ui.Text(c, itoa(n)).FontSize(10).FontWeight(600).TextColor(k.Background).SingleLine()
	})
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// CurrentPage marks the element of a set that is the place shown, as
// Element.Current takes it.
const CurrentPage = ui.CurrentPage
