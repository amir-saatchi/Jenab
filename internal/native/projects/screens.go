// Package projects is the project screens on MyGo (SPEC 3.9, 5.12): new
// project and the welcome page, a project's settings with its approval
// level and private-network exceptions, and the exports of a project and
// of a chat, as frontend/src/shell's new-project, welcome,
// project-settings, folder-warning, private-hosts, export-project,
// export-chat and chat-menu. It uses the kit and the stores only.
package projects

import (
	"errors"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
)

// Screens holds what the project screens keep between frames: what the
// dialogs show and the settings page's place. Build the dialogs with
// Dialogs each frame, after the main area.
type Screens struct {
	Stores *state.Stores
	// ProjectMemory builds the project's memory section in its settings
	// (SPEC 3.3), Settings' memory editor; nil leaves the section out.
	ProjectMemory func(c *kit.Context, p id.Project)

	post state.Post

	// New project.
	name, problem string
	creating      bool

	// The project's export.
	exportOpen   bool
	exportFor    id.Project
	exportFormat string
	approvals    bool

	// The chat's export.
	chatOpen   bool
	chatFor    chatTarget
	chatFormat string

	// The settings page: the project shown and at which section, the
	// page chosen in the list, and the section to bring into view.
	page              string
	pageFor           id.Project
	pageAt            string
	shown, wasShown   bool
	reveal            string
	host, hostProblem string
	levelBusy         bool
}

type chatTarget struct {
	project id.Project
	chat    id.Chat
	title   string
}

// New makes the screens. post is how work gets back to the UI thread, as
// the stores'.
func New(st *state.Stores, post state.Post) *Screens {
	return &Screens{Stores: st, post: post}
}

// Dialogs builds the dialogs: new project and the exports.
func (s *Screens) Dialogs(c *kit.Context) {
	s.newProject(c)
	s.exportDialog(c)
	s.chatExportDialog(c)
	s.wasShown, s.shown = s.shown, false
}

// ProjectMenu fills the menu of the project's name over the chat list:
// *Project settings* and *Export…* (SPEC 5.12).
func (s *Screens) ProjectMenu(m *kit.Menu, p id.Project) {
	if m.Item("Project settings").Chosen() {
		s.OpenSettings(p, "")
	}
	if m.Item("Export…").Chosen() {
		s.OpenExport(p)
	}
}

// ChatMenu fills a chat's ⋯ menu, in its header and its row (chat-menu.tsx):
// *Export chat…*.
func (s *Screens) ChatMenu(m *kit.Menu, p id.Project, ch id.Chat, title string) {
	if m.Item("Export chat…").Chosen() {
		s.OpenChatExport(p, ch, title)
	}
}

// OpenSettings shows the project's settings, at a section if given.
func (s *Screens) OpenSettings(p id.Project, section string) {
	s.Stores.Nav.Go(state.Place{View: state.ViewProjectSettings, Project: p, Section: section})
	s.Stores.Shell.LeftOverlay = false
}

// OpenExport opens the project's export dialog.
func (s *Screens) OpenExport(p id.Project) {
	if s.exportFor != "" && s.exportFor != p {
		s.Stores.Exports.Close(s.exportFor)
	}
	s.exportFor, s.exportOpen = p, true
	s.exportFormat, s.approvals = string(project.ExportZip), false
	s.Stores.Exports.Open(p)
}

// OpenChatExport opens the chat's export dialog; title is the chat's.
func (s *Screens) OpenChatExport(p id.Project, ch id.Chat, title string) {
	s.chatFor, s.chatOpen = chatTarget{p, ch, title}, true
	s.chatFormat = string(chatexport.FormatMarkdown)
	s.Stores.Exports.OpenChat(ch)
}

// ProjectName is the project's name, from the list or as opened.
func (s *Screens) ProjectName(p id.Project) string {
	if it := s.item(p); it != nil {
		return it.Name
	}
	return s.Stores.Projects.Opened[p].Name
}

