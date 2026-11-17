package chat_test

import (
	"slices"
	"testing"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// The rewind dialog acts on its own chat (TASK-066): its question's answer
// coming after the user moved to another chat doesn't open it there, and
// a rewind still running when the user moves on ends in its own chat.
func TestTheRewindDialogActsOnItsOwnChat(t *testing.T) {
	x := open(t, false,
		statetest.Text(1, jc.RoleUser, "First"), answer(1, text("One")),
		statetest.Text(2, jc.RoleUser, "Second "+persian), answer(2, text("Two")))
	a := x.ch
	b := x.f.AddChat(x.p, "Other")
	x.f.AddMessages(b, statetest.Text(1, jc.RoleUser, "Elsewhere"), answer(1, text("Three")))
	x.st.OpenChat(x.p, b)
	if !x.loop.Drain(func() bool { return x.st.Threads.Threads[b] != nil }) {
		t.Fatal("the other chat did not open")
	}
	x.draw()

	// Asked in a, answered once b shows.
	clickLast(t, x.tt, "Edit and resend")
	x.ch = b
	x.draw()
	if x.tt.HasText("Edit and resend?") {
		t.Fatal("a's rewind dialog opened over b")
	}
	x.ch = a
	x.draw()
	if x.tt.HasText("Edit and resend?") {
		t.Fatal("a's put-away dialog came back")
	}

	// Confirmed in a, done once b shows.
	clickLast(t, x.tt, "Edit and resend")
	x.until(t, "the dialog", func() bool { return x.tt.HasText("Edit and resend?") })
	clickLast(t, x.tt, "Edit and resend") // the dialog's
	x.ch = b
	x.until(t, "the rewind", func() bool { return len(x.f.Rewinds()) == 1 })
	x.draw()
	if got := x.f.Rewound(); !slices.Equal(got, []id.Chat{a}) {
		t.Errorf("rewound %v, want %v", got, []id.Chat{a})
	}
	if got := x.st.Threads.Fills[a]; got != "Second "+persian {
		t.Errorf("a's composer gets %q", got)
	}
	if got, ok := x.st.Threads.Fills[b]; ok {
		t.Errorf("b's composer gets %q", got)
	}
	if x.tt.HasText("Edit and resend?") {
		t.Error("the dialog shows over b")
	}
}

// The dialog shows the plan of its own opening (TASK-072): an earlier
// opening's plan, coming last, doesn't take its place.
func TestTheRewindDialogShowsItsOwnPlan(t *testing.T) {
	x := open(t, false,
		statetest.Text(1, jc.RoleUser, "First"), answer(1, text("One")),
		statetest.Text(2, jc.RoleUser, "Second "+persian), answer(2, text("Two")))
	a := x.ch
	b := x.f.AddChat(x.p, "Other")
	x.f.AddMessages(b,
		statetest.Text(1, jc.RoleUser, "Elsewhere"), answer(1, text("Three")),
		statetest.Text(2, jc.RoleUser, "Again"), answer(2, text("Four")))
	x.st.OpenChat(x.p, b)
	if !x.loop.Drain(func() bool { return x.st.Threads.Threads[b] != nil }) {
		t.Fatal("the other chat did not open")
	}
	release := make(chan struct{})
	x.f.HoldRewindState = func(c id.Chat) {
		if c == a {
			<-release
		}
	}
	x.draw()

	clickLast(t, x.tt, "Edit and resend") // in a: its plan is held back
	x.ch = b
	x.draw()
	clickLast(t, x.tt, "Edit and resend") // in b
	x.until(t, "b's dialog", func() bool { return x.tt.HasText("Edit and resend?") })
	close(release) // a's plan comes last
	x.draw()
	if !x.tt.HasText("Edit and resend?") {
		t.Fatal("a's late plan put b's dialog away")
	}
	clickLast(t, x.tt, "Edit and resend") // the dialog's
	x.until(t, "the rewind", func() bool { return len(x.f.Rewinds()) == 1 })
	if got := x.f.Rewound(); !slices.Equal(got, []id.Chat{b}) {
		t.Errorf("rewound %v, want %v", got, []id.Chat{b})
	}
	x.draw()
	if m, _ := x.tt.Access("Message"); m.Value != "Again" {
		t.Errorf("b's composer holds %q", m.Value)
	}
}
