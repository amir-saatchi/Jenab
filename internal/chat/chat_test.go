package chat

import (
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var answered = time.Date(2026, 10, 3, 10, 14, 0, 0, time.UTC)

func question(headers ...string) *Question {
	q := &Question{}
	for _, h := range headers {
		q.Questions = append(q.Questions, QuestionItem{Header: h, Question: "Which " + h + "?",
			Options: []Option{{Label: "EUR", Recommended: true}, {Label: "USD", Description: "As the API returns them"}}})
	}
	return q
}

// goodParts has one valid part of every kind.
func goodParts() []Part {
	return []Part{
		{Kind: PartText, Text: &Text{Text: "سلام، BTC closed at 62,410."}},
		{Kind: PartText, Text: &Text{Text: "cut off", Stopped: true}},
		{Kind: PartThinking, Thinking: &Thinking{Text: "Check the table first.", Signature: "EqQBCkYIBxgCKkCz+/sig=="}},
		{Kind: PartThinking, Thinking: &Thinking{Redacted: "opaque-data=="}},
		{Kind: PartToolCall, ToolCall: &ToolCall{ID: "call_1", Name: "query", Args: json.RawMessage(`{"sql":"SELECT 1"}`),
			Extra: `{ "google": {"thought_signature": "CiQB<x>&y=="} }`}},
		{Kind: PartToolResult, ToolResult: &ToolResult{CallID: "call_1", Text: "[3 rows, showing 3]", Ref: "cache/tool/c/m-1"}},
		{Kind: PartToolResult, ToolResult: &ToolResult{CallID: "call_2", Text: "cancelled by user", IsError: true}},
		{Kind: PartImage, Image: &Image{Ref: "charts/btc.png", MIME: "image/png", Alt: "BTC price"}},
		{Kind: PartNotice, Notice: &Notice{Kind: NoticeTaskFinished, Text: "[task t_12 finished: 20 pages read]"}},
		{Kind: PartApproval, Approval: &Approval{ID: "01K6Q2X5E8V3M9T4R7W2N6B1CA", Kind: "host", Ask: "Allow api.coingecko.com?",
			Why: "The price pipeline needs it.", Risk: "Data is sent to a new host.",
			Options: []string{"Allow for this project", "Deny"}, Answer: "Deny", Note: "use binance", By: "user", AnsweredAt: &answered}},
		{Kind: PartQuestion, Question: question("Currency")},
		{Kind: PartQuestion, Question: func() *Question {
			q := question("Currency", "Period", "Chart", "Coins")
			q.Questions[3].Multi = true
			q.Answers = map[string][]string{"Currency": {"EUR"}, "Coins": {"EUR", "USD"}, "Period": {"the last 7 days"}}
			q.AnsweredAt = &answered
			return q
		}()},
	}
}

func TestValidPartsPass(t *testing.T) {
	for i, p := range goodParts() {
		if err := p.Validate(); err != nil {
			t.Errorf("part %d (%s): %v", i, p.Kind, err)
		}
	}
}

func TestInvalidParts(t *testing.T) {
	text := &Text{Text: "hi"}
	call := func(c ToolCall) Part { return Part{Kind: PartToolCall, ToolCall: &c} }
	q := func(f func(*Question)) Part {
		qq := question("Currency", "Period")
		f(qq)
		return Part{Kind: PartQuestion, Question: qq}
	}
	tests := []struct {
		name string
		p    Part
		want string
	}{
		{"unknown kind", Part{Kind: "video", Text: text}, "unknown kind"},
		{"empty kind", Part{Text: text}, "unknown kind"},
		{"no field", Part{Kind: PartText}, "0 fields"},
		{"two fields", Part{Kind: PartText, Text: text, Notice: &Notice{Kind: NoticePageOpened, Text: "x"}}, "2 fields"},
		{"wrong field", Part{Kind: PartNotice, Text: text}, "1 fields"},
		{"redacted with text", Part{Kind: PartThinking, Thinking: &Thinking{Text: "x", Redacted: "y"}}, "redacted"},
		{"call without ID", call(ToolCall{Name: "query", Args: json.RawMessage(`{}`)}), "without an ID"},
		{"call without name", call(ToolCall{ID: "c", Args: json.RawMessage(`{}`)}), "without an ID or name"},
		{"call without args", call(ToolCall{ID: "c", Name: "query"}), "not valid JSON"},
		{"call with cut-off args", call(ToolCall{ID: "c", Name: "query", Args: json.RawMessage(`{"sql":"SEL`)}), "not valid JSON"},
		{"call with bad extra", call(ToolCall{ID: "c", Name: "query", Args: json.RawMessage(`{}`), Extra: "{"}), "extra"},
		{"result without call", Part{Kind: PartToolResult, ToolResult: &ToolResult{Text: "x"}}, "call ID"},
		{"image without ref", Part{Kind: PartImage, Image: &Image{MIME: "image/png"}}, "ref"},
		{"image without MIME", Part{Kind: PartImage, Image: &Image{Ref: "a.png"}}, "MIME"},
		{"notice without text", Part{Kind: PartNotice, Notice: &Notice{Kind: NoticeEarlyStop}}, "notice"},
		{"approval with one option", Part{Kind: PartApproval, Approval: &Approval{ID: "a", Kind: "host", Ask: "x", Options: []string{"Deny"}}}, "1 options"},
		{"approval without ask", Part{Kind: PartApproval, Approval: &Approval{ID: "a", Kind: "host", Options: []string{"Allow", "Deny"}}}, "ask"},
		{"no questions", q(func(q *Question) { q.Questions = nil }), "0 questions"},
		{"five questions", q(func(q *Question) { q.Questions = question("a", "b", "c", "d", "e").Questions }), "5 questions"},
		{"same header twice", q(func(q *Question) { q.Questions[1].Header = "Currency" }), "used twice"},
		{"one option", q(func(q *Question) { q.Questions[0].Options = q.Questions[0].Options[:1] }), "1 options"},
		{"five options", q(func(q *Question) {
			q.Questions[0].Options = append(q.Questions[0].Options, Option{Label: "a"}, Option{Label: "b"}, Option{Label: "c"})
		}), "5 options"},
		{"option without label", q(func(q *Question) { q.Questions[0].Options[1].Label = "" }), "without a label"},
		{"question without text", q(func(q *Question) { q.Questions[0].Question = "" }), "without a header or text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if err == nil {
				t.Fatal("passed")
			}
			if !errors.Is(err, ErrInvalidPart) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrInvalidPart mentioning %q", err, tt.want)
			}
		})
	}
}

