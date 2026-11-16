package kit

import "github.com/egoist/mygo/ui"

// The icons of the settings' pages (TASK-055).
var (
	IconBack        = icon("arrow-left")
	IconKey         = icon("key-round")
	IconServer      = icon("server")
	IconFolderOpen  = icon("folder-open")
	IconFolderInput = icon("folder-input")
	IconRuntime     = icon("activity")
	IconSun         = icon("sun")
	IconMoon        = icon("moon")
	IconMonitor     = icon("monitor")
)

// SettingsNav is where a settings view is: the page chosen in its list,
// and the page to scroll to in the frame it was chosen.
type SettingsNav struct {
	Active string
	jump   string
}

// Go chooses page and scrolls to its section, as a click on it in the
// list does.
func (n *SettingsNav) Go(page string) { n.Active, n.jump = page, page }

// SettingsLayout is a full-view settings screen (SPEC 5.12,
// settings-layout.tsx): a page list on the left under "Back to chats",
// and one scrolling page on the right, 672 px wide at most (1152 when
// wide). A page in the list scrolls to its SettingsSection. more builds
// further groups under the list, as the open project's settings; nil for
// none. back reports a click on Back to chats. In a narrow window the
// list is narrower, as settings-layout.tsx's w-44 below 900 px.
func SettingsLayout(c *Context, label string, pages []string, nav *SettingsNav, wide bool, more, body func()) (back bool) {
	if nav.Active == "" && len(pages) > 0 {
		nav.Active = pages[0]
	}
	Row(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(Stretch).Children(func() {
		SidePanel(c, "Settings pages", func() {
			if SmallButton(c, IconBack, "Back to chats", Ghost).AlignSelf(Start).Clicked() {
				back = true
			}
			if label != "" {
				Group(c, label)
			}
			for _, p := range pages {
				if PageItem(c, nil, p, p == nav.Active).Clicked() {
					nav.Go(p)
				}
			}
			if more != nil {
				more()
			}
		}).Width(listWidth(c))
		Scroll(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			Column(c).AlignItems(Center).Children(func() {
				w := float32(672)
				if wide {
					w = 1152
				}
				Column(c).FillWidth().MaxWidth(w).Padding(32).Gap(32).AlignItems(Stretch).Children(func() {
					settingsJump = nav
					defer func() { settingsJump = nil }()
					body()
				})
			})
		})
	})
	return back
}

// settingsJump is the nav of the SettingsLayout being built, for its
// sections to find the page to scroll to. Frames are built on the UI
// thread.
var settingsJump *SettingsNav

// PageItem is a page of a settings list: icon (nil for none) and label,
// filled while active. Clicked reports a click.
func PageItem(c *Context, icon *SVG, label string, active bool) Element {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Label(label).MinHeight(32).Padding(6, 8).Gap(8).Radius(Radius * 0.8).AlignItems(ui.Center).Justify(ui.Start).
		TextColor(k.SidebarForeground)
	switch {
	case active:
		b.Current(ui.CurrentPage).Background(k.SidebarAccent).TextColor(k.SidebarAccentFg)
	case b.Hovered():
		b.Background(k.SidebarAccent)
	}
	return b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(16, 16).Shrink(0)
		}
		ui.Text(c, label).FontSize(14).SingleLine().Shrink(1)
	})
}

// SettingsTitle is a settings view's title, with a line under it ("" for
// none).
func SettingsTitle(c *Context, title, description string) Element {
	return Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(20).FontWeight(600).Role(ui.RoleHeading).Level(1)
		if description != "" {
			ui.Text(c, description).FontSize(14).TextColor(TokensOf(c).MutedForeground)
		}
	})
}

// SettingsSection is a page of a settings view: its title, a line about
// it ("" for none) and what fn builds. A click on the page in the list
// scrolls to it.
func SettingsSection(c *Context, title, description string, fn func()) Element {
	k := TokensOf(c)
	s := Column(c).Gap(12).AlignItems(Stretch).Role(ui.RoleGroup).Label(title)
	if n := settingsJump; n != nil && n.jump == title {
		n.jump = ""
		s.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollStart, Animation: &ui.Smooth})
	}
	return s.Children(func() {
		Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(500).Role(ui.RoleHeading).Level(2)
			if description != "" {
				ui.Text(c, description).FontSize(14).TextColor(k.MutedForeground)
			}
		})
		fn()
	})
}

// SettingsDivider is the line between the pages of a settings view.
func SettingsDivider(c *Context) Element {
	return ui.Box(c).Height(1).FillWidth().Background(TokensOf(c).Border).Role(ui.RoleSeparator).Shrink(0)
}

// FactRow is a row of a label, 144 px wide in the muted color, and what
// value builds, with action's buttons at its end (nil for none), as
// settings show a fact or a setting.
func FactRow(c *Context, label string, value, action func()) Element {
	return Row(c).Gap(16).AlignItems(Center).Children(func() {
		ui.Text(c, label).FontSize(14).TextColor(TokensOf(c).MutedForeground).Width(144).Shrink(0)
		Row(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(Center).Children(value)
		if action != nil {
			Row(c).Gap(8).Shrink(0).Children(action)
		}
	})
}

// Hint is a line of help under a setting: small and muted, wrapping.
func Hint(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(12).TextColor(TokensOf(c).MutedForeground)
}

// SettingRow is a setting with a control at its end, as a switch: its
// label, a line about it ("" for none), and what control builds.
func SettingRow(c *Context, label, description string, control func()) Element {
	return Row(c).Gap(16).AlignItems(Center).Children(func() {
		Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(14).FontWeight(500)
			if description != "" {
				ui.Text(c, description).FontSize(14).TextColor(TokensOf(c).MutedForeground)
			}
		})
		Row(c).Shrink(0).Children(control)
	})
}

// LeftToRight lays e out left to right in a right-to-left language too,
// as a URL, a path or code reads (Element.Direction).
var LeftToRight = ui.LeftToRight

// Subheading is a heading inside a settings page, as "By model": small
// and medium weight, a heading to assistive technology.
func Subheading(c *Context, title string) Element {
	return ui.Text(c, title).FontSize(14).FontWeight(500).Role(ui.RoleHeading).Level(3)
}

// listWidth is the page list's width: w-56, w-44 in a narrow window.
func listWidth(c *Context) float32 {
	if Narrow(c) {
		return 176
	}
	return 224
}
