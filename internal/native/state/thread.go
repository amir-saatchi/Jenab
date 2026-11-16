package state

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Thread is an open chat's messages and turn state (Q32). Events with a
// sequence number up to SnapSeq are already in Messages; a write that
// skips a number means one was missed, and the chat is read again. The
// turn state was read before the messages, at LiveSeq: a status carries
// the whole state, so one from LiveSeq on is newer than the snapshot's.
//
// A Thread is not changed once made: WithPart and WithStatus return a new
// one, and a message that didn't change keeps its pointer, so a view can
// keep what it built for it.
type Thread struct {
	Project  id.Project
	Chat     chat.Chat
	Messages []*chat.Message
	From     int    // the first turn loaded; 1 means the whole chat
	Seq      uint64 // the last write applied
	SnapSeq  uint64
	LiveSeq  uint64
	State    chat.State
	Waiting  *chat.Waiting
	Retry    *chat.Retry
	Doing    string       // what a running tool call shows, such as a long schema change
	Stream   id.Message   // the answer streaming now, not yet stored (Status.Streaming)
	Streamed []id.Message // answers streamed since the chat was read: the assistant's
	Joining  []id.Message // sent during a turn; they join at its next request
	Rewinds  []store.RewindInfo
}

// Share keeps the old pointer of every message that didn't change, so
// reading a chat again rebuilds only what is new.
func Share(old []*chat.Message, next []chat.Message) []*chat.Message {
	byID := make(map[id.Message]*chat.Message, len(old))
	for _, m := range old {
		byID[m.ID] = m
	}
	out := make([]*chat.Message, len(next))
	for i := range next {
		if o := byID[next[i].ID]; o != nil && sameMessage(o, &next[i]) {
			out[i] = o
		} else {
			m := next[i]
			out[i] = &m
		}
	}
	return out
}

// sameMessage compares two messages as their JSON, as the React store
// does.
func sameMessage(a, b *chat.Message) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ja) == string(jb)
}

// FromSnapshot is the thread a snapshot gives. Older turns loaded before
// are kept: a rewind or its undo changes only turns after the chat's last
// one now, which the snapshot holds.
func FromSnapshot(prev *Thread, p id.Project, snap app.ChatSnapshot) *Thread {
	var older, prevMessages []*chat.Message
	var prevJoining []id.Message
	if prev != nil {
		prevMessages, prevJoining = prev.Messages, prev.Joining
		for _, m := range prev.Messages {
			if m.Turn < snap.From {
				older = append(older, m)
			}
		}
	}
	live := snap.Live
	t := &Thread{
		Project:  p,
		Chat:     snap.Chat,
		Messages: append(older, Share(prevMessages, snap.Messages)...),
		From:     snap.From,
		Seq:      snap.Seq,
		SnapSeq:  snap.Seq,
		LiveSeq:  live.Seq,
		State:    chat.StateIdle,
		Waiting:  live.Waiting,
		Retry:    live.Retry,
		Doing:    live.Doing,
		Stream:   live.Streaming,
		Rewinds:  snap.Rewinds,
	}
	if prev != nil && len(older) > 0 {
		t.From = prev.From
	}
	switch {
	case live.Waiting != nil:
		t.State = chat.StateWaiting
	case live.Running:
		t.State = chat.StateWorking
	}
	if live.Streaming != "" {
		t.Streamed = []id.Message{live.Streaming}
	}
	if live.Running {
		for _, m := range append(slices.Clone(prevJoining), joiningIn(snap.Messages, live)...) {
			if !slices.Contains(t.Joining, m) {
				t.Joining = append(t.Joining, m)
			}
		}
	}
	return t
}

// joiningIn is the user messages of the running turn sent after its last
// answer: they join at its next request, so they show below the answer
// still streaming, as the store will sort them once it is saved.
func joiningIn(messages []chat.Message, live agent.Live) []id.Message {
	var turn []chat.Message
	for _, m := range messages {
		if m.Turn == live.Turn {
			turn = append(turn, m)
		}
	}
	after := live.Streaming
	for _, m := range turn {
		if m.Role == chat.RoleAssistant && m.ID > after {
			after = m.ID
		}
	}
	var out []id.Message
	for i, m := range turn {
		if i == 0 || m.Role != chat.RoleUser || m.ID <= after {
			continue
		}
		if slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.Kind != chat.PartNotice }) {
			out = append(out, m.ID)
		}
	}
	return out
}