func TestMessageSaysWhichPart(t *testing.T) {
	m := Message{ID: "m", Chat: "c", Role: RoleAssistant, Parts: goodParts()}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.Parts = append(m.Parts, Part{Kind: PartImage, Image: &Image{}})
	err := m.Validate()
	var pe *PartError
	if !errors.As(err, &pe) || pe.Index != len(m.Parts)-1 || !errors.Is(err, ErrInvalidPart) {
		t.Errorf("err = %v, want a PartError for part %d", err, len(m.Parts)-1)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	m := Message{ID: "01K6Q2X5E8V3M9T4R7W2N6B1CB", Chat: "01K6Q2X5E8V3M9T4R7W2N6B1CC", Turn: 3, Role: RoleAssistant,
		Model: "gemma4:31b", Usage: Usage{Input: 1200, Output: 80, CacheRead: 9000, CacheWrite: 400},
		CreatedAt: answered, Parts: goodParts()}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back Message
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	// Args are compared as JSON values: encoding/json compacts them, which
	// providers don't mind. Everything else must come back equal.
	opt := cmp.Comparer(func(a, b json.RawMessage) bool { return jsonEqual(t, a, b) })
	if d := cmp.Diff(m, back, opt); d != "" {
		t.Errorf("round trip (-sent +back):\n%s", d)
	}
	// The values sent back to providers come back byte for byte.
	for i, p := range m.Parts {
		switch p.Kind {
		case PartThinking:
			if back.Parts[i].Thinking.Signature != p.Thinking.Signature || back.Parts[i].Thinking.Redacted != p.Thinking.Redacted {
				t.Errorf("part %d: thinking signature changed", i)
			}
		case PartToolCall:
			if back.Parts[i].ToolCall.Extra != p.ToolCall.Extra {
				t.Errorf("part %d: extra = %q, want %q", i, back.Parts[i].ToolCall.Extra, p.ToolCall.Extra)
			}
		}
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	return cmp.Equal(x, y)
}

func TestJSONShape(t *testing.T) {
	b, _ := json.Marshal(Part{Kind: PartToolResult, ToolResult: &ToolResult{CallID: "call_1", Text: "ok"}})
	if want := `{"kind":"tool_result","tool_result":{"call_id":"call_1","text":"ok"}}`; string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	b, _ = json.Marshal(Chat{ID: "c", Kind: KindMother, CreatedBy: "app"})
	for _, k := range []string{`"title_fixed":false`, `"created_by":"app"`, `"default_page":""`, `"kind":"mother"`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("chat JSON %s lacks %s", b, k)
		}
	}
}

// TestImportsOnlyID keeps chat at the bottom of the import graph: every layer
// uses it, so it must not import store, provider or agent.
func TestImportsOnlyID(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if strings.Contains(p, ".") && p != "github.com/amir-saatchi/jenab/internal/id" {
				t.Errorf("%s imports %s; chat may import only id and the standard library", f, p)
			}
		}
	}
}
