package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// rewindChat is a chat of three turns, each a user message and an answer
// with its own word, and session notes and skills changed in turns 2 and
// 3.
func rewindChat(t *testing.T, c *ChatsDB) (chat.Chat, []chat.Message) {
	t.Helper()
	ctx := context.Background()
	ch := newChat(t, c, chat.Chat{Title: "r"})
	var ms []chat.Message
	words := []string{"apple", "banana", "cherry"}
	for i, w := range words {
		n := i + 1
		ms = append(ms, addTurn(t, c, ch.ID, n, "ask "+w))
		a, _, err := c.AppendMessage(ctx, chat.Message{Chat: ch.ID, Turn: n, Role: chat.RoleAssistant, Model: "fake/m",
			Usage: chat.Usage{Input: 100, Output: 10}, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "answer " + w}}}})
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, a)
		notes, _ := c.Notes(ctx, ch.ID)
		if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: ch.ID, Content: "after " + w, Revision: notes.Revision}); err != nil {
			t.Fatal(err)
		}
		if n >= 2 {
			if _, err := c.SetSkills(ctx, ch.ID, words[1:n]); err != nil {
				t.Fatal(err)
			}
		}
	}
	return ch, ms
}

func TestRewind(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch, ms := rewindChat(t, c)
	before, _ := c.Usage(ctx, time.Time{}, 0)

	pv, err := c.RewindPreview(ctx, ch.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pv.LastTurn != 3 || len(pv.Messages) != 4 || !pv.Notes || !reflect.DeepEqual(pv.Unload, []string{"banana", "cherry"}) || len(pv.Load) != 0 {
		t.Fatalf("RewindPreview = %+v", pv)
	}
	notes, _ := c.Notes(ctx, ch.ID)
	notice := &chat.Message{Chat: ch.ID, Turn: 1, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeRewound, Text: "[rewound]"}}}}
	r, err := c.Rewind(ctx, ch.ID, 1, notice)
	if err != nil {
		t.Fatal(err)
	}
	if r.Turn != 1 || r.LastTurn != 3 || len(r.Messages) != 4 || !r.Notes || !reflect.DeepEqual(r.Skills, []string{}) || r.Notice == "" {
		t.Fatalf("Rewind = %+v", r)
	}

	// The chat ends with turn 1 and its notice.
	got, _, err := c.Messages(ctx, ch.ID, 0, 0)
	if err != nil || len(got) != 3 || got[2].ID != r.Notice {
		t.Fatalf("Messages after the rewind = %d, %v", len(got), err)
	}
	if last, _ := c.LastTurn(ctx, ch.ID); last != 1 {
		t.Errorf("LastTurn = %d, want 1", last)
	}
	// The history index follows the chat.
	for w, want := range map[string]int{"apple": 2, "banana": 0, "cherry": 0} {
		if n := ftsCount(t, c, w); n != want {
			t.Errorf("index rows for %q = %d, want %d", w, n, want)
		}
		res, err := c.Search(ctx, SearchReq{Query: w, Chat: ch.ID})
		if err != nil || len(res.Hits) != want {
			t.Errorf("Search(%q) = %d hits, %v; want %d", w, len(res.Hits), err, want)
		}
		res, err = c.Search(ctx, SearchReq{Query: w, Chat: ch.ID, Substring: true})
		if err != nil || len(res.Hits) != want {
			t.Errorf("Search(%q, substring) = %d hits, %v; want %d", w, len(res.Hits), err, want)
		}
	}
	// The notes and skills as they were after turn 1; the notes' revision
	// goes on, so an edit from the old one is refused.
	n, _ := c.Notes(ctx, ch.ID)
	if n.Content != "after apple" || n.Revision != notes.Revision+1 {
		t.Errorf("notes after the rewind = %+v, want turn 1's at revision %d", n, notes.Revision+1)
	}
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: ch.ID, Content: "x", Revision: notes.Revision}); !errors.Is(err, ErrConflict) {
		t.Errorf("SaveNotes from the old revision: %v, want a conflict", err)
	}
	if got, _ := c.Chat(ctx, ch.ID); len(got.Skills) != 0 {
		t.Errorf("skills after the rewind = %v, want none", got.Skills)
	}
	// Spent tokens still count, and the change log can still name the
	// chat of a message that left.
	after, _ := c.Usage(ctx, time.Time{}, 0)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("Usage after the rewind = %+v, want %+v", after, before)
	}
	if mc, err := c.MessageChats(ctx, []id.Message{ms[5].ID}); err != nil || mc[ms[5].ID] != ch.ID {
		t.Errorf("MessageChats of a rewound message = %v, %v", mc, err)
	}
	if got, err := c.RewindMessages(ctx, ch.ID, r.ID); err != nil || len(got) != 4 || got[0] != ms[2].ID {
		t.Errorf("RewindMessages = %v, %v", got, err)
	}
	rs, err := c.Rewinds(ctx, ch.ID)
	if err != nil || len(rs) != 1 || !rs[0].Undoable || rs[0].Turn != 1 || rs[0].LastTurn != 3 {
		t.Fatalf("Rewinds = %+v, %v", rs, err)
	}

	// Undo brings it all back, the index too, without the notice.
	u, err := c.UndoRewind(ctx, ch.ID, r.ID)
	if err != nil || len(u.Messages) != 4 || !u.Notes {
		t.Fatalf("UndoRewind = %+v, %v", u, err)
	}
	got, _, _ = c.Messages(ctx, ch.ID, 0, 0)
	if len(got) != 6 || !reflect.DeepEqual(got, withSeqless(ms)) {
		t.Errorf("Messages after the undo differ:\n%+v\nwant\n%+v", got, ms)
	}
	for _, w := range []string{"apple", "banana", "cherry"} {
		if n := ftsCount(t, c, w); n != 2 {
			t.Errorf("index rows for %q after the undo = %d, want 2", w, n)
		}
	}
	if n, _ := c.Notes(ctx, ch.ID); n.Content != "after cherry" {
		t.Errorf("notes after the undo = %+v", n)
	}
	if got, _ := c.Chat(ctx, ch.ID); !reflect.DeepEqual(got.Skills, []string{"banana", "cherry"}) {
		t.Errorf("skills after the undo = %v", got.Skills)
	}
	if rs, _ := c.Rewinds(ctx, ch.ID); len(rs) != 0 {
		t.Errorf("Rewinds after the undo = %+v", rs)
	}
	if _, err := c.UndoRewind(ctx, ch.ID, r.ID); !errors.Is(err, ErrRewindGone) {
		t.Errorf("a second UndoRewind: %v, want ErrRewindGone", err)
	}
	// The turn_state rows came back: a rewind to 2 puts back turn 3's.
	if r, err := c.Rewind(ctx, ch.ID, 2, nil); err != nil || !r.Notes || !reflect.DeepEqual(r.Skills, []string{"banana"}) {
		t.Errorf("Rewind to 2 after the undo = %+v, %v", r, err)
	}
	if n, _ := c.Notes(ctx, ch.ID); n.Content != "after banana" {
		t.Errorf("notes after a rewind to 2 = %+v", n)
	}
}

