package state

// Ported from frontend/src/state/thread.test.ts. Its tests of rows,
// copyText, lastNotice, firstRows and splitJoining belong to @/chat/rows,
// which ports with the chat screen (Part B).

import (
	"reflect"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

var (
	testTime = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	testChat = chat.Chat{ID: "c1", Kind: chat.KindChat, Title: "Coins", Skills: []string{}, Model: "default", CreatedBy: "user", CreatedAt: testTime}
)

func msg(mid string, turn int, role chat.Role, texts ...string) chat.Message {
	m := chat.Message{ID: id.Message(mid), Chat: "c1", Turn: turn, Role: role, CreatedAt: testTime, Parts: []chat.Part{}}
	for _, t := range texts {
		m.Parts = append(m.Parts, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: t}})
	}
	return m
}

// snap is a snapshot whose Live was read at seq unless live sets it.
func snap(messages []chat.Message, seq uint64, from int, live agent.Live) app.ChatSnapshot {
	if live.Turn == 0 {
		live.Turn = 1
	}
	if live.Seq == 0 {
		live.Seq = seq
	}
	return app.ChatSnapshot{Chat: testChat, Messages: messages, From: from, Seq: seq, Live: live}
}

func part(message string, index int, seq uint64, text string, kind chat.PartKind) chat.PartDone {
	e := chat.PartDone{Project: "p1", Chat: "c1", Seq: seq, Message: id.Message(message), Index: index}
	switch kind {
	case chat.PartText:
		e.Part = chat.Part{Kind: kind, Text: &chat.Text{Text: text}}
	case chat.PartToolResult:
		e.Part = chat.Part{Kind: kind, ToolResult: &chat.ToolResult{CallID: "k1", Text: text}}
	default:
		e.Part = chat.Part{Kind: kind, Notice: &chat.Notice{Kind: chat.NoticeTurnFailed, Text: text}}
	}
	return e
}

func textPartDone(message string, index int, seq uint64, text string) chat.PartDone {
	return part(message, index, seq, text, chat.PartText)
}

func status(seq uint64, state chat.State, streaming string) chat.Status {
	return chat.Status{Project: "p1", Chat: "c1", Seq: seq, State: state, Streaming: id.Message(streaming)}
}

func ids(ms []*chat.Message) (out []string) {
	for _, m := range ms {
		out = append(out, string(m.ID))
	}
	return out
}

func texts(ps []chat.Part) (out []string) {
	for _, p := range ps {
		if p.Text != nil {
			out = append(out, p.Text.Text)
		} else {
			out = append(out, "")
		}
	}
	return out
}

func same[T any](t *testing.T, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSnapshotSetsTheTurnStateFromLive(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap([]chat.Message{msg("m1", 1, chat.RoleUser, "hi")}, 4, 1, agent.Live{Running: true, Streaming: "a1"}))
	same(t, th.State, chat.StateWorking)
	same(t, th.Stream, id.Message("a1"))
	same(t, th.Streamed, []id.Message{"a1"})
	same(t, th.Seq, uint64(4))
}

func TestPartsInTheSnapshotAreDroppedAndAGapAsksForANewOne(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap([]chat.Message{msg("m1", 1, chat.RoleUser, "hi")}, 4, 1, agent.Live{}))
	if WithPart(th, textPartDone("m1", 0, 4, "hi")) != th {
		t.Fatal("a part the snapshot has changed the thread")
	}
	if WithPart(th, textPartDone("m2", 0, 6, "hi")) != nil {
		t.Fatal("a gap was not seen")
	}
	next := WithPart(th, textPartDone("m2", 0, 5, "hi"))
	same(t, ids(next.Messages), []string{"m1", "m2"})
	same(t, next.Seq, uint64(5))
	// Parts of one write share its number.
	two := WithPart(next, textPartDone("m2", 1, 5, "more"))
	same(t, texts(two.Messages[1].Parts), []string{"hi", "more"})
}

func TestAPartWithTheSameIndexReplacesTheOldOne(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap([]chat.Message{msg("m1", 1, chat.RoleUser, "a", "b")}, 1, 1, agent.Live{}))
	next := WithPart(th, textPartDone("m1", 1, 2, "B"))
	same(t, texts(next.Messages[0].Parts), []string{"a", "B"})
	same(t, texts(th.Messages[0].Parts), []string{"a", "b"}) // the old thread is unchanged
	if next.Messages[0] == th.Messages[0] {
		t.Fatal("the changed message kept its pointer")
	}
}

