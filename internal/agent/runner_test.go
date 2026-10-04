package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// stubborn waits for release even after Stop, like a tool that is slow to
// notice.
func stubborn(started chan<- string, release <-chan struct{}) tool.Tool {
	return tool.Func(tool.Spec{Name: "stubborn", Description: "Wait.",
		Schema: json.RawMessage(`{"type": "object"}`)},
		func(_ context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			started <- string(env.Message)
			<-release
			return tool.Result{Text: "released"}, nil
		})
}

func TestMessageAfterStop(t *testing.T) {
	// The message comes while the stopped turn is still ending.
	setup := func(t *testing.T) (*harness, chat.Chat, chan struct{}) {
		started, release := make(chan string, 1), make(chan struct{})
		h := newHarness(t, t.TempDir(), "", testSettings(), stubborn(started, release))
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.ToolCall("c1", "stubborn", struct{}{}), fake.Text("second answer"))
		h.send(c.ID, "one")
		<-started
		h.o.Stop(h.pid, c.ID)
		h.send(c.ID, "two")
		return h, c, release
	}
	t.Run("starts a new turn", func(t *testing.T) {
		h, c, release := setup(t)
		defer h.stop()
		h.send(c.ID, "three") // joins the queued turn
		close(release)
		h.wait()
		ms := h.messages(c.ID)
		want := []string{"user: one", "assistant: call stubborn", "tool: result released", "user: two", "user: three", "assistant: second answer"}
		if got := texts(ms); !slices.Equal(got, want) {
			t.Fatalf("stored %q\nwant %q", got, want)
		}
		if got := turnsOf(ms); !slices.Equal(got, []int{1, 2}) {
			t.Errorf("turns %v", got)
		}
		validHistory(t, ms)
		if st := h.pub.allStatuses(); st[len(st)-1].State != chat.StateIdle {
			t.Errorf("last status %+v", st[len(st)-1])
		}
	})
	t.Run("a second Stop drops it", func(t *testing.T) {
		h, c, release := setup(t)
		defer h.stop()
		h.o.Stop(h.pid, c.ID)
		close(release)
		h.wait()
		if n := len(h.fp.Calls()); n != 1 {
			t.Errorf("%d requests, want 1", n)
		}
		h.turn(c.ID, "three")
		ms := h.messages(c.ID)
		want := []string{"user: one", "assistant: call stubborn", "tool: result released", "user: two", "user: three", "assistant: second answer"}
		if got := texts(ms); !slices.Equal(got, want) {
			t.Errorf("stored %q\nwant %q", got, want)
		}
		if got := turnsOf(ms); !slices.Equal(got, []int{1, 2, 3}) {
			t.Errorf("turns %v", got)
		}
	})
}

func TestTitleStopsAtShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		c := h.newChat(chat.Chat{})
		h.fp.Push(fake.Text("answer"), fake.Reply{Hang: true})
		h.send(c.ID, "one")
		synctest.Wait() // the title request hangs
		calls := h.fp.Calls()
		if len(calls) != 2 || calls[1].MaxTokens != titleMaxTokens {
			t.Fatalf("%d requests; want the title request with MaxTokens %d", len(calls), titleMaxTokens)
		}
		start := time.Now()
		h.stop()
		if d := time.Since(start); d != 0 {
			t.Errorf("the shutdown waited %s for the title", d)
		}
	})
}

func TestRetryShownOnlyWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, release := make(chan string, 1), make(chan struct{})
		h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(provider.TransportError("p", "connection reset", nil)), fake.ToolCall("c1", "block", struct{}{}))
		h.send(c.ID, "price?")
		<-started
		st := h.pub.allStatuses()
		if last := st[len(st)-1]; last.Retry != nil || last.State != chat.StateWorking {
			t.Errorf("status while the tool runs %+v", last)
		}
		if l := h.o.Live(h.pid, c.ID); l.Retry != nil {
			t.Errorf("Live while the tool runs %+v", l)
		}
		if err := h.o.Retry(context.Background(), h.pid, c.ID); !errors.Is(err, ErrNoRetry) {
			t.Errorf("Retry while the tool runs: %v", err)
		}
		close(release)
		h.wait()
	})
}

func TestPublisherPanics(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.pub.mu.Lock()
	h.pub.panics = true
	h.pub.mu.Unlock()
	h.turn(c.ID, "one")
	h.turn(c.ID, "two")
	want := []string{"user: one", "assistant: ok", "user: two", "assistant: ok"}
	if got := texts(h.messages(c.ID)); !slices.Equal(got, want) {
		t.Errorf("stored %q", got)
	}
}

func TestMessageDuringRetryWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(rateLimited(30*time.Second)), fake.Text("both"))
		h.send(c.ID, "one")
		synctest.Wait()
		h.send(c.ID, "two")
		h.wait()
		calls := h.fp.Calls()
		if len(calls) != 2 {
			t.Fatalf("%d requests, want 2", len(calls))
		}
		if got := texts(calls[1].Messages); !slices.Equal(got, []string{"user: one", "user: two"}) {
			t.Errorf("the retried request has %q", got)
		}
		if got := texts(h.messages(c.ID)); !slices.Equal(got, []string{"user: one", "user: two", "assistant: both"}) {
			t.Errorf("stored %q", got)
		}
	})
}

func TestRetryAfterRestart(t *testing.T) {
	root := t.TempDir()
	h := newHarness(t, root, "", testSettings())
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.Fail(&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 401, Message: "bad key"}))
	h.turn(c.ID, "price?")
	h.stop()

	h = newHarness(t, root, h.pid, testSettings())
	if err := h.o.Retry(context.Background(), h.pid, c.ID); err != nil {
		t.Fatal(err)
	}
	h.wait()
	ms := h.messages(c.ID)
	if last := ms[len(ms)-1]; texts(ms[len(ms)-1:])[0] != "assistant: ok" || last.Turn != 1 {
		t.Errorf("after Retry: %q in turn %d", texts(ms[len(ms)-1:]), last.Turn)
	}
	h.stop()

	h = newHarness(t, root, h.pid, testSettings())
	defer h.stop()
	if err := h.o.Retry(context.Background(), h.pid, c.ID); !errors.Is(err, ErrNoRetry) {
		t.Errorf("Retry after a good turn and a restart: %v", err)
	}
}

// A provider paused by an overload shows that kind, and the wait goes
// away when the pause ends, before the answer is done.
func TestOverloadPause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(&provider.Error{Kind: provider.Overloaded, Status: 529, RetryAfter: time.Minute}))
		for range h.models.Stream(context.Background(), limit.Background, provider.Request{Model: "default"}) {
		}
		slow := fake.Text("done")
		slow.Wait = time.Second
		h.fp.Push(slow)
		start := time.Now()
		h.send(c.ID, "price?")
		synctest.Wait()
		rs := h.pub.retries()
		if len(rs) != 1 || rs[0].Kind != "overloaded" || !rs[0].At.Equal(start.Add(time.Minute)) {
			t.Fatalf("retries %+v, want one overloaded wait of a minute", rs)
		}
		time.Sleep(time.Minute + time.Millisecond)
		synctest.Wait()
		st := h.pub.allStatuses()
		if last := st[len(st)-1]; last.Retry != nil || last.State != chat.StateWorking {
			t.Errorf("status after the pause %+v", last)
		}
		h.wait()
	})
}

func TestGiveUpWhenTheWaitIsTooLong(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.Fail(rateLimited(9*time.Minute)), fake.Fail(rateLimited(9*time.Minute)))
		start := time.Now()
		h.turn(c.ID, "price?")
		if d := time.Since(start); d != 9*time.Minute {
			t.Errorf("gave up after %s, want 9 minutes", d)
		}
		ms := h.messages(c.ID)
		if n := ms[len(ms)-1].Parts[0].Notice; n == nil || !strings.Contains(n.Text, "p is rate limited; retried for 9 minutes.") {
			t.Errorf("last message %+v", ms[len(ms)-1].Parts)
		}
	})
}

func TestStopped(t *testing.T) {
	text := func(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }
	thinking := chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "plan"}}
	call := chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "x", Name: "echo"}}
	for _, tc := range []struct {
		name  string
		parts []chat.Part
		kind  chat.PartKind
		text  string
		want  []string // * marks stopped
	}{
		{"cut-off text", []chat.Part{text("A")}, chat.PartText, "B", []string{"A", "B*"}},
		{"after a finished text", []chat.Part{thinking, text("A"), call}, chat.PartThinking, "hmm", []string{"A*"}},
		{"blank", []chat.Part{text(" "), text("A")}, chat.PartText, " \n", []string{"A*"}},
		{"nothing", []chat.Part{thinking}, "", "", nil},
	} {
		resp := &response{parts: tc.parts, kind: tc.kind}
		resp.text.WriteString(tc.text)
		var got []string
		for _, p := range resp.stopped() {
			s := p.Text.Text
			if p.Text.Stopped {
				s += "*"
			}
			got = append(got, s)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
		for _, p := range tc.parts {
			if p.Text != nil && p.Text.Stopped {
				t.Errorf("%s: the answer's part was changed", tc.name)
			}
		}
	}
}

