package store

import (
	"context"
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// TestTurnsWith: the turns of one chat before a turn whose text has a word
// starting with the word, as Search matches it: normalized, with a ZWNJ
// or without, and never in tool results.
func TestTurnsWith(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a, b := newChat(t, c, chat.Chat{Title: "a"}), newChat(t, c, chat.Chat{Title: "b"})
	addTurn(t, c, a.ID, 1, "the weekly volatility table")
	addTurn(t, c, a.ID, 2, "Volatility again")
	addTurn(t, c, a.ID, 3, "نمودار میانگین ۲۰ روزه")
	addTurn(t, c, a.ID, 4, "volatile prices")
	addTurn(t, c, a.ID, 5, "volatility, later")
	addTurn(t, c, b.ID, 1, "volatility in chat b")
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 3, Role: chat.RoleTool, Parts: []chat.Part{
		{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c1", Text: "volatility 4.1"}},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		word   string
		before int
		want   []int
	}{
		{"volatility", 5, []int{1, 2}},
		{"volatil", 6, []int{1, 2, 4, 5}},
		{"VOLATILITY", 2, []int{1}},
		{"میانگین", 5, []int{3}},
		{"۲۰", 5, []int{3}},
		{"20", 5, []int{3}},
		{"absent", 5, nil},
	} {
		got, err := c.TurnsWith(ctx, a.ID, tc.word, tc.before)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("TurnsWith(%q, %d) = %v, %v; want %v", tc.word, tc.before, got, err, tc.want)
		}
	}
}

// TestPartsWith: text parts across the project's chats, counted up to the
// limit.
func TestPartsWith(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a, b := newChat(t, c, chat.Chat{Title: "a"}), newChat(t, c, chat.Chat{Title: "b"})
	for i := range 5 {
		addTurn(t, c, a.ID, i+1, "make the chart")
	}
	addTurn(t, c, b.ID, 1, "make a table")
	addTurn(t, c, b.ID, 2, "something else")
	if n, err := c.TextParts(ctx); err != nil || n != 7 {
		t.Errorf("TextParts = %d, %v; want 7", n, err)
	}
	for _, tc := range []struct {
		word  string
		limit int
		want  int
	}{
		{"make", 100, 6},
		{"make", 3, 3},
		{"chart", 100, 5},
		{"else", 100, 1},
		{"absent", 100, 0},
	} {
		if n, err := c.PartsWith(ctx, tc.word, tc.limit); err != nil || n != tc.want {
			t.Errorf("PartsWith(%q, %d) = %d, %v; want %d", tc.word, tc.limit, n, err, tc.want)
		}
	}
}
