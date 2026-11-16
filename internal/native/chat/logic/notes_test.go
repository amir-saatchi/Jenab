package logic

// Ported from frontend/src/chat/notes.test.ts.

import (
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func TestNotesLine(t *testing.T) {
	now := time.Date(2026, 10, 27, 15, 0, 0, 0, time.Local)
	n := func(revision int, at time.Time) chat.SessionNote {
		return chat.SessionNote{Chat: "c", Revision: revision, UpdatedAt: at.UTC()}
	}
	if got := NotesLine(n(0, now), now); got != "What this chat keeps on its current work." {
		t.Errorf("revision 0: %q", got)
	}
	today := NotesLine(n(3, time.Date(2026, 10, 27, 9, 5, 0, 0, time.Local)), now)
	if !strings.HasPrefix(today, "Revision 3 · updated ") || strings.Contains(today, "2026") {
		t.Errorf("today: %q", today)
	}
	if today != "Revision 3 · updated 9:05 AM" {
		t.Errorf("today: %q", today)
	}
	before := NotesLine(n(4, time.Date(2026, 10, 20, 9, 5, 0, 0, time.Local)), now)
	if !strings.Contains(before, "2026") || before != "Revision 4 · updated Oct 20, 2026" {
		t.Errorf("before: %q", before)
	}
}