func TestANewMessagesRoleIsGuessed(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap(nil, 1, 1, agent.Live{}))
	th = WithStatus(th, status(1, chat.StateWorking, "a1"))
	th = WithPart(th, textPartDone("a1", 0, 2, "hi"))
	th = WithPart(th, part("a2", 0, 3, "ok", chat.PartToolResult))
	th = WithPart(th, part("a3", 0, 4, "failed", chat.PartNotice))
	var roles []chat.Role
	for _, m := range th.Messages {
		roles = append(roles, m.Role)
	}
	same(t, roles, []chat.Role{chat.RoleAssistant, chat.RoleTool, chat.RoleUser})
}

func TestANewTryEndsTheJoiningMessagesAndOldStatusesAreDropped(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap(nil, 5, 1, agent.Live{Running: true, Streaming: "a1"}))
	th.Joining = []id.Message{"u2"}
	if WithStatus(th, status(4, chat.StateIdle, "")) != th {
		t.Fatal("an old status was applied")
	}
	same(t, WithStatus(th, status(5, chat.StateWorking, "a1")).Joining, []id.Message{"u2"})
	next := WithStatus(th, status(6, chat.StateWorking, "a2"))
	if len(next.Joining) != 0 {
		t.Fatalf("joining %v", next.Joining)
	}
	same(t, next.Streamed, []id.Message{"a1", "a2"})
}

func TestWhatALongToolCallIsDoing(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap(nil, 2, 1, agent.Live{Running: true, Doing: "trying a schema change, about 3 minutes"}))
	same(t, th.Doing, "trying a schema change, about 3 minutes")
	th = WithStatus(th, status(3, chat.StateWorking, ""))
	same(t, th.Doing, "")
	s := status(4, chat.StateWorking, "")
	s.Doing = "applying a schema change, about 3 minutes"
	th = WithStatus(th, s)
	same(t, th.Doing, "applying a schema change, about 3 minutes")
	same(t, WithStatus(th, status(5, chat.StateIdle, "")).Doing, "")
}

func TestANewSnapshotKeepsOlderTurnsAndUnchangedMessages(t *testing.T) {
	old := Share(nil, []chat.Message{msg("m1", 1, chat.RoleUser, "one"), msg("m2", 2, chat.RoleUser, "two")})
	prev := FromSnapshot(nil, "p1", snap([]chat.Message{*old[1]}, 3, 2, agent.Live{}))
	prev.Messages, prev.From = old, 1
	next := FromSnapshot(prev, "p1", snap([]chat.Message{msg("m2", 2, chat.RoleUser, "two"), msg("m3", 3, chat.RoleUser, "x")}, 5, 2, agent.Live{}))
	same(t, ids(next.Messages), []string{"m1", "m2", "m3"})
	if next.Messages[1] != old[1] {
		t.Fatal("an unchanged message lost its pointer")
	}
	same(t, next.From, 1)
	changed := *old[0]
	changed.Parts = []chat.Part{}
	if Share(old, []chat.Message{changed})[0] == old[0] {
		t.Fatal("a changed message kept its pointer")
	}
}

func delta(message string, p int, text string, kind chat.PartKind, offset int) chat.Delta {
	return chat.Delta{Project: "p1", Chat: "c1", Seq: 1, Message: id.Message(message), Part: p, Kind: kind, Text: text, Offset: offset}
}

func TestDeltasBuildTheAnswerPartByPart(t *testing.T) {
	s := AddDelta(nil, delta("a1", 0, "Let me ", chat.PartThinking, 0))
	s = AddDelta(s, delta("a1", 0, "think", chat.PartText, 0))
	same(t, s.Kind, chat.PartText)
	same(t, s.Text, "think")
	s = AddDelta(s, delta("a1", 0, " more", chat.PartText, 0))
	s = AddDelta(s, delta("a1", 2, "Done", chat.PartText, 0))
	same(t, texts(s.Parts), []string{"think more"})
	same(t, []any{s.Index, s.Text}, []any{2, "Done"})
	// Another answer starts over.
	if n := AddDelta(s, delta("a2", 0, "x", chat.PartText, 0)); len(n.Parts) != 0 {
		t.Fatalf("parts %v", n.Parts)
	}
}

