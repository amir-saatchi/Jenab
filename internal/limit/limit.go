// Package limit has the shared limits: the 10:1 rule for writes and Gate,
// the slots for LLM calls and pipeline runs (Q19, SPEC 7.3, 7.6).
package limit

import (
	"context"
	"sync"
)

// Priority orders work that waits for the same resource.
type Priority int

const (
	Interactive Priority = iota // chat turns, forms, memory edits, undo
	Background                  // pipelines, Refresh memory
)

// burst is how many interactive items may go in a row while background
// work waits.
const burst = 10

// Fair is the 10:1 rule: interactive work goes first, but after 10
// interactive items in a row one background item goes, so background work
// is never starved. The zero value is ready to use; it is not safe for
// concurrent use.
type Fair struct {
	streak int
}

// Next picks the priority to serve, given which queues have work waiting.
// Call it only when at least one has.
func (f *Fair) Next(interactive, background bool) Priority {
	switch {
	case interactive && background:
		if f.streak >= burst {
			f.streak = 0
			return Background
		}
		f.streak++
		return Interactive
	case interactive:
		f.streak = 0 // nothing is waiting behind it
		return Interactive
	default:
		f.streak = 0
		return Background
	}
}

// Gate has n slots. Interactive callers never wait: they take a slot even
// when all are in use, so a busy app never freezes a chat (SPEC 7.6).
// Background callers wait first come, first served, until fewer than n slots
// are in use, interactive ones included.
type Gate struct {
	mu      sync.Mutex
	size    int
	inUse   int
	waiting []*waiter // background callers, oldest first
}

type waiter struct {
	ready   chan struct{}
	granted bool // set under the gate's lock when a slot is handed over
}

// GateStats is a snapshot for the runtime panel (Q19a). InUse can be above
// Size while chats are busy or after the size was lowered.
type GateStats struct {
	Size, InUse, Waiting int
}

// NewGate returns a gate with n slots; n below 1 is treated as 1.
func NewGate(n int) *Gate {
	return &Gate{size: max(n, 1)}
}

// SetSize changes the number of slots; n below 1 is treated as 1. More slots
// start waiting callers at once. With fewer, running work keeps its slot and
// nothing new starts until fewer than n are in use.
func (g *Gate) SetSize(n int) {
	g.mu.Lock()
	g.size = max(n, 1)
	g.dispatch()
	g.mu.Unlock()
}

// Acquire takes a slot, waiting if p is Background and none is free. It
// returns a release function, which must be called once the work is done;
// calling it again does nothing. If ctx ends first, Acquire returns
// ctx.Err() and holds no slot.
func (g *Gate) Acquire(ctx context.Context, p Priority) (release func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	if p == Interactive || (g.inUse < g.size && len(g.waiting) == 0) {
		g.inUse++
		g.mu.Unlock()
		return g.releaser(), nil
	}
	w := &waiter{ready: make(chan struct{})}
	g.waiting = append(g.waiting, w)
	g.mu.Unlock()

	select {
	case <-w.ready:
		return g.releaser(), nil
	case <-ctx.Done():
		g.mu.Lock()
		if w.granted { // the slot arrived as ctx ended: pass it on
			g.inUse--
			g.dispatch()
		} else {
			g.remove(w)
		}
		g.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (g *Gate) releaser() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.inUse--
			g.dispatch()
			g.mu.Unlock()
		})
	}
}

// dispatch hands free slots to waiters. The caller holds g.mu.
func (g *Gate) dispatch() {
	for g.inUse < g.size && len(g.waiting) > 0 {
		w := g.waiting[0]
		g.waiting[0] = nil
		g.waiting = g.waiting[1:]
		w.granted = true
		g.inUse++
		close(w.ready)
	}
}

// remove takes a waiter that gave up out of the queue. The caller holds g.mu.
func (g *Gate) remove(w *waiter) {
	for i, x := range g.waiting {
		if x == w {
			g.waiting = append(g.waiting[:i:i], g.waiting[i+1:]...)
			return
		}
	}
}

// Stats returns the slots in use and the callers waiting.
func (g *Gate) Stats() GateStats {
	g.mu.Lock()
	defer g.mu.Unlock()
	return GateStats{Size: g.size, InUse: g.inUse, Waiting: len(g.waiting)}
}
