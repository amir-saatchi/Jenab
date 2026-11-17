package logic

import "testing"

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
