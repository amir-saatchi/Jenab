package state

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/update"
)

func TestAnUpdateToastsOncePerVersionAndState(t *testing.T) {
	s, _, _, _ := newStores(t)
	s.UpdateStatus(update.Status{State: update.StateChecking, Current: "0.1.0"})
	same(t, len(s.Toasts), 0)
	avail := update.Status{State: update.StateAvailable, Current: "0.1.0", Version: "0.1.1"}
	s.UpdateStatus(avail)
	s.UpdateStatus(update.Status{State: update.StateChecking, Current: "0.1.0"})
	s.UpdateStatus(avail) // the next check finds it again
	same(t, len(s.Toasts), 1)
	same(t, s.Toasts[0].Text, "Jenab 0.1.1 is available. Install it in Settings › General.")
	s.UpdateStatus(update.Status{State: update.StateReady, Current: "0.1.0", Version: "0.1.1"})
	same(t, len(s.Toasts), 2)
	same(t, s.Toasts[1].Text, "Jenab 0.1.1 is installed. It starts with the next restart.")
	same(t, s.Updates.Status.State, update.StateReady)
}

func TestBusyWhileAChatWorksOrWaits(t *testing.T) {
	s, _, _, _ := newStores(t)
	same(t, s.Busy(), false)
	s.Apply(app.EventStatus, waitStatus("a", chat.StateWorking, nil))
	same(t, s.Busy(), true)
	s.Apply(app.EventStatus, waitStatus("a", chat.StateIdle, nil))
	same(t, s.Busy(), false)
}
