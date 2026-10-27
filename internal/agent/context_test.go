package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
	"github.com/amir-saatchi/jenab/internal/web"
)

// Block 1 tells the model only of work its tools can do (TASK-011,
// TASK-014). With the built-in tools, which work on tables only (P2-05),
// pipelines and views, the prompt rule, Mother's delegation lines and the
// config-guide skill are left out; the sql-queries skill is listed.
func TestBlockOneBuiltin(t *testing.T) {
	tools := append(tool.Builtin(tool.Deps{Web: web.NewClient(web.Options{})}), Tools()...)
	h := newHarness(t, t.TempDir(), "", testSettings(), tools...)
	defer h.stop()
	var err error
	if h.o.d.Skills, err = skill.Builtin(); err != nil {
		t.Fatal(err)
	}
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.turn(c.ID, "hi")
	h.turn(h.mother().ID, "hi")
	for i, req := range h.fp.Calls() {
		sys := req.System[0].Text
		if strings.Contains(sys, "Requests with several parts") || strings.Contains(sys, "You direct the other chats") ||
			strings.Contains(sys, "config-guide") || !strings.Contains(sys, "- sql-queries: ") ||
			!strings.Contains(sys, "keeps a project: tables in a local database. This version has no pipelines or views yet. You work") ||
			!strings.Contains(sys, "If no tool can do what the user asks") {
			t.Errorf("request %d, block 1:\n%s", i, sys)
		}
	}
}

// Block 1 names the parts of a project its tools work on (TASK-014).
func TestBlockOneAreas(t *testing.T) {
	with := func(as ...tool.Areas) []tool.Tool {
		var out []tool.Tool
		for _, a := range as {
			s := tool.Spec{Name: "t", Description: "Test.", Schema: json.RawMessage(`{"type": "object"}`), Areas: a}
			out = append(out, tool.Func(s, func(context.Context, *tool.Env, struct{}) (tool.Result, error) { return tool.Result{}, nil }))
		}
		return out
	}
	for _, c := range []struct {
		tools []tool.Tool
		want  string
	}{
		{nil, "keeps a project. This version has no tables, pipelines, or views yet. You work"},
		{with(0), "keeps a project. This version has no tables, pipelines, or views yet. You work"},
		{with(tool.Views), "keeps a project: views. This version has no tables or pipelines yet. You work"},
		{with(tool.Tables, tool.Tables|tool.Views), "keeps a project: tables in a local database and views. This version has no pipelines yet. You work"},
		{with(tool.Pipelines, tool.Views, tool.Tables), "keeps a project: tables in a local database, pipelines that fetch data, and views. You work"},
	} {
		if got := base(c.tools); !strings.Contains(got, c.want) || strings.Contains(got, "{") {
			t.Errorf("want %q in:\n%s", c.want, got)
		}
	}
}

// The prompt files name the app and the Mother chat only through {app}
// and {mother}, and only the files that go through promptNames use them.
func TestPromptNames(t *testing.T) {
	if got := base(nil); !strings.HasPrefix(got, "You are the agent of a "+appinfo.Name+" project. "+appinfo.Name+" is ") {
		t.Errorf("base: %q", got)
	}
	if got := promptNames.Replace(motherPrompt); !strings.HasPrefix(got, "This is the "+appinfo.Mother+" chat:") {
		t.Errorf("mother: %q", got)
	}
	for name, p := range map[string]string{"rule.md": rulePrompt, "delegate.md": delegatePrompt} {
		if strings.Contains(p, "{app}") || strings.Contains(p, "{mother}") {
			t.Errorf("%s uses a name placeholder, but isn't filled in", name)
		}
	}
}

