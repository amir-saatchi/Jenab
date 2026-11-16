package logic

import (
	"fmt"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// NotesLine is the session notes sheet's description (SPEC 3.4): the
// revision and when the notes last changed, at a time today and as a date
// before, in now's zone. short and date write them in the user's formats
// (kit.ShortTime and kit.MediumDate).
func NotesLine(n chat.SessionNote, now time.Time, short, date func(time.Time) string) string {
	if n.Revision == 0 {
		return "What this chat keeps on its current work."
	}
	t := n.UpdatedAt.In(now.Location())
	ty, tm, td := t.Date()
	ny, nm, nd := now.Date()
	when := date(t)
	if ty == ny && tm == nm && td == nd {
		when = short(t)
	}
	return fmt.Sprintf("Revision %d · updated %s", n.Revision, when)
}
