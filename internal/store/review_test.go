package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func TestReviewProgress(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{})
	m1 := addTurn(t, c, a.ID, 1, "one")
	m2 := addTurn(t, c, a.ID, 2, "two")
	m3 := addTurn(t, c, a.ID, 3, "three")

	r, err := c.Review(ctx, a.ID)
	if err != nil || r.UpTo != "" || r.UpToTurn != 0 || len(r.Chunks) != 0 {
		t.Fatalf("a new chat's review = %+v, %v", r, err)
	}
	// Chunks are kept as their map calls end, without a seq bump.
	seq, _ := c.Seq(ctx, a.ID)
	for _, k := range []ReviewChunk{{First: m1.ID, Last: m2.ID, Result: `{"items":[]}`}, {First: m3.ID, Last: m3.ID, Result: "x"}} {
		if err := c.SaveReviewChunk(ctx, a.ID, k); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.SaveReviewChunk(ctx, a.ID, ReviewChunk{First: m3.ID, Last: m3.ID, Result: "y"}); err != nil {
		t.Fatal(err)
	}
	if s, _ := c.Seq(ctx, a.ID); s != seq {
		t.Errorf("a saved chunk bumped the seq: %d, was %d", s, seq)
	}
	r, _ = c.Review(ctx, a.ID)
	if len(r.Chunks) != 2 || r.Chunks[0].First != m1.ID || r.Chunks[1].Result != "y" {
		t.Fatalf("chunks = %+v", r.Chunks)
	}
	if err := c.SaveReviewChunk(ctx, id.Chat(id.New()), ReviewChunk{First: m1.ID, Last: m1.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a chunk of a missing chat: %v", err)
	}

	// The end of a refresh: the summary, the progress and the chip, with
	// one bump.
	sum := "Did: imported prices."
	f, err := c.FinishReview(ctx, FinishReview{Chat: a.ID, Summary: &sum, UpTo: m2.ID, Chip: func(serr error) *chat.Message {
		if serr != nil {
			t.Errorf("summary error %v", serr)
		}
		return &chat.Message{Chat: a.ID, Turn: 3, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice,
			Notice: &chat.Notice{Kind: chat.NoticeMemoryRefreshed, Text: "Memory refreshed: 1 added"}}}}
	}})
	if err != nil || f.Seq != seq+1 || f.Message == nil || f.Message.ID == "" || f.Notes.Summary != sum || f.Notes.Revision != 1 {
		t.Fatalf("FinishReview = %+v, %v", f, err)
	}
	r, _ = c.Review(ctx, a.ID)
	if r.UpTo != m2.ID || r.UpToTurn != 2 || len(r.Chunks) != 1 || r.Chunks[0].First != m3.ID {
		t.Fatalf("after FinishReview: %+v", r)
	}
	ms, _, _ := c.Messages(ctx, a.ID, 3, 3)
	if len(ms) != 2 || ms[1].Parts[0].Notice == nil {
		t.Fatalf("the chip isn't stored: %+v", ms)
	}

	// A stale summary is no error: the rest is written.
	f, err = c.FinishReview(ctx, FinishReview{Chat: a.ID, Summary: &sum, NotesRevision: 0, UpTo: m3.ID})
	if err != nil || !errors.Is(f.SummaryErr, ErrConflict) || f.Message != nil {
		t.Fatalf("a stale summary = %+v, %v", f, err)
	}
	if r, _ = c.Review(ctx, a.ID); r.UpTo != m3.ID || len(r.Chunks) != 0 {
		t.Errorf("progress after a stale summary: %+v", r)
	}

	// The end drops the chunks up to the progress, as a Full refresh's.
	if err := c.SaveReviewChunk(ctx, a.ID, ReviewChunk{First: m1.ID, Last: m3.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FinishReview(ctx, FinishReview{Chat: a.ID, UpTo: m3.ID}); err != nil {
		t.Fatal(err)
	}
	n, err := readTx(ctx, c.DB, func(tx *sql.Tx) (n int, err error) {
		return n, tx.QueryRow(`SELECT count(*) FROM review_chunks WHERE chat_id = ?`, a.ID).Scan(&n)
	})
	if err != nil || n != 0 {
		t.Errorf("chunks after the end: %d, %v", n, err)
	}
	if _, err := c.FinishReview(ctx, FinishReview{Chat: id.Chat(id.New()), UpTo: m3.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("FinishReview(missing) = %v", err)
	}
	if _, err := c.Review(ctx, id.Chat(id.New())); !errors.Is(err, ErrNotFound) {
		t.Errorf("Review(missing) = %v", err)
	}
}

// A summary near the notes' cap (P2-10's open point): refused over the
// cap, let through when the notes get shorter, and reported, not failed,
// by FinishReview.
func TestReviewSummaryCap(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{})
	m := addTurn(t, c, a.ID, 1, "one")
	notes := strings.Repeat("abcd", MaxNotesTokens-100)
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: notes}); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("s", 4*200)
	f, err := c.FinishReview(ctx, FinishReview{Chat: a.ID, Summary: &big, NotesRevision: 1, UpTo: m.ID})
	var ce *NotesCapError
	if err != nil || !errors.As(f.SummaryErr, &ce) || ce.Summary || ce.Tokens != MaxNotesTokens+100 {
		t.Fatalf("a summary over the notes' cap = %+v, %v", f, err)
	}
	fits := strings.Repeat("s", 4*100)
	if f, err = c.FinishReview(ctx, FinishReview{Chat: a.ID, Summary: &fits, NotesRevision: 1, UpTo: m.ID}); err != nil || f.SummaryErr != nil || f.Notes.Revision != 2 {
		t.Fatalf("a summary that fits = %+v, %v", f, err)
	}
	// Over the cap after the notes grew past it, but shorter than before.
	mustExec(t, c.DB, "UPDATE session_notes SET content = content || 'abcdabcd' WHERE chat_id = ?", a.ID)
	shorter := strings.Repeat("s", 4*99)
	if f, err = c.FinishReview(ctx, FinishReview{Chat: a.ID, Summary: &shorter, NotesRevision: 2, UpTo: m.ID}); err != nil || f.SummaryErr != nil {
		t.Errorf("a shorter summary over the cap = %+v, %v", f, err)
	}
	// The summary's own cap still holds.
	huge := strings.Repeat("s", 4*(MaxSummaryTokens+1))
	if _, err := c.SaveSummary(ctx, a.ID, huge, 3); !errors.As(err, &ce) || !ce.Summary {
		t.Errorf("a summary over its own cap = %v", err)
	}
}