// The lines come back with the tools: the prompt rule with a background
// tool, the delegation lines with create_chat, and a skill with a tool in
// its load_with.
func TestBlockOneFollowsTools(t *testing.T) {
	noop := func(s tool.Spec) tool.Tool {
		s.Description, s.Schema = "Test.", json.RawMessage(`{"type": "object"}`)
		return tool.Func(s, func(context.Context, *tool.Env, struct{}) (tool.Result, error) { return tool.Result{}, nil })
	}
	h := newHarness(t, t.TempDir(), "", testSettings(), loadSkill(),
		noop(tool.Spec{Name: "subagent", Background: true}), noop(tool.Spec{Name: "create_chat", Mother: true}), noop(tool.Spec{Name: "save_view"}))
	defer h.stop()
	var err error
	if h.o.d.Skills, err = skill.Builtin(); err != nil {
		t.Fatal(err)
	}
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.turn(c.ID, "hi")
	h.turn(h.mother().ID, "hi")
	calls := h.fp.Calls()
	if sys := calls[0].System[0].Text; !strings.Contains(sys, strings.TrimSpace(rulePrompt)) ||
		strings.Contains(sys, "You direct the other chats") || !strings.Contains(sys, "\n- config-guide: ") {
		t.Errorf("block 1:\n%s", sys)
	}
	if sys := calls[1].System[0].Text; !strings.Contains(sys, strings.TrimSpace(rulePrompt)) || !strings.Contains(sys, strings.TrimSpace(delegatePrompt)) {
		t.Errorf("Mother's block 1:\n%s", sys)
	}
}

// Block 1 has today's date, and a cut on a new day changes it.
func TestDateAtCut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		day1 := today(time.Now())
		h.turn(c.ID, "hi")
		time.Sleep(time.Minute)
		h.turn(c.ID, "and now?") // no cut: the date stays
		time.Sleep(24 * time.Hour)
		day2 := today(time.Now())
		h.turn(c.ID, "and today?")
		var got []bool
		for _, req := range h.fp.Calls() {
			got = append(got, strings.Contains(req.System[0].Text, day1))
		}
		if !slices.Equal(got, []bool{true, true, false}) || day1 == day2 || !strings.Contains(h.fp.Calls()[2].System[0].Text, day2) {
			t.Errorf("day 1 in block 1: %v; day 2 %q", got, day2)
		}
	})
}

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

func TestCardBlock(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	var asked []id.Project
	h.o.d.Card = func(_ context.Context, p *project.Project) (string, error) {
		asked = append(asked, p.ID)
		return "Project card\nTables: prices", nil
	}
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.turn(c.ID, "hello")
	h.turn(h.mother().ID, "hello")
	var names [][]string
	for _, req := range h.fp.Calls() {
		var ns []string
		for _, b := range req.System {
			ns = append(ns, b.Name)
		}
		names = append(names, ns)
		if last := req.System[len(req.System)-1]; !last.Cache {
			t.Errorf("no cache point after %q", last.Name)
		}
	}
	want := [][]string{
		{"System prompt, role and skills", "Project card"},
		{"System prompt, role and skills", "Project card", "Chat list"},
	}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("blocks %q, want %q", names, want)
	}
	if card := h.fp.Calls()[1].System[1]; card.Text != "Project card\nTables: prices" || card.Cache {
		t.Errorf("card block %+v", card)
	}
	if len(asked) != 2 || asked[0] != h.pid {
		t.Errorf("Card asked for %v, want %s twice", asked, h.pid)
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

// TestToolIndexHook: Deps.ToolIndex's text joins block 1 after the prompt
// rule; nil, or "", leaves block 1 as it is (SPIKE-028, spike-only).
func TestToolIndexHook(t *testing.T) {
	block1 := func(index func(chat.Kind) string) string {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.o.d.ToolIndex = index
		c := h.newChat(chat.Chat{Title: "Index"})
		h.fp.Push(fake.Text("ok"))
		h.turn(c.ID, "hi")
		return h.fp.Calls()[0].System[0].Text
	}
	plain := block1(nil)
	if got := block1(func(chat.Kind) string { return " " }); got != plain {
		t.Errorf("an empty index changed block 1:\n%s", got)
	}
	var kinds []chat.Kind
	index := "Tool index:\n- x: does x"
	got := block1(func(k chat.Kind) string { kinds = append(kinds, k); return index + "\n" })
	if strings.Replace(got, "\n\n"+index, "", 1) != plain || !strings.Contains(got, index+"\n\nToday is") {
		t.Errorf("block 1 with the index:\n%s\nwithout:\n%s", got, plain)
	}
	if !slices.Equal(kinds, []chat.Kind{chat.KindChat}) {
		t.Errorf("kinds %v", kinds)
	}
}