func TestASnapshotsStreamStartsAfterItsFinishedParts(t *testing.T) {
	if FromLive(agent.Live{Running: true, Turn: 1, Seq: 1}) != nil {
		t.Fatal("a stream without Streaming")
	}
	s := FromLive(agent.Live{Running: true, Turn: 1, Seq: 1, Streaming: "a1", Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "a"}}}, Kind: chat.PartText, Text: "b"})
	same(t, []any{s.Index, s.Text}, []any{1, "b"})
}

func deltaTexts(q []Event) (out []string) {
	for _, e := range q {
		if e.Delta != nil {
			out = append(out, e.Delta.Text)
		} else {
			out = append(out, "")
		}
	}
	return out
}

func TestFreshDropsTheTextASnapshotHasByOffset(t *testing.T) {
	live := &Stream{Message: "a1", Index: 0, Kind: chat.PartText, Text: "Hello wor"}
	d := func(text string, offset int) Event {
		x := delta("a1", 0, text, chat.PartText, offset)
		return Event{Delta: &x}
	}
	same(t, deltaTexts(Fresh([]Event{d("Hello ", 0), d("wor", 6), d("ld", 9)}, live)), []string{"ld"})
	same(t, deltaTexts(Fresh([]Event{d("wo", 6), d("rld", 8)}, live)), []string{"ld"}) // half held: the rest is kept
	// A repeated end is a real token when its offset is past the snapshot.
	same(t, deltaTexts(Fresh([]Event{d("r", 9)}, live)), []string{"r"})
	if len(Fresh([]Event{d("x", 0)}, nil)) != 1 {
		t.Fatal("no stream dropped a delta")
	}
}

func TestFreshCountsOffsetsInUTF16(t *testing.T) {
	// "😀" is two UTF-16 units, as JavaScript and Delta.Offset count.
	live := &Stream{Message: "a1", Kind: chat.PartText, Text: "a😀b"}
	x := delta("a1", 0, "😀bc", chat.PartText, 1)
	same(t, deltaTexts(Fresh([]Event{{Delta: &x}}, live)), []string{"c"})
}

// IDs below grow in the order they were made, as ULIDs do.

func TestALateDeltaForAnEarlierPartLeavesTheStreamAlone(t *testing.T) {
	s := AddDelta(nil, delta("a1", 0, "One", chat.PartText, 0))
	s = AddDelta(s, delta("a1", 1, "Two", chat.PartText, 0))
	late := AddDelta(s, delta("a1", 0, " more", chat.PartText, 0))
	if late != s {
		t.Fatal("a late delta changed the stream")
	}
	same(t, []any{late.Index, late.Text, texts(late.Parts)}, []any{1, "Two", []string{"One"}})
}

func TestAMessageSentDuringATurnSortsAfterTheAnswerMadeBeforeIt(t *testing.T) {
	th := FromSnapshot(nil, "p1", snap([]chat.Message{msg("00U1", 1, chat.RoleUser, "go")}, 3, 1, agent.Live{Running: true, Streaming: "01A1"}))
	// The user writes while the answer streams; it joins.
	th = WithPart(th, textPartDone("01U2", 0, 4, "also this"))
	th.Joining = []id.Message{"01U2"}
	same(t, ids(th.Messages), []string{"00U1", "01U2"})
	same(t, th.Messages[1].Turn, 1)
	// The answer and the tool message got their IDs when the answer started.
	call := textPartDone("01A1", 0, 5, "")
	call.Part = chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "k1", Name: "query", Args: []byte("{}")}}
	th = WithPart(th, call)
	th = WithPart(th, part("01A2", 0, 6, "1 row", chat.PartToolResult))
	same(t, ids(th.Messages), []string{"00U1", "01A1", "01A2", "01U2"})
}

func last(th *Thread) *chat.Message { return th.Messages[len(th.Messages)-1] }

func TestANewMessagesTurnComesFromPartDoneOrIsGuessed(t *testing.T) {
	ms := []chat.Message{msg("01U3", 3, chat.RoleUser, "third"), msg("01A3", 3, chat.RoleAssistant, "Done.")}
	idle := FromSnapshot(nil, "p1", snap(ms, 7, 1, agent.Live{}))
	// Sent while idle: the next turn.
	same(t, last(WithPart(idle, textPartDone("01U4", 0, 8, "fourth"))).Turn, 4)
	// A notice while idle, such as a new approval level, stays in the last turn.
	same(t, last(WithPart(idle, part("01N4", 0, 8, "x", chat.PartNotice))).Turn, 3)
	// Sent during a turn: it joins that turn.
	busy := WithStatus(idle, status(7, chat.StateWorking, ""))
	same(t, last(WithPart(busy, textPartDone("01U4", 0, 8, "fourth"))).Turn, 3)
	// The turn PartDone carries wins.
	known := textPartDone("01U4", 0, 8, "fourth")
	known.Turn = 9
	same(t, last(WithPart(busy, known)).Turn, 9)
}

