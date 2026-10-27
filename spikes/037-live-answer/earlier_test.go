package main

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

	got := earlierTurns{}.earlier(ms, 3)
	if jsonOf(t, ms) != before {
		t.Error("earlier changed its input")
	}
	answersOnly(t, got, 3)
	want := []string{"user: find jobs", "assistant: Two jobs.", "user: more", "user: [cut]", "user: now", "assistant: call web_search"}
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
	got = earlierTurns{trace: true, line: line}.earlier(ms, 3)
	answersOnly(t, got, 3)
	want = []string{
		"user: find jobs",
		"assistant: [fetch_page ×2: jobs.example.com/1 → ref cache/pages/a.txt; jobs.example.com/2 → failed]\n" +
			"[create_table jobs]\n" +
			"[subagent \"search LinkedIn\" → task t_4]\n" +
			"[full steps: read_messages(from: 1, to: 1)]\n\nTwo jobs.",
		"user: more",
		"assistant: [web_search x]\n[full steps: read_messages(from: 2, to: 2)]",
		"user: [cut]", "user: now", "assistant: call web_search",
	}
	if g := texts(got); !slices.Equal(g, want) {
		t.Errorf("trace:\n%q\nwant\n%q", g, want)
	}
}

// answersOnly checks that the turns before cur hold only text and
// notices, none of it blank, so every provider accepts them.
func answersOnly(t *testing.T, ms []chat.Message, cur int) {
	t.Helper()
	for i, m := range ms {
		if m.Turn >= cur {
			continue
		}
		for _, p := range m.Parts {
			if (p.Text == nil && p.Notice == nil) || (p.Text != nil && strings.TrimSpace(p.Text.Text) == "") {
				t.Errorf("message %d (turn %d, %s): part %s", i, m.Turn, m.Role, p.Kind)
			}
		}
	}
}

// texts lists the text of every part, for messages compared in tests.
func texts(ms []chat.Message) []string {
	var out []string
	for _, m := range ms {
		for _, p := range m.Parts {
			switch {
			case p.Text != nil:
				out = append(out, string(m.Role)+": "+p.Text.Text)
			case p.ToolCall != nil:
				out = append(out, string(m.Role)+": call "+p.ToolCall.Name)
			case p.ToolResult != nil:
				out = append(out, string(m.Role)+": result "+p.ToolResult.Text)
			case p.Notice != nil:
				out = append(out, string(m.Role)+": "+p.Notice.Text)
			default:
				out = append(out, string(m.Role)+": "+string(p.Kind))
			}
		}
	}
	return out
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
