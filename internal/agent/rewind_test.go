package agent

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// rewindHarness has a chat of three turns: turn 2 loads the sql skill,
// writes session notes and adds two rows to prices, the way its tools
// would.
func rewindHarness(t *testing.T, root string, pid id.Project) (*harness, chat.Chat) {
	t.Helper()
	h := newHarness(t, root, pid, testSettings(), loadSkill(), tool.UpdateSessionNotes())
	h.o.d.Skills = skillSet(t, sqlSkill, viewsSkill)
	c := h.newChat(chat.Chat{Title: "Fruit"})
	h.fp.Push(fake.Text("answer apple"))
	h.turn(c.ID, "first apple")
	h.fp.Push(fake.ToolCall("c1", "load_skill", map[string]string{"name": "sql"}),
		fake.ToolCall("c2", "update_session_notes", map[string]any{"content": "plan banana", "expected_revision": 0}),
		fake.Text("answer banana"))
	m := h.send(c.ID, "second banana")
	h.wait()
	ctx := context.Background()
	h.open(func(p *project.Project) {
		if _, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			_, err := tx.Exec("CREATE TABLE prices (id INTEGER PRIMARY KEY, price REAL)")
			return 0, err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.DB.Insert(ctx, limit.Interactive, id.SourceOf("message", string(m)), "prices",
			[]store.Row{{"id": 1, "price": 10}, {"id": 2, "price": 20}}, store.ConflictFail); err != nil {
			t.Fatal(err)
		}
	})
	h.fp.Push(fake.Text("answer cherry"))
	h.turn(c.ID, "third cherry")
	if got := h.chat(c.ID).Skills; !slices.Equal(got, []string{"sql"}) {
		t.Fatalf("skills before the rewind: %q", got)
	}
	return h, c
}

func (h *harness) rows() int {
	h.t.Helper()
	var n int
	h.open(func(p *project.Project) {
		ns, err := store.Query(context.Background(), p.DB.DB, "SELECT count(*) FROM prices", nil, func(r *sql.Rows) (int, error) {
			var n int
			return n, r.Scan(&n)
		})
		if err != nil {
			h.t.Fatal(err)
		}
		n = ns[0]
	})
	return n
}

func (h *harness) hits(c id.Chat, q string) int {
	h.t.Helper()
	var n int
	h.open(func(p *project.Project) {
		res, err := p.Chats.Search(context.Background(), store.SearchReq{Query: q, Chat: c})
		if err != nil {
			h.t.Fatal(err)
		}
		n = len(res.Hits)
	})
	return n
}

// lastRequest is the system blocks and messages of the last request the
// model got, as JSON.
func (h *harness) lastRequest() string {
	h.t.Helper()
	calls := h.fp.Calls()
	r := calls[len(calls)-1]
	return jsonOf(h.t, []any{r.System, r.Messages})
}

// A rewound chat answers as if the later turns never happened: they leave
// the history index, the context, the notes and the skills, also after a
// restart. Their changes to the project stay; the model is told so, and
// Undo turn from the rewind's mark reverts them.
func TestRewind(t *testing.T) {
	root := t.TempDir()
	h, c := rewindHarness(t, root, "")
	ctx := context.Background()

	st, err := h.o.RewindState(ctx, h.pid, c.ID, 1)
	if err != nil || st.LastTurn != 3 || st.Changes.Summary != "2 rows in prices" || !st.Notes ||
		!slices.Equal(st.Unload, []string{"sql"}) || len(st.Load) != 0 {
		t.Fatalf("RewindState = %+v, %v", st, err)
	}
	r, err := h.o.Rewind(ctx, h.pid, c.ID, 1)
	if err != nil || r.Rewind == 0 || r.LastTurn != 3 || !r.Notes || !slices.Equal(r.Skills, []string{}) || r.Changes.Changes != 1 {
		t.Fatalf("Rewind = %+v, %v", r, err)
	}
	ms := h.messages(c.ID)
	want := []string{"user: first apple", "assistant: answer apple",
		"user: [the user took turns 2–3 out of this chat; what they changed in the project stays: 2 rows in prices]"}
	if got := texts(ms); !slices.Equal(got, want) {
		t.Errorf("after the rewind: %q", got)
	}
	if !slices.Equal(turnsOf(ms), []int{1}) {
		t.Errorf("turns %v", turnsOf(ms))
	}
	for q, n := range map[string]int{"apple": 2, "banana": 0, "cherry": 0} {
		if got := h.hits(c.ID, q); got != n {
			t.Errorf("history search for %q: %d hits, want %d", q, got, n)
		}
	}
	if got := h.chat(c.ID).Skills; len(got) != 0 {
		t.Errorf("skills after the rewind: %q", got)
	}
	if h.rows() != 2 {
		t.Error("the rewind changed the project's data")
	}

	// The next turn is turn 2, from a new cut, with nothing of the turns
	// that left.
	h.fp.Push(fake.Text("answer date"))
	h.turn(c.ID, "second date")
	req := h.lastRequest()
	for _, gone := range []string{"banana", "cherry", "plan banana", "Use named parameters"} {
		if strings.Contains(req, gone) {
			t.Errorf("the request after the rewind has %q:\n%s", gone, req)
		}
	}
	if !strings.Contains(req, "first apple") || !strings.Contains(req, "2 rows in prices") {
		t.Errorf("the request after the rewind lacks turn 1 or the notice:\n%s", req)
	}
	if ms := h.messages(c.ID); ms[len(ms)-1].Turn != 2 {
		t.Errorf("the new turn is turn %d, want 2", ms[len(ms)-1].Turn)
	}
	if err := h.o.UndoRewind(ctx, h.pid, c.ID, r.Rewind); !errors.Is(err, store.ErrRewindMoved) {
		t.Errorf("UndoRewind after a new turn: %v, want ErrRewindMoved", err)
	}

	// The changes of the turns that left can still be undone, by the
	// rewind.
	us, err := h.o.RewindUndoState(ctx, h.pid, c.ID, r.Rewind)
	if err != nil || us.Undo.Summary != "2 rows in prices" {
		t.Fatalf("RewindUndoState = %+v, %v", us, err)
	}
	u, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Rewind: r.Rewind})
	if err != nil || !u.Done || u.Rewind != r.Rewind {
		t.Fatalf("UndoTurn of the rewind = %+v, %v", u, err)
	}
	if h.rows() != 0 {
		t.Error("the rewind's changes were not undone")
	}
	got := texts(h.messages(c.ID))
	if last := got[len(got)-1]; last != "user: [the changes of turns 2–3, taken out of this chat, were undone: 2 rows in prices]" {
		t.Errorf("notice %q", last)
	}
	h.stop()

	// After a restart, the chat is still as rewound.
	h = newHarness(t, root, h.pid, testSettings(), loadSkill(), tool.UpdateSessionNotes())
	defer h.stop()
	h.o.d.Skills = skillSet(t, sqlSkill, viewsSkill)
	h.fp.Push(fake.Text("answer elderberry"))
	h.turn(c.ID, "third elderberry")
	req = h.lastRequest()
	for _, gone := range []string{"banana", "cherry", "Use named parameters"} {
		if strings.Contains(req, gone) {
			t.Errorf("the request after a restart has %q", gone)
		}
	}
	if !strings.Contains(req, "second date") {
		t.Errorf("the request after a restart lacks turn 2:\n%s", req)
	}
	if ms := h.messages(c.ID); ms[len(ms)-1].Turn != 3 {
		t.Errorf("the turn after a restart is turn %d, want 3", ms[len(ms)-1].Turn)
	}
}

