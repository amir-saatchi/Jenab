package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// put is a write that can carry the answer. It records the arguments it
// got, and fails for the text "bad".
func put(got *[]string) tool.Tool {
	return tool.Func(tool.Spec{Name: "put", Description: "Save text.", Effects: tool.WritesDB, Answers: true,
		Schema: json.RawMessage(`{"type": "object", "properties": {"text": {"type": "string"}}, "required": ["text"], "additionalProperties": false}`)},
		func(_ context.Context, env *tool.Env, a struct{ Text string }) (tool.Result, error) {
			*got = append(*got, a.Text)
			if a.Text == "bad" {
				return tool.Result{}, tool.Errorf("bad text")
			}
			return tool.Result{Text: "saved " + a.Text}, nil
		})
}

// oneCall is one call of a multi-call answer.
func oneCall(id, name string, args any) chat.ToolCall {
	b, _ := json.Marshal(args)
	return chat.ToolCall{ID: id, Name: name, Args: b}
}

// toolCalls is a complete answer of several tool calls.
func toolCalls(cs ...chat.ToolCall) fake.Reply {
	var r fake.Reply
	for _, c := range cs {
		r.Events = append(r.Events, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: &c}})
	}
	r.Events = append(r.Events, fake.Done(provider.StopToolUse, chat.Usage{Input: 10, Output: 5}))
	return r
}

// A write with an answer ends the turn without another request; the tool
// gets its arguments without the answer, and the answer is the turn's
// last message.
func TestAnswerWithWrite(t *testing.T) {
	var got []string
	h := newHarness(t, t.TempDir(), "", testSettings(), put(&got), echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Notes"})
	h.fp.Push(fake.ToolCall("c1", "put", map[string]string{"text": "a", "answer": "Saved a."}))
	h.turn(c.ID, "save a")
	calls := h.fp.Calls()
	if len(calls) != 1 {
		t.Fatalf("%d requests, want 1", len(calls))
	}
	if !slices.Equal(got, []string{"a"}) {
		t.Errorf("the tool got %q", got)
	}
	ms := h.messages(c.ID)
	if tx := texts(ms); tx[len(tx)-1] != "assistant: Saved a." || tx[len(tx)-2] != "tool: result saved a" {
		t.Errorf("the turn ends with %q", tx[len(tx)-2:])
	}
	validHistory(t, ms)

	// The model sees the argument last, and block 1 says how to use it;
	// echo, which can't carry an answer, has no such argument.
	for _, d := range calls[0].Tools {
		has := strings.Contains(string(d.Schema), `"answer"`)
		if d.Name == "put" && (!has || !strings.HasSuffix(strings.TrimSpace(string(d.Schema[:strings.LastIndex(string(d.Schema), `"required"`)])), "},")) {
			t.Errorf("put's schema: %s", d.Schema)
		}
		if d.Name == "echo" && has {
			t.Errorf("echo's schema has an answer: %s", d.Schema)
		}
	}
	if sys := calls[0].System[0].Text; !strings.Contains(sys, answerLine) {
		t.Errorf("block 1 lacks the answer line:\n%s", sys)
	}

	// The next turn's request has the answer as the assistant's message.
	h.fp.Push(fake.Text("ok"))
	h.turn(c.ID, "thanks")
	if tx := texts(h.fp.Calls()[1].Messages); !slices.Contains(tx, "assistant: Saved a.") {
		t.Errorf("the next request: %q", tx)
	}
}

// A failed call, or a call without an answer next to it, goes on as
// usual: the model gets the results and answers in another request.
func TestAnswerWithWriteFails(t *testing.T) {
	for _, tt := range []struct {
		name  string
		first fake.Reply
	}{
		{"the write fails", fake.ToolCall("c1", "put", map[string]string{"text": "bad", "answer": "Saved."})},
		{"no answer", fake.ToolCall("c1", "put", map[string]string{"text": "a"})},
		{"an answer to a tool that can't carry one", fake.ToolCall("c1", "echo", map[string]string{"text": "a", "answer": "Saved."})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			h := newHarness(t, t.TempDir(), "", testSettings(), put(&got), echo())
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "Notes"})
			h.fp.Push(tt.first, fake.Text("done"))
			h.turn(c.ID, "save it")
			if n := len(h.fp.Calls()); n != 2 {
				t.Fatalf("%d requests, want 2", n)
			}
			ms := h.messages(c.ID)
			if tx := texts(ms); tx[len(tx)-1] != "assistant: done" || slices.Contains(tx, "assistant: Saved.") {
				t.Errorf("the turn: %q", tx)
			}
			validHistory(t, ms)
		})
	}
}

