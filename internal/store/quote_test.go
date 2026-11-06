package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// TestQuoteSearch: a quote is stored with its message and found by history
// search, by words and inside words, and leaves the index with its chat
// (TASK-019).
func TestQuoteSearch(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{Title: "a"})
	src, _, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{
		{Kind: chat.PartText, Text: &chat.Text{Text: "The pelican index rose. قیمت بیت‌کوین بالا رفت."}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	q := &chat.Quote{Text: "قیمت بیت‌کوین بالا رفت", Message: src.ID, Turn: 1, Role: chat.RoleAssistant}
	reply, _, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 2, Role: chat.RoleUser, Parts: []chat.Part{
		{Kind: chat.PartQuote, Quote: q},
		{Kind: chat.PartText, Text: &chat.Text{Text: "why did the walrus say that?"}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	ms, _, err := c.Messages(ctx, a.ID, 2, 2)
	if err != nil || len(ms) != 1 || len(ms[0].Parts) != 2 || ms[0].Parts[0].Quote == nil || *ms[0].Parts[0].Quote != *q {
		t.Fatalf("Messages = %+v, %v; want the quote back", ms, err)
	}
	head, err := c.MessageHead(ctx, src.ID)
	if err != nil || head.Chat != a.ID || head.Turn != 1 || head.Role != chat.RoleAssistant || len(head.Parts) != 0 {
		t.Errorf("MessageHead = %+v, %v", head, err)
	}
	if _, err := c.MessageHead(ctx, "01JNOPE"); !errors.Is(err, ErrNotFound) {
		t.Errorf("MessageHead of a missing message: %v, want ErrNotFound", err)
	}

	// Both the source and the reply have the quoted words; only the reply
	// has "walrus".
	for _, sub := range []bool{false, true} {
		res, err := c.Search(ctx, SearchReq{Query: "قيمت بیتکوین", Substring: sub})
		if err != nil || len(res.Hits) != 2 {
			t.Fatalf("substring %v: %+v, %v; want the source and the reply", sub, res.Hits, err)
		}
		var hit *Hit
		for i := range res.Hits {
			if res.Hits[i].Message == reply.ID {
				hit = &res.Hits[i]
			}
		}
		if hit == nil || hit.Turn != 2 || !strings.Contains(hit.Snippet, "بالا رفت") {
			t.Errorf("substring %v: the reply's hit is %+v", sub, hit)
		}
		res, err = c.Search(ctx, SearchReq{Query: "walrus pelican", Substring: sub})
		if err != nil || len(res.Hits) != 0 {
			t.Errorf("substring %v: words of two messages found %+v, %v", sub, res.Hits, err)
		}
	}
	if n, err := c.TextParts(ctx); err != nil || n != 3 {
		t.Errorf("TextParts = %d, %v; want 3 with the quote", n, err)
	}

	if _, err := c.Clear(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	all, err := Query(ctx, c.DB, `SELECT count(*) FROM messages_fts`, nil, func(r *sql.Rows) (int, error) {
		var n int
		return n, r.Scan(&n)
	})
	if err != nil || all[0] != 0 {
		t.Errorf("after Clear: %v rows in the index, %v; want none", all, err)
	}
}

// TestQuoteFormatStep opens a chats.db made before quotes: the format step
// makes deleting a quote take its words out of the index.
func TestQuoteFormatStep(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	old := Format{Name: ChatsFormat.Name, Steps: ChatsFormat.Steps[:4]} // before quotes
	db, err := Open(ctx, dir+"/chats.db", Options{Format: old, Snapshots: dir + "/snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	c := &ChatsDB{DB: db}
	ch, err := c.EnsureMother(ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := addTurn(t, c, ch.ID, 1, "zebra crossing")
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
	c, err = OpenChats(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: ch.ID, Turn: 2, Role: chat.RoleUser, Parts: []chat.Part{
		{Kind: chat.PartQuote, Quote: &chat.Quote{Text: "zebra", Message: src.ID, Turn: 1, Role: chat.RoleUser}},
		{Kind: chat.PartText, Text: &chat.Text{Text: "more"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if n := ftsCount(t, c, "zebra"); n != 2 {
		t.Fatalf("%d index rows, want 2", n)
	}
	if _, err := c.Clear(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	if n := ftsCount(t, c, "zebra"); n != 0 {
		t.Errorf("after Clear: %d index rows, want 0", n)
	}
}
