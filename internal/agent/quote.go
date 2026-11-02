package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Reply to part of a message (TASK-019): a user message may quote a
// passage of an earlier user or assistant message of its chat. The quote
// is stored as its own part, before the text, and the model reads it as
// plain text marked as a quote, the same for every model (SPEC 1).

// quoteOf is q checked against its source in chat c, with the source's
// turn and role filled in, and its text cut at chat.QuoteMax.
func quoteOf(ctx context.Context, proj *project.Project, c id.Chat, q chat.Quote) (chat.Quote, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" || q.Message == "" {
		return q, ErrQuote
	}
	src, err := proj.Chats.MessageHead(ctx, q.Message)
	if errors.Is(err, store.ErrNotFound) {
		return q, ErrQuote
	}
	if err != nil {
		return q, err
	}
	if src.Chat != c || src.Role != chat.RoleUser && src.Role != chat.RoleAssistant {
		return q, ErrQuote
	}
	out := chat.Quote{Message: q.Message, Turn: src.Turn, Role: src.Role}
	var cut bool
	out.Text, cut = cutQuote(text)
	out.Cut = cut || q.Cut
	return out, nil
}

// cutQuote cuts s to at most chat.QuoteMax bytes, at a character. It
// reports whether it cut.
func cutQuote(s string) (string, bool) {
	if len(s) <= chat.QuoteMax {
		return s, false
	}
	i := chat.QuoteMax
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return strings.TrimSpace(s[:i]), true
}

// quoteText is a quote as the model reads it: a line naming the source,
// then the passage with each line marked "> ". A cut quote ends with where
// the whole message is.
func quoteText(c id.Chat, q *chat.Quote) string {
	whose := "your answer"
	if q.Role == chat.RoleUser {
		whose = "my message"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Quote from %s in turn %d]", whose, q.Turn)
	for line := range strings.SplitSeq(q.Text, "\n") {
		b.WriteString("\n>")
		if line = strings.TrimRight(line, " \t\r"); line != "" {
			b.WriteString(" " + line)
		}
	}
	if q.Cut {
		fmt.Fprintf(&b, "\n[The quote is cut. read_messages(chat_id: %s, from: %d) has the whole message.]", c, q.Turn)
	}
	return b.String()
}

// quotePart is a quote part as the model reads it: a text part.
func quotePart(c id.Chat, q *chat.Quote) chat.Part {
	return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: quoteText(c, q)}}
}
