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
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// windowStart is the first turn of the window at the start of turn n with
// min 3 and max 8 (3.6): the first turn cuts, then a cut whenever more
// than 8 earlier turns are in the window.
func windowStart(n int) int {
	start := 1
	for t := 2; t <= n; t++ {
		if t-start > 8 {
			start = t - 3
		}
	}
	return start
}

func TestThirtyTurnsCut(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), big())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	// Turn 8 reads a big result, so the window holds a preview until the
	// cut at turn 10 makes it a stub.
	for n := 1; n <= 30; n++ {
		if n == 8 {
			h.fp.Push(fake.ToolCall("call-8", "big", map[string]int{"n": 20000}), fake.Text("read it"))
		}
		h.turn(c.ID, fmt.Sprintf("message %d", n))
	}
	calls := h.fp.Calls()
	if len(calls) != 31 {
		t.Fatalf("%d requests, want 31", len(calls))
	}
	var prev provider.Request
	prevN := 0
	for i, req := range calls {
		n := i + 1
		if i >= 8 {
			n = i // turn 8 has two requests
		}
		start := windowStart(n)
		want := []int{}
		for x := start; x <= n; x++ {
			want = append(want, x)
		}
		if got := turnsOf(req.Messages); !slices.Equal(got, want) {
			t.Errorf("turn %d: turns %v, want %v", n, got, want)
		}
		last := req.System[len(req.System)-1]
		line := strings.Contains(last.Text, "earlier turns")
		if wantLine := start > 1; line != wantLine || !last.Cache {
			t.Errorf("turn %d: last block %q (cache %v), want the earlier-turns line %v", n, last.Text, last.Cache, wantLine)
		}
		if start > 1 && !strings.Contains(last.Text, fmt.Sprintf("has %d earlier turns", start-1)) {
			t.Errorf("turn %d: %q", n, last.Text)
		}
		// Between cuts, a request only grows at the end (3.1).
		if i > 0 && windowStart(n) == windowStart(prevN) {
			if !strings.HasPrefix(jsonOf(t, req.Messages), strings.TrimSuffix(jsonOf(t, prev.Messages), "]")) || jsonOf(t, req.System) != jsonOf(t, prev.System) {
				t.Errorf("request %d (turn %d): the request changed before its end", i, n)
			}
		}
		prev, prevN = req, n
	}
	// The big result is a preview while turn 8 is in the window, and a stub
	// after the cut.
	result := func(req provider.Request) string {
		for _, m := range req.Messages {
			for _, p := range m.Parts {
				if p.ToolResult != nil {
					return p.ToolResult.Text
				}
			}
		}
		return ""
	}
	preview, stub := result(calls[9]), result(calls[10]) // turns 9 and 10
	if !strings.Contains(preview, "showing first") || !strings.Contains(preview, "row of data") {
		t.Errorf("turn 9 sees %q, want the preview", preview)
	}
	if !strings.HasPrefix(stub, "[output of big — ref: cache/tool/") || strings.Contains(stub, "\n") {
		t.Errorf("turn 10 sees %q, want the stub", stub)
	}
	ms := h.messages(c.ID)
	if len(ms) != 62 {
		t.Errorf("%d messages stored, want 62", len(ms))
	}
	validHistory(t, ms)
}

func TestMessageDuringTurnJoinsNextStep(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "block", struct{}{}), fake.Text("both done"))
	h.send(c.ID, "first")
	<-started
	h.open(func(p *project.Project) {
		if w := p.Activity().Work; len(w) != 1 || w[0].ID != string(c.ID) || w[0].Kind != "turn" || w[0].Progress != "turn 1, request 1" {
			t.Errorf("activity %+v", w)
		}
	})
	if s := h.o.ChatStatus(c.ID); s != "in a turn" {
		t.Errorf("ChatStatus %q", s)
	}
	h.send(c.ID, "second") // while the tool runs
	close(release)
	h.wait()

	calls := h.fp.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d requests, want 2", len(calls))
	}
	want := []string{"user: first", "assistant: call block", "tool: result released", "user: second"}
	if got := texts(calls[1].Messages); !slices.Equal(got, want) {
		t.Errorf("second request:\n%q\nwant\n%q", got, want)
	}
	ms := h.messages(c.ID)
	if got := turnsOf(ms); !slices.Equal(got, []int{1}) {
		t.Errorf("turns %v, want one turn", got)
	}
	if got := texts(ms); !slices.Equal(got, append(want, "assistant: both done")) {
		t.Errorf("stored %q", got)
	}
	validHistory(t, ms)
}

