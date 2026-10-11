package provider_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/provider"
)

func TestHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var h provider.Heartbeat
		n := 0
		yield := func(ev provider.Event, err error) bool {
			if ev.Kind != provider.EventAlive || err != nil {
				t.Fatalf("got %+v, %v", ev, err)
			}
			n++
			return true
		}
		for range 10 { // 10 chunks in 900 ms: one beat
			h.Beat(yield)
			time.Sleep(100 * time.Millisecond)
		}
		if n != 1 {
			t.Fatalf("%d beats in 900 ms, want 1", n)
		}
		time.Sleep(100 * time.Millisecond)
		h.Beat(yield)
		if n != 2 {
			t.Fatalf("%d beats after a second, want 2", n)
		}
		time.Sleep(provider.AliveEvery)
		if h.Beat(func(provider.Event, error) bool { return false }) {
			t.Error("Beat went on after yield said stop")
		}
	})
}