func TestLive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		if l := h.o.Live(h.pid, c.ID); l.Running {
			t.Errorf("Live before any turn %+v", l)
		}
		h.fp.Push(fake.Reply{Events: []provider.Event{
			{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "Looking"},
			{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "Looking"}}},
			{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "The pri"},
		}, Hang: true})
		h.send(c.ID, "price?")
		synctest.Wait()
		st := h.pub.allStatuses()
		l := h.o.Live(h.pid, c.ID)
		if !l.Running || l.Turn != 1 || l.Streaming == "" || l.Streaming != st[len(st)-1].Streaming ||
			len(l.Parts) != 1 || l.Parts[0].Text.Text != "Looking" || l.Kind != chat.PartText || l.Text != "The pri" {
			t.Errorf("Live %+v", l)
		}
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if l2 := h.o.Live(h.pid, c.ID); l2.Running || l2.Streaming != "" {
			t.Errorf("Live after Stop %+v", l2)
		}
		st = h.pub.allStatuses()
		if last := st[len(st)-1]; last.Streaming != "" {
			t.Errorf("last status %+v", last)
		}
		if ms := h.messages(c.ID); ms[1].ID != l.Streaming {
			t.Errorf("stored answer %s, streamed %s", ms[1].ID, l.Streaming)
		}
	})
}

func TestClearDuringTitle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{})
		slow := fake.Text("Prices")
		slow.Wait = time.Second
		h.fp.Push(fake.Text("answer"), slow)
		h.send(c.ID, "one")
		synctest.Wait() // the title streams
		if err := h.o.Clear(context.Background(), h.pid, c.ID); err != nil {
			t.Fatal(err)
		}
		h.wait()
		h.open(func(p *project.Project) {
			ch, err := p.Chats.Chat(context.Background(), c.ID)
			if err != nil || ch.Title != "" {
				t.Errorf("title %q (%v): a title for a cleared chat", ch.Title, err)
			}
		})
	})
}

func TestDelete(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "block", struct{}{}))
	h.send(c.ID, "wait")
	<-started
	if err := h.o.Delete(context.Background(), h.pid, c.ID); !errors.Is(err, ErrInTurn) {
		t.Errorf("Delete during a turn: %v", err)
	}
	close(release)
	h.wait()
	if err := h.o.Delete(context.Background(), h.pid, c.ID); err != nil {
		t.Fatal(err)
	}
	h.open(func(p *project.Project) {
		if _, err := p.Chats.Chat(context.Background(), c.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("deleted chat: %v", err)
		}
	})
	if h.o.find(h.pid, c.ID) != nil {
		t.Error("the deleted chat's state is kept")
	}
	if err := h.o.Delete(context.Background(), h.pid, h.mother().ID); err == nil {
		t.Error("Mother was deleted")
	}
}

func TestWrote(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.turn(c.ID, "one")
	var seq uint64
	h.open(func(p *project.Project) {
		var err error
		if seq, err = p.Chats.SetRole(context.Background(), c.ID, "a poet", id.SourceUser); err != nil {
			t.Fatal(err)
		}
	})
	if l := h.o.Live(h.pid, c.ID); l.Seq >= seq {
		t.Fatalf("seq %d before Wrote, the role change has %d", l.Seq, seq)
	}
	h.o.Wrote(h.pid, c.ID, seq)
	if l := h.o.Live(h.pid, c.ID); l.Seq != seq {
		t.Errorf("seq %d after Wrote, want %d", l.Seq, seq)
	}
}

func TestCancelDuringRetryWait(t *testing.T) {
	for _, how := range []string{"stop", "app"} {
		t.Run(how, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root := t.TempDir()
				h := newHarness(t, root, "", testSettings())
				c := h.newChat(chat.Chat{Title: "Prices"})
				h.fp.Push(fake.Fail(rateLimited(time.Minute)))
				start := time.Now()
				h.send(c.ID, "price?")
				synctest.Wait()
				if how == "stop" {
					h.o.Stop(h.pid, c.ID)
					h.wait()
					st := h.pub.allStatuses()
					if last := st[len(st)-1]; last.Retry != nil || last.State != chat.StateIdle {
						t.Errorf("last status %+v", last)
					}
					if err := h.o.Retry(context.Background(), h.pid, c.ID); !errors.Is(err, ErrNoRetry) {
						t.Errorf("Retry after Stop: %v", err)
					}
				}
				h.stop()
				if d := time.Since(start); d != 0 {
					t.Errorf("the turn ended after %s", d)
				}
				h = newHarness(t, root, h.pid, testSettings())
				defer h.stop()
				if got := texts(h.messages(c.ID)); !slices.Equal(got, []string{"user: price?"}) {
					t.Errorf("stored %q, want no error card", got)
				}
			})
		})
	}
}