func TestMessageDuringAnswerGetsAnotherStep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		slow := fake.Text("first answer")
		slow.Wait = time.Second
		h.fp.Push(slow, fake.Text("second answer"))
		h.send(c.ID, "first")
		time.Sleep(time.Second + time.Millisecond) // the answer is streaming
		h.send(c.ID, "second")
		h.wait()
		want := []string{"user: first", "assistant: first answer", "user: second", "assistant: second answer"}
		ms := h.messages(c.ID)
		if got := texts(ms); !slices.Equal(got, want) {
			t.Errorf("stored %q\nwant %q", got, want)
		}
		if got := turnsOf(ms); !slices.Equal(got, []int{1}) {
			t.Errorf("turns %v, want one turn", got)
		}
		if n := len(h.fp.Calls()); n != 2 {
			t.Errorf("%d requests, want 2", n)
		}
	})
}

func TestStopLeavesValidHistory(t *testing.T) {
	t.Run("while streaming", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings(), echo())
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "Prices"})
			h.fp.Push(fake.Reply{Events: []provider.Event{
				{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "plan", Signature: "s"}}},
				{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "Looking"},
				{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "Looking"}}},
				{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "x", Name: "echo", Args: []byte(`{}`)}}},
				{Kind: provider.EventDelta, PartKind: chat.PartThinking, Text: "hmm"},
				{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "The price is"},
			}, Hang: true})
			h.send(c.ID, "price?")
			synctest.Wait()
			h.o.Stop(h.pid, c.ID)
			h.wait()
			ms := h.messages(c.ID)
			want := []string{"user: price?", "assistant: Looking", "assistant: The price is"}
			if got := texts(ms); !slices.Equal(got, want) {
				t.Fatalf("stored %q\nwant %q", got, want)
			}
			if p := ms[1].Parts[1].Text; !p.Stopped || ms[1].Parts[0].Text.Stopped {
				t.Errorf("only the cut-off text is marked stopped: %+v", ms[1].Parts)
			}
			h.turn(c.ID, "go on")
			validHistory(t, h.fp.Calls()[1].Messages)
		})
	})
	t.Run("while a tool runs", func(t *testing.T) {
		started, release := make(chan string, 1), make(chan struct{})
		h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release), echo())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		two := fake.ToolCall("c1", "block", struct{}{})
		two.Events = slices.Insert(two.Events, 1, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall,
			ToolCall: &chat.ToolCall{ID: "c2", Name: "echo", Args: []byte(`{"text": "hi"}`)}}})
		h.fp.Push(two)
		h.send(c.ID, "price?")
		<-started
		h.o.Stop(h.pid, c.ID)
		h.wait()
		ms := h.messages(c.ID)
		want := []string{"user: price?", "assistant: call block", "assistant: call echo", "tool: result cancelled by user", "tool: result cancelled by user"}
		if got := texts(ms); !slices.Equal(got, want) {
			t.Fatalf("stored %q\nwant %q", got, want)
		}
		for _, p := range ms[2].Parts {
			if !p.ToolResult.IsError {
				t.Errorf("a cancelled result is not an error: %+v", p.ToolResult)
			}
		}
		validHistory(t, ms)
		h.turn(c.ID, "go on")
		validHistory(t, h.fp.Calls()[1].Messages)
		if st := h.pub.allStatuses(); st[len(st)-1].State != chat.StateIdle {
			t.Errorf("last status %+v", st[len(st)-1])
		}
	})
}

func TestRestartKeepsHistory(t *testing.T) {
	root := t.TempDir()
	h := newHarness(t, root, "", testSettings(), big())
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.turn(c.ID, "one")
	h.fp.Push(fake.ToolCall("c1", "big", map[string]int{"n": 20000}), fake.Text("read it"))
	h.turn(c.ID, "two")
	// A crash between the tool call and its result: the call has none.
	h.open(func(p *project.Project) {
		_, _, err := p.Chats.AppendMessage(context.Background(), chat.Message{Chat: c.ID, Turn: 2, Role: chat.RoleAssistant,
			Parts: []chat.Part{{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "lost", Name: "big", Args: []byte(`{}`)}}}})
		if err != nil {
			t.Fatal(err)
		}
	})
	before := h.messages(c.ID)
	h.stop()

	h = newHarness(t, root, h.pid, testSettings(), big())
	defer h.stop()
	h.turn(c.ID, "three")
	ms := h.messages(c.ID)
	if got, want := texts(ms[:len(before)]), texts(before); !slices.Equal(got, want) {
		t.Errorf("history after the restart:\n%q\nwant\n%q", got, want)
	}
	req := h.fp.Calls()[0]
	if got := turnsOf(req.Messages); !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("turns in the first request %v", got)
	}
	validHistory(t, req.Messages)
	got := texts(req.Messages)
	if !slices.Contains(got, "tool: result no result: the app closed before this call finished") {
		t.Errorf("the lost call has no result: %q", got)
	}
	// The restart cut the window: the preview of the turn before is a stub.
	if r := got[4]; !strings.HasPrefix(r, "tool: result [output of big — ref: ") || strings.Contains(r, "showing") {
		t.Errorf("old result %q, want a stub", r)
	}
}

