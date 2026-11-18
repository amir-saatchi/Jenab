package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

func TestTurnWhileBackgroundSlotsAreTaken(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	// A background call holds the provider's only slot and one global
	// slot; another holder takes the second global slot.
	bctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.fp.Push(fake.Reply{Hang: true})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range h.models.Stream(bctx, limit.Background, provider.Request{Model: "default"}) {
		}
	}()
	for len(h.fp.Calls()) == 0 {
		time.Sleep(time.Millisecond)
	}
	release, err := h.gate.Acquire(context.Background(), limit.Background)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	wctx, wcancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer wcancel()
	if _, err := h.gate.Acquire(wctx, limit.Background); err == nil {
		t.Fatal("a background slot is still free")
	}

	h.fp.Push(fake.Text("answered"))
	h.turn(c.ID, "price?")
	if got := texts(h.messages(c.ID)); !slices.Equal(got, []string{"user: price?", "assistant: answered"}) {
		t.Errorf("stored %q", got)
	}
	cancel()
	<-done
}

func rateLimited(wait time.Duration) error {
	return &provider.Error{Kind: provider.RateLimited, Status: 429, RetryAfter: wait, Message: "too many requests"}
}

func TestRateLimitedTurnWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(rateLimited(30*time.Second)), fake.Text("done"))
		start := time.Now()
		h.send(c.ID, "price?")
		synctest.Wait()
		rs := h.pub.retries()
		if len(rs) != 1 || rs[0].Provider != "p" || rs[0].Kind != "rate_limited" || !rs[0].At.Equal(start.Add(30*time.Second)) {
			t.Fatalf("retries shown %+v, want one 30 s wait", rs)
		}
		if st := h.o.find(h.pid, c.ID).Status(); len(st) != 1 || st[0].State != "waiting" || !strings.Contains(st[0].Err, "p is rate limited") {
			t.Errorf("activity %+v", st)
		}
		h.wait()
		if d := time.Since(start); d != 30*time.Second {
			t.Errorf("finished after %s, want 30 s", d)
		}
		if got := texts(h.messages(c.ID)); !slices.Equal(got, []string{"user: price?", "assistant: done"}) {
			t.Errorf("stored %q", got)
		}
		st := h.pub.allStatuses()
		if last := st[len(st)-1]; last.Retry != nil || last.State != chat.StateIdle {
			t.Errorf("last status %+v", last)
		}
	})
}

func TestRetryNow(t *testing.T) {
	for _, pause := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider paused %v", pause), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t, t.TempDir(), "", testSettings())
				defer h.stop()
				c := h.newChat(chat.Chat{Title: "Prices"})
				if pause {
					// Another call's rate limit paused the provider: the
					// turn waits in the Registry.
					h.fp.Push(fake.Fail(rateLimited(time.Minute)))
					for range h.models.Stream(context.Background(), limit.Background, provider.Request{Model: "default"}) {
					}
				} else {
					h.fp.Push(fake.Fail(&provider.Error{Kind: provider.Overloaded, Status: 529, RetryAfter: time.Minute}))
				}
				h.fp.Push(fake.Text("done"))
				start := time.Now()
				if err := h.o.Retry(context.Background(), h.pid, c.ID); !errors.Is(err, ErrNoRetry) {
					t.Errorf("Retry before any turn: %v", err)
				}
				h.send(c.ID, "price?")
				synctest.Wait()
				if len(h.pub.retries()) != 1 {
					t.Fatalf("retries shown %+v", h.pub.retries())
				}
				if err := h.o.Retry(context.Background(), h.pid, c.ID); err != nil {
					t.Fatal(err)
				}
				h.wait()
				if d := time.Since(start); d != 0 {
					t.Errorf("finished after %s, want at once", d)
				}
				if got := texts(h.messages(c.ID)); !slices.Equal(got, []string{"user: price?", "assistant: done"}) {
					t.Errorf("stored %q", got)
				}
			})
		})
	}
}

