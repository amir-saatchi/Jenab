// Package state holds the native app's stores (SPIKE-048): what
// the React app's state code holds, as plain Go. Their tests are
// ported with them and are the spec.
//
// # Threads
//
// A store is read and changed on the UI thread only, where the view
// builds: no locks. Work that waits, a service call or a timer, runs in a
// goroutine and hands its result back with Post, which the native app
// makes from mygo's Window.Update. Post returns before its function runs,
// so a method that starts such work returns at once; one whose caller may
// wait takes a then callback, or returns a channel that gets the result
// once it is applied.
//
// Screens read the stores' exported fields while they build and call the
// stores' methods from their event handlers. Nothing here imports MyGo.
package state

import (
	"context"
	"errors"
	"sync"

	"github.com/amir-saatchi/jenab/internal/app"
)

// Post runs fn on the UI thread, later, and returns at once. The native app
// gives Window.Update; tests give a Loop.
type Post func(fn func())

// errorText is what the user sees of err: a service's UIError message, or
// the error itself.
func errorText(err error) string {
	if ue, ok := errors.AsType[*app.UIError](err); ok {
		return ue.Message
	}
	return err.Error()
}

// Mailbox holds what is posted before the UI thread can take it, as events
// sent while the window is being made, and posts it in order once Attach
// gives it somewhere to go. It is safe from any goroutine.
type Mailbox struct {
	mu    sync.Mutex
	post  Post
	queue []func()
}

// Post posts fn, or keeps it until Attach.
func (m *Mailbox) Post(fn func()) {
	m.mu.Lock()
	if m.post == nil {
		m.queue = append(m.queue, fn)
		m.mu.Unlock()
		return
	}
	post := m.post
	m.mu.Unlock()
	post(fn)
}

// Attach makes post where functions go from now on, after those kept.
// They are posted under the lock, so one posted meanwhile can't overtake
// them.
func (m *Mailbox) Attach(post Post) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, fn := range m.queue {
		post(fn)
	}
	m.queue = nil
	m.post = post
}

// background is the context of the stores' service calls.
var background = context.Background()
