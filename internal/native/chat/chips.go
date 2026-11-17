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

// chip is a chip's undo between frames. It offers Undo or Redo only once
// it knows which applies, and reads that again after each Undo turn or
// Redo turn in its chat, which may have reverted or put back its change.
type chip struct {
	read   bool       // whether it is undone was asked, after the undo below
	undo   id.Message // the chat's newest turn undo when it was asked
	known  bool       // the answer is in
	undone bool
	busy   bool
	asks   int // reads and undos sent, so a late answer to an older one is dropped
}

// ask reads whether the chip's change is undone, if it hasn't since the
// chat's newest turn undo, undo. A read that fails leaves the chip as it
// was and known, so a click says what is wrong.
func (k *chip) ask(undo id.Message, read func(then func(bool, error))) {
	if k.read && k.undo == undo {
		return
	}
	k.read, k.undo, k.known = true, undo, false
	k.asks++
	n := k.asks
	read(func(u bool, err error) {
		if n != k.asks {
			return
		}
		if err == nil {
			k.undone = u
		}
		k.known = true
	})
}

// sent marks an undo or redo sent; it returns the check that its answer
// is still the newest, since a turn undo meanwhile asks again.
func (k *chip) sent() func() bool {
	k.busy = true
	k.asks++
	n := k.asks
	return func() bool { return n == k.asks }
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

// undoButton is the chip's Undo or Redo, off until the chip knows which
// applies and while one runs.
func undoButton(c *kit.Context, k *chip) bool {
	return chipButton(c, undoLabel(k.undone), k.busy || !k.known)
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
	if undoable {
		k.ask(s.viewOf(t).undo, func(then func(bool, error)) { s.st.Actions.MemoryUndone(p, req, then) })
	}
	kit.Column(c).Gap(4).AlignItems(kit.Stretch).Children(func() {
		kit.Row(c).Gap(6).Wrap().AlignItems(kit.Center).Children(func() {
			kit.Record(c, kit.IconBrain, kit.Neutral, func() { kit.Text(c, chipText(n.Text, k.undone)) })
			if undoable && undoButton(c, k) {
				newest := k.sent()
				r := req
				r.Redo = k.undone
				s.st.Actions.UndoMemory(p, r, func(res app.MemoryUndo, err error) {
					k.busy = false
					if err != nil {
						s.st.Error(err)
						return
					}
					if newest() {
						k.undone = res.Undone
					}
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
	if n.Change != "" {
		k.ask(s.viewOf(t).undo, func(then func(bool, error)) { s.st.Actions.DesktopSaveUndone(req, then) })
	}
	kit.Column(c).Gap(4).AlignItems(kit.Stretch).Children(func() {
		kit.Row(c).Gap(6).Wrap().AlignItems(kit.Center).Children(func() {
			kit.Record(c, kit.IconAppWindow, kit.Neutral, func() { kit.Text(c, chipText(n.Text, k.undone)) })
			if n.Change != "" && undoButton(c, k) {
				newest := k.sent()
				r := req
				r.Redo = k.undone
				s.st.Actions.UndoDesktopSave(r, func(res store.DesktopUndo, err error) {
					k.busy = false
					if err != nil {
						s.st.Error(err)
						return
					}
					if newest() {
						k.undone = res.Undone
					}
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
	k.ask(s.viewOf(t).undo, func(then func(bool, error)) { s.st.Actions.RefreshUndone(p, n.Edits, then) })
	kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
		kit.Row(c).Gap(6).Wrap().AlignItems(kit.Center).Children(func() {
			kit.Record(c, kit.IconBrain, kit.Neutral, func() { kit.Text(c, chipText(n.Text, k.undone)) })
			if undoButton(c, k) {
				newest := k.sent()
				s.st.Actions.UndoRefresh(p, app.RefreshUndoRequest{Edits: n.Edits, Redo: k.undone}, func(res app.RefreshUndo, err error) {
					k.busy = false
					if err != nil {
						s.st.Error(err)
						return
					}
					if newest() {
						k.undone = res.Undone
					}
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