func TestTurnGivesUpAfterTenMinutes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		for range 30 {
			h.fp.Push(fake.Fail(provider.TransportError("p", "connection reset", nil)))
		}
		start := time.Now()
		h.turn(c.ID, "price?")
		if d := time.Since(start); d != maxRetrying {
			t.Errorf("gave up after %s, want %s", d, maxRetrying)
		}
		ms := h.messages(c.ID)
		last := ms[len(ms)-1]
		if n := last.Parts[0].Notice; n == nil || n.Kind != chat.NoticeTurnFailed || !strings.Contains(n.Text, "p is not reachable; retried for 10 minutes") {
			t.Fatalf("last message %+v", last.Parts)
		}
		// Retry continues the turn.
		tries := len(h.fp.Calls())
		h.fp.Replace(fake.Text("back"))
		if err := h.o.Retry(context.Background(), h.pid, c.ID); err != nil {
			t.Fatal(err)
		}
		h.wait()
		ms = h.messages(c.ID)
		if got := texts(ms[len(ms)-1:]); !slices.Equal(got, []string{"assistant: back"}) || ms[len(ms)-1].Turn != 1 {
			t.Errorf("after Retry: %q in turn %d", got, ms[len(ms)-1].Turn)
		}
		if got := texts(h.fp.Calls()[tries].Messages); got[len(got)-1] != "user: [the turn stopped here on an error, and the user retried it]" {
			t.Errorf("the retried request ends with %q", got[len(got)-1])
		}
		if err := h.o.Retry(context.Background(), h.pid, c.ID); !errors.Is(err, ErrNoRetry) {
			t.Errorf("Retry after a good turn: %v", err)
		}
	})
}

func TestErrorsThatAreNotRetried(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&provider.Error{Kind: provider.Quota, Provider: "p", Status: 429, Message: "daily limit"}, "p says the quota or credit is used up: daily limit."},
		{&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 401, Message: "bad key"}, "p refused the request: bad key"},
		{&provider.Error{Kind: provider.TooLarge, Provider: "p", Status: 400}, "too large for this model"},
	} {
		t.Run(string(tc.err.(*provider.Error).Kind), func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings())
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "Prices"})
			h.fp.Push(fake.Fail(tc.err))
			h.turn(c.ID, "price?")
			ms := h.messages(c.ID)
			if n := ms[len(ms)-1].Parts[0].Notice; n == nil || !strings.Contains(n.Text, tc.want) {
				t.Errorf("last message %+v", ms[len(ms)-1].Parts)
			}
			if n := len(h.fp.Calls()); n != 1 || len(h.pub.retries()) != 0 {
				t.Errorf("%d requests, %d waits; want no retry", n, len(h.pub.retries()))
			}
		})
	}
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices", Model: "nobody/m1"})
	h.turn(c.ID, "price?")
	ms := h.messages(c.ID)
	if n := ms[len(ms)-1].Parts[0].Notice; n == nil || !strings.Contains(n.Text, `the model "nobody/m1" is not set up`) {
		t.Errorf("unknown model: %+v", ms[len(ms)-1].Parts)
	}
}

func TestStalledStreamIsRetried(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Reply{Events: []provider.Event{{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "The pri"}}, Hang: true},
			fake.Text("The price is 10."))
		start := time.Now()
		h.turn(c.ID, "price?")
		d := time.Since(start)
		if d < provider.Between+10*time.Second || d > provider.Between+12*time.Second {
			t.Errorf("finished after %s, want the stall timeout and one backoff", d)
		}
		if rs := h.pub.retries(); len(rs) != 1 || rs[0].Kind != "transport" {
			t.Errorf("retries shown %+v", rs)
		}
		ms := h.messages(c.ID)
		if got := texts(ms); !slices.Equal(got, []string{"user: price?", "assistant: The price is 10."}) {
			t.Errorf("stored %q: the cut-off try must leave nothing", got)
		}
		ds := h.pub.allDeltas()
		if len(ds) != 2 || ds[0].Message == ds[1].Message || ds[1].Message != ms[1].ID {
			t.Errorf("deltas %+v: each try has its own message", ds)
		}
	})
}

func TestRetryNowIsNotAFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(rateLimited(time.Minute)))
		for range h.models.Stream(context.Background(), limit.Background, provider.Request{Model: "default"}) {
		}
		h.fp.Push(fake.Fail(provider.TransportError("p", "connection reset", nil)), fake.Text("done"))
		h.send(c.ID, "price?")
		synctest.Wait()
		if err := h.o.Retry(context.Background(), h.pid, c.ID); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		// The first failed try waits the first backoff step.
		rs := h.pub.retries()
		if len(rs) != 2 {
			t.Fatalf("retries %+v, want two", rs)
		}
		if wait := time.Until(rs[1].At); wait < 10*time.Second || wait > 12*time.Second {
			t.Errorf("second wait %s, want 10 to 12 s", wait)
		}
		h.wait()
	})
}

// The try after a retry wait starts its own clock, so the runtime panel
// doesn't flag it as stuck.
func TestTryAfterWaitIsNotStuck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(rateLimited(30*time.Second)), fake.Reply{Hang: true})
		start := time.Now()
		h.send(c.ID, "price?")
		time.Sleep(31 * time.Second)
		synctest.Wait()
		st := h.o.find(h.pid, c.ID).Status()
		if len(st) != 1 || st[0].State != "running" || !st[0].LastActivity.Equal(start.Add(30*time.Second)) {
			t.Errorf("activity %+v, want the last activity at the second try", st)
		}
		h.o.Stop(h.pid, c.ID)
		h.wait()
	})
}

// A long tool call's heartbeats move the turn's last activity and are not
// stored (TASK-006).
func TestHeartbeatsMoveTheTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), echo())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		call := fake.ToolCall("c1", "echo", map[string]string{"text": "hi"})
		alive := provider.Event{Kind: provider.EventAlive}
		h.fp.Push(fake.Reply{Wait: 40 * time.Second, Events: append([]provider.Event{alive, alive, alive}, call.Events...)}, fake.Text("done"))
		start := time.Now()
		h.send(c.ID, "price?")
		time.Sleep(85 * time.Second)
		synctest.Wait()
		st := h.o.find(h.pid, c.ID).Status()
		if len(st) != 1 || !st[0].LastActivity.Equal(start.Add(80*time.Second)) {
			t.Errorf("activity %+v, want the last activity at the second heartbeat", st)
		}
		h.wait()
		ms := h.messages(c.ID)
		if len(ms) != 4 || len(ms[1].Parts) != 1 || ms[1].Parts[0].ToolCall == nil || ms[3].Parts[0].Text == nil || ms[3].Parts[0].Text.Text != "done" {
			t.Fatalf("messages = %+v", ms)
		}
	})
}

// TestWhatTheModelReadsOfAFailedTurn: a failed turn's notice is advice for
// the user. The model reads only that the turn stopped there, and whether
// the user retried it; the stored notice the user sees is unchanged
// (8.3, TASK-080).
func TestWhatTheModelReadsOfAFailedTurn(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	quota := &provider.Error{Kind: provider.Quota, Provider: "p", Status: 429, Message: "daily limit"}
	h.fp.Push(fake.Fail(quota), fake.Text("ten"))
	h.turn(c.ID, "price?")
	if err := h.o.Retry(context.Background(), h.pid, c.ID); err != nil {
		t.Fatal(err)
	}
	h.wait()
	retried := "user: [the turn stopped here on an error, and the user retried it]"
	if got, want := texts(h.fp.Calls()[1].Messages), []string{"user: price?", retried}; !slices.Equal(got, want) {
		t.Errorf("the retried request:\n got %q\nwant %q", got, want)
	}

	// A turn that failed and wasn't retried: the user asked on.
	h.fp.Push(fake.Fail(quota), fake.Text("eleven"))
	h.turn(c.ID, "and tomorrow?")
	h.turn(c.ID, "once more")
	want := []string{"user: price?", retried, "assistant: ten", "user: and tomorrow?", "user: [the turn stopped here on an error]", "user: once more"}
	if got := texts(h.fp.Calls()[3].Messages); !slices.Equal(got, want) {
		t.Errorf("the next turn's request:\n got %q\nwant %q", got, want)
	}

	var shown int
	for _, m := range h.messages(c.ID) {
		for _, p := range m.Parts {
			if n := p.Notice; n != nil && n.Kind == chat.NoticeTurnFailed {
				shown++
				if !strings.Contains(n.Text, "Add credit, or pick another model") {
					t.Errorf("the user's notice is %q", n.Text)
				}
			}
		}
	}
	if shown != 2 {
		t.Errorf("%d failed notices stored, want 2", shown)
	}
}
