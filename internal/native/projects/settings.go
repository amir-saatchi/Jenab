package projects

import (
	"errors"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Levels are the approval levels (SPEC 8.8), as the page offers them.
var Levels = []kit.Choice{
	{Value: "strict", Label: "Strict"},
	{Value: "standard", Label: "Standard"},
	{Value: "auto", Label: "Auto"},
}

// The page's sections, as its list names them.
const (
	pageGeneral   = "General"
	pageMemory    = "Memory"
	pageApprovals = "Approvals"
	pageHosts     = "Private-network exceptions"
	pageExport    = "Export"
)

// memoryText is the line under the project memory's section.
const memoryText = "What every chat in this project knows: its goals, decisions and conventions. Keep values that change, such as prices, in tables."

// Settings is a project's settings (SPEC 5.12, project-settings.tsx): one
// scrolling page titled with the project's name, in Settings' frame
// (kit.SettingsLayout): *Back to chats* and the list of its sections
// beside it. section ("" for the top) is where it opens, as a chat's
// memory chip opens Memory.
func (s *Screens) Settings(c *kit.Context, p id.Project, section string) {
	if !s.wasShown || s.pageFor != p || s.pageAt != section {
		s.openPage(p, section)
	}
	s.shown = true
	pages := []string{pageGeneral}
	if s.ProjectMemory != nil {
		pages = append(pages, pageMemory)
	}
	pages = append(pages, pageApprovals, pageHosts, pageExport)
	name := s.ProjectName(p)
	label := "Project"
	if name != "" {
		label = "Project: " + name
	}
	if kit.SettingsLayout(c, label, pages, &s.nav, false, nil, func() { s.body(c, p, name) }) {
		s.Stores.BackToChats()
	}
}

// openPage readies the page as it opens, or opens at another project or
// section.
func (s *Screens) openPage(p id.Project, section string) {
	st := s.Stores
	s.pageFor, s.pageAt = p, section
	s.nav = kit.SettingsNav{}
	if section != "" {
		s.nav.Go(section)
	}
	s.host, s.hostProblem, s.levelBusy = "", "", false
	st.Hosts.Load(p)
	if _, ok := st.Projects.Opened[p]; !ok {
		st.Projects.Open(p, func(_ app.OpenedProject, err error) {
			if err != nil {
				st.Error(err)
			}
		})
	}
}

// body is the page in its frame: the title and the sections, as
// project-settings.tsx's.
func (s *Screens) body(c *kit.Context, p id.Project, name string) {
	it := s.item(p)
	if it != nil {
		kit.SettingsTitle(c, name, "Project settings · created "+kit.MediumDate(c, it.CreatedAt))
	} else {
		kit.SettingsTitle(c, "Project settings", "")
	}
	kit.SettingsSection(c, pageGeneral, "", func() {
		kit.FactRow(c, "Name", func() { kit.Text(c, name).FontSize(14).Shrink(1) }, nil)
		kit.FactRow(c, "Folder", func() {
			if it != nil {
				kit.Path(c, it.Folder).SingleLine().Shrink(1)
			}
		}, nil)
		s.FolderWarning(c)
	})
	if s.ProjectMemory != nil {
		kit.SettingsDivider(c)
		kit.SettingsSection(c, pageMemory, memoryText, func() { s.ProjectMemory(c, p) })
	}
	kit.SettingsDivider(c)
	kit.SettingsSection(c, pageApprovals, "How much the agent asks before it acts. A change applies from the next tool call.", func() {
		kit.FactRow(c, "Approval level", func() { s.level(c, p) }, nil)
	})
	kit.SettingsDivider(c)
	s.hosts(c, p)
	kit.SettingsDivider(c)
	kit.SettingsSection(c, pageExport, "A copy of the project as it is now, its data, chats and files, as a zip file or a folder.", func() {
		if kit.ActionButton(c, kit.IconDownload, "Export…", kit.Outline, false).AlignSelf(kit.Start).Clicked() {
			s.OpenExport(p)
		}
	})
}

// level is the approval level's choice. One change runs at a time, so
// the calls can't finish out of order.
func (s *Screens) level(c *kit.Context, p id.Project) {
	st := s.Stores
	op, ok := st.Projects.Opened[p]
	level := op.Level
	kit.Disable(kit.Segmented(c, &level, Levels, "Approval level"), !ok || s.levelBusy)
	if !ok || s.levelBusy || level == op.Level {
		return
	}
	s.levelBusy = true
	st.Actions.SetLevel(p, op.Mother, level, func(err error) {
		s.levelBusy = false
		if err != nil {
			st.Error(err)
			return
		}
		st.Projects.SetLevel(p, level)
	})
}

// hosts is the project's private-network exceptions (SPEC 5.12, 6.7,
// private-hosts.tsx): hosts whose requests may reach this computer or the
// local network without a card. A card's "Allow in this project" adds
// one; the next request uses the list.
func (s *Screens) hosts(c *kit.Context, p id.Project) kit.Element {
	st := s.Stores
	hosts, loaded := st.Hosts.ByProject[p]
	busy := st.Hosts.Busy[p]
	return kit.SettingsSection(c, pageHosts, "Requests from this project to these hosts may reach this computer or your local network without asking. Other hosts ask in a card each time.", func() {
		if len(hosts) > 0 {
			kit.Column(c).Gap(8).AlignItems(kit.Stretch).Children(func() {
				for _, h := range hosts {
					kit.ItemRow(c, h, func() {
						kit.Path(c, h).Grow(1).Shrink(1)
						remove := kit.IconButton(c, kit.IconX, "Remove "+h, kit.Ghost).Size(28, 28)
						if kit.Disable(remove, busy).Clicked() {
							st.Hosts.Change(p, func(hs []string) []string { return state.WithoutHost(hs, h) }, s.hostDone(""))
						}
					}).Key(h)
				}
			})
		}
		kit.Field(c, "Add a host", state.HostAdvice, s.hostProblem, func() {
			kit.Row(c).Gap(8).AlignItems(kit.Center).Children(func() {
				in := kit.Disable(kit.PathInput(c, &s.host, "Add a host"), !loaded).Placeholder("127.0.0.1 or nas.local").Grow(1)
				if in.Changed() {
					s.hostProblem = ""
				}
				add := kit.ActionButton(c, kit.IconPlus, "Add", kit.Outline, false)
				clicked := kit.Disable(add, busy || !loaded || strings.TrimSpace(s.host) == "").Clicked()
				if (clicked || in.Submitted()) && loaded && !busy {
					s.addHost(p)
				}
			})
		})
	})
}

func (s *Screens) addHost(p id.Project) {
	s.hostProblem = state.HostProblem(s.host)
	if s.hostProblem != "" {
		return
	}
	text := s.host
	s.Stores.Hosts.Change(p, func(hs []string) []string { return state.WithHost(hs, text) }, s.hostDone(text))
}

// hostDone takes a change's end: an added host's text leaves the field;
// a host refused shows under it, and other errors in a toast.
func (s *Screens) hostDone(added string) func(error) {
	return func(err error) {
		if err == nil {
			if added != "" && s.host == added {
				s.host = ""
			}
			return
		}
		if ue, ok := errors.AsType[*app.UIError](err); ok && ue.Kind == app.KindInvalid {
			s.hostProblem = ue.Message
			return
		}
		s.Stores.Error(err)
	}
}
