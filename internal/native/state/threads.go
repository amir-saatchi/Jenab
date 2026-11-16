package state

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// ThreadAPI is what Threads reads; *app.ChatService has it.
type ThreadAPI interface {
	Snapshot(ctx context.Context, p id.Project, c id.Chat) (app.ChatSnapshot, error)
	Messages(ctx context.Context, p id.Project, c id.Chat, from, to int) ([]chat.Message, error)
}

const (
	olderTurns = 10 // turns read per scroll up
	keptChats  = 20 // threads kept: the chats opened last
)

// retryWaits are the waits before a failed first read is tried again;
// tests shorten them.
var retryWaits = []time.Duration{500 * time.Millisecond, 2 * time.Second}

// Threads holds the open chats' threads and streams (state/thread.ts and
// stream.ts). Only the chat opened last is shown.
type Threads struct {
	Threads map[id.Chat]*Thread
	Failed  map[id.Chat]string // chats whose first read failed, with the error
	// Drafts are the composer's text per chat, kept while another chat is
	// open. Fills put text in a chat's composer from outside it, as *Edit
	// and resend* does (TASK-018); the composer takes it and calls Take.
	Drafts map[id.Chat]string
	Fills  map[id.Chat]string

	api     ThreadAPI
	post    Post
	onError func(error)
	streams map[id.Chat]*Stream
	reading map[id.Chat]*readRun // reads in flight; events that come meanwhile wait in queue
	older   map[id.Chat]bool
	recent  []id.Chat
}

type readRun struct {
	queue []Event
	again bool
	then  []func(error)
}

// NewThreads makes the store. onError gets the errors of work no caller
// waits for, as reading a chat again after a missed event.
func NewThreads(api ThreadAPI, post Post, onError func(error)) *Threads {
	if onError == nil {
		onError = func(error) {}
	}
	return &Threads{
		Threads: map[id.Chat]*Thread{}, Failed: map[id.Chat]string{}, Drafts: map[id.Chat]string{}, Fills: map[id.Chat]string{},
		api: api, post: post, onError: onError,
		streams: map[id.Chat]*Stream{}, reading: map[id.Chat]*readRun{}, older: map[id.Chat]bool{},
	}
}

// Stream is the chat's answer streaming now, or nil.
func (s *Threads) Stream(c id.Chat) *Stream { return s.streams[c] }

// read reads the chat again and replays the events that came meanwhile.
// A read already running reads once more instead; then, if given, runs
// when the read is done.
func (s *Threads) read(p id.Project, c id.Chat, then func(error)) {
	if r := s.reading[c]; r != nil {
		r.again = true
		if then != nil {
			then(nil)
		}
		return
	}
	run := &readRun{}
	if then != nil {
		run.then = append(run.then, then)
	}
	s.reading[c] = run
	s.readOnce(p, c, run)
}

func (s *Threads) readOnce(p id.Project, c id.Chat, run *readRun) {
	run.again = false
	go func() {
		snap, err := s.api.Snapshot(background, p, c)
		s.post(func() {
			if err == nil {
				live := FromLive(snap.Live)
				s.Threads[c] = FromSnapshot(s.Threads[c], p, snap)
				s.streams[c] = live
				queue := run.queue
				run.queue = nil
				for _, e := range Fresh(queue, live) {
					s.apply(e)
				}
				if run.again {
					s.readOnce(p, c, run)
					return
				}
			}
			delete(s.reading, c)
			for _, fn := range run.then {
				fn(err)
			}
		})
	}()
}

func (s *Threads) report(err error) {
	if err != nil {
		s.onError(err)
	}
}

func (s *Threads) apply(e Event) {
	switch {
	case e.Delta != nil:
		if t := s.Threads[e.Delta.Chat]; t != nil && t.Stream == e.Delta.Message {
			s.streams[e.Delta.Chat] = AddDelta(s.streams[e.Delta.Chat], *e.Delta)
		}
	case e.Part != nil:
		t := s.Threads[e.Part.Chat]
		if t == nil {
			return
		}
		next := WithPart(t, *e.Part)
		if next == nil {
			s.read(t.Project, t.Chat.ID, s.report)
			return
		}
		s.endStream(e.Part.Chat, e.Part.Message)
		s.Threads[e.Part.Chat] = next
	case e.Status != nil:
		t := s.Threads[e.Status.Chat]
		if t == nil {
			return
		}
		next := WithStatus(t, *e.Status)
		if next == t {
			return
		}
		if next.Stream == "" {
			s.endStream(t.Chat.ID, "")
		}
		s.Threads[t.Chat.ID] = next
		// The turn's end: read the stored messages, with their turn, model
		// and time.
		if t.State != chat.StateIdle && next.State == chat.StateIdle {
			s.read(t.Project, t.Chat.ID, s.report)
		}
	}
}

// endStream drops the chat's stream when it is message, or any stream when
// message is empty: the answer was stored, or its try failed.
func (s *Threads) endStream(c id.Chat, message id.Message) {
	if st := s.streams[c]; st != nil && (message == "" || st.Message == message) {
		s.streams[c] = nil
	}
}

