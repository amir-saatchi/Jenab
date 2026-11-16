// Package settings is the native Settings screen (TASK-055), ported from
// the React app's settings and shell/settings-view.tsx: General with About,
// Memory, Appearance, Models with the limits, Usage, Desktop (Windows)
// and Developer, as one scrolling page beside a page list. It imports the
// kit and the stores only; its pure logic is in settings/logic.
package settings

import (
	"runtime"
	"slices"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Screen is Settings as a full view (SPEC 5.12).
type Screen struct {
	// Windows shows the Desktop page and the desktop control switch:
	// desktop control is Windows-only in v1 (SPEC 8.10).
	Windows bool

	st     *state.Stores
	post   state.Post
	nav    kit.SettingsNav
	memory *MemoryEditor
	folder folderDialog
	models modelsPage
	desk   desktopPage
}

// New makes the screen on the stores; post runs work on the UI thread, as
// the stores' does.
func New(st *state.Stores, post state.Post) *Screen {
	s := &Screen{Windows: runtime.GOOS == "windows", st: st, post: post}
	s.memory = &MemoryEditor{Scope: "user", Load: st.Pages.UserMemory, Save: st.Pages.SaveUserMemory, Error: st.Error}
	return s
}

// wait gives done's error to then on the UI thread.
func (s *Screen) wait(done <-chan error, then func(error)) {
	go func() {
		err := <-done
		s.post(func() { then(err) })
	}()
}

// report shows done's error, if any.
func (s *Screen) report(done <-chan error) {
	s.wait(done, func(err error) { s.st.Error(err) })
}

// edit changes the settings, showing the error if the save fails.
func (s *Screen) edit(change func(*config.Settings)) { s.report(s.st.Settings.Edit(change)) }

// enter reads what the pages show, as the view opens.
func (s *Screen) enter(place *state.Place) {
	st := s.st
	s.report(st.Settings.Load())
	s.report(st.Settings.LoadModels())
	if st.Settings.Started == nil {
		s.report(st.Settings.LoadStarted())
	}
	st.Pages.LoadVersion()
	st.Pages.LoadUsage(st.Pages.UsageDays)
	if s.Windows {
		st.Pages.LoadDesktop()
	}
	s.memory.Reload()
	s.nav = kit.SettingsNav{}
	if place.Section != "" {
		s.nav.Go(place.Section)
	}
}

// View builds the screen at place. A visit reads the pages again: a new
// place, or the screen built again after another view.
func (s *Screen) View(c *kit.Context, place *state.Place) {
	root := kit.Column(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(kit.Stretch).Key("settings-screen")
	if at := kit.Local(root, "place", func() *state.Place { return nil }); *at != place {
		*at = place
		s.enter(place)
	}
	root.Children(func() { s.layout(c) })
	s.folder.view(c, s)
	s.models.dialogs(c, s)
}

func (s *Screen) layout(c *kit.Context) {
	view := s.st.Settings.View
	more := func() { s.openProjectPages(c) }
	if kit.SettingsLayout(c, "", logic.Pages(s.Windows), &s.nav, false, more, func() {
		kit.SettingsTitle(c, "Settings", "")
		if view == nil {
			kit.Spinner(c, "Loading the settings")
			return
		}
		s.problems(c, view.Problems)
		s.general(c, view.Settings)
		kit.SettingsDivider(c)
		s.memoryPage(c)
		kit.SettingsDivider(c)
		s.appearance(c, view.Settings)
		kit.SettingsDivider(c)
		s.modelsPage(c, view.Settings)
		s.limits(c, view.Settings)
		kit.SettingsDivider(c)
		s.usagePage(c)
		kit.SettingsDivider(c)
		if s.Windows {
			s.desktopPage(c)
			kit.SettingsDivider(c)
		}
		s.developer(c, view.Settings)
	}) {
		s.st.BackToChats()
	}
}

// OpenProject is the open project: the one of the chat used last, if it
// is still there.
func OpenProject(st *state.Stores) *app.ProjectItem {
	if len(st.Nav.MRU) == 0 {
		return nil
	}
	p := st.Nav.MRU[0].Project
	if i := slices.IndexFunc(st.Projects.List, func(it app.ProjectItem) bool { return it.ID == p }); i >= 0 {
		return &st.Projects.List[i]
	}
	return nil
}

// OpenProjectSettings opens the project's settings, at a section if
// given.
func OpenProjectSettings(st *state.Stores, p id.Project, section string) {
	st.Nav.Go(state.Place{View: state.ViewProjectSettings, Project: p, Section: section})
	st.Shell.LeftOverlay = false
}

// openProjectPages is the open project in the page list, under its name:
// its Project settings. Nothing with no project open.
func (s *Screen) openProjectPages(c *kit.Context) {
	p := OpenProject(s.st)
	if p == nil {
		return
	}
	kit.Group(c, p.Name)
	if kit.PageItem(c, kit.IconFolderCog, "Project settings", false).Clicked() {
		OpenProjectSettings(s.st, p.ID, "")
	}
}

// problems lists the settings in config.yaml that were ignored.
func (s *Screen) problems(c *kit.Context, ps []config.Problem) {
	if len(ps) == 0 {
		return
	}
	text := ""
	for i, p := range ps {
		if i > 0 {
			text += "\n"
		}
		text += "• " + logic.ProblemLine(p)
	}
	kit.Alert(c, kit.Amber, "Some settings in config.yaml were ignored", text)
}

// general is Settings › General (SPEC 3.9): the data folder, the approval
// level for new projects, desktop control and the app's version.
func (s *Screen) general(c *kit.Context, set config.Settings) {
	st := s.st
	started := st.Settings.Started
	kit.SettingsSection(c, "General", "", func() {
		saved := set.DataFolder
		if saved == "" && started != nil {
			saved = started.DefaultDataFolder
		}
		kit.FactRow(c, "Data folder", func() {
			if started != nil {
				kit.Tooltip(kit.Mono(c, started.DataFolder).SingleLine().Shrink(1), started.DataFolder)
			}
		}, func() {
			b := kit.Disable(kit.Button(c, "Change…", kit.Outline).Height(32).PaddingX(12), started == nil)
			if b.Clicked() && started != nil {
				s.folder.open(saved, started.DefaultDataFolder)
			}
		})
		if started != nil && !logic.SamePath(saved, started.DataFolder) {
			kit.Alert(c, kit.Neutral, "At the next start, the projects move to "+saved+". Settings and logs stay where they are.", "")
		}
		if m := started; m != nil && m.Moved != nil {
			if text := logic.MovedText(m.Moved.Moved, m.Moved.Failed); text != "" {
				tone := kit.Neutral
				if len(m.Moved.Failed) > 0 {
					tone = kit.Amber
				}
				kit.Alert(c, tone, text, "")
			}
		}
		if w := st.Projects.FolderWarning; w != nil {
			kit.Alert(c, kit.Amber, w.Text, "")
		}
		kit.FactRow(c, "Approval level", func() {
			level := set.Approvals.DefaultLevel
			choices := make([]kit.Choice, len(logic.Levels))
			for i, l := range logic.Levels {
				choices[i] = kit.Choice{Value: l.Value, Label: l.Label}
			}
			if kit.Segmented(c, &level, choices, "Approval level").Changed() {
				s.edit(func(c *config.Settings) { c.Approvals.DefaultLevel = level })
			}
		}, nil)
		kit.Hint(c, "The approval level for new projects. Each project can change its own in its settings.")
		if s.Windows {
			kit.SettingRow(c, "Let agents use other apps' windows",
				"In a chat you start, the agent can ask you to pick one window, such as your browser or an editor, and then read it and click or type in it. Each pick, and the first key it presses, asks you.",
				func() {
					on := set.DesktopControl
					if kit.Switch(c, &on, "Let agents use other apps' windows").Changed() {
						s.edit(func(c *config.Settings) { c.DesktopControl = on })
					}
				})
		}
		s.about(c)
		s.updates(c, set)
	})
}

// about is the app's version and build (TASK-037); Copy gives both lines,
// for a bug report.
func (s *Screen) about(c *kit.Context) {
	v := s.st.Pages.Version
	if v == nil {
		return
	}
	line, build := logic.VersionLine(*v), logic.BuildLine(*v)
	kit.FactRow(c, "About", func() {
		kit.Column(c).Gap(2).Shrink(1).MinWidth(0).Children(func() {
			kit.Mono(c, line).SingleLine()
			kit.Muted(c, build)
		})
	}, func() {
		if kit.SmallButton(c, kit.IconCopy, "Copy", kit.Ghost).Clicked() {
			c.WriteClipboard(line + "\n" + build)
			s.st.Toasts = append(s.st.Toasts, state.Toast{Text: "Copied the version"})
		}
	})
}

// appearance is the theme, which applies at once.
func (s *Screen) appearance(c *kit.Context, set config.Settings) {
	kit.SettingsSection(c, "Appearance", "Applies at once.", func() {
		kit.FactRow(c, "Theme", func() {
			theme := state.ReadTheme(set.UI.Theme)
			choices := make([]kit.Choice, len(logic.Themes))
			for i, t := range logic.Themes {
				choices[i] = kit.Choice{Value: t.Value, Label: t.Label}
			}
			if kit.Segmented(c, &theme, choices, "Theme").Changed() {
				s.report(s.st.Settings.SetTheme(theme))
			}
		}, nil)
	})
}

// developer turns the developer tools on (SPEC 8.4); they are set up as
// the app starts.
func (s *Screen) developer(c *kit.Context, set config.Settings) {
	started := s.st.Settings.Started
	kit.SettingsSection(c, "Developer", "", func() {
		hint := "The turn inspector, the runtime panel and debug logging."
		if started != nil && started.DevTools != set.DevTools {
			hint += " Applies at the next start."
		}
		kit.SettingRow(c, "Developer tools", hint, func() {
			on := set.DevTools
			if kit.Switch(c, &on, "Developer tools").Changed() {
				s.edit(func(c *config.Settings) { c.DevTools = on })
			}
		})
		if started != nil && started.DevTools {
			kit.Row(c).Gap(8).Children(func() {
				if kit.SmallButton(c, kit.IconSearch, "Turn inspector", kit.Outline).Clicked() {
					s.st.Nav.Go(state.Place{View: state.ViewInspector})
				}
				if kit.SmallButton(c, kit.IconRuntime, "Runtime", kit.Outline).Clicked() {
					s.st.Nav.Go(state.Place{View: state.ViewRuntime})
				}
			})
		}
	})
}

// dateTime is a time with its date, in the user's formats.
func dateTime(c *kit.Context) func(time.Time) string {
	return func(t time.Time) string { return kit.MediumDate(c, t) + ", " + kit.Clock(c, t) }
}
