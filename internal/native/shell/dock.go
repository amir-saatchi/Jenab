package shell

import (
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// The dock, the right sidebar and the bottom bar (SPEC 5.9, 5.12):
// frontend/src/shell's dock, right-sidebar and bottom-bar (TASK-056).

// chatArea is a chat, with what it has open next to it (app-shell.tsx's
// ChatArea, dock.tsx's Dock): a file for now. With nothing open it is the
// chat alone. With a file, the file fills the area and the chat sits in a
// resizable dock on the project's side, at least state.MinDock wide and a
// third to start; the user can swap the side, hide the chat, or close the
// file to get the full chat back.
func (s *Shell) chatArea(c *kit.Context, p id.Project, ch id.Chat) {
	sh := s.Stores.Shell
	ref := state.ChatRef{Project: p, Chat: ch}
	d, ok := sh.Open[ref]
	if !ok || d.Kind != "file" || s.Files == nil {
		s.chatView(c, p, ch)
		return
	}
	file := func() { s.Files.View(c, p, ch, d.Key, func() { s.dockControls(c, ref, "Close the file") }) }
	if sh.ChatHidden[ref] {
		file()
		return
	}
	width := float32(sh.Width[p])
	kit.Split(c, &width, state.MinDock, sh.Side[p] == state.DockLeft, "The file and the chat", file, func() {
		s.chatView(c, p, ch)
	})
	if int(width) != sh.Width[p] {
		sh.SetWidth(p, float64(width))
	}
}

// dockControls are the buttons in the header of what is docked
// (DockControls): swap the chat's side, hide or show the chat, and close.
func (s *Shell) dockControls(c *kit.Context, ref state.ChatRef, closeLabel string) {
	sh := s.Stores.Shell
	left := sh.Side[ref.Project] == state.DockLeft
	hidden := sh.ChatHidden[ref]
	if !hidden {
		label := "Move the chat to the left"
		if left {
			label = "Move the chat to the right"
		}
		if kit.Tooltip(kit.IconButton(c, kit.IconSwapSides, label, kit.Ghost).Size(32, 32), label).Clicked() {
			sh.Swap(ref.Project)
		}
	}
	if hidden {
		if kit.SmallButton(c, kit.IconChat, "Show chat", kit.Outline).Clicked() {
			delete(sh.ChatHidden, ref)
		}
	} else {
		icon := kit.IconPanelEnd
		if left {
			icon = kit.IconSidebarClose
		}
		if kit.Tooltip(kit.IconButton(c, icon, "Hide the chat", kit.Ghost).Size(32, 32), "Hide the chat").Clicked() {
			sh.ChatHidden[ref] = true
		}
	}
	if kit.Tooltip(kit.IconButton(c, kit.IconX, closeLabel, kit.Ghost).Size(32, 32), closeLabel).Clicked() {
		sh.Close(ref)
	}
}

// rightTabs are the right sidebar's tabs, in order.
var rightTabs = []state.RightTab{state.TabPages, state.TabPipelines, state.TabFiles, state.TabLinks}

// rightSidebar holds the project's Pages, Pipelines, Files and Links
// (right-sidebar.tsx). Pages and pipelines come in later phases; Files is
// the bucket. A jenab:// link in the chat switches to Files.
func (s *Shell) rightSidebar(c *kit.Context, p id.Project) {
	sh := s.Stores.Shell
	kit.EndPanel(c, "Project", func() {
		kit.Box(c).Padding(12).Shrink(0).Children(func() {
			tab := 0
			for i, t := range rightTabs {
				if t == sh.RightTab {
					tab = i
				}
			}
			if kit.Tabs(c, &tab, "Project", "Pages", "Pipelines", "Files", "Links").Changed() {
				sh.RightTab = rightTabs[tab]
			}
		})
		switch sh.RightTab {
		case state.TabFiles:
			if s.Files != nil {
				s.Files.Panel(c, p)
			}
		case state.TabPipelines:
			nothing(c, "No pipelines yet", "Pipelines are built by the agent and run on a schedule.")
		case state.TabLinks:
			nothing(c, "No links yet", "Links the agent saves for this project show here.")
		default:
			nothing(c, "No pages yet", "Pages are built by the agent. Ask in the chat to add one.")
		}
	})
}

func nothing(c *kit.Context, title, text string) {
	kit.Column(c).Gap(4).Padding(16).AlignItems(kit.Center).Children(func() {
		kit.Text(c, title).FontSize(14).FontWeight(500).TextAlign(kit.Center)
		kit.Text(c, text).FontSize(13).TextColor(kit.TokensOf(c).MutedForeground).TextAlign(kit.Center)
	})
}

// bottomBar shows only what changes and what the user can act on
// (bottom-bar.tsx): the open project's chats at work, provider pauses,
// and the memory the system can still hand out.
func (s *Shell) bottomBar(c *kit.Context, place *state.Place) {
	st := s.Stores
	kit.StatusBar(c, func() {
		if place != nil && place.Project != "" {
			items := st.Chats.ByProject[place.Project]
			if text := state.WorkText(items); text != "" && kit.StatusButton(c, text).Clicked() {
				if it, ok := state.WorkChat(items); ok {
					st.OpenChat(place.Project, it.Chat.ID)
				}
			}
		}
		kit.Spacer(c)
		for _, p := range st.Bar.Paused() {
			kit.StatusText(c, state.ProblemText(p), kit.Amber, p.LastProblem).Key(p.Name)
		}
		if m := st.Bar.Memory; m != nil {
			tone := kit.Neutral
			switch m.Level {
			case sysmem.Low:
				tone = kit.Amber
			case sysmem.Critical:
				tone = kit.Red
			}
			kit.StatusText(c, state.MemoryText(*m), tone, state.MemoryTip(*m))
		}
	})
}

// railChats are the project's chats in the rail while the chat list is
// hidden (rail.tsx's RailChats), with New chat under them.
func (s *Shell) railChats(c *kit.Context, p id.Project, open id.Chat) {
	st := s.Stores
	kit.Separator(c, 24, false)
	kit.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
		kit.Column(c).Gap(8).AlignItems(kit.Center).Children(func() {
			for _, it := range state.Visible(st.Chats.ByProject[p]) {
				title := it.Chat.Title
				if title == "" {
					title = "New chat"
				}
				mother := it.Chat.Kind == chat.KindMother
				active := it.Chat.ID == open
				label := StatusLabel(title, it.State)
				c.Key("rail-" + string(it.Chat.ID))
				tile := kit.RailTile(c, label, active, func() {
					if mother {
						kit.IconFace(c, kit.IconDiamond)
					} else {
						kit.TileFace(c, state.Initials(title), false, active)
					}
					switch it.State {
					case chat.StateWaiting:
						kit.Dot(c, kit.Amber)
					case chat.StateWorking:
						kit.SmallSpinner(c, "Working").Absolute().Bottom(0).End(8)
					}
				})
				if tile.Clicked() {
					st.OpenChat(p, it.Chat.ID)
				}
			}
		})
	})
	if kit.Tooltip(kit.IconButton(c, kit.IconPlus, "New chat", kit.Ghost), "New chat").Clicked() {
		st.NewChat(p)
	}
}
