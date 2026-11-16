package state

// Ported from frontend/src/state/thread-store.test.ts.

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// fakeChats answers Snapshot with the next of snaps, and Messages with
// messages.
type fakeChats struct {
	mu       sync.Mutex
	snaps    []func() (app.ChatSnapshot, error)
	messages []chat.Message
}

func (f *fakeChats) push(fns ...func() (app.ChatSnapshot, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snaps = append(f.snaps, fns...)
}

func (f *fakeChats) Snapshot(context.Context, id.Project, id.Chat) (app.ChatSnapshot, error) {
	f.mu.Lock()
	if len(f.snaps) == 0 {
		f.mu.Unlock()
		return app.ChatSnapshot{}, errors.New("no snapshot")
	}
	fn := f.snaps[0]
	f.snaps = f.snaps[1:]
	f.mu.Unlock()
	return fn()
}

func (f *fakeChats) Messages(_ context.Context, _ id.Project, _ id.Chat, from, to int) ([]chat.Message, error) {
	var out []chat.Message
	for _, m := range f.messages {
		if m.Turn >= from && m.Turn <= to {
			out = append(out, m)
		}
	}
	return out, nil
}

// later is a snapshot answer that waits for the test to give it.
func later() (func() (app.ChatSnapshot, error), chan<- app.ChatSnapshot) {
	ch := make(chan app.ChatSnapshot, 1)
	return func() (app.ChatSnapshot, error) { return <-ch, nil }, ch
}

func storeSnap(c string, seq uint64, live agent.Live) app.ChatSnapshot {
	s := snap([]chat.Message{msg("01U1", 1, chat.RoleUser, "go")}, seq, 1, live)
	s.Chat.ID, s.Chat.Title = id.Chat(c), c
	s.Messages[0].Chat = id.Chat(c)
	return s
}

func chatStatus(c string, seq uint64, state chat.State, streaming string) chat.Status {
	s := status(seq, state, streaming)
	s.Chat = id.Chat(c)
	return s
}

func chatDelta(c, message string, p int, text string, offset int) chat.Delta {
	d := delta(message, p, text, chat.PartText, offset)
	d.Chat = id.Chat(c)
	return d
}

func chatPart(c, message string, seq uint64) chat.PartDone {
	e := textPartDone(message, 0, seq, "go")
	e.Chat = id.Chat(c)
	return e
}

func newThreads() (*Threads, *fakeChats, *loop) {
	f, l := &fakeChats{}, newLoop()
	return NewThreads(f, l.post, nil), f, l
}

func TestOpenReplaysTheEventsThatCameDuringTheRead(t *testing.T) {
	s, f, l := newThreads()
	answer, give := later()
	f.push(answer)
	opening := s.Open("p1", "r1")
	// Live was read at 4, the messages at 5: the status at 4 came between.
	s.Status(chatStatus("r1", 4, chat.StateWorking, "01A1"))
	s.Part(chatPart("r1", "01U1", 5))
	s.Delta(chatDelta("r1", "01A1", 0, "Hello", 0))
	s.Delta(chatDelta("r1", "01A1", 0, " there", 5))
	give <- storeSnap("r1", 5, agent.Live{Running: true, Seq: 4})
	if err := await(t, l, opening); err != nil {
		t.Fatal(err)
	}
	th := s.Threads["r1"]
	same(t, th.State, chat.StateWorking)
	same(t, th.Stream, id.Message("01A1"))
	same(t, ids(th.Messages), []string{"01U1"})
	same(t, s.Stream("r1").Text, "Hello there")

	// A read again knows which deltas the snapshot holds from their
	// offsets.
	answer, give = later()
	f.push(answer)
	s.Delta(chatDelta("r1", "01A1", 0, "\n", 11))
	s.Part(chatPart("r1", "01X1", 9))             // a gap: the chat is read again
	s.Delta(chatDelta("r1", "01A1", 0, "\n", 12)) // came after the snapshot read Live
	give <- storeSnap("r1", 9, agent.Live{Running: true, Seq: 9, Streaming: "01A1", Kind: chat.PartText, Text: "Hello there\n"})
	until(t, l, func() bool { return len(s.reading) == 0 })
	same(t, s.Stream("r1").Text, "Hello there\n\n")
}

func shortRetries(t *testing.T) {
	old := retryWaits
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryWaits = old })
}