// guessRole is a new message's role, guessed from its first part until
// the chat is read again: answers were streamed, tool messages hold
// results and cards, and the app's notices go in user messages.
func guessRole(t *Thread, e chat.PartDone) chat.Role {
	if slices.Contains(t.Streamed, e.Message) {
		return chat.RoleAssistant
	}
	switch e.Part.Kind {
	case chat.PartToolResult, chat.PartApproval, chat.PartQuestion:
		return chat.RoleTool
	case chat.PartThinking, chat.PartToolCall:
		return chat.RoleAssistant
	}
	return chat.RoleUser
}

// guessTurn is a new message's turn, as PartDone gives it. Without one it
// is guessed: a message the user sends while the chat is idle starts the
// next turn, and anything else belongs to the last one.
func guessTurn(t *Thread, e chat.PartDone, r chat.Role) int {
	if e.Turn > 0 {
		return e.Turn
	}
	last := 0
	if n := len(t.Messages); n > 0 {
		last = t.Messages[n-1].Turn
	}
	if r == chat.RoleUser && e.Part.Kind != chat.PartNotice && t.State == chat.StateIdle {
		return last + 1
	}
	return max(last, 1)
}

// sortsBefore tells whether message a comes before b, as the store sorts
// them: by turn, then by ID, and IDs grow in the order they were made. A
// message sent during a turn sorts after the answer and results that were
// being made when it came.
func sortsBefore(a, b *chat.Message) bool {
	if a.Turn != b.Turn {
		return a.Turn < b.Turn
	}
	return a.ID < b.ID
}

// now is when a message made from an event was made; tests set it.
var now = time.Now

// WithPart applies a finished part: it replaces the part at its index or
// adds it, in a new message if needed, at its place in the store's order.
// It returns t for a part the snapshot has, and nil on a gap.
func WithPart(t *Thread, e chat.PartDone) *Thread {
	if e.Seq <= t.SnapSeq {
		return t
	}
	if e.Seq > t.Seq+1 {
		return nil
	}
	messages := slices.Clone(t.Messages)
	i := len(messages) - 1
	for i >= 0 && messages[i].ID != e.Message {
		i--
	}
	if i < 0 {
		r := guessRole(t, e)
		m := &chat.Message{ID: e.Message, Chat: t.Chat.ID, Turn: guessTurn(t, e, r), Role: r, CreatedAt: now().UTC(), Parts: []chat.Part{}}
		i = len(messages)
		for i > 0 && sortsBefore(m, messages[i-1]) {
			i--
		}
		messages = slices.Insert(messages, i, m)
	}
	m := *messages[i]
	parts := slices.Clone(m.Parts)
	for len(parts) <= e.Index {
		parts = append(parts, chat.Part{})
	}
	parts[e.Index] = e.Part
	m.Parts = parts
	messages[i] = &m
	n := *t
	n.Messages, n.Seq = messages, max(t.Seq, e.Seq)
	return &n
}

// WithStatus applies a chat:status event, or returns t for one older than
// the snapshot's Live. A new try takes the messages written during the
// turn, so they no longer join.
func WithStatus(t *Thread, s chat.Status) *Thread {
	if s.Seq < t.LiveSeq {
		return t
	}
	started := s.Streaming != "" && s.Streaming != t.Stream
	n := *t
	n.State, n.Waiting, n.Retry, n.Doing, n.Stream = s.State, s.Waiting, s.Retry, s.Doing, s.Streaming
	if started {
		n.Streamed = append(slices.Clone(t.Streamed), s.Streaming)
	}
	if started || s.State == chat.StateIdle {
		n.Joining = nil
	}
	return &n
}

// Event is one chat event: exactly one field is set.
type Event struct {
	Delta  *chat.Delta
	Part   *chat.PartDone
	Status *chat.Status
}

// Fresh is the events that came while a snapshot was read, less the text
// its stream already holds: a delta's offset says where its text starts in
// the part, so what lies within the snapshot's text is dropped. Parts and
// statuses the snapshot has are dropped by WithPart and WithStatus.
func Fresh(queue []Event, live *Stream) []Event {
	if live == nil {
		return queue
	}
	held := utf16Len(live.Text)
	var out []Event
	for _, e := range queue {
		d := e.Delta
		if d == nil || d.Message != live.Message || d.Part != live.Index || d.Kind != live.Kind {
			out = append(out, e)
			continue
		}
		if d.Offset+utf16Len(d.Text) <= held {
			continue
		}
		if d.Offset >= held {
			out = append(out, e)
			continue
		}
		cut := *d
		cut.Text, cut.Offset = utf16Cut(d.Text, held-d.Offset), held
		out = append(out, Event{Delta: &cut})
	}
	return out
}
