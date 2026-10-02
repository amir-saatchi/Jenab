package limit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestFair(t *testing.T) {
	var f Fair
	var got strings.Builder
	for range 23 {
		if f.Next(true, true) == Interactive {
			got.WriteByte('i')
		} else {
			got.WriteByte('b')
		}
	}
	want := "iiiiiiiiiib" + "iiiiiiiiiib" + "i"
	if got.String() != want {
		t.Errorf("both waiting: got %s, want %s", got.String(), want)
	}

	f = Fair{streak: 9}
	if f.Next(true, false) != Interactive || f.streak != 0 {
		t.Error("interactive alone should reset the streak")
	}
	if f.Next(false, true) != Background {
		t.Error("background alone should be served")
	}
}

// order holds the slot of a one-slot gate, queues background waiters one
// after the other, then releases and returns the order they got the slot.
func order(t *testing.T, n int) string {
	t.Helper()
	var got strings.Builder
	synctest.Test(t, func(t *testing.T) {
		g := NewGate(1)
		hold, err := g.Acquire(t.Context(), Background)
		if err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		for i := range n {
			go func() {
				release, err := g.Acquire(context.Background(), Background)
				if err != nil {
					t.Errorf("waiter %d: %v", i, err)
					return
				}
				mu.Lock()
				fmt.Fprintf(&got, "b%d ", i)
				mu.Unlock()
				release()
			}()
			synctest.Wait() // queued before the next one starts
		}
		hold()
		synctest.Wait()
	})
	return strings.TrimSpace(got.String())
}

func TestGateBackgroundFIFO(t *testing.T) {
	if got, want := order(t, 4), "b0 b1 b2 b3"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestGateInteractiveNeverWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := NewGate(2)
		b1, _ := g.Acquire(t.Context(), Background)
		b2, _ := g.Acquire(t.Context(), Background)

		// Both slots are taken by background work; chats still start at once.
		start := time.Now()
		i1, err := g.Acquire(t.Context(), Interactive)
		if err != nil {
			t.Fatal(err)
		}
		i2, _ := g.Acquire(t.Context(), Interactive)
		if time.Since(start) != 0 {
			t.Error("an interactive caller waited")
		}
		if s := g.Stats(); s.InUse != 4 || s.Size != 2 {
			t.Fatalf("stats = %+v, want 4 in use of 2", s)
		}

		// Chats count as in use: background waits until fewer than 2 are.
		got := make(chan struct{})
		go func() {
			r, err := g.Acquire(t.Context(), Background)
			if err != nil {
				t.Error(err)
				return
			}
			close(got)
			r()
		}()
		synctest.Wait()
		b1()
		b2() // the two chats still hold 2 of 2
		synctest.Wait()
		select {
		case <-got:
			t.Fatal("background started with 2 of 2 slots in use")
		default:
		}
		i1()
		<-got
		i2()
	})
}

func TestGateSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := NewGate(2)
		r1, _ := g.Acquire(t.Context(), Background)
		r2, _ := g.Acquire(t.Context(), Background)

		done := make(chan struct{})
		go func() {
			r3, err := g.Acquire(t.Context(), Background)
			if err != nil {
				t.Error(err)
			}
			r3()
			close(done)
		}()
		synctest.Wait()
		if s := g.Stats(); s.InUse != 2 || s.Waiting != 1 {
			t.Fatalf("stats with both slots taken = %+v", s)
		}
		r1()
		r1() // a second call does nothing
		<-done
		if s := g.Stats(); s.InUse != 1 || s.Waiting != 0 {
			t.Errorf("stats after release = %+v, want 1 in use", s)
		}
		r2()
		if s := g.Stats(); s.InUse != 0 {
			t.Errorf("stats at the end = %+v", s)
		}
	})
}

func TestGateSetSize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := NewGate(1)
		hold, _ := g.Acquire(t.Context(), Background)
		started := make(chan func(), 3)
		for range 3 {
			go func() {
				r, err := g.Acquire(t.Context(), Background)
				if err != nil {
					t.Error(err)
					return
				}
				started <- r
			}()
		}
		synctest.Wait()

		g.SetSize(3) // two more slots: two waiters start without a release
		synctest.Wait()
		if s := g.Stats(); len(started) != 2 || s.InUse != 3 || s.Waiting != 1 {
			t.Fatalf("after growing: %d started, stats %+v", len(started), s)
		}

		g.SetSize(1) // running work keeps its slot
		hold()
		(<-started)()
		synctest.Wait()
		if s := g.Stats(); s.InUse != 1 || s.Waiting != 1 {
			t.Fatalf("after shrinking: stats %+v, want 1 in use and 1 waiting", s)
		}
		(<-started)()
		synctest.Wait()
		if s := g.Stats(); s.InUse != 1 || s.Waiting != 0 || len(started) != 1 {
			t.Fatalf("last waiter: stats %+v", s)
		}
		(<-started)()

		g.SetSize(0)
		if s := g.Stats(); s.Size != 1 {
			t.Errorf("SetSize(0) size = %d, want 1", s.Size)
		}
	})
}

func TestGateCancelWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := NewGate(1)
		hold, _ := g.Acquire(t.Context(), Background)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		start := time.Now()
		_, err := g.Acquire(ctx, Background)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want deadline exceeded", err)
		}
		if waited := time.Since(start); waited != 5*time.Second {
			t.Errorf("waited %v, want 5s", waited)
		}
		if s := g.Stats(); s.Waiting != 0 {
			t.Errorf("a waiter that gave up is still queued: %+v", s)
		}
		hold()
		if s := g.Stats(); s.InUse != 0 {
			t.Errorf("slot not free after release: %+v", s)
		}
	})
}

func TestGateCancelledContext(t *testing.T) {
	g := NewGate(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, p := range []Priority{Interactive, Background} {
		if _, err := g.Acquire(ctx, p); !errors.Is(err, context.Canceled) {
			t.Errorf("priority %d: err = %v, want canceled", p, err)
		}
	}
	if s := g.Stats(); s.InUse != 0 {
		t.Errorf("a cancelled caller took a slot: %+v", s)
	}
}

func TestNewGateMinimum(t *testing.T) {
	if s := NewGate(0).Stats(); s.Size != 1 {
		t.Errorf("NewGate(0) size = %d, want 1", s.Size)
	}
}
