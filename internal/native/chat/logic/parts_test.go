package logic

// Ported from frontend/src/chat/parts.test.ts: what its assertions check
// about text. The MemoryNote ones check markup only (a <p>, dir="auto",
// nothing for no note) and have no text to port.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A desktop step shows the lengths of its texts, not the texts (SPEC 8.10).
func TestDesktopStepShowsLengthsNotTexts(t *testing.T) {
	cases := []struct {
		args json.RawMessage
		want string
	}{
		{json.RawMessage(`{"id":"2:e7","action":"set_value","text":"my secret sentence"}`), "2:e7 · set_value · text: 18 characters"},
		{raw(t, `{"full":true}`), "full true"},
		// A macro's params are values typed into the window: only their count (SPEC 8.11).
		{json.RawMessage(`{"name":"site_search","params":["query=blue folders"]}`), "site_search · params: 1"},
	}
	for _, c := range cases {
		if got := DesktopArgs(c.args); got != c.want {
			t.Errorf("DesktopArgs(%s) = %q, want %q", c.args, got, c.want)
		}
	}
	chip := ToolChip(
		chat.ToolCall{ID: "c1", Name: "desktop_snapshot", Args: json.RawMessage(`{}`)},
		&chat.ToolResult{CallID: "c1", Text: "Window \"Notes\" (snapshot 1)\n- button \"Save\" [1:e1]"},
		false,
	)
	if !chip.Desktop || chip.State != ChipDone || chip.Detail != "50 characters" || strings.Contains(chip.Detail, "Save") {
		t.Errorf("chip = %+v", chip)
	}
}

func TestDesktopArgsKeepJavaScriptsKeyOrder(t *testing.T) {
	// Index-like keys come first, as Object.entries gives them; a text
	// outside the BMP counts two, as its length in JavaScript does.
	got := DesktopArgs(json.RawMessage(`{"text":"a😀","2":null,"1":{"x":1},"x":1.50}`))
	if want := "1 [object Object] · 2 null · text: 3 characters · x 1.5"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := DesktopArgs(json.RawMessage(`"not json"`)); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestToolChip(t *testing.T) {
	call := chat.ToolCall{ID: "k1", Name: "query", Args: json.RawMessage(`{"sql":"SELECT 1","limit":5,"all":true}`)}
	cases := []struct {
		result  *chat.ToolResult
		running bool
		want    Chip
	}{
		{nil, true, Chip{State: ChipRunning, Detail: "SELECT 1 · 5"}},
		{nil, false, Chip{State: ChipFailed, Detail: "not run"}},
		{&chat.ToolResult{Text: "\n  3 rows\nmore"}, false, Chip{State: ChipDone, Detail: "3 rows"}},
		{&chat.ToolResult{Text: "no such table", IsError: true}, false, Chip{State: ChipFailed, Detail: "no such table"}},
		{&chat.ToolResult{Text: strings.Repeat("x", 130)}, false, Chip{State: ChipDone, Detail: strings.Repeat("x", 119) + "…"}},
	}
	for _, c := range cases {
		if got := ToolChip(call, c.result, c.running); got != c.want {
			t.Errorf("ToolChip(%v, %v) = %+v, want %+v", c.result, c.running, got, c.want)
		}
	}
	if got := ToolChip(chat.ToolCall{Name: "query", Args: json.RawMessage(`{}`)}, nil, true); got.Detail != "running" {
		t.Errorf("no args, running: %+v", got)
	}
	desk := chat.ToolCall{Name: "desktop_click", Args: json.RawMessage(`{"id":"1:e2"}`)}
	if got := ToolChip(desk, nil, true); got.Detail != "1:e2 · running" || got.State != ChipRunning {
		t.Errorf("desktop running: %+v", got)
	}
	if got := ToolChip(desk, &chat.ToolResult{Text: "x", IsError: true}, false); got.Detail != "x" || got.State != ChipFailed {
		t.Errorf("desktop error: %+v", got)
	}
}

func TestArgsText(t *testing.T) {
	cases := []struct {
		args json.RawMessage
		want string
	}{
		{json.RawMessage(`{"b":1,"a":["x",{}],"c":{"d":"</>"}}`), "{\n  \"b\": 1,\n  \"a\": [\n    \"x\",\n    {}\n  ],\n  \"c\": {\n    \"d\": \"</>\"\n  }\n}"},
		{json.RawMessage(`{}`), "{}"},
		{raw(t, `{"q":"قیمت"}`), "{\n  \"q\": \"قیمت\"\n}"},
		{raw(t, "not json"), "not json"},
	}
	for _, c := range cases {
		if got := ArgsText(c.args); got != c.want {
			t.Errorf("ArgsText(%s) = %q, want %q", c.args, got, c.want)
		}
	}
	long := ArgsText(raw(t, map[string]string{"s": strings.Repeat("a", 5000)}))
	if !strings.HasSuffix(long, "\n…") || utf16Len(long) != 4002 {
		t.Errorf("long args: %d characters", utf16Len(long))
	}
}

func TestNoticeText(t *testing.T) {
	cases := map[string]string{
		"[turn 1 was undone]": "turn 1 was undone",
		"[two\nlines]":        "two\nlines",
		"plain":               "plain",
		"[":                   "[",
		"[]":                  "",
		"[a] b":               "[a] b",
	}
	for in, want := range cases {
		if got := NoticeText(in); got != want {
			t.Errorf("NoticeText(%q) = %q, want %q", in, got, want)
		}
	}
}
