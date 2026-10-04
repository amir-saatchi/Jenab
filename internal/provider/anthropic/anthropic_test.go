package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

const testKey = "sk-ant-test-SECRET-0123456789"

type server struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	heads  []http.Header
}

func newServer(t *testing.T, status int, body string) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(b))
		s.heads = append(s.heads, r.Header.Clone())
		s.mu.Unlock()
		if strings.HasPrefix(body, "event:") {
			w.Header().Set("content-type", "text/event-stream")
		} else {
			w.Header().Set("content-type", "application/json")
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) lastBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[len(s.bodies)-1]
}

func newBackend(t *testing.T, s *server) provider.Provider {
	t.Helper()
	p, err := New(provider.Connection{Name: "claude", Kind: provider.KindAnthropic, BaseURL: s.URL + "/", Key: secret.NewValue("provider:claude", testKey), HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func events(evs ...string) string {
	var b strings.Builder
	for _, e := range evs {
		var t struct {
			Type string `json:"type"`
		}
		json.Unmarshal([]byte(e), &t)
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", t.Type, e)
	}
	return b.String()
}

func run(p provider.Provider, req provider.Request) ([]provider.Event, error) {
	var evs []provider.Event
	for ev, err := range p.Stream(context.Background(), req) {
		if err != nil {
			return evs, err
		}
		evs = append(evs, ev)
	}
	return evs, nil
}

func parts(evs []provider.Event) []chat.Part {
	var out []chat.Part
	for _, e := range evs {
		if e.Kind == provider.EventPart {
			out = append(out, *e.Part)
		}
	}
	return out
}

func errKind(err error) provider.ErrorKind {
	var pe *provider.Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return ""
}

var opus = &provider.Model{Kind: provider.KindAnthropic, ID: "claude-opus-5-5", Thinking: true}

var hello = provider.Request{Model: "claude-opus-5-5", Known: opus, Thinking: true,
	System:   []provider.Block{{Text: "You are Jenab.", Cache: true}, {Text: "Today is Friday."}},
	Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Count the BTC rows."}}}}}}

var start = `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5-5","stop_reason":null,"usage":{"input_tokens":30,"cache_creation_input_tokens":2000,"cache_read_input_tokens":5000,"output_tokens":1}}}`

func toolTurn() string {
	return events(start,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Use the query tool."}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"EqQBCkYIBxgC+sig=="}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Checking."}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"query","input":{}}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"sql\": \"SELECT"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":" count(*) FROM btc\"}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu_2","name":"describe_table","input":{}}}`,
		`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"table\":\"btc\"}"}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":88}}`,
		`{"type":"message_stop"}`)
}

func TestToolTurnAndSendBack(t *testing.T) {
	s := newServer(t, 200, toolTurn())
	p := newBackend(t, s)
	evs, err := run(p, hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 4 || ps[0].Thinking.Signature != "EqQBCkYIBxgC+sig==" || ps[0].Thinking.Text != "Use the query tool." ||
		ps[1].Text.Text != "Checking." || string(ps[2].ToolCall.Args) != `{"sql": "SELECT count(*) FROM btc"}` || ps[3].ToolCall.ID != "toolu_2" {
		t.Fatalf("parts = %+v", ps)
	}
	d := evs[len(evs)-1]
	if d.Stop != provider.StopToolUse || *d.Usage != (chat.Usage{Input: 30, Output: 88, CacheRead: 5000, CacheWrite: 2000}) {
		t.Fatalf("done = %+v %+v", d, d.Usage)
	}

	var first map[string]any
	json.Unmarshal([]byte(s.lastBody()), &first)
	th := first["thinking"].(map[string]any)
	if th["type"] != "adaptive" || th["display"] != "summarized" || first["max_tokens"] != float64(defaultMaxTokens) {
		t.Errorf("thinking %v, max_tokens %v", th, first["max_tokens"])
	}
	sys := first["system"].([]any)
	if sys[0].(map[string]any)["cache_control"] == nil || sys[1].(map[string]any)["cache_control"] != nil || first["cache_control"] == nil {
		t.Errorf("cache points: system %v, top %v", sys, first["cache_control"])
	}

	// Send back: thinking unchanged before its calls, and both results
	// with the tool's image in one user message.
	next := hello
	next.Image = func(context.Context, string) ([]byte, error) { return []byte("PNG"), nil }
	next.Messages = append(append([]chat.Message(nil), hello.Messages...),
		chat.Message{Role: chat.RoleAssistant, Parts: ps},
		chat.Message{Role: chat.RoleTool, Parts: []chat.Part{
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "toolu_1", Text: "42"}},
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "toolu_2", Text: "chart", IsError: false}},
			{Kind: chat.PartImage, Image: &chat.Image{Ref: "img", MIME: "image/png"}},
		}},
		chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: "task_finished", Text: "Task 2 finished."}}}})
	run(p, next)
	var req struct {
		Messages []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal([]byte(s.lastBody()), &req)
	if len(req.Messages) != 3 {
		t.Fatalf("%d messages, want user, assistant, user: %s", len(req.Messages), s.lastBody())
	}
	a := req.Messages[1].Content
	if a[0]["type"] != "thinking" || a[0]["signature"] != "EqQBCkYIBxgC+sig==" || a[0]["thinking"] != "Use the query tool." || a[2]["type"] != "tool_use" {
		t.Errorf("assistant = %v", a)
	}
	u := req.Messages[2].Content
	if len(u) != 3 || u[0]["tool_use_id"] != "toolu_1" || u[1]["tool_use_id"] != "toolu_2" || u[2]["type"] != "text" {
		t.Fatalf("results message = %v", u)
	}
	if c := u[1]["content"].([]any); len(c) != 2 || c[1].(map[string]any)["type"] != "image" {
		t.Errorf("the image is not inside its result: %v", u[1])
	}
}

