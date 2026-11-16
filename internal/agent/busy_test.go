package agent

import (
	"testing"
	"testing/synctest"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// The update restart asks Busy (TASK-067): a running turn counts, and so
// does a memory refresh, which runs in the background of its chat.
func TestBusyWhileATurnOrARefreshRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Work"})
		if h.o.Busy() {
			t.Fatal("busy before any work")
		}
		h.fp.Push(fake.Reply{Hang: true})
		h.send(c.ID, "go")
		synctest.Wait() // the request hangs
		if !h.o.Busy() {
			t.Fatal("not busy while a turn runs")
		}
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if h.o.Busy() {
			t.Fatal("busy after the turn stopped")
		}

		h.addTurns(c.ID, 2, 3)
		h.fp.Push(fake.Reply{Hang: true})
		h.refresh(c.ID, true)
		synctest.Wait() // the refresh's first request hangs
		if !h.o.Busy() {
			t.Fatal("not busy while a memory refresh runs")
		}
	})
}
