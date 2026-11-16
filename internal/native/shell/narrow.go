package shell

import (
	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// The narrow window (SPEC 5.12, TASK-062), as app-shell.tsx, chat-view.tsx
// and dock.tsx below 900 px: the rail with the chat list and the right
// sidebar open as overlays from the window's edges, a place other than a
// chat gets a header with ☰, and a chat docked next to a file is a drawer
// over it.

// drawerWidth is the docked chat's drawer, at most the window: 28rem.
const drawerWidth = 448

// PageTitle is the title of the header of a place other than a chat, in a
// narrow window (pageTitle in app-shell.tsx).
func PageTitle(place *state.Place) string {
	if place == nil {
		return appinfo.Name
	}
	switch place.View {
	case state.ViewSettings:
		return "Settings"
	case state.ViewProjectSettings:
		return "Project settings"
	case state.ViewInspector:
		return "Turn inspector"
	case state.ViewRuntime:
		return "Runtime"
	}
	return "Chat"
}

// pageHeader is the header of a place other than a chat (PageHeader): ☰
// in a narrow window, and the place's title. It is the bar at the
// window's end edge.
func (s *Shell) pageHeader(c *kit.Context, title string) {
	kit.Bar(c, func() {
		if kit.Narrow(c) {
			s.openSidebar(c)
		}
		kit.Title(c, title)
		kit.Spacer(c)
		kit.ControlsRoom(c)
	})
}

// openSidebar is ☰, opening the rail with the chat list.
func (s *Shell) openSidebar(c *kit.Context) {
	if kit.IconButton(c, kit.IconMenu, "Open the sidebar", kit.Ghost).Size(32, 32).Clicked() {
		s.Stores.Shell.LeftOverlay = true
	}
}

// overlays are the rail with the open chat's list from the start edge,
// and the right sidebar from the end edge (Overlays); ☰ and the tabs'
// icons in the bar open them.
func (s *Shell) overlays(c *kit.Context, place *state.Place, chatOpen bool) {
	sh := s.Stores.Shell
	kit.Sheet(c, &sh.LeftOverlay, false, "Projects and chats", func() {
		kit.Row(c).Grow(1).MinHeight(0).AlignItems(kit.Stretch).Children(func() {
			s.rail(c, place, false)
			if chatOpen {
				s.chatList(c, place.Project, place.Chat)
			}
		})
	})
	if !chatOpen {
		sh.RightOverlay = false
		return
	}
	kit.Sheet(c, &sh.RightOverlay, true, "Pages, pipelines, files and links", func() {
		kit.Row(c).Grow(1).MinHeight(0).AlignItems(kit.Stretch).Children(func() { s.rightSidebar(c, place.Project) })
	})
}

// drawer is the chat docked next to a file, in a narrow window: the file
// fills the area and the chat opens over it from the end edge, from the
// file's *Chat* button (Dock below 900 px).
func (s *Shell) drawer(c *kit.Context, p id.Project, ch id.Chat, file func()) {
	sh := s.Stores.Shell
	ref := state.ChatRef{Project: p, Chat: ch}
	file()
	open := sh.Drawer[ref]
	win, _ := c.Size()
	kit.Sheet(c, &open, true, "Chat", func() {
		// The width holds the sheet's border, as Tailwind's w-[28rem] does.
		// The drawer runs to the window's top end corner, under its buttons.
		kit.Column(c).Width(min(win, drawerWidth) - 1).Grow(1).MinHeight(0).AlignItems(kit.Stretch).Children(func() {
			s.chatView(c, p, ch, endRoom)
		})
	})
	if open != sh.Drawer[ref] {
		sh.Drawer[ref] = open
	}
}