// Several calls: the turn ends only when all of them succeed, with each
// distinct answer once, in order.
func TestAnswerWithWrites(t *testing.T) {
	var got []string
	h := newHarness(t, t.TempDir(), "", testSettings(), put(&got))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Notes"})
	h.fp.Push(toolCalls(
		oneCall("c1", "put", map[string]string{"text": "a", "answer": "Saved a."}),
		oneCall("c2", "put", map[string]string{"text": "b", "answer": "Saved b."}),
		oneCall("c3", "put", map[string]string{"text": "c", "answer": "Saved b."}),
	))
	h.turn(c.ID, "save them")
	if n := len(h.fp.Calls()); n != 1 {
		t.Fatalf("%d requests, want 1", n)
	}
	if tx := texts(h.messages(c.ID)); tx[len(tx)-1] != "assistant: Saved a.\n\nSaved b." {
		t.Errorf("the answer: %q", tx[len(tx)-1])
	}
	h.fp.Push(toolCalls(
		oneCall("c4", "put", map[string]string{"text": "bad"}),
		oneCall("c5", "put", map[string]string{"text": "d", "answer": "Saved d."}),
	), fake.Text("one failed"))
	h.turn(c.ID, "save more")
	if n := len(h.fp.Calls()); n != 3 {
		t.Fatalf("%d requests, want 3", n)
	}
}

func TestWithAnswer(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{`{"type": "object", "properties": {"b": {"type": "string"}, "a": {"type": "integer"}}}`, `"properties":{"b":{"type":"string"},"a":{"type":"integer"},"answer":`},
		{`{"type": "object", "properties": {}}`, `"properties":{"answer":`},
		{`{"type": "object"}`, `{"type": "object"}`},
		{`not json`, `not json`},
	} {
		got := string(withAnswer(json.RawMessage(tt.in)))
		if !strings.Contains(got, tt.want) {
			t.Errorf("withAnswer(%s) = %s", tt.in, got)
		}
		var v any
		if tt.in != "not json" && json.Unmarshal([]byte(got), &v) != nil {
			t.Errorf("withAnswer(%s) isn't JSON: %s", tt.in, got)
		}
	}
	if got := string(withAnswer(json.RawMessage(`{"type": "object", "properties": {"a": {"type": "string"}}, "required": ["a"]}`))); !strings.Contains(got, `"required":["a","answer"]`) {
		t.Errorf("required: %s", got)
	}
	if got := string(withAnswer(json.RawMessage(`{"type": "object", "properties": {}}`))); !strings.Contains(got, `"required":["answer"]`) {
		t.Errorf("required without others: %s", got)
	}
	rest, a := splitAnswer(json.RawMessage(`{"text": "a", "answer": " Done. "}`))
	if string(rest) != `{"text":"a"}` || a != "Done." {
		t.Errorf("splitAnswer: %s, %q", rest, a)
	}
	if rest, a := splitAnswer(json.RawMessage(`{"text": "a"}`)); string(rest) != `{"text": "a"}` || a != "" {
		t.Errorf("splitAnswer without an answer: %s, %q", rest, a)
	}
}
