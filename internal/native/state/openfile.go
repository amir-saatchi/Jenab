package state

import (
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
)

// Opening, deleting and linking files (SPEC 4.5, 5.12):
// frontend/src/shell/open-file.ts.

// OpenFile opens a file in the main area with the chat in the dock, and
// shows its folder in the Files panel.
func (s *Stores) OpenFile(p id.Project, c id.Chat, key string) {
	s.Shell.SetFolder(p, ParentOf(key))
	s.Shell.RightTab, s.Shell.RightOverlay = TabFiles, false
	s.Shell.Dock(ChatRef{p, c}, Docked{Kind: "file", Key: key})
}

// OpenHere opens a file of the open chat's project, such as a tool's
// stored output.
func (s *Stores) OpenHere(key string) {
	if pl := s.Nav.Current(); pl != nil && pl.View == ViewChat && key != "" {
		s.OpenFile(pl.Project, pl.Chat, key)
	}
}

// OpenLink opens a jenab:// link from the chat in the open chat. A link
// to another project, or to a file that is gone, says so and opens
// nothing. done, if not nil, runs on the UI thread once it is decided.
func (s *Stores) OpenLink(href string, done func()) {
	finish := func() {
		if done != nil {
			done()
		}
	}
	p, key, ok := ParseLink(href)
	place := s.Nav.Current()
	if !ok || place == nil || place.View != ViewChat {
		finish()
		return
	}
	if p != place.Project {
		s.Toasts = append(s.Toasts, Toast{Text: "This link is to a file in another project."})
		finish()
		return
	}
	if s.bucket == nil {
		finish()
		return
	}
	from := *place
	go func() {
		_, err := s.bucket.Head(background, p, key)
		s.post(func() {
			defer finish()
			switch {
			case isNotFound(err):
				s.Toasts = append(s.Toasts, Toast{Text: NameOf(key) + " no longer exists. It was deleted or moved."})
			case err != nil:
				s.Error(err)
			default:
				// The place may have changed while the file was looked up.
				if now := s.Nav.Current(); now != nil && SamePlace(&from, now) {
					s.OpenFile(p, from.Chat, key)
				}
			}
		})
	}()
}

// IsLink tells a link the app opens itself: jenab://, which OpenLink
// takes. Other links open in the browser.
func IsLink(href string) bool { return strings.HasPrefix(href, "jenab://") }

// DeleteFile deletes a file as the user. The toast's Undo puts it back
// while the change is in the undo window (SPEC 4.4).
func (s *Stores) DeleteFile(p id.Project, key string) {
	if s.bucket == nil {
		return
	}
	go func() {
		d, err := s.bucket.Delete(background, p, key)
		s.post(func() {
			if err != nil {
				s.Error(err)
				return
			}
			s.Shell.CloseFile(p, key)
			s.Shell.Changed(p)
			t := Toast{Text: "Deleted " + NameOf(key), Action: "Undo"}
			if strings.HasPrefix(key, Cache) {
				t.Detail = "From the app cache."
			}
			t.OnAction = func() { s.undoDelete(p, key, d.Change) }
			s.Toasts = append(s.Toasts, t)
		})
	}()
}

func (s *Stores) undoDelete(p id.Project, key string, change id.Change) {
	go func() {
		err := s.bucket.UndoDelete(background, p, change)
		s.post(func() {
			if err != nil {
				s.Error(err)
				return
			}
			s.Shell.Changed(p)
			s.Toasts = append(s.Toasts, Toast{Text: NameOf(key) + " is back"})
		})
	}()
}

// OpenWithSystem opens a file's copy with the system app (SPEC 4.7).
func (s *Stores) OpenWithSystem(p id.Project, key string) {
	if s.bucket == nil {
		return
	}
	go func() {
		if err := s.bucket.OpenWithSystem(background, p, key); err != nil {
			s.post(func() { s.Error(err) })
		}
	}()
}

// ListHidden tells whether the chat list beside the rail is hidden, its
// chats then in the rail. While something is docked in the open chat it
// hides, as for an open page, until the user shows it again (SPEC 5.12);
// otherwise it is the user's own choice.
func (s *Stores) ListHidden() bool {
	if s.docked() {
		return !s.Shell.ListShown
	}
	return s.Nav.SidebarHidden
}

// SetListHidden hides or shows the chat list.
func (s *Stores) SetListHidden(hidden bool) {
	if s.docked() {
		s.Shell.ListShown = !hidden
		return
	}
	s.Nav.SidebarHidden = hidden
}

func (s *Stores) docked() bool {
	pl := s.Nav.Current()
	if pl == nil || pl.View != ViewChat {
		return false
	}
	_, ok := s.Shell.Open[ChatRef{pl.Project, pl.Chat}]
	return ok
}
