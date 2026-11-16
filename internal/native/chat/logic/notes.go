package logic

import (
	"fmt"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// NotesLine is the session notes sheet's description (SPEC 3.4): the
// revision and when the notes last changed, at a time today and as a date
// before, in now's zone.
func NotesLine(n chat.SessionNote, now time.Time) string {
	if n.Revision == 0 {
		return "What this chat keeps on its current work."
	}
	t := n.UpdatedAt.In(now.Location())
	ty, tm, td := t.Date()
	ny, nm, nd := now.Date()
	when := t.Format("Jan 2, 2006")
	if ty == ny && tm == nm && td == nd {
		when = t.Format("3:04 PM")
	}
	return fmt.Sprintf("Revision %d · updated %s", n.Revision, when)
}