// withSeqless is ms as Messages reads them back.
func withSeqless(ms []chat.Message) []chat.Message {
	out := make([]chat.Message, len(ms))
	for i, m := range ms {
		m.Usage.Thought = 0
		out[i] = m
	}
	return out
}

func TestRewindRules(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch, _ := rewindChat(t, c)
	for _, turn := range []int{-1, 3, 4} {
		if _, err := c.Rewind(ctx, ch.ID, turn, nil); !errors.Is(err, ErrNothingToRewind) {
			t.Errorf("Rewind to %d: %v, want ErrNothingToRewind", turn, err)
		}
	}
	// The notice goes in the last turn kept.
	bad := &chat.Message{Chat: ch.ID, Turn: 2, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeRewound, Text: "x"}}}}
	if _, err := c.Rewind(ctx, ch.ID, 1, bad); err == nil {
		t.Error("Rewind with a notice in a turn that left: no error")
	}
	if last, _ := c.LastTurn(ctx, ch.ID); last != 3 {
		t.Fatalf("a failed rewind changed the chat: last turn %d", last)
	}

	a, err := c.Rewind(ctx, ch.ID, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Rewind(ctx, ch.ID, 0, nil) // empties the chat
	if err != nil {
		t.Fatal(err)
	}
	if last, _ := c.LastTurn(ctx, ch.ID); last != 0 {
		t.Errorf("LastTurn after a rewind to 0 = %d", last)
	}
	if n, _ := c.Notes(ctx, ch.ID); n.Content != "" || n.Summary != "" {
		t.Errorf("notes after a rewind to 0 = %+v, want empty", n)
	}
	// Only the newest rewind can be undone.
	if _, err := c.UndoRewind(ctx, ch.ID, a.ID); !errors.Is(err, ErrRewindMoved) {
		t.Errorf("UndoRewind of an older rewind: %v, want ErrRewindMoved", err)
	}
	if rs, _ := c.Rewinds(ctx, ch.ID); len(rs) != 2 || rs[0].Undoable || !rs[1].Undoable {
		t.Errorf("Rewinds = %+v", rs)
	}
	if _, err := c.UndoRewind(ctx, ch.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UndoRewind(ctx, ch.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if last, _ := c.LastTurn(ctx, ch.ID); last != 3 {
		t.Errorf("LastTurn after both undos = %d, want 3", last)
	}

	// Once the chat goes on, the turns can't come back.
	r, err := c.Rewind(ctx, ch.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	addTurn(t, c, ch.ID, 2, "ask date")
	if _, err := c.UndoRewind(ctx, ch.ID, r.ID); !errors.Is(err, ErrRewindMoved) {
		t.Errorf("UndoRewind after a new turn: %v, want ErrRewindMoved", err)
	}
	if rs, _ := c.Rewinds(ctx, ch.ID); len(rs) != 1 || rs[0].Undoable {
		t.Errorf("Rewinds after a new turn = %+v", rs)
	}
	// The notes of the new turn 2 start from turn 1's, and a later rewind
	// to 1 finds them by the new turn's record.
	n, _ := c.Notes(ctx, ch.ID)
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: ch.ID, Content: "after date", Revision: n.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Rewind(ctx, ch.ID, 1, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.Notes(ctx, ch.ID); n.Content != "after apple" {
		t.Errorf("notes after the second rewind to 1 = %q", n.Content)
	}

	// The undo window: purged rewinds can't be undone, and their messages
	// leave.
	if n, err := c.PurgeRewinds(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Errorf("PurgeRewinds of none = %d, %v", n, err)
	}
	if n, err := c.PurgeRewinds(ctx, time.Now().Add(time.Hour)); err != nil || n != 2 {
		t.Errorf("PurgeRewinds = %d, %v; want 2", n, err)
	}
	left, _ := Query(ctx, c.DB, `SELECT count(*) FROM rewound_messages`, nil, func(r *sql.Rows) (int, error) {
		var n int
		return n, r.Scan(&n)
	})
	if left[0] != 0 {
		t.Errorf("%d rewound messages left after the purge", left[0])
	}

	// Clear drops the rewinds and the turn records.
	if _, err := c.Rewind(ctx, ch.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Clear(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"rewinds", "turn_state"} {
		n, _ := Query(ctx, c.DB, `SELECT count(*) FROM `+tbl, nil, func(r *sql.Rows) (int, error) {
			var n int
			return n, r.Scan(&n)
		})
		if n[0] != 0 {
			t.Errorf("%d rows in %s after Clear", n[0], tbl)
		}
	}
}

// TestRewindReview: a rewind over turns Refresh memory reviewed takes its
// progress back to the last message kept and drops its chunks over them;
// the undo puts both back.
func TestRewindReview(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch, ms := rewindChat(t, c)
	if err := c.SaveReviewChunk(ctx, ch.ID, ReviewChunk{First: ms[0].ID, Last: ms[1].ID, Result: "[]"}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveReviewChunk(ctx, ch.ID, ReviewChunk{First: ms[2].ID, Last: ms[5].ID, Result: "[]"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FinishReview(ctx, FinishReview{Chat: ch.ID, UpTo: ms[3].ID}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveReviewChunk(ctx, ch.ID, ReviewChunk{First: ms[4].ID, Last: ms[5].ID, Result: "[]"}); err != nil {
		t.Fatal(err)
	}
	r, err := c.Rewind(ctx, ch.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Review(ctx, ch.ID)
	if err != nil || st.UpTo != ms[1].ID || st.UpToTurn != 1 || len(st.Chunks) != 0 {
		t.Errorf("Review after the rewind = %+v, %v", st, err)
	}
	if _, err := c.UndoRewind(ctx, ch.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	st, err = c.Review(ctx, ch.ID)
	if err != nil || st.UpTo != ms[3].ID || len(st.Chunks) != 1 || st.Chunks[0].First != ms[4].ID {
		t.Errorf("Review after the undo = %+v, %v", st, err)
	}
}
