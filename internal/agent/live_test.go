package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func TestEarlierTurns(t *testing.T) {
	text := func(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }
	callArgs := func(id, name, args string) chat.Part {
		return chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: id, Name: name, Args: json.RawMessage(args)}}
	}
	res := func(id, txt, ref string, bad bool) chat.Part {
		return chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: id, Text: txt, Ref: ref, IsError: bad}}
	}
	cut := chat.Notice{Kind: chat.NoticeAnswerCut, Text: "[cut]"}
	ms := []chat.Message{
		{Turn: 1, Role: chat.RoleUser, Parts: []chat.Part{text("find jobs")}},
		{Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "hm"}}, text("looking"),
			callArgs("a", "fetch_page", `{"url": "jobs.example.com/1"}`), callArgs("b", "fetch_page", `{"url": "jobs.example.com/2"}`)}},
		{Turn: 1, Role: chat.RoleTool, Parts: []chat.Part{res("a", "page 1", "cache/pages/a.txt", false), res("b", "gone", "", true)}},
		{Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{callArgs("c", "create_table", `{"columns": [], "name": "jobs"}`), callArgs("d", "subagent", `{"task": "search   LinkedIn", "background": true}`)}},
		{Turn: 1, Role: chat.RoleTool, Parts: []chat.Part{res("c", "ok", "", false), res("d", "started: task t_4", "", false)}},
		{Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{text("Two jobs.")}},
		{Turn: 2, Role: chat.RoleUser, Parts: []chat.Part{text("more")}},
		{Turn: 2, Role: chat.RoleAssistant, Parts: []chat.Part{callArgs("e", "web_search", `{"query": "x"}`)}},
		{Turn: 2, Role: chat.RoleTool, Parts: []chat.Part{res("e", "r", "", false)}},
		{Turn: 2, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &cut}}},
		{Turn: 3, Role: chat.RoleUser, Parts: []chat.Part{text("now")}},
		{Turn: 3, Role: chat.RoleAssistant, Parts: []chat.Part{callArgs("f", "web_search", `{"query": "y"}`)}},
	}
	before := jsonOf(t, ms)

	o := New(Deps{})
	if got := o.earlier(ms, 3); len(got) != len(ms) {
		t.Errorf("today's window changed the messages: %d, want %d", len(got), len(ms))
	}

	o = New(Deps{EarlierTurns: EarlierTurns{Mode: EarlierAnswers}})
	got := shape(o.earlier(ms, 3), &window{stubbed: map[string]bool{}})
	if jsonOf(t, ms) != before {
		t.Error("earlier changed its input")
	}
	validHistory(t, got)
	want := []string{"user: find jobs", "assistant: Two jobs.", "user: more", "user: [cut]", "user: now", "assistant: call web_search", "tool: result no result: the app closed before this call finished"}
	if g := texts(got); !slices.Equal(g, want) {
		t.Errorf("answers:\n%q\nwant\n%q", g, want)
	}

	line := func(c chat.ToolCall, r *chat.ToolResult) (string, string) {
		if c.Name == "subagent" && r != nil {
			_, id, _ := strings.Cut(r.Text, "task ")
			return "", "task " + id
		}
		return "", ""
	}
	o = New(Deps{EarlierTurns: EarlierTurns{Mode: EarlierTrace, Line: line}})
	got = shape(o.earlier(ms, 3), &window{stubbed: map[string]bool{}})
	validHistory(t, got)
	want = []string{
		"user: find jobs",
		"assistant: [fetch_page ×2: jobs.example.com/1 → ref cache/pages/a.txt; jobs.example.com/2 → failed]\n" +
			"[create_table jobs]\n" +
			"[subagent \"search LinkedIn\" → task t_4]\n" +
			"[full steps: read_messages(from: 1, to: 1)]\n\nTwo jobs.",
		"user: more",
		"assistant: [web_search x]\n[full steps: read_messages(from: 2, to: 2)]",
		"user: [cut]", "user: now", "assistant: call web_search", "tool: result no result: the app closed before this call finished",
	}
	if g := texts(got); !slices.Equal(g, want) {
		t.Errorf("trace:\n%q\nwant\n%q", g, want)
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
