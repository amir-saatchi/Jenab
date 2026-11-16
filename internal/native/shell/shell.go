// Package shell is the window's frame on MyGo (SPEC 5.12): the rail, the
// chat list and the main area, as the React app's shell/app-shell, rail,
// chat-sidebar and chat-view. The dock, the right sidebar and the bottom bar are
// TASK-056's (dock.go).
package shell

import (
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/files"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/projects"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// ChatScreen shows a chat in the main area, under its header: the thread
// and the composer.
type ChatScreen interface {
	View(c *kit.Context, t *state.Thread, mother, readOnly bool)
}

// DevScreen shows the developer tools (SPEC 8.4) in the main area: the
// turn inspector or the runtime, as place says.
type DevScreen interface {
	View(c *kit.Context, place *state.Place)
}

// Shell is the window's view.
type Shell struct {
	Stores *state.Stores
	Chat   ChatScreen
	// Dev is the developer tools; nil leaves them out.
	Dev DevScreen
	// Files shows the bucket: the Files tab and a file opened next to the
	// chat (TASK-056); nil shows no files.
	Files *files.Screen
	// Settings is the Settings view (TASK-055); nil shows a placeholder.
	Settings SettingsScreen
	// Post runs work on the UI thread, as the stores' does.
	Post state.Post
	// Quit ends the app, as the tray's Quit does; nil hides the button.
	Quit func()
	// Now is the clock for the chat list's "Last active"; nil is time.Now.
	Now func() time.Time
	// Projects is the project screens: new project, welcome, project
	// settings and the exports (TASK-057).
	Projects *projects.Screens
}

// SettingsScreen is the Settings view at a place (package settings).
type SettingsScreen interface {
	View(c *kit.Context, place *state.Place)
}

// New makes the shell's view.
func New(stores *state.Stores, screen ChatScreen, post state.Post) *Shell {
	return &Shell{Stores: stores, Chat: screen, Files: files.New(stores), Post: post, Projects: projects.New(stores, post)}
}

// Start loads what the window shows first, on the UI thread.
func (s *Shell) Start() {
	st := s.Stores
	s.report(st.Projects.Load())
	s.report(st.Settings.Load())
	s.report(st.Settings.LoadModels())
	s.report(st.Chats.LoadWaiting())
	s.report(st.Settings.LoadStarted())
	st.Bar.Start()
}

func (s *Shell) report(done <-chan error) {
	go func() {
		if err := <-done; err != nil {
			s.Post(func() { s.Stores.Error(err) })
		}
	}()
}

// View builds the window.
func (s *Shell) View(c *kit.Context) {
	kit.Root(c, func() {
		for _, t := range s.Stores.TakeToasts() {
			kind := kit.ToastInfo
			switch {
			case t.Error:
				kind = kit.ToastError
			case t.Warning:
				kind = kit.ToastWarning
			}
			if t.Action != "" {
				kit.ToastAction(c, t.Text, t.Detail, t.Action, t.OnAction)
				continue
			}
			kit.Toast(c, t.Text, kind, t.Sticky)
		}
		place := s.Stores.Nav.Current()
		chatOpen := place != nil && place.View == state.ViewChat
		// Below 900 px the rail and both sidebars are overlays and the
		// bottom bar is hidden (app-shell.tsx).
		narrow := kit.Narrow(c)
		kit.Row(c).Grow(1).MinHeight(0).AlignItems(kit.Stretch).Children(func() {
			if !narrow {
				s.rail(c, place, chatOpen && s.Stores.ListHidden())
			}
			if !narrow && chatOpen && !s.Stores.ListHidden() {
				s.chatList(c, place.Project, place.Chat)
			}
			kit.Main(c, func() {
				if narrow && !chatOpen {
					s.pageHeader(c, PageTitle(place))
				}
				s.main(c, place)
			})
			if !narrow && chatOpen {
				s.rightSidebar(c, place.Project)
			}
		})
		if narrow {
			s.overlays(c, place, chatOpen)
		} else {
			s.bottomBar(c, place)
		}
		s.Projects.Dialogs(c)
		kit.Toasts(c)
	})
}

func (s *Shell) main(c *kit.Context, place *state.Place) {
	st := s.Stores
	switch {
	case !st.Projects.Loaded:
		center(c, func() { kit.Spinner(c, "Loading the projects") })
	case place == nil:
		s.Projects.Welcome(c)
	case place.View == state.ViewChat:
		s.chatArea(c, place.Project, place.Chat)
	case devView(place.View) && s.Dev != nil:
		s.Dev.View(c, place)
	case place.View == state.ViewProjectSettings:
		s.Projects.Settings(c, place.Project, place.Section)
	case place.View == state.ViewSettings && s.Settings != nil:
		s.Settings.View(c, place)
	default:
		// Settings and the developer tools are not part of the spike.
		center(c, func() {
			kit.Empty(c, kit.IconSettings, "Settings", "The settings screens are not built on MyGo yet (SPIKE-048 Part B).", func() {
				if kit.Button(c, "Back to the chats", kit.Outline).Clicked() {
					st.BackToChats()
				}
			})
		})
	}
}

func center(c *kit.Context, fn func()) {
	kit.Column(c).Grow(1).AlignItems(kit.Center).Justify(kit.Center).Padding(24).Children(fn)
}

// item is the chat's row in its project's list, or nil.
func (s *Shell) item(p id.Project, ch id.Chat) *app.ChatItem {
	for i, it := range s.Stores.Chats.ByProject[p] {
		if it.Chat.ID == ch {
			return &s.Stores.Chats.ByProject[p][i]
		}
	}
	return nil
}

// chatView is the main area with one chat (chat-view.tsx): its header, a
// read-only alert, and the chat screen.
func (s *Shell) chatView(c *kit.Context, p id.Project, ch id.Chat) {
	st := s.Stores
	t := st.Threads.Threads[ch]
	it := s.item(p, ch)
	var info chat.Chat
	switch {
	case it != nil:
		info = it.Chat
	case t != nil:
		info = t.Chat
	}
	mother := info.Kind == chat.KindMother
	title := info.Title
	if title == "" {
		title = "New chat"
	}
	sub := ""
	if it != nil {
		sub = s.subline(c, *it)
	}
	s.header(c, p, ch, title, sub, mother, info.Title)
	damage := st.Projects.Opened[p].Damage
	if damage != nil {
		kit.Box(c).Padding(12, 16, 0, 16).Children(func() {
			kit.Alert(c, kit.Amber, "This project is open read-only", damage.File+" failed its check: "+damage.Problem)
		})
	}
	switch {
	case t != nil && s.Chat != nil:
		s.Chat.View(c, t, mother, damage != nil)
	case t != nil:
		center(c, func() { kit.Empty(c, kit.IconChat, title, "", nil) })
	case st.Threads.Failed[ch] != "":
		center(c, func() {
			kit.Alert(c, kit.Red, "Can't read this chat", st.Threads.Failed[ch]).MaxWidth(420)
			if kit.Button(c, "Try again", kit.Outline).Clicked() {
				st.OpenChat(p, ch)
			}
		})
	default:
		center(c, func() { kit.Spinner(c, "Opening the chat") })
	}
}

// header is the chat's title bar (ChatHeader): the button hiding the chat
// list, Mother's diamond, the title and its second line, and the chat's
// ⋯ menu at the end (chat-menu.tsx). raw is the chat's title as stored.
func (s *Shell) header(c *kit.Context, p id.Project, ch id.Chat, title, sub string, mother bool, raw string) {
	hidden := s.Stores.ListHidden()
	narrow := kit.Narrow(c)
	kit.Bar(c, func() {
		label, icon := "Hide the chat list", kit.IconSidebarClose
		if hidden {
			label, icon = "Show the chat list", kit.IconSidebarOpen
		}
		switch {
		case narrow:
			s.openSidebar(c)
		case kit.Tooltip(kit.IconButton(c, icon, label, kit.Ghost).Size(32, 32), label).Clicked():
			s.Stores.SetListHidden(!hidden)
		}
		if mother {
			kit.Icon(c, kit.IconDiamond).Size(16, 16).Shrink(0)
		}
		kit.Column(c).Shrink(1).MinWidth(0).Children(func() {
			kit.Title(c, title)
			if sub != "" {
				kit.Muted(c, sub)
			}
		})
		kit.Spacer(c)
		kit.MoreButton(c, "Chat menu", func(m *kit.Menu) { s.Projects.ChatMenu(m, p, ch, raw) }).Size(32, 32)
		if narrow && kit.SmallButton(c, kit.IconPages, "Pages", kit.Ghost).Clicked() {
			s.Stores.Shell.RightOverlay = true
		}
	})
}

func (s *Shell) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Subline is a chat row's second line (chat-sidebar.tsx): what the chat is
// doing, else its role, else when it was last active, at a time today and
// as a date before, which short and date write in the user's formats
// (kit.ShortTime and kit.MediumDate).
func Subline(it app.ChatItem, now time.Time, short, date func(time.Time) string) string {
	switch {
	case it.State == chat.StateWaiting:
		return "Waiting for you"
	case it.State == chat.StateWorking:
		return "Working…"
	case it.Chat.Kind == chat.KindMother:
		return "Project home"
	}
	role := it.Chat.Role
	for i, r := range role {
		if r == '\n' {
			role = role[:i]
			break
		}
	}
	if role != "" {
		return role
	}
	if it.LastActivity == nil {
		return "No messages yet"
	}
	t := it.LastActivity.In(now.Location())
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "Last active " + short(t)
	}
	return "Last active " + date(t)
}

// StatusLabel names a chat with what it is doing, for its tile and row.
func StatusLabel(title string, st chat.State) string {
	switch st {
	case chat.StateWaiting:
		return title + " · waiting for you"
	case chat.StateWorking:
		return title + " · working"
	}
	return title
}

// subline is Subline with the times in the user's formats.
func (s *Shell) subline(c *kit.Context, it app.ChatItem) string {
	short := func(t time.Time) string { return kit.ShortTime(c, t) }
	date := func(t time.Time) string { return kit.MediumDate(c, t) }
	return Subline(it, s.now(), short, date)
}
