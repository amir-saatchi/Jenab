package logic

// Ported from the React app's chat/rewind-text.test.ts.

import (
	"reflect"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

var day = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

func rewindMsg(mid string, turn int, role chat.Role, notice bool) chat.Message {
	p := chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: mid}}
	if notice {
		p = chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeTurnUndone, Text: "n"}}
	}
	return chat.Message{ID: id.Message(mid), Chat: "c1", Turn: turn, Role: role, CreatedAt: day, Parts: []chat.Part{p}}
}

func rw(rid int64, turn, last int, undoable bool) store.RewindInfo {
	return store.RewindInfo{ID: rid, Turn: turn, LastTurn: last, At: day, Undoable: undoable}
}

func TestTurnsText(t *testing.T) {
	if got := TurnsText(4, 4); got != "turn 4" {
		t.Errorf("got %q", got)
	}
	if got := TurnsText(4, 6); got != "turns 4–6" {
		t.Errorf("got %q", got)
	}
}

// Only the user message that starts a turn gets the actions: not one
// that joined the turn, nor a notice.
func TestTurnStarts(t *testing.T) {
	ms := []chat.Message{
		rewindMsg("u1", 1, chat.RoleUser, false), rewindMsg("a1", 1, chat.RoleAssistant, false), rewindMsg("j1", 1, chat.RoleUser, false),
		rewindMsg("n2", 2, chat.RoleUser, true), rewindMsg("a2", 2, chat.RoleAssistant, false),
		rewindMsg("u3", 3, chat.RoleUser, false),
	}
	want := map[id.Message]int{"u1": 1, "u3": 3}
	if got := TurnStarts(ms); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A later rewind to an earlier turn moves the marks of the rewinds
// before it up to its own place.
func TestMarks(t *testing.T) {
	a, b, c := rw(1, 5, 6, false), rw(2, 2, 4, false), rw(3, 3, 3, true)
	var got [][2]int64
	for _, m := range Marks([]store.RewindInfo{c, a, b}) {
		got = append(got, [2]int64{m.Rewind.ID, int64(m.At)})
	}
	if want := [][2]int64{{1, 2}, {2, 2}, {3, 3}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := MarkText(a); got != "Turn 6 taken out" {
		t.Errorf("got %q", got)
	}
	if got := MarkText(b); got != "Turns 3–4 taken out" {
		t.Errorf("got %q", got)
	}
}

func TestRewindLines(t *testing.T) {
	st := agent.RewindState{Turn: 1, LastTurn: 3, Changes: agent.TurnPlan{Changes: 1, Display: "2 rows in prices"}, Notes: true, Unload: []string{"sql"}, Load: []string{}}
	want := []string{
		"Turns 2–3 leave this chat and what the model knows of it.",
		"What they changed in the project stays: 2 rows in prices. Undo them from the rewind's mark.",
		"The session notes go back to how they were.",
		"Skills unloaded: sql.",
		"Undo rewind brings the turns back, until the chat goes on.",
	}
	if got := RewindLines(st, false); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	one := agent.RewindState{Turn: 2, LastTurn: 3, Unload: nil, Load: []string{"views"}}
	want = []string{
		"Turn 3 leaves this chat and what the model knows of it. Your message goes back in the box, to edit and send again.",
		"Skills loaded again: views.",
		"Undo rewind brings the turns back, until the chat goes on.",
	}
	if got := RewindLines(one, true); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}
