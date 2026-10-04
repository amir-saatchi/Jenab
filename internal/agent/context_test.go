package agent

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func TestCutOnTokens(t *testing.T) {
	set := testSettings()
	set.Context.HistoryMaxTokens = 100 // about 400 bytes
	h := newHarness(t, t.TempDir(), "", set)
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	long := strings.Repeat("word ", 30) // 150 bytes
	for n := 1; n <= 6; n++ {
		h.turn(c.ID, fmt.Sprint(n, " ", long))
	}
	// Each turn adds about 40 tokens: the window passes 100 tokens with
	// three earlier turns, and is cut back to three.
	var got [][]int
	for _, req := range h.fp.Calls() {
		got = append(got, turnsOf(req.Messages))
	}
	want := [][]int{{1}, {1, 2}, {1, 2, 3}, {1, 2, 3, 4}, {2, 3, 4, 5}, {3, 4, 5, 6}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("windows %v, want %v", got, want)
	}
	sys := h.fp.Calls()[4].System
	if last := sys[len(sys)-1].Text; !strings.Contains(last, "has 1 earlier turn.") {
		t.Errorf("turn 5: %q", last)
	}
}

func call(id string) chat.Part {
	return chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: id, Name: "big", Args: []byte(`{}`)}}
}

func result(id, ref string) chat.Part {
	text := "small"
	if ref != "" {
		text = "[output of big — 2,000 tokens, showing first 1,500 — ref: " + ref + "]\nrows\n[use read_ref(ref, offset) or search_ref(ref, query) for more; the next offset is 6000]"
	}
	return chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: id, Text: text, Ref: ref}}
}

func TestShape(t *testing.T) {
	user := func(turn int) chat.Message {
		return chat.Message{Turn: turn, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "hi"}}}}
	}
	ms := []chat.Message{
		user(1),
		{Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{call("a"), call("b")}},
		{Turn: 1, Role: chat.RoleTool, Parts: []chat.Part{result("a", "cache/tool/a"), result("b", "")}},
		{Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{call("lost1")}}, // a crash
		user(2),
		{Turn: 2, Role: chat.RoleAssistant, Parts: []chat.Part{call("c"), call("lost2")}},
		{Turn: 2, Role: chat.RoleTool, Parts: []chat.Part{result("c", "cache/tool/c")}},
		{Turn: 2, Role: chat.RoleAssistant, Parts: []chat.Part{call("lost3")}}, // the last message
	}
	before := jsonOf(t, ms)
	got := shape(ms, &window{stubBelow: 2, stubbed: map[string]bool{}})
	if jsonOf(t, ms) != before {
		t.Error("shape changed its input")
	}
	validHistory(t, got)
	want := []string{
		"user: hi", "assistant: call big", "assistant: call big",
		"tool: result [output of big — ref: cache/tool/a]", "tool: result small",
		"assistant: call big", "tool: result no result: the app closed before this call finished",
		"user: hi", "assistant: call big", "assistant: call big",
		"tool: result " + result("c", "cache/tool/c").ToolResult.Text,
		"tool: result no result: the app closed before this call finished",
		"assistant: call big", "tool: result no result: the app closed before this call finished",
	}
	if g := texts(got); !slices.Equal(g, want) {
		t.Errorf("shape:\n%q\nwant\n%q", g, want)
	}
	// A repair is in the turn of the call it closes.
	if got[4].Turn != 1 || got[8].Turn != 2 || got[10].Turn != 2 {
		t.Errorf("repairs in turns %d, %d and %d; want 1, 2 and 2", got[4].Turn, got[8].Turn, got[10].Turn)
	}
	// A result trimmed in its turn is a stub too.
	got = shape(ms, &window{stubBelow: 1, stubbed: map[string]bool{"c": true}})
	if g := texts(got); g[3] != "tool: result "+result("a", "cache/tool/a").ToolResult.Text || g[10] != "tool: result [output of big — ref: cache/tool/c]" {
		t.Errorf("trimmed: %q", g)
	}
}

func TestEarlierTurnsAndFirstLine(t *testing.T) {
	if got := earlierTurns(1); got != "This chat has 1 earlier turn. Use search_history or read_messages." {
		t.Error(got)
	}
	if got := earlierTurns(38); got != "This chat has 38 earlier turns. Use search_history or read_messages." {
		t.Error(got)
	}
	for in, want := range map[string]string{"": "", "\n  \nTrack prices.\nDaily.": "Track prices", "One line": "One line"} {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}