func TestAnswerCutNotices(t *testing.T) {
	half := chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "Half an"}}
	plan := chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "plan"}}
	for _, tc := range []struct {
		name   string
		events []provider.Event
		want   []string
	}{
		{"output limit", []provider.Event{{Kind: provider.EventPart, Part: &half}, fake.Done(provider.StopMaxTokens, chat.Usage{})},
			[]string{"assistant: Half an", "user: [the answer was cut off: it reached the model's output limit]"}},
		{"refused", []provider.Event{fake.Done(provider.StopRefused, chat.Usage{})},
			[]string{"user: [the answer stopped: the model or the provider's content filter refused it]"}},
		{"empty", []provider.Event{fake.Done(provider.StopEnd, chat.Usage{})},
			[]string{"user: [the model sent an empty answer]"}},
		{"thinking only", []provider.Event{{Kind: provider.EventPart, Part: &plan}, fake.Done(provider.StopEnd, chat.Usage{})},
			[]string{"assistant: thinking plan", "user: [the model sent an empty answer]"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings())
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "Prices"})
			h.fp.Push(fake.Reply{Events: tc.events})
			h.turn(c.ID, "price?")
			ms := h.messages(c.ID)
			if got := texts(ms); !slices.Equal(got, append([]string{"user: price?"}, tc.want...)) {
				t.Errorf("stored %q", got)
			}
			if n := ms[len(ms)-1].Parts[0].Notice; n == nil || n.Kind != chat.NoticeAnswerCut {
				t.Errorf("last part %+v", ms[len(ms)-1].Parts[0])
			}
			validHistory(t, ms)
		})
	}
}

// Block 1 is built at a cut and kept until the next one, so a role change
// does not break the provider's cache (3.1).
func TestRoleChangeWaitsForTheCut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices", Role: "a translator"})
		h.turn(c.ID, "one")
		h.open(func(p *project.Project) {
			if _, err := p.Chats.SetRole(context.Background(), c.ID, "a poet", id.SourceUser); err != nil {
				t.Fatal(err)
			}
		})
		h.turn(c.ID, "two")
		calls := h.fp.Calls()
		if jsonOf(t, calls[1].System) != jsonOf(t, calls[0].System) || !strings.Contains(calls[1].System[0].Text, "a translator") {
			t.Errorf("block 1 changed between cuts: %q", calls[1].System[0].Text)
		}
		time.Sleep(cacheIdle + time.Second)
		h.turn(c.ID, "three")
		if s := h.fp.Calls()[2].System[0].Text; !strings.Contains(s, "a poet") {
			t.Errorf("after the cut block 1 is %q", s)
		}
	})
}

// A turn that ends while the last title request still runs starts no
// second one.
func TestOneTitleAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{})
		slowTitle := fake.Text("Prices")
		slowTitle.Wait = time.Second
		h.fp.Push(fake.Text("one answer"), slowTitle, fake.Text("two answer"))
		h.send(c.ID, "one")
		synctest.Wait() // the title request is streaming
		h.turn(c.ID, "two")
		if n := len(h.fp.Calls()); n != 3 {
			t.Errorf("%d requests, want 3: two answers and one title", n)
		}
		h.open(func(p *project.Project) {
			if ch, err := p.Chats.Chat(context.Background(), c.ID); err != nil || ch.Title != "Prices" {
				t.Errorf("title %q (%v)", ch.Title, err)
			}
		})
	})
}

// The title is background work: it waits for a background slot, while
// the turn does not.
func TestTitleIsBackgroundWork(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{})
	var releases []func()
	for range h.set.LLM.MaxParallelCalls {
		release, err := h.gate.Acquire(context.Background(), limit.Background)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	h.send(c.ID, "one")
	for h.o.Live(h.pid, c.ID).Running {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(h.fp.Calls()); n != 1 {
		t.Errorf("%d requests while the background slots are taken, want only the turn's", n)
	}
	for _, release := range releases {
		release()
	}
	h.wait()
	if n := len(h.fp.Calls()); n != 2 {
		t.Errorf("%d requests, want the title's too", n)
	}
}
