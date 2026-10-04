package agent

import (
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// deltaEvery is the shortest gap between two deltas of a chat (Q17).
const deltaEvery = 16 * time.Millisecond

// coalescer joins streamed text into at most one Delta per deltaEvery; a
// timer sends the rest. Text for another part is sent at once, so the
// order stays right.
type coalescer struct {
	every time.Duration
	send  func(chat.Delta)

	mu      sync.Mutex
	pending *chat.Delta
	last    time.Time // when the last delta went out
	timer   *time.Timer
	gen     int // counts timers, so a stale one sends nothing
}

func newCoalescer(every time.Duration, send func(chat.Delta)) *coalescer {
	return &coalescer{every: every, send: send}
}

func (c *coalescer) add(d chat.Delta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.pending; p != nil && (p.Message != d.Message || p.Part != d.Part || p.Kind != d.Kind) {
		c.sendLocked()
	}
	if c.pending == nil {
		c.pending = &d
	} else {
		c.pending.Text += d.Text
		c.pending.Seq = d.Seq
	}
	wait := c.every - time.Since(c.last)
	if wait <= 0 {
		c.sendLocked()
		return
	}
	if c.timer == nil {
		gen := c.gen
		c.timer = time.AfterFunc(wait, func() { c.fire(gen) })
	}
}

func (c *coalescer) fire(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen == c.gen {
		c.sendLocked()
	}
}

// flush sends what is pending now.
func (c *coalescer) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendLocked()
}

func (c *coalescer) sendLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
		c.gen++
	}
	if c.pending == nil {
		return
	}
	c.send(*c.pending)
	c.pending, c.last = nil, time.Now()
}
