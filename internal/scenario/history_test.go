package scenario

import (
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

func text(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }

// splitCards takes the cards off the turn's user message and the text of
// the turns before it, cards left out.
func TestSplitCards(t *testing.T) {
	card := "[Turn 3 of this chat, before the recent turns. read_messages(chat_id: 01C, from: 3) has its tool results.\nUser: I paid 48,200 euros.]"
	ms := []chat.Message{
		{Turn: 22, Role: chat.RoleUser, Parts: []chat.Part{text("What is BTC worth?")}},
		{Turn: 22, Role: chat.RoleAssistant, Parts: []chat.Part{text("About 60,000 euros.")}},
		{Turn: 25, Role: chat.RoleUser, Parts: []chat.Part{text(card), text("What did I pay?")}},
	}
	cards, window := splitCards(ms, 25)
	if cards != card {
		t.Errorf("cards: %q", cards)
	}
	if !strings.Contains(window, "60,000") || strings.Contains(window, "48,200") || strings.Contains(window, "What did I pay") {
		t.Errorf("window: %q", window)
	}
}

// from_history passes on a card or a search_history result, and fails
// when the window already holds the match or nothing brought it back.
func TestFromHistory(t *testing.T) {
	sc := &Scenario{set: &Set{}}
	a := Assert{Type: "from_history", Test: "x", Match: "48,200", Turn: "2"}
	events := []event{{kind: evUser, turn: 1}, {kind: evUser, turn: 2}}
	for name, c := range map[string]struct {
		rec  record
		want string
	}{
		"card":   {record{cards: map[int]string{2: "[Turn 1 … 48,200]"}, window: map[int]string{2: ""}}, Pass},
		"window": {record{cards: map[int]string{2: "48,200"}, window: map[int]string{2: "I paid 48,200"}}, Fail},
		"none":   {record{cards: map[int]string{}, window: map[int]string{2: "hi"}}, Fail},
		"search": {record{window: map[int]string{2: ""}, events: []event{{kind: evResult, turn: 2, tool: "search_history", text: "turn 1: 48,200 euros"}}}, Pass},
		"other":  {record{window: map[int]string{2: ""}, events: []event{{kind: evResult, turn: 2, tool: "query", text: "48,200"}}}, Fail},
		"unseen": {record{}, Skip},
	} {
		rec := c.rec
		rec.sc = sc
		rec.events = append(slices.Clone(events), rec.events...)
		if st, why, _ := rec.check(a); st != c.want {
			t.Errorf("%s: %s (%s), want %s", name, st, why, c.want)
		}
	}
}

// memory_contains reads user and project memory.
func TestMemoryContains(t *testing.T) {
	sc := &Scenario{set: &Set{}}
	a := Assert{Type: "memory_contains", Test: "x", Match: "YYYY-MM-DD"}
	sec := []store.MemorySection{{Section: "conventions", Content: "Write dates as YYYY-MM-DD."}}
	for name, c := range map[string]struct {
		rec  record
		want string
	}{
		"user":    {record{userMemory: sec}, Pass},
		"project": {record{projectMemory: sec}, Pass},
		"none":    {record{userMemory: []store.MemorySection{{Section: "goals", Content: "Track BTC."}}}, Fail},
	} {
		rec := c.rec
		rec.sc = sc
		if st, why, _ := rec.check(a); st != c.want {
			t.Errorf("%s: %s (%s), want %s", name, st, why, c.want)
		}
	}
}
