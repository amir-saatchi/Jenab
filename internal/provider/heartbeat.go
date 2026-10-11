package provider

import "time"

// AliveEvery is how often at most a backend sends EventAlive.
const AliveEvery = time.Second

// Heartbeat sends EventAlive for stream data a backend keeps to itself, at
// most once every AliveEvery, so a long tool call is not taken for a
// stalled stream. The zero value is ready.
type Heartbeat struct{ last time.Time }

// Beat sends EventAlive if one is due. It reports whether to go on, as
// yield does.
func (h *Heartbeat) Beat(yield func(Event, error) bool) bool {
	now := time.Now()
	if !h.last.IsZero() && now.Sub(h.last) < AliveEvery {
		return true
	}
	h.last = now
	return yield(Event{Kind: EventAlive}, nil)
}
