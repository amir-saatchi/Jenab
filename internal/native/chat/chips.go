package chat

import (
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat/logic"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The chips of what the agent changed without asking, each with its undo
// (parts.tsx, refresh.tsx): a memory edit (SPEC 3.3), a desktop macro or
// recipe saved (8.11), and a Refresh memory batch (3.5). The backend says
// whether the change is undone, so a chip is right after a reload or an
// undo of its turn.

// chip is a chip's undo between frames.
type chip struct {
	asked  bool // whether it is undone was asked
	undone bool
	busy   bool
}

// chipOf is the chip key of chat ch, kept by chat so a deleted chat's go.
func (s *Screen) chipOf(ch id.Chat, key string) *chip {
	m := s.chips[ch]
	if m == nil {
		m = map[string]*chip{}
		s.chips[ch] = m
	}
	k := m[key]
	if k == nil {
		k = &chip{}
		m[key] = k
	}
	return k
}

// info shows a toast that informs.
func (s *Screen) info(text string) {
	if text == "" {
		return
	}
	s.st.Toasts = append(s.st.Toasts, state.Toast{Text: text})
}

// chipText is a chip's text, with "· undone" after it while it is.
func chipText(text string, undone bool) string {
	if undone {
		return text + " · undone"
	}
	return text
}

// undoLabel is the chip's button: Undo, or Redo once undone.
func undoLabel(undone bool) string {
	if undone {
		return "Redo"
	}
	return "Undo"
}

// chipButton is a chip's small ghost button, off while busy; it reports a
// click.
func chipButton(c *kit.Context, label string, busy bool) bool {
	return kit.Disable(kit.SmallButton(c, nil, label, kit.Ghost), busy).Clicked() && !busy
}

// memoryNote is a memory chip's line under it, such as that the edit came
// from a chat that read web pages (SPEC 3.3, 3.5). A chip without one, as
// those saved before chips had it, shows none.
func memoryNote(c *kit.Context, note string) {
	if note == "" {
		return
	}
	kit.Row(c).Gap(6).AlignItems(kit.Center).Children(func() {
		kit.Icon(c, kit.IconInfo).Size(14, 14).Shrink(0).TextColor(kit.TokensOf(c).MutedForeground)
		kit.Muted(c, note).Shrink(1)
	})
}

// memoryChip is "Memory updated: preferences [Undo] [Edit]" with its note
// under it (MemoryChip). Undo reverts that edit, and the chats see it as
// the user's; Redo puts it back. Edit opens the section in the settings.
func (s *Screen) memoryChip(c *kit.Context, t *state.Thread, key string, n *jc.Notice) {
	p, ch := t.Project, t.Chat.ID
	k := s.chipOf(ch, key)
	req := app.MemoryUndoRequest{Target: n.Target, Change: n.Change}
	undoable := n.Target != "" && n.Change != ""
	if undoable && !k.asked {
		k.asked = true
		s.st.Actions.MemoryUndone(p, req, func(u bool, err error) {
			if err == nil { // the chip keeps Undo; a click says what is wrong
				k.undone = u
			}
		})
	}
	kit.Column(c).Gap(4).AlignItems(kit.Stretch).Children(func() {
		kit.Row(c).Gap(6).Wrap().AlignItems(kit.Center).Children(func() {
			kit.Record(c, kit.IconBrain, kit.Neutral, func() { kit.Text(c, chipText(n.Text, k.undone)) })
			if undoable && chipButton(c, undoLabel(k.undone), k.busy) {
				k.busy = true
				r := req
				r.Redo = k.undone
				s.st.Actions.UndoMemory(p, r, func(res app.MemoryUndo, err error) {
					k.busy = false
					if err != nil {
						s.st.Error(err)
						return
					}
					k.undone = res.Undone
					s.info(logic.MemoryUndoText(res.Changed, res.Done, res.Undone))
				})
			}
			if n.Target != "" && chipButton(c, "Edit", false) {
				s.st.OpenMemory(p, n.Target)
			}
		})
		memoryNote(c, n.Note)
	})
}

// desktopChip is "Macro saved: site_search (6 steps) [View steps] [Undo]
// [Edit]" (DesktopSavedChip). Undo removes the save, or puts back the
// version before it; Redo puts the save back. Edit opens Settings ›
// Desktop.
func (s *Screen) desktopChip(c *kit.Context, t *state.Thread, key string, n *jc.Notice) {
	k := s.chipOf(t.Chat.ID, key)
	req := app.DesktopUndoRequest{Change: n.Change}
	if n.Change != "" && !k.asked {
		k.asked = true
		s.st.Actions.DesktopSaveUndone(req, func(u bool, err error) {
			if err == nil {
				k.undone = u
			}
		})
	}
	kit.Column(c).Gap(4).AlignItems(kit.Stretch).Children(func() {
		kit.Row(c).Gap(6).Wrap().AlignItems(kit.Center).Children(func() {
			kit.Record(c, kit.IconAppWindow, kit.Neutral, func() { kit.Text(c, chipText(n.Text, k.undone)) })
			if n.Change != "" && chipButton(c, undoLabel(k.undone), k.busy) {
				k.busy = true
				r := req
				r.Redo = k.undone
				s.st.Actions.UndoDesktopSave(r, func(res store.DesktopUndo, err error) {
					k.busy = false
					if err != nil {
						s.st.Error(err)
						return
					}
					k.undone = res.Undone
					s.info(logic.DesktopUndoText(res.Changed, res.Done, res.Undone))
				})
			}
			if chipButton(c, "Edit", false) {
				s.st.OpenDesktop()
			}
		})
		if n.Details != "" {
			label := "View text"
			if strings.HasPrefix(n.Target, "desktop:macro:") {
				label = "View steps"
			}
			kit.Disclosure(c, label, s.flag(key), false, func() { kit.Text(c, label) }, func() {
				kit.Panel(c, func() { kit.Pre(c, n.Details) })
			})
		}
	})
}

// refreshChip is a Refresh memory batch: "Memory refreshed: 3 added, 1
// changed [Undo]", its note, and View changes with each section before
// and after (RefreshChip). Undo reverts the batch's edits together, or
// nothing when a section changed since; Redo puts them back.
func (s *Screen) refreshChip(c *kit.Context, t *state.Thread, key string, n *jc.Notice) {
	p := t.Project
	k := s.chipOf(t.Chat.ID, key)
	if !k.asked {
		k.asked = true
		s.st.Actions.RefreshUndone(p, n.Edits, func(u bool, err error) {
			if err == nil {
				k.undone = u
			}
		})
	}
	kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
		kit.Row(c).Gap(6).Wrap().AlignItems(kit.Center).Children(func() {
			kit.Record(c, kit.IconBrain, kit.Neutral, func() { kit.Text(c, chipText(n.Text, k.undone)) })
			if chipButton(c, undoLabel(k.undone), k.busy) {
				k.busy = true
				s.st.Actions.UndoRefresh(p, app.RefreshUndoRequest{Edits: n.Edits, Redo: k.undone}, func(res app.RefreshUndo, err error) {
					k.busy = false
					if err != nil {
						s.st.Error(err)
						return
					}
					k.undone = res.Undone
					if res.Changed {
						s.info(logic.UndoChangedText(res.Sections))
					} else if !res.Done {
						s.info(logic.BatchUndoText(res.Undone))
					}
				})
			}
		})
		memoryNote(c, n.Note)
		kit.Disclosure(c, "View changes", s.flag(key+":changes"), false, func() { kit.Text(c, "View changes") }, func() {
			kit.Panel(c, func() {
				kit.Column(c).Gap(16).AlignItems(kit.Stretch).Children(func() {
					for _, e := range n.Edits {
						kit.Column(c).Key(e.Target).AlignItems(kit.Stretch).Children(func() {
							s.memoryEdit(c, key+e.Target, logic.MemoryTitle(e.Target), e.Before, e.After)
						})
					}
				})
			})
		})
	})
}
