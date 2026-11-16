package state

import (
	"strings"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// CutQuote trims s and cuts it to chat.QuoteMax bytes, at a character
// (TASK-019). cut tells whether it was cut.
func CutQuote(s string) (text string, cut bool) {
	t := strings.TrimSpace(s)
	if len(t) <= chat.QuoteMax {
		return t, false
	}
	end := 0
	for i, r := range t {
		if i+utf8.RuneLen(r) > chat.QuoteMax {
			break
		}
		end = i + utf8.RuneLen(r)
	}
	return strings.TrimSpace(t[:end]), true
}

// MessageText is a message's text, for *Reply* on the whole message.
func MessageText(m *chat.Message) string {
	var out []string
	for _, p := range m.Parts {
		if p.Text != nil {
			if t := strings.TrimSpace(p.Text.Text); t != "" {
				out = append(out, t)
			}
		}
	}
	return strings.Join(out, "\n\n")
}

// QuoteOf is a quote of text from message m, or nil when m can't be
// quoted: only the user's and the assistant's text can.
func QuoteOf(m *chat.Message, text string) *chat.Quote {
	if m.Role != chat.RoleUser && m.Role != chat.RoleAssistant {
		return nil
	}
	t, cut := CutQuote(text)
	if t == "" {
		return nil
	}
	return &chat.Quote{Text: t, Message: m.ID, Turn: m.Turn, Role: m.Role, Cut: cut}
}

// Quotes is the composer's quote per chat: one per message, kept while
// another chat is open, like the draft (state/quote.ts).
type Quotes map[id.Chat]chat.Quote

// Set sets the chat's quote, or removes it when q is nil.
func (qs Quotes) Set(c id.Chat, q *chat.Quote) {
	if q == nil {
		delete(qs, c)
		return
	}
	qs[c] = *q
}
