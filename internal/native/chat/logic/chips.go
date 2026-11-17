package logic

import (
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// The toasts of the chips' undo (parts.tsx, refresh.tsx): what happened
// when an undo or redo did nothing. An undo that worked says nothing; the
// chip shows it.

// MemoryUndoText is the toast after a memory chip's undo or redo: changed
// when the section changed since, done when it worked, undone what the
// edit is now. It is "" when there is nothing to say.
func MemoryUndoText(changed, done, undone bool) string {
	switch {
	case changed:
		return "The section changed since, so it was left as it is. Edit it in the settings."
	case done:
		return ""
	case undone:
		return "This edit was already undone."
	}
	return "This edit is already back."
}

// DesktopUndoText is the toast after a desktop chip's undo or redo.
func DesktopUndoText(changed, done, undone bool) string {
	switch {
	case changed:
		return "It changed since, so it was left as it is. Edit it in Settings › Desktop."
	case done:
		return ""
	case undone:
		return "This save was already undone."
	}
	return "This save is already back."
}

// BatchUndoText is the toast when a Refresh memory batch's undo or redo
// found it already as asked; UndoChangedText is the one when a section
// changed since.
func BatchUndoText(undone bool) string {
	if undone {
		return "These edits were already undone."
	}
	return "These edits are already back."
}

// LastUndo is the newest message noting an Undo turn or Redo turn, "" for
// none. A chip reads its state again when it changes, since the undo of a
// turn reverts or puts back the chips' changes with the rest.
func LastUndo(messages []chat.Message) id.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		for _, p := range messages[i].Parts {
			if p.Kind == chat.PartNotice && p.Notice != nil && p.Notice.Kind == chat.NoticeTurnUndone {
				return messages[i].ID
			}
		}
	}
	return ""
}
