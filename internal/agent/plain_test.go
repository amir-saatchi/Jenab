package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// TestPlain: the plain chat harness (P2-13) sends block 1 alone and every
// earlier turn, past history_max_turns, with tool results as they came.
// Only when the history passes plainFull of the model's window are the
// oldest turns dropped, until it is under plainKeep.
func TestPlain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		size  int // bytes of each earlier user message
		first int // the first turn the request has
	}{
		{"whole history", 200, 1},
		{"near the window", 4000, 0}, // 0: some turn after 1
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings(), echo())
			defer h.stop()
			h.o.d.Plain = true
			c := h.newChat(chat.Chat{Title: "Coins"})
			turns := make([]oldTurn, 30)
			for i := range turns {
				turns[i] = oldTurn{user: fmt.Sprintf("Turn %d: %s", i+1, strings.Repeat("x", tc.size)), answer: "ok"}
			}
			turns[1] = oldTurn{user: "echo it", tool: "echo", args: map[string]any{"text": "kept"}, result: "the result, kept", answer: "done"}
			seedChat(h, c.ID, turns)
			h.fp.Push(fake.Text("ok"))
			h.turn(c.ID, "Next.")
			req := h.fp.Calls()[0]
			if len(req.System) != 1 || !strings.Contains(req.System[0].Text, "Today is") || !req.System[0].Cache {
				t.Errorf("system: %+v", req.System)
			}
			first := req.Messages[0].Turn
			switch {
			case tc.first == 1 && first != 1:
				t.Errorf("the request starts at turn %d", first)
			case tc.first == 0 && first <= 1:
				t.Error("nothing was dropped near the window")
			}
			if tc.first == 0 {
				if n := provider.EstimateTokens(provider.Request{System: req.System, Messages: req.Messages}); n > 32000/2+200 {
					t.Errorf("%d tokens after the drop", n)
				}
				return
			}
			found := false
			for _, m := range req.Messages {
				for _, p := range m.Parts {
					if p.ToolResult != nil && p.ToolResult.Text == "the result, kept" {
						found = true
					}
				}
			}
			if !found {
				t.Error("the tool result isn't in the request as it came")
			}
		})
	}
}