func (s *Screens) item(p id.Project) *app.ProjectItem {
	for i, it := range s.Stores.Projects.List {
		if it.ID == p {
			return &s.Stores.Projects.List[i]
		}
	}
	return nil
}

// FolderWarning is the 2.1 warning: the data folder is on a network drive
// or in a synced folder, where SQLite files can be damaged.
func (s *Screens) FolderWarning(c *kit.Context) {
	if w := s.Stores.Projects.FolderWarning; w != nil {
		kit.Alert(c, kit.Amber, w.Text, "")
	}
}

// Welcome is the first run (SPEC 3.9): no wizard, just *Create project*.
// *Open the example* comes with the Bitcoin example.
func (s *Screens) Welcome(c *kit.Context) {
	st := s.Stores
	folder := "~/" + appinfo.Name
	if v := st.Settings.View; v != nil && v.Settings.DataFolder != "" {
		folder = v.Settings.DataFolder
	}
	col := kit.Column(c).Grow(1).AlignItems(kit.Center).Justify(kit.Center).Padding(24).Gap(8)
	col.Children(func() {
		kit.Empty(c, kit.IconProjectFolder, "Welcome to "+appinfo.Name, "Each project is one folder with its data, chats and files.", func() {
			if kit.ActionButton(c, kit.IconPlus, "Create project", kit.Default, false).Clicked() {
				st.Shell.NewProject = true
			}
			kit.Disable(kit.Tooltip(kit.Button(c, "Open the example", kit.Outline), "The example comes with a later version"), true)
		}).Grow(0)
		k := kit.TokensOf(c)
		kit.Row(c).Gap(4).AlignItems(kit.Center).FontSize(12).TextColor(k.MutedForeground).Children(func() {
			kit.Text(c, "Saved in")
			kit.Path(c, folder)
			kit.Text(c, "·")
			if kit.TextLink(c, "change in Settings").Clicked() {
				st.Nav.Go(state.Place{View: state.ViewSettings})
			}
		})
		kit.Column(c).MaxWidth(480).AlignItems(kit.Stretch).Children(func() { s.FolderWarning(c) })
	})
}

// newProject asks for a name and opens the project's Mother chat
// (new-project.tsx).
func (s *Screens) newProject(c *kit.Context) {
	st := s.Stores
	if !st.Shell.NewProject {
		s.name, s.problem = "", ""
		return
	}
	kit.Dialog(c, kit.Locked(&st.Shell.NewProject, s.creating), "New project", func() {
		kit.DialogText(c, "A project is one folder with its data, chats and files.")
		kit.Field(c, "Name", "", s.problem, func() {
			in := kit.TextInput(c, &s.name, "Name").AutoFocus()
			if in.Changed() {
				s.problem = ""
			}
			if in.Submitted() {
				s.create()
			}
		})
		kit.DialogFooter(c, func() {
			if kit.Button(c, "Cancel", kit.Outline).Clicked() && !s.creating {
				st.Shell.NewProject = false
			}
			create := kit.ActionButton(c, nil, "Create", kit.Default, s.creating)
			if kit.Disable(create, s.creating || strings.TrimSpace(s.name) == "").Clicked() {
				s.create()
			}
		})
	})
}

func (s *Screens) create() {
	st := s.Stores
	name := strings.TrimSpace(s.name)
	if name == "" || s.creating {
		return
	}
	s.creating = true
	st.Projects.Create(name, func(op app.OpenedProject, err error) {
		s.creating = false
		if err != nil {
			if ue, ok := errors.AsType[*app.UIError](err); ok && ue.Kind == app.KindInvalid {
				s.problem = ue.Message
				return
			}
			st.Error(err)
			return
		}
		st.Shell.NewProject = false
		s.report(st.Chats.Load(op.ID))
		st.OpenChat(op.ID, op.Mother)
	})
}

// report shows the error done gives, if any.
func (s *Screens) report(done <-chan error) {
	go func() {
		if err := <-done; err != nil {
			s.post(func() { s.Stores.Error(err) })
		}
	}()
}
