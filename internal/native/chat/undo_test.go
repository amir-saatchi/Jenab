package chat_test

import (
	"sync/atomic"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// Undo turn's dialog opens with the newest state read (TASK-076): the
// answer to the click's own read, coming after a later read's, doesn't
// open it with what the turn could do before.
func TestUndoTurnOpensWithTheNewestState(t *testing.T) {
	f := statetest.New()
	plan := agent.TurnPlan{Changes: 13, Display: "12 rows in prices, 1 file"}
	f.TurnUndo = agent.TurnUndoState{Turn: 1, Undo: plan}
	var armed atomic.Bool
	started, release := make(chan struct{}), make(chan struct{})
	f.HoldUndoState = func(turn int) {
		if turn == 1 && armed.CompareAndSwap(true, false) {
			close(started)
			<-release
		}
	}
	x := openOn(t, false, f, toolTurn()...)
	shows(t, x.tt, "Undo turn")

	armed.Store(true)
	click(t, x, "Undo turn") // its read answers that turn 1 can be undone…
	<-started
	f.SetTurnUndo(agent.TurnUndoState{Turn: 1, Redo: plan}) // …but it was undone since
	f.AddMessages(x.ch, statetest.Text(2, jc.RoleUser, "Next"), answer(2, text("Done")))
	x.st.Threads.Reload(x.ch)
	x.until(t, "the newer read", func() bool { return kit.Shows(x.tt, "Redo turn") })
	close(release)
	x.until(t, "the dialog", func() bool { return kit.Shows(x.tt, "Redo turn 1?") || kit.Shows(x.tt, "Undo turn 1?") })
	if kit.Shows(x.tt, "Undo turn 1?") {
		t.Fatal("the dialog opened with the older state: Undo turn 1?")
	}
	shows(t, x.tt, "This puts back 12 rows in prices, 1 file.")
}
