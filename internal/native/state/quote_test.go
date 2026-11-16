package state

// Ported from the React app's state/quote.test.ts. The chip's test ports with
// the composer (Part B).

import (
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func quoted(role chat.Role, texts ...string) *chat.Message {
	m := msg("m1", 3, role, texts...)
	return &m
}

func TestAShortQuoteIsKeptTrimmed(t *testing.T) {
	text, cut := CutQuote("  سلام دنیا \n")
	same(t, []any{text, cut}, []any{"سلام دنیا", false})
}

func TestALongQuoteIsCutAtACharacter(t *testing.T) {
	text, cut := CutQuote("a" + strings.Repeat("ب", chat.QuoteMax/2))
	same(t, cut, true)
	same(t, len(text), chat.QuoteMax-1) // the last letter didn't fit
	if !strings.HasSuffix(text, "ب") {
		t.Fatal("cut inside a letter")
	}
	// Astral characters are not split either.
	emoji, _ := CutQuote(strings.Repeat("😀", chat.QuoteMax))
	same(t, len(emoji), chat.QuoteMax)
	same(t, emoji, strings.Repeat("😀", chat.QuoteMax/4))
}

func TestAQuoteNamesItsMessageTurnAndRole(t *testing.T) {
	same(t, *QuoteOf(quoted(chat.RoleAssistant, "x"), " the passage "),
		chat.Quote{Text: "the passage", Message: "m1", Turn: 3, Role: chat.RoleAssistant})
	same(t, QuoteOf(quoted(chat.RoleUser, "x"), strings.Repeat("ب", chat.QuoteMax)).Cut, true)
}

func TestToolMessagesAndEmptySelectionsCantBeQuoted(t *testing.T) {
	if QuoteOf(quoted(chat.RoleTool, "x"), "x") != nil {
		t.Fatal("a tool message was quoted")
	}
	if QuoteOf(quoted(chat.RoleUser, "x"), " \n ") != nil {
		t.Fatal("an empty selection was quoted")
	}
}

func TestReplyOnAWholeMessageQuotesItsTextParts(t *testing.T) {
	same(t, MessageText(quoted(chat.RoleAssistant, "first", " ", "second\nline")), "first\n\nsecond\nline")
}

func TestOneQuotePerChatRemovable(t *testing.T) {
	qs := Quotes{}
	q := QuoteOf(quoted(chat.RoleUser, "x"), "x")
	qs.Set("c1", q)
	y := *q
	y.Text = "y"
	qs.Set("c2", &y)
	same(t, qs["c1"].Text, "x")
	qs.Set("c1", nil)
	if _, ok := qs["c1"]; ok {
		t.Fatal("the quote was kept")
	}
	same(t, qs["c2"].Text, "y")
}
