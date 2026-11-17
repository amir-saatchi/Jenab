package chat_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// toolTurn is turn 1: a question, a tool call and its result, and the
// answer.
func toolTurn() []jc.Message {
	args, _ := json.Marshal(map[string]string{"table": "prices"})
	return []jc.Message{
		statetest.Text(1, jc.RoleUser, "Save the prices"),
		answer(1, jc.Part{Kind: jc.PartToolCall, ToolCall: &jc.ToolCall{ID: "call1", Name: "insert_rows", Args: args}}),
		{Turn: 1, Role: jc.RoleTool, Parts: []jc.Part{{Kind: jc.PartToolResult, ToolResult: &jc.ToolResult{CallID: "call1", Text: "12 rows"}}}},
		answer(1, text("Saved "+persian)),
	}
}

// Undo turn asks first, with what it reverts, then reverts it; the
// button is then Redo turn, which puts it back (SPEC 2.6).
func TestUndoTurnRevertsAndPutsBack(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		f := statetest.New()
		f.TurnUndo = agent.TurnUndoState{Turn: 1, Undo: agent.TurnPlan{Changes: 13, Display: "12 rows in prices, 1 file"}}
		x := openOn(t, dark, f, toolTurn()...)
		shows(t, x.tt, "Undo turn")
		click(t, x, "Undo turn")
		x.until(t, "the dialog", func() bool { return kit.Shows(x.tt, "Undo turn 1?") })
		shows(t, x.tt, "This reverts 12 rows in prices, 1 file.")
		shot(t, x, "undo-turn")
		// Cancel changes nothing.
		click(t, x, "Cancel")
		if len(x.f.UndoTurns()) != 0 || kit.Shows(x.tt, "Undo turn 1?") {
			t.Fatalf("Cancel undid %v", x.f.UndoTurns())
		}
		click(t, x, "Undo turn")
		x.until(t, "the dialog", func() bool { return kit.Shows(x.tt, "Undo turn 1?") })
		clickLast(t, x.tt, "Undo turn") // the dialog's
		x.until(t, "the undo", func() bool { return len(x.f.UndoTurns()) == 1 })
		x.until(t, "Redo turn", func() bool { return kit.Shows(x.tt, "Redo turn") })
		if r := x.f.UndoTurns()[0]; r.Turn != 1 || r.Redo {
			t.Errorf("undo %+v", r)
		}
		click(t, x, "Redo turn")
		x.until(t, "the dialog", func() bool { return kit.Shows(x.tt, "Redo turn 1?") })
		shows(t, x.tt, "This puts back 12 rows in prices, 1 file.")
		clickLast(t, x.tt, "Redo turn")
		x.until(t, "the redo", func() bool { return len(x.f.UndoTurns()) == 2 })
		x.until(t, "Undo turn", func() bool { return kit.Shows(x.tt, "Undo turn") })
		if r := x.f.UndoTurns()[1]; !r.Redo {
			t.Errorf("redo %+v", r)
		}
		if got := toasts(x); len(got) != 0 {
			t.Errorf("toasts %q", got)
		}
	})
}

// A turn that changed nothing says so rather than asking.
func TestUndoTurnOfNothingSaysSo(t *testing.T) {
	x := open(t, false, toolTurn()...)
	click(t, x, "Undo turn")
	x.until(t, "the toast", func() bool { return len(x.st.Toasts) > 0 })
	if got := toasts(x); len(got) != 1 || got[0] != "This turn changed nothing that can be undone." {
		t.Errorf("toasts %q", got)
	}
}

// A message sent while the turn works is sent at once, and shows below
// the answer as joining the turn at its next step.
func TestAMessageSentDuringATurnJoinsIt(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, "Write a long report"))
		x.status(jc.Status{State: jc.StateWorking})
		x.draw()
		focus(t, x, "Message")
		x.tt.Type("Add a table too, " + persian)
		x.tt.Key(0, kit.KeyEnter)
		x.until(t, "the send", func() bool { return len(x.f.Sent()) == 1 })
		if s := x.f.Sent()[0]; s.Text != "Add a table too, "+persian || s.Chat != x.ch {
			t.Errorf("sent %+v", s)
		}
		if a, _ := x.tt.Access("Message"); a.Value != "" {
			t.Errorf("the composer kept %q", a.Value)
		}
		m := statetest.Text(1, jc.RoleUser, "Add a table too, "+persian)
		m.ID = x.f.Sent()[0].ID
		x.f.AddMessages(x.ch, m)
		x.f.SetLive(x.ch, agent.Live{Running: true, Turn: 1})
		x.st.Threads.Reload(x.ch)
		x.until(t, "the joining message", func() bool { return kit.Shows(x.tt, "Joins at the next step") })
		shows(t, x.tt, "Add a table too, "+persian, "Stop")
		shot(t, x, "joining")
	})
}

