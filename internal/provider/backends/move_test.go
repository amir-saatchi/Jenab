package backends

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// TestMoveToAnthropic: a chat whose calls came from a Chat Completions
// model, with Kimi's IDs and with no IDs in two steps, moves to an
// Anthropic model, and the next request is one the API accepts
// (TASK-007).
func TestMoveToAnthropic(t *testing.T) {
	steps := []string{
		// Kimi's IDs.
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"functions.read_file:0","function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}},{"index":1,"id":"functions.read_file:1","function":{"name":"read_file","arguments":"{\"path\":\"b\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n",
		// No IDs, twice.
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"read_file","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n",
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"read_file","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n",
	}
	step := 0
	compat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, steps[step])
		step++
	}))
	defer compat.Close()
	var sent []byte
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"+
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n"+
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer claude.Close()

	all := All()
	kimi, err := all[provider.KindCompatible](provider.Connection{Name: "kimi", Kind: provider.KindCompatible, BaseURL: compat.URL + "/v1/", Key: secret.NewValue("provider:kimi", "k"), HTTP: compat.Client()})
	if err != nil {
		t.Fatal(err)
	}
	anthropic, err := all[provider.KindAnthropic](provider.Connection{Name: "claude", Kind: provider.KindAnthropic, BaseURL: claude.URL + "/", Key: secret.NewValue("provider:claude", "k"), HTTP: claude.Client()})
	if err != nil {
		t.Fatal(err)
	}

	history := []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Read the files."}}}}}
	for range steps {
		var calls, results []chat.Part
		for ev, err := range kimi.Stream(context.Background(), provider.Request{Model: "kimi-k2", Messages: history}) {
			if err != nil {
				t.Fatal(err)
			}
			if ev.Kind == provider.EventPart && ev.Part.ToolCall != nil {
				calls = append(calls, *ev.Part)
				results = append(results, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: ev.Part.ToolCall.ID, Text: "ok"}})
			}
		}
		history = append(history, chat.Message{Role: chat.RoleAssistant, Parts: calls}, chat.Message{Role: chat.RoleTool, Parts: results})
	}

	for _, err := range anthropic.Stream(context.Background(), provider.Request{Model: "claude-opus-5-5", Messages: history}) {
		if err != nil {
			t.Fatal(err)
		}
	}
	var body struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				ToolUseID string `json:"tool_use_id"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(sent, &body); err != nil {
		t.Fatal(err)
	}
	valid := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	uses := map[string]bool{}
	var open []string
	for _, m := range body.Messages {
		for _, c := range m.Content {
			switch c.Type {
			case "tool_use":
				if !valid.MatchString(c.ID) || uses[c.ID] {
					t.Errorf("tool_use ID %q is not valid or repeats", c.ID)
				}
				uses[c.ID] = true
				open = append(open, c.ID)
			case "tool_result":
				if len(open) == 0 || open[0] != c.ToolUseID {
					t.Errorf("tool_result %q doesn't match its call (open %q)", c.ToolUseID, open)
				} else {
					open = open[1:]
				}
			}
		}
	}
	if len(uses) != 4 || len(open) != 0 {
		t.Errorf("%d calls, %d without a result: %s", len(uses), len(open), sent)
	}
}