func (s *Threads) event(c id.Chat, e Event) {
	if r := s.reading[c]; r != nil {
		r.queue = append(r.queue, e)
		return
	}
	s.apply(e)
}

// Delta applies a chat:delta event.
func (s *Threads) Delta(d chat.Delta) { s.event(d.Chat, Event{Delta: &d}) }

// Part applies a chat:part event.
func (s *Threads) Part(e chat.PartDone) { s.event(e.Chat, Event{Part: &e}) }

// Status applies a chat:status event.
func (s *Threads) Status(st chat.Status) { s.event(st.Chat, Event{Status: &st}) }

// Open reads a chat the first time it is shown, trying again a few times;
// threads of chats not opened lately are dropped. The channel gets the
// result once it is applied.
func (s *Threads) Open(p id.Project, c id.Chat) <-chan error {
	done := make(chan error, 1)
	s.recent = append(slices.DeleteFunc(s.recent, func(r id.Chat) bool { return r == c }), c)
	if n := len(s.recent) - keptChats; n > 0 {
		old := slices.Clone(s.recent[:n])
		s.recent = s.recent[n:]
		for _, o := range old {
			s.Forget(o, false)
		}
	}
	if s.Threads[c] != nil {
		done <- nil
		return done
	}
	delete(s.Failed, c)
	s.tryOpen(p, c, 0, done)
	return done
}

func (s *Threads) tryOpen(p id.Project, c id.Chat, i int, done chan error) {
	s.read(p, c, func(err error) {
		if err == nil {
			done <- nil
			return
		}
		if i >= len(retryWaits) {
			s.Failed[c] = errorText(err)
			done <- err
			return
		}
		time.AfterFunc(retryWaits[i], func() {
			s.post(func() {
				if s.Threads[c] != nil {
					done <- nil
					return
				}
				s.tryOpen(p, c, i+1, done)
			})
		})
	})
}

// Older reads the turns before the first one loaded, when the user
// scrolls up to them.
func (s *Threads) Older(c id.Chat) <-chan error {
	done := make(chan error, 1)
	t := s.Threads[c]
	if t == nil || t.From <= 1 || s.older[c] {
		done <- nil
		return done
	}
	s.older[c] = true
	from := max(1, t.From-olderTurns)
	go func() {
		ms, err := s.api.Messages(background, t.Project, c, from, t.From-1)
		s.post(func() {
			delete(s.older, c)
			if cur := s.Threads[c]; err == nil && cur != nil && cur.From == t.From {
				n := *cur
				n.Messages = append(Share(nil, ms), cur.Messages...)
				n.From = from
				s.Threads[c] = &n
			}
			done <- err
		})
	}()
	return done
}

// Joined marks a message sent during a turn. Send returns after the events
// it caused, so the turn may have ended, or a new try taken the message, by
// then: message IDs grow, and a try's answer made after the message took
// it.
func (s *Threads) Joined(c id.Chat, m id.Message) {
	t := s.Threads[c]
	if t == nil || t.State == chat.StateIdle || slices.Contains(t.Joining, m) ||
		slices.ContainsFunc(t.Streamed, func(a id.Message) bool { return a > m }) {
		return
	}
	n := *t
	n.Joining = append(slices.Clone(t.Joining), m)
	s.Threads[c] = &n
}

// SetChat replaces an open chat's row, as after a rename.
func (s *Threads) SetChat(c chat.Chat) {
	if t := s.Threads[c.ID]; t != nil {
		n := *t
		n.Chat = c
		s.Threads[c.ID] = &n
	}
}

// Reload reads an open chat again after a rewind or its undo, which change
// the chat without events.
func (s *Threads) Reload(c id.Chat) <-chan error {
	done := make(chan error, 1)
	t := s.Threads[c]
	if t == nil {
		done <- nil
		return done
	}
	s.read(t.Project, c, func(err error) { done <- err })
	return done
}

// Forget drops a chat's thread and stream; a deleted chat's draft too.
func (s *Threads) Forget(c id.Chat, deleted bool) {
	delete(s.streams, c)
	if deleted {
		delete(s.Drafts, c)
		s.recent = slices.DeleteFunc(s.recent, func(r id.Chat) bool { return r == c })
	}
	delete(s.Threads, c)
	delete(s.Failed, c)
}

// Gone forgets the project's chats a new chat list no longer has, but the
// one shown, which the list may predate.
func (s *Threads) Gone(p id.Project, chats []id.Chat) {
	var shown id.Chat
	if n := len(s.recent); n > 0 {
		shown = s.recent[n-1]
	}
	for _, c := range slices.Collect(maps.Keys(s.Threads)) {
		if t := s.Threads[c]; t.Project == p && !slices.Contains(chats, c) && c != shown {
			s.Forget(c, true)
		}
	}
}

// Fill puts text in a chat's composer.
func (s *Threads) Fill(c id.Chat, text string) { s.Fills[c] = text }

// Take is the composer taking its fill.
func (s *Threads) Take(c id.Chat) { delete(s.Fills, c) }