func TestClear(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	for n := 1; n <= 12; n++ {
		h.turn(c.ID, fmt.Sprint("message ", n))
	}
	h.fp.Push(fake.ToolCall("c1", "block", struct{}{}))
	h.send(c.ID, "wait")
	<-started
	if err := h.o.Clear(context.Background(), h.pid, c.ID); !errors.Is(err, ErrInTurn) {
		t.Errorf("Clear during a turn: %v", err)
	}
	close(release)
	h.wait()
	if err := h.o.Clear(context.Background(), h.pid, c.ID); err != nil {
		t.Fatal(err)
	}
	h.turn(c.ID, "fresh start")
	calls := h.fp.Calls()
	req := calls[len(calls)-1]
	if got := texts(req.Messages); !slices.Equal(got, []string{"user: fresh start"}) {
		t.Errorf("request after Clear: %q", got)
	}
	if last := req.System[len(req.System)-1].Text; strings.Contains(last, "earlier turn") {
		t.Errorf("the context counts earlier turns after Clear: %q", last)
	}
	if ms := h.messages(c.ID); len(ms) != 2 || ms[0].Turn != 1 {
		t.Errorf("after Clear: %d messages, first in turn %d", len(ms), ms[0].Turn)
	}
}

// A turn's runner names the chat after the turn has ended, so the next
// turn can start before the title is done. The old runner must leave the
// new one's state alone.
func TestTitleWhileNextTurnRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, release := make(chan string, 1), make(chan struct{})
		h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
		defer h.stop()
		c := h.newChat(chat.Chat{})
		slowTitle := fake.Text("Prices")
		slowTitle.Wait = time.Second
		h.fp.Push(fake.Text("answer"), slowTitle, fake.ToolCall("c1", "block", struct{}{}), fake.Text("done"))
		h.send(c.ID, "one")
		synctest.Wait() // the title request is streaming
		h.send(c.ID, "two")
		<-started
		h.open(func(p *project.Project) {
			if w := p.Activity().Work; len(w) != 1 {
				t.Errorf("activity %+v, want the turn once", w)
			}
		})
		time.Sleep(4 * time.Second) // the title (three events, a second each) is done and the first runner ends
		synctest.Wait()
		if s := h.o.ChatStatus(c.ID); s != "in a turn" {
			t.Errorf("ChatStatus %q while the second turn runs", s)
		}
		close(release)
		h.wait()
		if st := h.pub.allStatuses(); st[len(st)-1].State != chat.StateIdle {
			t.Errorf("last status %+v", st[len(st)-1])
		}
	})
}

// The app shuts down, or closes the project while a turn runs (shutdown
// after Wait timed out): the turn closes its open calls before the
// databases close, and no error card is written.
func TestShutdownMidTurn(t *testing.T) {
	for _, how := range []string{"app", "project"} {
		t.Run(how, func(t *testing.T) {
			root := t.TempDir()
			started, release := make(chan string, 1), make(chan struct{})
			h := newHarness(t, root, "", testSettings(), blocker(started, release))
			c := h.newChat(chat.Chat{Title: "Prices"})
			h.fp.Push(fake.ToolCall("c1", "block", struct{}{}))
			h.send(c.ID, "wait")
			<-started
			if how == "project" {
				start := time.Now()
				if err := h.pm.CloseAll(context.Background()); err != nil {
					t.Fatal(err)
				}
				if d := time.Since(start); d >= closeWait {
					t.Errorf("the close took %s: it did not see the turn end", d)
				}
				h.wait()
			}
			h.stop()
			h = newHarness(t, root, h.pid, testSettings())
			defer h.stop()
			want := []string{"user: wait", "assistant: call block", "tool: result cancelled: the app closed"}
			if how == "project" {
				want[2] = "tool: result cancelled: the project closed"
			}
			if got := texts(h.messages(c.ID)); !slices.Equal(got, want) {
				t.Errorf("stored %q\nwant %q", got, want)
			}
		})
	}
}

func TestBumpSeq(t *testing.T) {
	var cs chatState
	for _, s := range []uint64{3, 7, 5, 7, 9} {
		cs.bumpSeq(s)
	}
	if got := cs.seq.Load(); got != 9 {
		t.Errorf("seq %d, want 9", got)
	}
	cs.bumpSeq(2)
	if got := cs.seq.Load(); got != 9 {
		t.Errorf("seq went down to %d", got)
	}
}
