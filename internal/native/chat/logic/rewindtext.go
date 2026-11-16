package logic

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// TurnsText is "turn 4" or "turns 4–6", as agent.turnsText.
func TurnsText(first, last int) string {
	if first == last {
		return fmt.Sprintf("turn %d", first)
	}
	return fmt.Sprintf("turns %d–%d", first, last)
}

// TurnStarts maps the ID of each user message that starts a turn to its
// turn: the turns *Rewind to here* and *Edit and resend* work on
// (TASK-018). A message sent during a turn joins it, so it starts none.
func TurnStarts(messages []chat.Message) map[id.Message]int {
	out := map[id.Message]int{}
	turn := -1
	for _, m := range messages {
		if m.Turn == turn {
			continue
		}
		turn = m.Turn
		if m.Role == chat.RoleUser && slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.Kind != chat.PartNotice }) {
			out[m.ID] = m.Turn
		}
	}
	return out
}

// Mark is a rewind's mark in the chat, after turn At.
type Mark struct {
	Rewind store.RewindInfo
	At     int
}

// Marks places each rewind after the turn it took the chat back to. A
// later rewind to an earlier turn took that place out too, so the mark
// moves up to the later rewind's: the turns it names were gone before
// the chat went on from there.
func Marks(rewinds []store.RewindInfo) []Mark {
	byID := slices.Clone(rewinds)
	slices.SortStableFunc(byID, func(a, b store.RewindInfo) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	out := make([]Mark, len(byID))
	at := math.MaxInt
	for i := len(byID) - 1; i >= 0; i-- {
		at = min(at, byID[i].Turn)
		out[i] = Mark{Rewind: byID[i], At: at}
	}
	return out
}

// MarkText is a mark's line, such as "Turns 4–6 taken out".
func MarkText(r store.RewindInfo) string {
	return upperFirst(TurnsText(r.Turn+1, r.LastTurn)) + " taken out"
}

// RewindLines say what a rewind would do, for its dialog: the turns that
// leave, the changes that stay, and the notes and skills that go back.
// edit is *Edit and resend*, whose message goes back in the box.
func RewindLines(st agent.RewindState, edit bool) []string {
	turns := upperFirst(TurnsText(st.Turn+1, st.LastTurn))
	one := st.Turn+1 == st.LastTurn
	leave, they := "leave", "they"
	if one {
		leave, they = "leaves", "it"
	}
	first := fmt.Sprintf("%s %s this chat and what the model knows of it.", turns, leave)
	if edit {
		first += " Your message goes back in the box, to edit and send again."
	}
	lines := []string{first}
	if st.Changes.Changes > 0 {
		what := st.Changes.Display
		if what == "" {
			what = st.Changes.Summary
		}
		if what == "" {
			what = "their changes"
		}
		lines = append(lines, fmt.Sprintf("What %s changed in the project stays: %s. Undo them from the rewind's mark.", they, what))
	}
	if st.Notes {
		lines = append(lines, "The session notes go back to how they were.")
	}
	if len(st.Unload) > 0 {
		lines = append(lines, fmt.Sprintf("Skills unloaded: %s.", strings.Join(st.Unload, ", ")))
	}
	if len(st.Load) > 0 {
		lines = append(lines, fmt.Sprintf("Skills loaded again: %s.", strings.Join(st.Load, ", ")))
	}
	return append(lines, "Undo rewind brings the turns back, until the chat goes on.")
}
