package logic

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func TestTheChipsUndoTexts(t *testing.T) {
	for _, c := range []struct {
		got, want string
	}{
		{MemoryUndoText(false, true, true), ""},
		{MemoryUndoText(true, false, false), "The section changed since, so it was left as it is. Edit it in the settings."},
		{MemoryUndoText(false, false, true), "This edit was already undone."},
		{MemoryUndoText(false, false, false), "This edit is already back."},
		{DesktopUndoText(false, true, false), ""},
		{DesktopUndoText(true, false, true), "It changed since, so it was left as it is. Edit it in Settings › Desktop."},
		{DesktopUndoText(false, false, true), "This save was already undone."},
		{DesktopUndoText(false, false, false), "This save is already back."},
		{BatchUndoText(true), "These edits were already undone."},
		{BatchUndoText(false), "These edits are already back."},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestLastUndoIsTheNewestTurnUndoneNotice(t *testing.T) {
	undo := func(m id.Message) chat.Message {
		return chat.Message{ID: m, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice,
			Notice: &chat.Notice{Kind: chat.NoticeTurnUndone, Text: "Turn 1 undone"}}}}
	}
	other := chat.Message{ID: "m3", Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice,
		Notice: &chat.Notice{Kind: chat.NoticeTurnFailed, Text: "stopped"}}}}
	if got := LastUndo(nil); got != "" {
		t.Errorf("no messages: %q", got)
	}
	if got := LastUndo([]chat.Message{other}); got != "" {
		t.Errorf("no undo: %q", got)
	}
	if got := LastUndo([]chat.Message{undo("m1"), undo("m2"), other}); got != "m2" {
		t.Errorf("got %q, want m2", got)
	}
}
