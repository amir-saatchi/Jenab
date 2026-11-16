package shell

import (
	"fmt"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// rail is the rail (rail.tsx): the projects, New project, Search (to
// come), Waiting, the open project's chats while the chat list is hidden,
// and Settings at the bottom.
func (s *Shell) rail(c *kit.Context, place *state.Place) {
	st := s.Stores
	var open id.Project
	if place != nil {
		open = place.Project
	}
	kit.Rail(c, "Projects", func() {
		for _, p := range st.Projects.List {
			if kit.RailTile(c, p.Name, p.ID == open, func() { kit.TileFace(c, state.Initial(p.Name), true, p.ID == open) }).Key(string(p.ID)).Clicked() {
				st.OpenProject(p.ID, "")
			}
		}
		if kit.Tooltip(kit.IconButton(c, kit.IconPlus, "New project", kit.Ghost), "New project").Clicked() {
			st.Shell.NewProject, st.Shell.LeftOverlay = true, false
		}
		kit.Separator(c, 24, false)
		kit.Disable(kit.Tooltip(kit.IconButton(c, kit.IconSearch, "Search", kit.Ghost), "Search comes with a later version"), true)
		s.waiting(c)
		if place != nil && place.View == state.ViewChat && st.ListHidden() {
			s.railChats(c, place.Project, place.Chat)
		}
		kit.Spacer(c)
		kit.Disable(kit.Tooltip(kit.IconButton(c, kit.IconSchedules, "Schedules", kit.Ghost), "Schedules come with pipelines"), true)
		if started := st.Settings.Started; s.Dev != nil && started != nil && started.DevTools {
			tools := kit.Tooltip(kit.IconButton(c, kit.IconWrench, "Developer tools", kit.Ghost), "Developer tools")
			if place != nil && devView(place.View) {
				tools.Current(kit.CurrentPage)
			}
			if tools.Clicked() {
				st.Nav.Go(state.Place{View: state.ViewInspector})
			}
		}
		settings := kit.Tooltip(kit.IconButton(c, kit.IconSettings, "Settings", kit.Ghost), "Settings")
		if place != nil && place.View != state.ViewChat && place.View != state.ViewProjectSettings && !devView(place.View) {
			settings.Current(kit.CurrentPage)
		}
		if settings.Clicked() {
			st.Nav.Go(state.Place{View: state.ViewSettings})
		}
		if s.Quit != nil && kit.Tooltip(kit.IconButton(c, kit.IconX, "Quit Jenab", kit.Ghost), "Quit Jenab").Clicked() {
			s.Quit()
		}
	})
}

// waiting is *Waiting*: the approvals and questions of every project
// (8.8); choosing one opens its chat.
func (s *Shell) waiting(c *kit.Context) {
	st := s.Stores
	ws := st.Chats.Waiting
	label := "Nothing is waiting for you"
	if len(ws) > 0 {
		label = fmt.Sprintf("Waiting for you: %d", len(ws))
	}
	b := kit.Tooltip(kit.IconButton(c, kit.IconInbox, label, kit.Ghost), label)
	b.Children(func() {
		if len(ws) > 0 {
			kit.Count(c, len(ws), kit.Amber)
		}
	})
	kit.MenuOn(b, func(m *kit.Menu) {
		if len(ws) == 0 {
			m.Item("Nothing is waiting.").Disabled(true)
			return
		}
		for _, w := range ws {
			title := w.Title
			if title == "" {
				title = "New chat"
			}
			if name := s.projectName(w.Project); name != "" {
				title += " · " + name
			}
			if m.Item(title).Subtitle(w.Waiting.Text).Chosen() {
				st.OpenProject(w.Project, w.Chat)
			}
		}
	})
}

func (s *Shell) projectName(p id.Project) string {
	for _, it := range s.Stores.Projects.List {
		if it.ID == p {
			return it.Name
		}
	}
	return s.Stores.Projects.Opened[p].Name
}

// railChats is the open project's chats in the rail while the chat list
// is hidden (RailChats): a tile each, Mother's with the diamond, a spinner
// on a chat that works and a dot on one that waits, then New chat.
func (s *Shell) railChats(c *kit.Context, p id.Project, open id.Chat) {
	st := s.Stores
	kit.Separator(c, 24, false)
	kit.Scroll(c).Shrink(1).AlignSelf(kit.Stretch).Children(func() {
		kit.Column(c).Gap(8).AlignItems(kit.Center).Children(func() {
			for _, it := range state.Visible(st.Chats.ByProject[p]) {
				ch := it.Chat
				title := ch.Title
				if title == "" {
					title = "New chat"
				}
				mother := ch.Kind == chat.KindMother
				active := ch.ID == open
				tile := kit.RailTile(c, StatusLabel(title, it.State), active, func() {
					if mother {
						kit.IconFace(c, kit.IconDiamond)
					} else {
						kit.TileFace(c, state.Initials(title), false, active)
					}
					switch it.State {
					case chat.StateWorking:
						if !mother {
							kit.TileSpinner(c, "Working")
						}
					case chat.StateWaiting:
						kit.Dot(c, kit.Amber)
					}
				}).Key(string(ch.ID))
				if tile.Clicked() {
					st.OpenChat(p, ch.ID)
				}
			}
		})
	})
	if kit.Tooltip(kit.IconButton(c, kit.IconPlus, "New chat", kit.Ghost), "New chat").Clicked() {
		st.NewChat(p)
	}
}

// chatList is the chat list beside the rail (chat-sidebar.tsx): the
// project's name with its menu (Project settings, Export…), New chat,
// and the chats with Mother first, each with its ⋯ menu.
func (s *Shell) chatList(c *kit.Context, p id.Project, open id.Chat) {
	st := s.Stores
	kit.SidePanel(c, "Chats", func() {
		folder := ""
		for _, it := range st.Projects.List {
			if it.ID == p {
				folder = it.Folder
			}
		}
		kit.MenuTitle(c, s.projectName(p), folder, "Project menu", func(m *kit.Menu) { s.Projects.ProjectMenu(m, p) })
		if kit.Button(c, "New chat", kit.Outline).Clicked() {
			st.NewChat(p)
		}
		kit.Group(c, "Chats")
		items, loaded := st.Chats.ByProject[p]
		if !loaded {
			kit.Spinner(c, "Loading the chats").AlignSelf(kit.Center)
			return
		}
		kit.Scroll(c).Grow(1).Children(func() {
			kit.Column(c).Gap(2).AlignItems(kit.Stretch).Children(func() {
				for _, it := range state.Visible(items) {
					s.chatRow(c, p, it.Chat, it.State, s.subline(c, it), it.Chat.ID == open)
				}
			})
		})
	})
}

func (s *Shell) chatRow(c *kit.Context, p id.Project, ch chat.Chat, st chat.State, sub string, active bool) {
	raw := ch.Title
	title := ch.Title
	if title == "" {
		title = "New chat"
	}
	mother := ch.Kind == chat.KindMother
	tone := kit.Neutral
	if st == chat.StateWaiting {
		tone = kit.Amber
	}
	face := func(menu bool) {
		icon := kit.IconChat
		if mother {
			icon = kit.IconDiamond
		}
		kit.Icon(c, icon).Size(16, 16).Shrink(0)
		kit.Lines(c, title, sub, tone)
		// The row's ⋯ button takes the badge's place while it shows.
		badge := kit.Box(c).Size(16, 16).Shrink(0).AlignItems(kit.Center).Justify(kit.Center)
		if menu {
			badge.Opacity(0)
		}
		badge.Children(func() {
			switch {
			case st == chat.StateWorking:
				kit.SmallSpinner(c, "Working")
			case st == chat.StateWaiting:
				kit.Icon(c, kit.IconWarning).Size(16, 16).TextColor(kit.TokensOf(c).Tone(kit.Amber).Text)
			case mother:
				kit.Icon(c, kit.IconPin).Size(14, 14).TextColor(kit.TokensOf(c).MutedForeground)
			}
		})
	}
	menu := func(m *kit.Menu) { s.Projects.ChatMenu(m, p, ch.ID, raw) }
	if kit.NavRow(c, string(ch.ID), StatusLabel(title, st), active, "Chat menu: "+title, face, menu) {
		s.Stores.OpenChat(p, ch.ID)
	}
}

// devView is whether v is a developer tool's.
func devView(v state.View) bool { return v == state.ViewInspector || v == state.ViewRuntime }
