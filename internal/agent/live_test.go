package agent

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// TestEarlierHook: Deps.Earlier gets the messages and the current turn,
// and the request gets what it returns (SPIKE-037 Part B, spike-only).
func TestEarlierHook(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	var mu sync.Mutex
	var curs []int
	h.o.d.Earlier = func(ms []chat.Message, cur int) []chat.Message {
		mu.Lock()
		curs = append(curs, cur)
		mu.Unlock()
		var out []chat.Message
		for _, m := range ms {
			if m.Turn < cur && m.Role == chat.RoleAssistant {
				m.Parts = []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "kept"}}}
			}
			out = append(out, m)
		}
		return out
	}
	c := h.newChat(chat.Chat{Title: "Hook"})
	h.fp.Push(fake.Text("first"), fake.Text("second"))
	h.turn(c.ID, "one")
	h.turn(c.ID, "two")
	calls := h.fp.Calls()
	want := []string{"user: one", "assistant: kept", "user: two"}
	if got := texts(calls[len(calls)-1].Messages); !slices.Equal(got, want) {
		t.Errorf("request:\n%q\nwant\n%q", got, want)
	}
	if g := texts(h.messages(c.ID)); !slices.Contains(g, "assistant: first") {
		t.Errorf("the store changed: %q", g)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(curs, 1) || !slices.Contains(curs, 2) || slices.ContainsFunc(curs, func(n int) bool { return n < 1 || n > 2 }) {
		t.Errorf("current turns %v", curs)
	}
}

func TestLiveRule(t *testing.T) {
	if got := New(Deps{}).rule(); got != strings.TrimSpace(rulePrompt) {
		t.Errorf("rule without a live answer:\n%s", got)
	}
	got := New(Deps{LiveAnswer: true}).rule()
	if !strings.Contains(got, liveRule) || strings.Contains(got, "without repeating") || !strings.Contains(got, "3. Say what is still running.") {
		t.Errorf("live rule:\n%s", got)
	}
}