func TestThinkingSettings(t *testing.T) {
	haiku := &provider.Model{ID: "claude-haiku-4-5-20251001", Thinking: true, ThinkingBudget: true}
	tests := []struct {
		name string
		req  provider.Request
		want string // the thinking field, "" for none
	}{
		{"adaptive", provider.Request{Known: opus, Thinking: true}, `{"type":"adaptive","display":"summarized"}`},
		{"budget", provider.Request{Known: haiku, Thinking: true, MaxTokens: 8000}, `{"type":"enabled","budget_tokens":4000}`},
		{"off: the API's default", provider.Request{Known: opus}, ``},
		{"unknown model", provider.Request{Thinking: true}, ``},
	}
	b := &backend{}
	for _, tt := range tests {
		body, err := b.body(context.Background(), tt.req)
		if err != nil {
			t.Fatal(err)
		}
		var r struct {
			Thinking json.RawMessage `json:"thinking"`
		}
		json.Unmarshal(body, &r)
		if string(r.Thinking) != tt.want {
			t.Errorf("%s: thinking = %s, want %s", tt.name, r.Thinking, tt.want)
		}
	}
}

func TestOtherProvidersThinkingSkipped(t *testing.T) {
	req := hello
	req.Messages = append(append([]chat.Message(nil), hello.Messages...), chat.Message{Role: chat.RoleAssistant, Parts: []chat.Part{
		{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "from Gemini"}},
		{Kind: chat.PartThinking, Thinking: &chat.Thinking{Signature: `{"id":"rs_1","encrypted_content":"x"}`}},
		{Kind: chat.PartThinking, Thinking: &chat.Thinking{Redacted: "opaque=="}},
		{Kind: chat.PartText, Text: &chat.Text{Text: "Done."}},
	}})
	body, _ := (&backend{}).body(context.Background(), req)
	if strings.Contains(string(body), "from Gemini") || strings.Contains(string(body), "rs_1") || !strings.Contains(string(body), `"redacted_thinking"`) {
		t.Errorf("body = %s", body)
	}
}

// A stopped answer can end in blank text; the API refuses blank text
// blocks (8.3).
func TestBlankTextSkipped(t *testing.T) {
	req := hello
	req.Messages = append(append([]chat.Message(nil), hello.Messages...),
		chat.Message{Role: chat.RoleAssistant, Parts: []chat.Part{
			{Kind: chat.PartText, Text: &chat.Text{Text: "Looking."}},
			{Kind: chat.PartText, Text: &chat.Text{Text: "\n\n ", Stopped: true}},
		}},
		chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: " "}}, {Kind: chat.PartText, Text: &chat.Text{Text: "go on"}}}},
	)
	body, err := (&backend{}).body(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), `"type":"text"`) != 5 { // two system blocks, the question, "Looking." and "go on"
		t.Errorf("body = %s", body)
	}
}