// Undo rewind brings the turns back, with their words in the index, the
// notes and the skill, and drops the notice.
func TestUndoRewind(t *testing.T) {
	h, c := rewindHarness(t, t.TempDir(), "")
	defer h.stop()
	ctx := context.Background()
	var resets int // the desktop session's EndChat in the app
	h.o.d.ChatReset = func(id.Project, id.Chat) { resets++ }
	before := texts(h.messages(c.ID))
	r, err := h.o.Rewind(ctx, h.pid, c.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.o.UndoRewind(ctx, h.pid, c.ID, r.Rewind); err != nil {
		t.Fatal(err)
	}
	if resets != 2 {
		t.Errorf("ChatReset called %d times, want 2 (the rewind and its undo)", resets)
	}
	if got := texts(h.messages(c.ID)); !slices.Equal(got, before) {
		t.Errorf("after the undo:\n%q\nwant\n%q", got, before)
	}
	if h.hits(c.ID, "banana") != 2 || !slices.Equal(h.chat(c.ID).Skills, []string{"sql"}) {
		t.Error("the index or the skills didn't come back")
	}
	h.fp.Push(fake.Text("answer date"))
	h.turn(c.ID, "fourth date")
	req := h.lastRequest()
	if !strings.Contains(req, "third cherry") || !strings.Contains(req, "plan banana") || strings.Contains(req, "took turns") {
		t.Errorf("the request after the undo:\n%s", req)
	}
	if ms := h.messages(c.ID); ms[len(ms)-1].Turn != 4 {
		t.Errorf("the turn after the undo is turn %d, want 4", ms[len(ms)-1].Turn)
	}
	if err := h.o.UndoRewind(ctx, h.pid, c.ID, r.Rewind); !errors.Is(err, store.ErrRewindGone) {
		t.Errorf("a second undo: %v", err)
	}
}

// A rewind is only between turns, like Clear: work the turn started, such
// as a subagent or a tool call, runs inside it, and Stop ends it first.
// A rewind to the start drops a title still being made, as Clear does.
func TestRewindBetweenTurns(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
	defer h.stop()
	ctx := context.Background()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.Text("one"), fake.ToolCall("c1", "block", struct{}{}), fake.Text("two"))
	h.turn(c.ID, "one")
	h.send(c.ID, "wait")
	<-started
	if _, err := h.o.Rewind(ctx, h.pid, c.ID, 1); !errors.Is(err, ErrInTurn) {
		t.Errorf("Rewind during a turn: %v", err)
	}
	if _, err := h.o.RewindState(ctx, h.pid, c.ID, 1); err != nil {
		t.Errorf("RewindState during a turn: %v", err)
	}
	close(release)
	h.wait()
	if _, err := h.o.Rewind(ctx, h.pid, c.ID, 2); !errors.Is(err, store.ErrNothingToRewind) {
		t.Errorf("Rewind to the last turn: %v", err)
	}
	if _, err := h.o.Rewind(ctx, h.pid, c.ID, 1); err != nil {
		t.Fatal(err)
	}
	if st := h.o.State(h.pid, c.ID); st != chat.StateIdle {
		t.Errorf("state after the rewind: %v", st)
	}
}

func TestRewindToStartDropsTitle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{})
		slow := fake.Text("Prices")
		slow.Wait = time.Second
		h.fp.Push(fake.Text("answer"), slow)
		h.send(c.ID, "one")
		synctest.Wait() // the turn ends and the title streams
		if _, err := h.o.Rewind(context.Background(), h.pid, c.ID, 0); err != nil {
			t.Fatal(err)
		}
		h.wait()
		if ch := h.chat(c.ID); ch.Title != "" {
			t.Errorf("title %q for a chat rewound to its start", ch.Title)
		}
		if ms := h.messages(c.ID); len(ms) != 0 {
			t.Errorf("messages after a rewind to the start: %q", texts(ms))
		}
	})
}
