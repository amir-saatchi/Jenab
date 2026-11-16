package shell

import (
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// The dock, the right sidebar and the bottom bar (SPEC 5.9, 5.12):
// the React app's shell/dock, right-sidebar and bottom-bar (TASK-056).

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
	docked := ok && d.Kind == "file" && s.Files != nil
	fileEnd, chatEnd := s.chatEnds(ref, docked)
	narrow := kit.Narrow(c)
	if narrow {
		// Narrow, the main area is at the window's end edge.
		fileEnd, chatEnd = endTabs, endTabs
	}
	if !docked {
		s.chatView(c, p, ch, chatEnd)
		return
	}
	file := func() {
		s.Files.View(c, p, ch, d.Key, func() {
			s.dockControls(c, ref, "Close the file")
			s.barEnd(c, fileEnd)
		})
	}
	if narrow {
		s.drawer(c, p, ch, file)
		return
	}
	if sh.ChatHidden[ref] {
		file()
		return
	}
	width := float32(sh.Width[p])
	kit.Split(c, &width, state.MinDock, sh.Side[p] == state.DockLeft, "The file and the chat", file, func() {
		s.chatView(c, p, ch, chatEnd)
	})
	if int(width) != sh.Width[p] {
		sh.SetWidth(p, float64(width))
	}
}

// dockControls are the buttons in the header of what is docked
// (DockControls): swap the chat's side, hide or show the chat (in a narrow
// window, open its drawer), and close.
func (s *Shell) dockControls(c *kit.Context, ref state.ChatRef, closeLabel string) {
	sh := s.Stores.Shell
	left := sh.Side[ref.Project] == state.DockLeft
	hidden := sh.ChatHidden[ref]
	narrow := kit.Narrow(c)
	// Narrow, the chat is a drawer: *Chat* opens it.
	if narrow && kit.SmallButton(c, kit.IconChat, "Chat", kit.Outline).Clicked() {
		sh.Drawer[ref] = true
	}
	if !narrow && !hidden {
		label := "Move the chat to the left"
		if left {
			label = "Move the chat to the right"
		}
		if kit.BarIconButton(c, kit.IconSwapSides, label).Clicked() {
			sh.Swap(ref.Project)
		}
	}
	switch {
	case narrow:
	case hidden:
		if kit.SmallButton(c, kit.IconChat, "Show chat", kit.Outline).Clicked() {
			delete(sh.ChatHidden, ref)
		}
	default:
		icon := kit.IconPanelEnd
		if left {
			icon = kit.IconSidebarClose
		}
		if kit.BarIconButton(c, icon, "Hide the chat").Clicked() {
			sh.ChatHidden[ref] = true
		}
	}
	if kit.BarIconButton(c, kit.IconX, closeLabel).Clicked() {
		sh.Close(ref)
	}
}

// rightSidebar holds the project's Pages, Pipelines, Files and Links
// (right-sidebar.tsx). Pages and pipelines come in later phases; Files is
// the bucket. A jenab:// link in the chat switches to Files. It is at the
// window's end edge, so its bar has the tabs' icons (TASK-063); its tab
// shows as a small title over its content, or as the Files panel's path
// bar.
func (s *Shell) rightSidebar(c *kit.Context, p id.Project) {
	sh := s.Stores.Shell
	kit.EndPanel(c, "Project", func() {
		kit.PanelHead(c, func() { s.barEnd(c, endTabs) }).Justify(kit.End)
		// The Files panel's path bar starts with "Files", which names it.
		if sh.RightTab != state.TabFiles {
			kit.PanelTitle(c, tabName(sh.RightTab))
		}
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