func TestEnds(t *testing.T) {
	tests := []struct {
		name string
		body string
		stop provider.StopReason
		kind provider.ErrorKind
	}{
		{"max tokens", events(start, `{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":5}}`, `{"type":"message_stop"}`), provider.StopMaxTokens, ""},
		{"refusal", events(start, `{"type":"message_delta","delta":{"stop_reason":"refusal"},"usage":{"output_tokens":5}}`, `{"type":"message_stop"}`), provider.StopRefused, ""},
		{"cut off", events(start, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"par"}}`), "", provider.Transport},
		{"bad tool JSON", events(start, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t","name":"q","input":{}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"sql\":"}}`, `{"type":"content_block_stop","index":0}`), "", provider.Transport},
		{"overloaded in stream", events(start, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`), "", provider.Overloaded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, 200, tt.body)
			evs, err := run(newBackend(t, s), hello)
			if tt.kind != "" {
				if errKind(err) != tt.kind {
					t.Fatalf("err = %v, want %s", err, tt.kind)
				}
				return
			}
			if err != nil || evs[len(evs)-1].Stop != tt.stop {
				t.Fatalf("err = %v, events %+v", err, evs)
			}
		})
	}
}

func TestHTTPErrors(t *testing.T) {
	tests := []struct {
		status int
		body   string
		kind   provider.ErrorKind
	}{
		{413, `{"type":"error","error":{"type":"request_too_large","message":"Request exceeds the maximum size"}}`, provider.TooLarge},
		{400, `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API."}}`, provider.Quota},
		{529, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, provider.Overloaded},
		{401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key ` + testKey + `"}}`, provider.BadRequest},
	}
	for _, tt := range tests {
		s := newServer(t, tt.status, tt.body)
		_, err := run(newBackend(t, s), hello)
		if errKind(err) != tt.kind {
			t.Errorf("%d: err = %v, want %s", tt.status, err, tt.kind)
		}
		if err != nil && strings.Contains(fmt.Sprintf("%v %+v", err, err), testKey) {
			t.Errorf("%d: the key is in the error", tt.status)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("retry-after", "17")
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(429)
		io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your per-minute rate limit"}}`)
	}))
	t.Cleanup(s.Close)
	_, err := run(newBackend(t, s), hello)
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.RateLimited || pe.RetryAfter != 17*time.Second {
		t.Fatalf("err = %#v", err)
	}
}

func TestNoKeyLeak(t *testing.T) {
	// With no key in the environment the SDK reads ANTHROPIC_CUSTOM_HEADERS,
	// and the Guard allows an Authorization header.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_BASE_URL", "https://elsewhere.example/")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "Authorization: Bearer env-key-header\nX-Leak: from-env")
	s := newServer(t, 200, toolTurn())
	if _, err := run(newBackend(t, s), hello); err != nil {
		t.Fatal(err)
	}
	h := s.heads[0]
	if h.Get("X-Api-Key") != testKey || h.Get("Anthropic-Version") == "" {
		t.Errorf("x-api-key %q, anthropic-version %q", h.Get("X-Api-Key"), h.Get("Anthropic-Version"))
	}
	for k, vs := range h {
		if strings.EqualFold(k, "X-Api-Key") {
			continue
		}
		for _, v := range vs {
			if strings.Contains(v, testKey) || strings.Contains(v, "env-key") || strings.Contains(v, "from-env") {
				t.Errorf("header %s = %q", k, v)
			}
		}
	}
	if strings.Contains(s.bodies[0], testKey) {
		t.Error("the key is in the body")
	}
}

func TestModels(t *testing.T) {
	s := newServer(t, 200, `{"data":[{"type":"model","id":"claude-opus-5-5","display_name":"Claude Opus 5.5","created_at":"2026-09-22T00:00:00Z","max_input_tokens":1000000,"max_tokens":128000,"capabilities":{}}],"has_more":false,"first_id":"claude-opus-5-5","last_id":"claude-opus-5-5"}`)
	ms, err := newBackend(t, s).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0] != (provider.ModelInfo{ID: "claude-opus-5-5", Name: "Claude Opus 5.5", Context: 1000000, Output: 128000}) {
		t.Errorf("models = %+v", ms)
	}
}