func TestOpenTriesAFailedFirstReadAgain(t *testing.T) {
	shortRetries(t)
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) { return app.ChatSnapshot{}, errors.New("busy") },
		func() (app.ChatSnapshot, error) { return storeSnap("r2", 1, agent.Live{}), nil })
	if err := await(t, l, s.Open("p1", "r2")); err != nil {
		t.Fatal(err)
	}
	same(t, s.Threads["r2"].Seq, uint64(1))
	if _, ok := s.Failed["r2"]; ok {
		t.Fatal("failed after a good read")
	}
	// Every try failing marks the chat failed.
	if err := await(t, l, s.Open("p1", "r3")); err == nil {
		t.Fatal("no error")
	}
	same(t, s.Failed["r3"], "no snapshot")
}

func TestJoinedSkipsAMessageTheTurnAlreadyTook(t *testing.T) {
	s, _, _ := newThreads()
	s.Threads["j1"] = &Thread{Project: "p1", Chat: chat.Chat{ID: "j1"}, State: chat.StateWorking, Streamed: []id.Message{"01B1"}}
	s.Joined("j1", "01A9") // sent before the try now streaming started
	if len(s.Threads["j1"].Joining) != 0 {
		t.Fatalf("joining %v", s.Threads["j1"].Joining)
	}
	s.Joined("j1", "01C1")
	same(t, s.Threads["j1"].Joining, []id.Message{"01C1"})
	s.Threads["j1"] = &Thread{Project: "p1", Chat: chat.Chat{ID: "j1"}, State: chat.StateIdle}
	s.Joined("j1", "01D1") // the turn ended first
	if len(s.Threads["j1"].Joining) != 0 {
		t.Fatalf("joining %v", s.Threads["j1"].Joining)
	}
}

func TestAChatGoneFromItsListIsForgottenButTheOneShown(t *testing.T) {
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) { return storeSnap("r1", 1, agent.Live{}), nil },
		func() (app.ChatSnapshot, error) { return storeSnap("r2", 1, agent.Live{}), nil })
	await(t, l, s.Open("p1", "r1"))
	await(t, l, s.Open("p1", "r2"))
	th := s.Threads["r1"]
	s.Threads["g1"] = &Thread{Project: "p1", Chat: chat.Chat{ID: "g1"}, Messages: th.Messages}
	s.Threads["g2"] = &Thread{Project: "p2", Chat: chat.Chat{ID: "g2"}}
	s.streams["g1"] = &Stream{Message: "x"}
	s.Drafts["g1"] = "half a thought"
	// r2 was opened last, so it is shown.
	s.Gone("p1", []id.Chat{"r1"})
	var left []string
	for c := range s.Threads {
		left = append(left, string(c))
	}
	slices.Sort(left)
	same(t, left, []string{"g2", "r1", "r2"})
	if _, ok := s.Drafts["g1"]; ok {
		t.Fatal("the gone chat's draft is kept")
	}
	if s.Stream("g1") != nil {
		t.Fatal("the gone chat's stream is kept")
	}
}

func TestOlderReadsTheTurnsBefore(t *testing.T) {
	s, f, l := newThreads()
	for i := 1; i <= 12; i++ {
		f.messages = append(f.messages, msg(string(rune('a'+i)), i, chat.RoleUser, "x"))
	}
	last := storeSnap("o1", 3, agent.Live{})
	last.From, last.Messages = 13, []chat.Message{msg("z", 13, chat.RoleUser, "13")}
	f.push(func() (app.ChatSnapshot, error) { return last, nil })
	await(t, l, s.Open("p1", "o1"))
	if err := await(t, l, s.Older("o1")); err != nil {
		t.Fatal(err)
	}
	th := s.Threads["o1"]
	same(t, th.From, 3)
	same(t, len(th.Messages), 11)
	await(t, l, s.Older("o1"))
	same(t, s.Threads["o1"].From, 1)
	same(t, len(s.Threads["o1"].Messages), 13)
}

func TestTheTurnsEndReadsTheChatAgain(t *testing.T) {
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) {
		return storeSnap("e1", 2, agent.Live{Running: true, Streaming: "01A1", Kind: chat.PartText, Text: "Hi"}), nil
	})
	await(t, l, s.Open("p1", "e1"))
	stored := storeSnap("e1", 4, agent.Live{})
	stored.Messages = append(stored.Messages, msg("01A1", 1, chat.RoleAssistant, "Hi there"))
	f.push(func() (app.ChatSnapshot, error) { return stored, nil })
	s.Status(chatStatus("e1", 3, chat.StateIdle, ""))
	if s.Stream("e1") != nil {
		t.Fatal("the stream outlived its turn")
	}
	until(t, l, func() bool { return len(s.Threads["e1"].Messages) == 2 })
	same(t, s.Threads["e1"].Seq, uint64(4))
}