// Deny with a note sends the note with the answer, and the card's record
// shows it once answered.
func TestDenyWithANoteSendsIt(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		card := answer(1, jc.Part{Kind: jc.PartApproval, Approval: &jc.Approval{ID: "ap1", Kind: "host", Target: "example.com",
			Ask: "Fetch pages from example.com?", Risk: "low", Options: []jc.ApprovalOption{
				{Label: "Allow once", Grant: jc.Grant("once")}, {Label: "Deny", Grant: jc.GrantDeny}}}})
		card.ID = "card1"
		x := open(t, dark, statetest.Text(1, jc.RoleUser, "Read example.com"), card)
		x.status(jc.Status{State: jc.StateWaiting, Waiting: &jc.Waiting{Message: "card1", Index: 0, Kind: jc.PartApproval, Text: "Fetch pages?"}})
		x.draw()
		focus(t, x, "Note for the agent")
		x.tt.Type("  Use the cached copy, " + persian + "  ")
		x.draw()
		click(t, x, "Deny")
		x.until(t, "the answer", func() bool { return len(x.f.Answers()) == 1 })
		if a := x.f.Answers()[0]; a.Message != "card1" || a.Grant != jc.GrantDeny || a.Note != "Use the cached copy, "+persian {
			t.Errorf("answered %+v", a)
		}
	})
	// Answered, the card is a record with the note.
	card := answer(1, jc.Part{Kind: jc.PartApproval, Approval: &jc.Approval{ID: "ap1", Kind: "host", Target: "example.com",
		Ask: "Fetch pages from example.com?", Answer: jc.GrantDeny, Note: persian,
		Options: []jc.ApprovalOption{{Label: "Deny", Grant: jc.GrantDeny}}}})
	x := open(t, false, statetest.Text(1, jc.RoleUser, "Read example.com"), card)
	if !strings.Contains(strings.Join(x.tt.Texts(), "|"), "· “"+persian+"”") || x.tt.HasText("Allow once") {
		t.Errorf("the record is %q", x.tt.Texts())
	}
}

// After Stop, a card the turn waited on is a record: stopped without an
// answer, with no buttons left to approve with.
func TestAnApprovalAfterStopIsARecord(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		card := answer(1, jc.Part{Kind: jc.PartApproval, Approval: &jc.Approval{ID: "ap1", Kind: "host", Target: "example.com",
			Ask: "Fetch pages from example.com?", Options: []jc.ApprovalOption{
				{Label: "Allow once", Grant: jc.Grant("once")}, {Label: "Deny", Grant: jc.GrantDeny}}}})
		card.ID = "card1"
		x := open(t, dark, statetest.Text(1, jc.RoleUser, "Read example.com"), card)
		x.status(jc.Status{State: jc.StateWaiting, Waiting: &jc.Waiting{Message: "card1", Index: 0, Kind: jc.PartApproval, Text: "Fetch pages?"}})
		x.draw()
		shows(t, x.tt, "Allow once", "Stop")
		click(t, x, "Stop")
		x.until(t, "Stop", func() bool { return x.f.Stops() == 1 })
		x.status(jc.Status{State: jc.StateIdle})
		x.draw()
		shows(t, x.tt, "Fetch pages from example.com? ·", "Stopped without an answer")
		if x.tt.HasText("Allow once") || x.tt.HasText("Deny") {
			t.Errorf("the card still answers: %q", x.tt.Texts())
		}
		if len(x.f.Answers()) != 0 {
			t.Errorf("answered %+v", x.f.Answers())
		}
	})
}
