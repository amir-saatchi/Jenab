package chat_test

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// Refresh memory in the header: its menu says what each refresh costs,
// a short one starts at once, and a long one asks first (SPEC 3.5).
func TestRefreshMemoryStartsAndAsksBeforeALongOne(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, persian), answer(1, text("Noted.")))
		x.f.SetEstimate(agent.RefreshEstimate{Turns: 12, Calls: 1, Tokens: 8000})
		// The keyboard opens the menu on its first item, with its cost.
		focus(t, x, "Refresh memory")
		x.tt.Key(0, kit.KeyEnter)
		x.until(t, "the estimate", func() bool { return kit.Shows(x.tt, "12 turns · 1 call · about 8,000 tokens") })
		shows(t, x.tt, "Full refresh: review the whole chat again")
		shot(t, x, "refresh-menu")
		x.tt.Key(0, kit.KeyEnter)
		x.until(t, "the refresh", func() bool { return len(x.f.Refreshed()) == 1 })
		if got := x.f.Refreshed(); got[0] {
			t.Error("Refresh memory started a full refresh")
		}

		// A long full refresh asks first; Cancel starts none.
		long := agent.RefreshEstimate{Turns: 30, Chunks: 6, Saved: 2, Calls: 5, Tokens: 24000, Long: true}
		x.f.SetEstimate(long)
		click(t, x, "Refresh memory")
		x.until(t, "the menu", func() bool { return kit.Shows(x.tt, "30 turns · 5 calls · about 24,000 tokens") })
		click(t, x, "Full refresh: review the whole chat again")
		x.until(t, "the dialog", func() bool { return kit.Shows(x.tt, "Review the whole chat again: 30 turns?") })
		shows(t, x.tt, "This makes 5 calls to No model, with about 24,000 tokens of input. 2 parts of 6 reviewed before are kept."+
			" Memory changes only when it ends, and Stop cancels it.")
		shot(t, x, "refresh-long")
		click(t, x, "Cancel")
		if kit.Shows(x.tt, "Review the whole chat again: 30 turns?") || len(x.f.Refreshed()) != 1 {
			t.Fatalf("Cancel left %q, refreshes %v", x.tt.Texts(), x.f.Refreshed())
		}
		// Asked again, Refresh starts it.
		click(t, x, "Refresh memory")
		click(t, x, "Full refresh: review the whole chat again")
		x.until(t, "the dialog", func() bool { return kit.Shows(x.tt, "Review the whole chat again: 30 turns?") })
		click(t, x, "Refresh")
		x.until(t, "the full refresh", func() bool { return len(x.f.Refreshed()) == 2 })
		if got := x.f.Refreshed(); !slices.Equal(got, []bool{false, true}) {
			t.Errorf("refreshes %v", got)
		}
	})
}

// While the chat is busy, as while a refresh runs, the button is off.
func TestRefreshMemoryIsOffWhileTheChatWorks(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, persian))
		x.status(jc.Status{State: jc.StateWorking, Doing: "Refreshing memory: 1 of 3"})
		x.draw()
		_ = x.tt.Click("Refresh memory")
		x.draw()
		if kit.Shows(x.tt, "Full refresh: review the whole chat again") {
			t.Error("the menu opened while the chat works")
		}
	})
}

// A refresh's chip: View changes shows each section before and after,
// Undo reverts the batch and Redo puts it back, and an undo that finds a
// section changed since says so.
func TestARefreshChipUndoesItsBatch(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		edits := []jc.MemoryDiff{
			{Target: "memory:project/goals", Before: "Ship the app", After: "Ship the app\n" + persian, Change: "5"},
			{Target: "memory:user/preferences", Before: "", After: "Short answers", Change: "6"},
		}
		// The batch's card is a user message of notices, which the model
		// never reads.
		refreshed := statetest.Text(2, jc.RoleUser, "")
		refreshed.Parts = []jc.Part{notice(jc.Notice{Kind: jc.NoticeMemoryRefreshed, Text: "Memory refreshed: 1 added, 1 changed",
			Edits: edits, Note: "From a chat that read web pages"})}
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, "Plan the week"),
			answer(1, text("Done.")),
			refreshed,
		)
		shows(t, x.tt, "Memory refreshed: 1 added, 1 changed", "Undo", "View changes", "From a chat that read web pages")
		click(t, x, "View changes")
		shows(t, x.tt, "PROJECT MEMORY · GOALS", "USER MEMORY · PREFERENCES", persian, "Short answers")
		shot(t, x, "refresh-chip")
		click(t, x, "Undo")
		x.until(t, "the undo", func() bool { return x.f.IsUndone("refresh:5,6,") })
		shows(t, x.tt, "Memory refreshed: 1 added, 1 changed · undone", "Redo")
		click(t, x, "Redo")
		x.until(t, "the redo", func() bool { return !x.f.IsUndone("refresh:5,6,") })
		if got := toasts(x); len(got) != 0 {
			t.Errorf("an undo and redo that worked said %q", got)
		}
		// A section changed since: nothing is undone, and the toast names it.
		x.f.SetChanged("refresh:5,6,")
		click(t, x, "Undo")
		x.until(t, "the toast", func() bool { return len(x.st.Toasts) > 0 })
		if got := toasts(x); len(got) != 1 || got[0] != "Memory:project/goals changed since, so nothing was undone. Edit memory in the settings." {
			t.Errorf("toasts %q", got)
		}
		shows(t, x.tt, "Undo")
	})
}