// A Refresh memory batch is undone and redone as one, by its changes, in
// project memory and in user memory (SPEC 3.5).
func TestMemoryBatchUndo(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	r := openTestRegistry(t)
	src := id.SourceOf("refresh", "c1")
	var changes []id.Change
	for _, s := range []string{"goals", "decisions"} {
		w, err := p.UpdateMemory(ctx, MemoryEdit{Section: s, Content: s + " text", Source: src})
		if err != nil {
			t.Fatal(err)
		}
		changes = append(changes, id.Change(w.Change))
	}
	other, err := p.UpdateMemory(ctx, MemoryEdit{Section: "conventions", Content: "kept", Source: src})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UndoChanges(ctx, changes, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	ss, _ := p.Memory(ctx)
	if section(ss, "goals").Content != "" || section(ss, "decisions").Content != "" || section(ss, "conventions").Content != "kept" {
		t.Fatalf("after the batch's undo: %+v", ss)
	}
	if undone, _ := p.ChangeUndone(ctx, changes[0]); !undone {
		t.Error("the batch's first change isn't undone")
	}
	if undone, _ := p.ChangeUndone(ctx, id.Change(other.Change)); undone {
		t.Error("a change outside the batch was undone")
	}
	if _, err := p.UndoChanges(ctx, changes, UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	if ss, _ = p.Memory(ctx); section(ss, "goals").Content != "goals text" {
		t.Errorf("after the redo: %+v", ss)
	}
	if _, err := p.UndoChanges(ctx, nil, UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("an empty batch: %v", err)
	}

	var ns []int64
	for _, s := range []string{"preferences", "goals"} {
		w, err := r.UpdateUserMemory(ctx, MemoryEdit{Section: s, Content: s + " text", Source: src}, "")
		if err != nil {
			t.Fatal(err)
		}
		var n int64
		if _, err := fmt.Sscan(w.Change, &n); err != nil {
			t.Fatal(err)
		}
		ns = append(ns, n)
	}
	if _, err := r.UndoUserMemoryChanges(ctx, ns, "", UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	us, _ := r.UserMemory(ctx)
	if section(us, "preferences").Content != "" || section(us, "goals").Content != "" {
		t.Fatalf("user memory after the batch's undo: %+v", us)
	}
	if _, err := r.UndoUserMemoryChanges(ctx, ns, "", UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("undo again: %v", err)
	}
	if _, err := r.UndoUserMemoryChanges(ctx, ns, "", UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	if us, _ = r.UserMemory(ctx); section(us, "goals").Content != "goals text" {
		t.Errorf("user memory after the redo: %+v", us)
	}
}
