package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// TestQuote: a reply to part of a message stores a quote part before its
// text, with the source's turn and role, and the model reads it as plain
// text marked as a quote; a long quote is cut and says where the whole
// message is. The requests stay append-only (TASK-019).
func TestQuote(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	// 1 + 2 bytes each, so the cut at chat.QuoteMax falls inside a letter.
	long := "a" + strings.Repeat("ب", chat.QuoteMax/2)
	h.fp.Push(fake.Text("Prices rose.\n\nسلام دنیا\nsecond line"))
	h.turn(c.ID, "How are prices?")
	ms := h.messages(c.ID)
	user, answer := ms[0].ID, id.Message("")
	for _, m := range ms {
		if m.Role == chat.RoleAssistant {
			answer = m.ID
		}
	}

	h.fp.Push(fake.Text("Because."))
	reply, err := h.o.Send(context.Background(), h.pid, c.ID, UserMessage{Text: "Why?",
		Quote: &chat.Quote{Text: " سلام دنیا\nsecond line \n", Message: answer, Turn: 9, Role: chat.RoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	h.wait()
	h.fp.Push(fake.Text("It is long."))
	if _, err := h.o.Send(context.Background(), h.pid, c.ID, UserMessage{Text: "And this?",
		Quote: &chat.Quote{Text: long, Message: user}}); err != nil {
		t.Fatal(err)
	}
	h.wait()

	ms = h.messages(c.ID)
	var got *chat.Message
	for i := range ms {
		if ms[i].ID == reply {
			got = &ms[i]
		}
	}
	want := chat.Quote{Text: "سلام دنیا\nsecond line", Message: answer, Turn: 1, Role: chat.RoleAssistant}
	if got == nil || len(got.Parts) != 2 || got.Parts[0].Quote == nil || *got.Parts[0].Quote != want || got.Parts[1].Text == nil {
		t.Fatalf("the reply is stored as %+v (quote %+v), want the quote %+v and then the text", got, got.Parts[0].Quote, want)
	}
	last := ms[len(ms)-2] // the second reply, before its answer
	q := last.Parts[0].Quote
	if q == nil || !q.Cut || q.Role != chat.RoleUser || len(q.Text) > chat.QuoteMax || !utf8.ValidString(q.Text) {
		t.Fatalf("the long quote is stored as %+v", q)
	}

	calls := h.fp.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d requests, want 3", len(calls))
	}
	sent := func(i int) []string {
		ms := calls[i].Messages
		var out []string
		for _, p := range ms[len(ms)-1].Parts {
			if p.Quote != nil || p.Text == nil {
				t.Fatalf("request %d sends a part that isn't text: %+v", i+1, p)
			}
			out = append(out, p.Text.Text)
		}
		return out
	}
	if got, want := sent(1), []string{"[Quote from your answer in turn 1]\n> سلام دنیا\n> second line", "Why?"}; !equal(got, want) {
		t.Errorf("the reply is sent as %q, want %q", got, want)
	}
	cut := sent(2)
	tail := "\n[The quote is cut. read_messages(chat_id: " + string(c.ID) + ", from: 1) has the whole message.]"
	if len(cut) != 2 || !strings.HasPrefix(cut[0], "[Quote from my message in turn 1]\n> aببب") || !strings.HasSuffix(cut[0], tail) || cut[1] != "And this?" {
		t.Errorf("the long quote is sent as %q", cut)
	}
	appendOnly(t, calls[1:])
}

func equal(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

// TestQuoteRefused: a quote must come from a user or assistant message of
// the same chat, have text, and come with a message.
func TestQuoteRefused(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c, other := h.newChat(chat.Chat{Title: "Coins"}), h.newChat(chat.Chat{Title: "Other"})
	h.fp.Push(fake.Text("ok"))
	h.turn(other.ID, "elsewhere")
	elsewhere := h.messages(other.ID)[0].ID
	var tool id.Message
	h.open(func(p *project.Project) {
		m, _, err := p.Chats.AppendMessage(context.Background(), chat.Message{Chat: c.ID, Turn: 1, Role: chat.RoleTool, Parts: []chat.Part{
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c1", Text: "a result"}}}})
		if err != nil {
			t.Fatal(err)
		}
		tool = m.ID
	})
	for _, tc := range []struct {
		name string
		m    UserMessage
		err  error
	}{
		{"another chat", UserMessage{Text: "x", Quote: &chat.Quote{Text: "elsewhere", Message: elsewhere}}, ErrQuote},
		{"a missing message", UserMessage{Text: "x", Quote: &chat.Quote{Text: "a", Message: "01JNOPE"}}, ErrQuote},
		{"a tool result", UserMessage{Text: "x", Quote: &chat.Quote{Text: "a result", Message: tool}}, ErrQuote},
		{"no text", UserMessage{Text: "x", Quote: &chat.Quote{Text: " \n", Message: elsewhere}}, ErrQuote},
		{"a quote alone", UserMessage{Text: " ", Quote: &chat.Quote{Text: "a", Message: tool}}, ErrEmpty},
	} {
		if _, err := h.o.Send(context.Background(), h.pid, c.ID, tc.m); !errors.Is(err, tc.err) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.err)
		}
	}
	if ms := h.messages(c.ID); len(ms) != 1 {
		t.Errorf("%d messages stored, want only the seeded one", len(ms))
	}
}

// TestQuoteCardText: a history card's request shows the quote with its
// lines marked, and the card's words include it.
func TestQuoteCardText(t *testing.T) {
	m := chat.Message{Role: chat.RoleUser, Parts: []chat.Part{
		{Kind: chat.PartQuote, Quote: &chat.Quote{Text: "line one\nline two", Message: "m", Turn: 1, Role: chat.RoleAssistant}},
		{Kind: chat.PartText, Text: &chat.Text{Text: "why?"}},
	}}
	if got, want := messageText(m), "> line one\n> line two\nwhy?"; got != want {
		t.Errorf("messageText = %q, want %q", got, want)
	}
}