func TestAStatusSentBetweenReadingLiveAndTheMessagesIsKept(t *testing.T) {
	// Live was read at 4; a status came at 4 and a write at 5 before the
	// messages were read.
	th := FromSnapshot(nil, "p1", snap([]chat.Message{msg("01U1", 1, chat.RoleUser, "go")}, 5, 1, agent.Live{Running: true, Seq: 4}))
	same(t, th.Stream, id.Message(""))
	same(t, WithStatus(th, status(4, chat.StateWorking, "01A1")).Stream, id.Message("01A1"))
	if WithStatus(th, status(3, chat.StateIdle, "")) != th {
		t.Fatal("an old status was applied")
	}
}

// replay runs a read's replay on the pure parts: the snapshot, then the
// queued events less the deltas it holds.
func replay(s app.ChatSnapshot, queue []Event) (*Thread, *Stream) {
	th := FromSnapshot(nil, "p1", s)
	st := FromLive(s.Live)
	for _, e := range Fresh(queue, st) {
		switch {
		case e.Delta != nil:
			if th.Stream == e.Delta.Message {
				st = AddDelta(st, *e.Delta)
			}
		case e.Part != nil:
			if n := WithPart(th, *e.Part); n != nil {
				th = n
			}
		default:
			th = WithStatus(th, *e.Status)
		}
	}
	return th, st
}

func TestAReadReplaysWhatCameMeanwhile(t *testing.T) {
	live := agent.Live{
		Running: true, Seq: 4, Streaming: "01A1", Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "One"}}},
		Kind: chat.PartText, Text: "foo\n",
	}
	dl := func(p int, text string, offset int) Event {
		x := delta("01A1", p, text, chat.PartText, offset)
		return Event{Delta: &x}
	}
	st := func(seq uint64, state chat.State, streaming string) Event {
		x := status(seq, state, streaming)
		return Event{Status: &x}
	}
	in := textPartDone("01U1", 0, 5, "go")
	queue := []Event{
		dl(0, "late", 0), // for the finished part
		dl(1, "\n", 3),   // already in Live's text
		st(4, chat.StateWorking, "01A1"),
		{Part: &in}, // in the snapshot
		dl(1, "bar", 4),
	}
	s := snap([]chat.Message{msg("01U1", 1, chat.RoleUser, "go")}, 5, 1, live)
	th, stream := replay(s, queue)
	same(t, ids(th.Messages), []string{"01U1"})
	same(t, th.Stream, id.Message("01A1"))
	same(t, []any{stream.Index, stream.Text, texts(stream.Parts)}, []any{1, "foo\nbar", []string{"One"}})
	// A new line after the snapshot is a real token: its offset is past it.
	_, stream = replay(s, []Event{dl(1, "\n", 4)})
	same(t, stream.Text, "foo\n\n")
	// A status older than Live is dropped; a newer one replaces Live's state.
	th, _ = replay(s, []Event{st(3, chat.StateIdle, "")})
	same(t, th.State, chat.StateWorking)
	th, _ = replay(s, []Event{st(5, chat.StateWaiting, "")})
	same(t, th.State, chat.StateWaiting)
}

func TestASnapshotMidTurnKnowsTheMessagesThatJoin(t *testing.T) {
	// IDs grow in the order the messages were made.
	messages := []chat.Message{
		msg("m1", 1, chat.RoleUser, "go"),
		msg("m2", 1, chat.RoleAssistant, "first step"),
		msg("m3", 1, chat.RoleUser, "sent before the second step"),
		msg("m5", 1, chat.RoleUser, "sent while it streams"),
	}
	th := FromSnapshot(nil, "p1", snap(messages, 5, 1, agent.Live{Running: true, Streaming: "m4"}))
	same(t, th.Joining, []id.Message{"m5"})
	same(t, FromSnapshot(nil, "p1", snap(messages, 5, 1, agent.Live{Running: true})).Joining, []id.Message{"m3", "m5"})
	if j := FromSnapshot(nil, "p1", snap(messages, 5, 1, agent.Live{})).Joining; len(j) != 0 {
		t.Fatalf("joining %v", j)
	}
}
