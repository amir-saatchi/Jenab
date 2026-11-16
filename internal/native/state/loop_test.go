package state

import (
	"sync"
	"testing"
	"time"
)

// loop stands in for the UI thread: post queues, and the test runs what was
// posted while it waits for a result.
type loop struct {
	mu     sync.Mutex
	queue  []func()
	signal chan struct{}
}

func newLoop() *loop { return &loop{signal: make(chan struct{}, 1)} }

func (l *loop) post(fn func()) {
	l.mu.Lock()
	l.queue = append(l.queue, fn)
	l.mu.Unlock()
	select {
	case l.signal <- struct{}{}:
	default:
	}
}

// drain runs what was posted so far, and what that posts.
func (l *loop) drain() {
	for {
		l.mu.Lock()
		q := l.queue
		l.queue = nil
		l.mu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

const waitLimit = 5 * time.Second

// await runs the loop until ch gets a value.
func await[T any](t *testing.T, l *loop, ch <-chan T) T {
	t.Helper()
	deadline := time.After(waitLimit)
	for {
		l.drain()
		select {
		case v := <-ch:
			return v
		case <-l.signal:
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

// until runs the loop until cond holds.
func until(t *testing.T, l *loop, cond func() bool) {
	t.Helper()
	deadline := time.After(waitLimit)
	for {
		l.drain()
		if cond() {
			return
		}
		select {
		case <-l.signal:
		case <-time.After(time.Millisecond):
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

func TestMailboxKeepsWhatComesBeforeAttach(t *testing.T) {
	var m Mailbox
	var got []int
	m.Post(func() { got = append(got, 1) })
	m.Post(func() { got = append(got, 2) })
	l := newLoop()
	m.Attach(l.post)
	m.Post(func() { got = append(got, 3) })
	l.drain()
	same(t, got, []int{1, 2, 3})
}
