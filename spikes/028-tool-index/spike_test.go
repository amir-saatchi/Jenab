package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
)

type echoArgs struct {
	URL  string   `json:"url"`
	Tags []string `json:"tags"`
}

func testRegistry() *tool.Registry {
	reg := tool.NewRegistry()
	reg.Add(
		tool.Func(tool.Spec{Name: "query", Description: "Run SQL.", Schema: json.RawMessage(`{"type":"object","properties":{"sql":{"type":"string"}},"required":["sql"]}`)},
			func(ctx context.Context, env *tool.Env, a struct{ SQL string }) (tool.Result, error) {
				return tool.Result{Text: "rows"}, nil
			}),
		tool.Func(tool.Spec{Name: "save_link", Description: "Save a link to Resources. With tags.",
			Schema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["url"],"additionalProperties":false}`)},
			func(ctx context.Context, env *tool.Env, a echoArgs) (tool.Result, error) {
				return tool.Result{Text: "saved " + a.URL + " " + strings.Join(a.Tags, ",")}, nil
			}),
		tool.Func(tool.Spec{Name: "list_chats", Description: "List the chats.", Mother: true, Schema: json.RawMessage(`{"type":"object"}`)},
			func(ctx context.Context, env *tool.Env, a struct{}) (tool.Result, error) {
				return tool.Result{Text: "chats"}, nil
			}),
	)
	return reg
}

func run(t *testing.T, reg *tool.Registry, name, args string) (tool.Result, error) {
	t.Helper()
	tl, ok := reg.Get(name)
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	return tool.Run(context.Background(), tl, tool.Call{Args: json.RawMessage(args)})
}

// The index keeps the core native, lists the rest in block 1, describes
// them and runs them with their own argument checks.
func TestIndex(t *testing.T) {
	reg, text := newIndex(testRegistry(), nil)
	var names []string
	for _, tl := range reg.For(chat.KindChat) {
		names = append(names, tl.Spec().Name)
	}
	if want := []string{"call_tool", "describe_tool", "query"}; !slices.Equal(names, want) {
		t.Errorf("native tools %v, want %v", names, want)
	}
	idx := text(chat.KindChat)
	if !strings.HasPrefix(idx, indexHead) || !strings.Contains(idx, "\n- save_link: Save a link to Resources.") || strings.Contains(idx, "list_chats") {
		t.Errorf("index:\n%s", idx)
	}
	if !strings.Contains(text(chat.KindMother), "- list_chats: List the chats.") {
		t.Errorf("Mother's index lacks list_chats:\n%s", text(chat.KindMother))
	}
	res, err := run(t, reg, describeName, `{"name":"save_link"}`)
	if err != nil || !strings.Contains(res.Text, `"additionalProperties":false`) {
		t.Errorf("describe: %v %q", err, res.Text)
	}
	res, err = run(t, reg, callName, `{"name":"save_link","arguments":{"url":"https://example.org","tags":["a","b"]}}`)
	if err != nil || res.Text != "saved https://example.org a,b" {
		t.Errorf("call: %v %q", err, res.Text)
	}
	_, err = run(t, reg, callName, `{"name":"save_link","arguments":{"link":"https://example.org"}}`)
	if !errors.Is(err, tool.ErrArgs) || !strings.Contains(err.Error(), "save_link takes no argument link") ||
		!strings.Contains(err.Error(), "save_link needs url") || !strings.Contains(err.Error(), "url (required)") {
		t.Errorf("wrong name: %v", err)
	}
	_, err = run(t, reg, callName, `{"name":"save_link","arguments":{"url":"https://example.org","tags":"a"}}`)
	if !errors.Is(err, tool.ErrArgs) {
		t.Errorf("wrong type: %v", err)
	}
	_, err = run(t, reg, callName, `{"name":"list_chats","arguments":{}}`)
	if err == nil || !strings.Contains(err.Error(), `there is no tool named "list_chats"`) {
		t.Errorf("a Mother tool in a chat: %v", err)
	}
}

func TestUnwrap(t *testing.T) {
	for _, c := range []struct{ name, args, wantName, wantArgs string }{
		{"query", `{"sql":"x"}`, "query", `{"sql":"x"}`},
		{"call_tool", `{"name":"save_link","arguments":{"url":"u"}}`, "save_link", `{"url":"u"}`},
		{"call_tool", `{"name":"save_link"}`, "save_link", `{}`},
		{"call_tool", `not json`, "call_tool", `not json`},
	} {
		if n, a := unwrap(c.name, c.args); n != c.wantName || a != c.wantArgs {
			t.Errorf("unwrap(%s, %s) = %s, %s", c.name, c.args, n, a)
		}
	}
}

// countCalls reads the transcript's calls the same way in both
// conditions.
func TestCountCalls(t *testing.T) {
	tr := strings.Join([]string{
		"## Turn 11",
		`> call save_link {"url":"u"}`,
		"> result save_link (error):",
		"> there is no tool named save_link",
		`> call call_tool→save_link {"link":"u"}`,
		"> result save_link (error):",
		"> wrong arguments: save_link takes no argument link.",
		`> call describe_tool {"name":"save_link"}`,
		"> result describe_tool:",
		"> save_link ...",
		`> call call_tool→save_link {"url":"u"}`,
		"> result save_link:",
		"> saved",
		`> call call_tool→search_history {"query":"chart"}`,
		"> result search_history:",
		"> 1 hit",
		`> call query {"sql":"x"}`,
		"> result query:",
		"> rows",
		`> call call_tool {"arguments":{"name":"save_link"}}`,
		"> result call_tool (error):",
		`> there is no tool named "call_tool". The tools in the index: save_link`,
	}, "\n")
	got := countCalls(tr)
	want := calls{Rare: 2, Indexed: 2, Describes: 1, ArgErrors: 1, Unknown: 2, Native: 1, Nested: 1, Blind: 2, Failed: 2, Recovered: 1, History: 1}
	if got != want {
		t.Errorf("countCalls = %+v\nwant %+v", got, want)
	}
	if fromTurn("# x\n\n## Turn 10\na\n## Turn 11\nb", 11) != "\n## Turn 11\nb" || fromTurn("x", 1) != "x" {
		t.Error("fromTurn")
	}
}

// The long chats' histories load with their own turn in place, and every
// referent turn is past the cut but the control's.
func TestHistories(t *testing.T) {
	for id, past := range map[string]bool{"H1_avg_chart": true, "H2_drop_alert": true, "H3_in_window": false, "H4_week_range": true, "H5_etf_digest": true} {
		h, err := loadHistory(historyDir, id)
		if err != nil {
			t.Fatal(err)
		}
		from := len(h.Turns) + 1
		if len(h.Turns) != 10 || (h.At < from-historyMin) != past {
			t.Errorf("%s: %d turns, at %d", id, len(h.Turns), h.At)
		}
		for i, tu := range h.Turns {
			if tu.User == "" || tu.Answer == "" {
				t.Errorf("%s: turn %d is not whole", id, i+1)
			}
		}
	}
}

// With the index, a skill whose load_with tools are all in the index stays
// listed and loadable, and call_tool loads it with its tool, as a native
// call would (8.9).
func TestSkillsWithIndex(t *testing.T) {
	all := testRegistry()
	set, err := skill.New(
		skill.Skill{Name: "links", Description: "Saving links. Load before save_link.", LoadWith: []string{"save_link"}, Body: "Tag every link."},
		skill.Skill{Name: "gone", Description: "About a tool this build lacks.", LoadWith: []string{"no_such_tool"}, Body: "x"},
	)
	if err != nil {
		t.Fatal(err)
	}
	native, _ := newIndex(all, set)
	has := func(n string) bool { _, ok := native.Get(n); return ok }
	if got := set.Usable(has).For(chat.KindChat); len(got) != 0 {
		t.Fatalf("the app's own Usable keeps %v; the test expects it to drop links", got)
	}
	kept := keepSkills(set, all, native).Usable(has)
	if _, ok := kept.Get(chat.KindChat, "links"); !ok {
		t.Error("links is hidden with the index")
	}
	if _, ok := kept.Get(chat.KindChat, "gone"); ok {
		t.Error("gone is listed, but condition 1 would hide it")
	}
	var loaded []string
	env := &tool.Env{Skill: func(ctx context.Context, name, file string) (tool.Result, error) {
		if _, ok := kept.Get(chat.KindChat, name); !ok {
			return tool.Result{}, tool.Errorf("there is no skill named %q", name)
		}
		loaded = append(loaded, name)
		return tool.Result{}, nil
	}}
	tl, _ := native.Get(callName)
	if _, err := tool.Run(context.Background(), tl, tool.Call{Args: json.RawMessage(`{"name":"save_link","arguments":{"url":"u"}}`), Env: env}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded, []string{"links"}) {
		t.Errorf("call_tool loaded %v, want [links]", loaded)
	}
}

// A native call of an index tool gets a pointer to call_tool in the
// request; other results, and requests without the index, stay.
func TestRedirect(t *testing.T) {
	res := func(id, text string) chat.Message {
		return chat.Message{Role: chat.RoleTool, Parts: []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: id, Text: text, IsError: true}}}}
	}
	req := provider.Request{
		System:   []provider.Block{{Text: "rules\n\n" + indexHead + "\n- save_link: Save a link."}},
		Tools:    []provider.ToolDef{{Name: callName}},
		Messages: []chat.Message{res("a", noTool+"save_link"), res("b", noTool+"made_up")},
	}
	out := redirect(req)
	if got := out.Messages[0].Parts[0].ToolResult.Text; !strings.Contains(got, `call_tool(name: "save_link"`) {
		t.Errorf("redirect: %q", got)
	}
	if out.Messages[1].Parts[0].ToolResult.Text != noTool+"made_up" || req.Messages[0].Parts[0].ToolResult.Text != noTool+"save_link" {
		t.Error("redirect changed a result it should leave, or the request it was given")
	}
	req.Tools = nil
	if redirect(req).Messages[0].Parts[0].ToolResult.Text != noTool+"save_link" {
		t.Error("redirect without the index")
	}
}

// call_tool refuses other argument names only where the tool's schema
// does, as a native call.
func TestCheckNamesOpen(t *testing.T) {
	open := tool.Spec{Name: "web_search", Schema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)}
	if err := checkNames(open, json.RawMessage(`{"query":"x","count":3}`)); err != nil {
		t.Errorf("an open schema: %v", err)
	}
	if err := checkNames(open, json.RawMessage(`{"count":3}`)); err == nil || !strings.Contains(err.Error(), "web_search needs query") {
		t.Errorf("a missing name: %v", err)
	}
}
