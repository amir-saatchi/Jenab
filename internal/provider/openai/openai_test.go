package openai

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

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

const testKey = "sk-test-SECRET-0123456789abcdef"

// server answers every request with status and body, and records what it
// got.
type server struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	heads  []http.Header
	paths  []string
}

func newServer(t *testing.T, status int, body string) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(b))
		s.heads = append(s.heads, r.Header.Clone())
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()
		if strings.HasPrefix(body, "data:") || strings.HasPrefix(body, "event:") {
			w.Header().Set("content-type", "text/event-stream")
		} else {
			w.Header().Set("content-type", "application/json")
		}
		if status == 429 {
			w.Header().Set("retry-after", "7")
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

func backend(t *testing.T, s *server, kind provider.Kind) provider.Provider {
	t.Helper()
	p, err := New(provider.Connection{Name: "p", Kind: kind, BaseURL: s.URL + "/v1/", Key: secret.NewValue("provider:p", testKey), HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// sse joins chunks as a Chat Completions stream.
func sse(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		fmt.Fprintf(&b, "data: %s\n\n", c)
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

func last(evs []provider.Event) provider.Event {
	if len(evs) == 0 {
		return provider.Event{}
	}
	return evs[len(evs)-1]
}

func errKind(err error) provider.ErrorKind {
	var pe *provider.Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return ""
}

var hello = provider.Request{Model: "m", MaxTokens: 100, Messages: []chat.Message{
	{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Say hi."}}}},
}}

func TestChatComplete(t *testing.T) {
	// Usage in its own chunk after finish_reason, as Groq and Ollama send it.
	s := newServer(t, 200, sse(
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning":"Short answer."}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"سلام"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":" there"},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":120,"completion_tokens":7,"total_tokens":127,"prompt_tokens_details":{"cached_tokens":100}}}`,
		`[DONE]`))
	evs, err := run(backend(t, s, provider.KindCompatible), hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 2 || ps[0].Thinking.Text != "Short answer." || ps[1].Text.Text != "سلام there" {
		t.Fatalf("parts = %+v", ps)
	}
	d := last(evs)
	if d.Kind != provider.EventDone || d.Stop != provider.StopEnd || *d.Usage != (chat.Usage{Input: 20, Output: 7, CacheRead: 100}) {
		t.Fatalf("done = %+v usage %+v", d, d.Usage)
	}
	var body map[string]any
	json.Unmarshal([]byte(s.lastBody()), &body)
	if body["stream_options"].(map[string]any)["include_usage"] != true || body["max_completion_tokens"] != 100.0 {
		t.Errorf("request = %s", s.lastBody())
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Errorf("reasoning_effort sent to a provider that isn't Ollama")
	}
}

func TestChatToolCallsGemini(t *testing.T) {
	// Gemini: both calls at index 0 with new ids, each with extra_content.
	extra := `{"google":{"thought_signature":"CiQB0e2Kb+/sig=="}}`
	s := newServer(t, 200, sse(
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"query","arguments":"{\"sql\":\"SELECT 1\"}"},"extra_content":`+extra+`}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c2","type":"function","function":{"name":"describe_table","arguments":"{\"table\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"btc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":9}}`,
		`[DONE]`))
	p := backend(t, s, provider.KindCompatible)
	evs, err := run(p, hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 2 || ps[0].ToolCall.ID != "c1" || ps[1].ToolCall.ID != "c2" || string(ps[1].ToolCall.Args) != `{"table":"btc"}` {
		t.Fatalf("calls = %+v %+v", ps[0].ToolCall, ps[len(ps)-1].ToolCall)
	}
	if ps[0].ToolCall.Extra != extra {
		t.Errorf("extra = %q, want %q", ps[0].ToolCall.Extra, extra)
	}
	if last(evs).Stop != provider.StopToolUse {
		t.Errorf("stop = %s", last(evs).Stop)
	}

	// The next request sends extra_content back unchanged, and the results.
	next := hello
	next.Messages = append(append([]chat.Message(nil), hello.Messages...),
		chat.Message{Role: chat.RoleAssistant, Parts: ps},
		chat.Message{Role: chat.RoleTool, Parts: []chat.Part{
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c1", Text: "1"}},
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c2", Text: "price REAL"}},
			{Kind: chat.PartImage, Image: &chat.Image{Ref: "img1", MIME: "image/png"}},
		}})
	next.Image = func(context.Context, string) ([]byte, error) { return []byte("PNG"), nil }
	run(p, next)
	body := s.lastBody()
	if !strings.Contains(body, `"extra_content":`+extra) {
		t.Errorf("extra_content not sent back unchanged: %s", body)
	}
	var req struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			Content    any    `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal([]byte(body), &req)
	roles := []string{}
	for _, m := range req.Messages {
		roles = append(roles, m.Role)
	}
	if got := strings.Join(roles, ","); got != "user,assistant,tool,tool,user" {
		t.Errorf("roles = %s, want the tool image in a user message after the results", got)
	}
	if c, ok := req.Messages[1].Content.(string); !ok || c != " " {
		t.Errorf("assistant content = %#v, want a single space (Cloudflare refuses a missing one, Z.ai an empty string, Ollama drops []'s calls)", req.Messages[1].Content)
	}
	if !strings.Contains(body, "data:image/png;base64,UE5H") {
		t.Errorf("image not sent as a data URL")
	}
}

func TestChatCutOff(t *testing.T) {
	for name, body := range map[string]string{
		"no finish_reason":         sse(`{"choices":[{"index":0,"delta":{"content":"par"}}]}`),
		"no finish_reason in call": sse(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c","function":{"name":"q","arguments":"{\"sql\":"}}]}}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			s := newServer(t, 200, body)
			_, err := run(backend(t, s, provider.KindCompatible), hello)
			if errKind(err) != provider.Transport || !errors.Is(err, provider.ErrCutOff) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// TestBadToolJSON: arguments that are not valid JSON in a complete stream
// make a call marked Invalid; with "length" the stop is max tokens (SPEC 3.8).
func TestChatBadToolJSON(t *testing.T) {
	for finish, stop := range map[string]provider.StopReason{"tool_calls": provider.StopToolUse, "length": provider.StopMaxTokens} {
		t.Run(finish, func(t *testing.T) {
			s := newServer(t, 200, sse(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c","function":{"name":"q","arguments":"{\"sql\":"}}]},"finish_reason":"`+finish+`"}]}`))
			evs, err := run(backend(t, s, provider.KindCompatible), hello)
			if err != nil {
				t.Fatal(err)
			}
			checkInvalid(t, evs, `{"sql":`, stop)
		})
	}
}

func checkInvalid(t *testing.T, evs []provider.Event, raw string, stop provider.StopReason) {
	t.Helper()
	ps := parts(evs)
	if len(ps) != 1 || ps[0].ToolCall == nil {
		t.Fatalf("parts = %+v", ps)
	}
	if c := ps[0].ToolCall; c.Invalid != raw || string(c.Args) != "{}" {
		t.Errorf("call = %+v, want Invalid %q and Args {}", c, raw)
	}
	if err := ps[0].Validate(); err != nil {
		t.Error(err)
	}
	if got := last(evs).Stop; got != stop {
		t.Errorf("stop = %s, want %s", got, stop)
	}
}

func TestChatErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		kind   provider.ErrorKind
	}{
		{"413", 413, `{"error":{"message":"Request too large for model","type":"tokens","code":"rate_limit_exceeded"}}`, provider.TooLarge},
		{"gemini quota", 429, `[{"error":{"code":429,"message":"You exceeded your current quota","details":[{"violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}]`, provider.Quota},
		{"rate limit", 429, `{"error":{"message":"slow down"}}`, provider.RateLimited},
		{"bad key", 401, `{"error":{"message":"Incorrect API key provided: ` + testKey + `"}}`, provider.BadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, tt.status, tt.body)
			_, err := run(backend(t, s, provider.KindCompatible), hello)
			var pe *provider.Error
			if !errors.As(err, &pe) || pe.Kind != tt.kind || pe.Status != tt.status {
				t.Fatalf("err = %v, want %s", err, tt.kind)
			}
			if tt.status == 429 && tt.kind == provider.RateLimited && pe.RetryAfter.Seconds() != 7 {
				t.Errorf("RetryAfter = %s", pe.RetryAfter)
			}
			if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, pe), testKey) {
				t.Errorf("the key is in the error: %v", err)
			}
		})
	}
}

// TestNoKeyLeak: the key goes only in the Authorization header, and the
// SDK's OPENAI_* variables never reach the provider.
func TestNoKeyLeak(t *testing.T) {
	t.Setenv("OPENAI_ORG_ID", "org-from-env")
	t.Setenv("OPENAI_PROJECT_ID", "proj-from-env")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Leak: from-env")
	t.Setenv("OPENAI_BASE_URL", "https://elsewhere.example/")
	s := newServer(t, 200, sse(`{"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`))
	if _, err := run(backend(t, s, provider.KindCompatible), hello); err != nil {
		t.Fatal(err)
	}
	h := s.heads[0]
	if h.Get("Authorization") != "Bearer "+testKey {
		t.Errorf("Authorization = %q", h.Get("Authorization"))
	}
	for k, vs := range h {
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		for _, v := range vs {
			if strings.Contains(v, testKey) || strings.Contains(v, "from-env") {
				t.Errorf("header %s = %q", k, v)
			}
		}
	}
	if strings.Contains(s.bodies[0], testKey) || strings.Contains(s.paths[0], testKey) {
		t.Error("the key is in the body or the URL")
	}
}

func TestGuardRefusesOtherHosts(t *testing.T) {
	g, err := provider.NewGuard(provider.Connection{BaseURL: "https://api.example.com/v1/"})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"https://evil.example.com/v1/chat", "http://api.example.com/v1/chat", "https://user@api.example.com/v1/"} {
		r, _ := http.NewRequest("POST", u, nil)
		if _, err := g.Do(r, func(*http.Request) (*http.Response, error) { t.Fatalf("%s was sent", u); return nil, nil }); err == nil {
			t.Errorf("%s: no error", u)
		}
	}
	for _, u := range []string{"http://api.example.com/", "https://x.example/?key=1", "ftp://x.example/", "http://192.168.1.5:11434/"} {
		if _, err := provider.CheckBaseURL(u); err == nil {
			t.Errorf("CheckBaseURL(%q) accepted", u)
		}
	}
	for _, u := range []string{"http://localhost:11434/", "http://127.0.0.1:8080/v1/", "https://ollama.com/v1/"} {
		if _, err := provider.CheckBaseURL(u); err != nil {
			t.Errorf("CheckBaseURL(%q): %v", u, err)
		}
	}
}

// A message with only tool calls has a single space as content on every
// endpoint: Cloudflare refuses a missing content, Z.ai refuses "", and
// Ollama's /v1 drops the calls of a [] message.
func TestToolCallsOnlyContent(t *testing.T) {
	req := hello
	req.Messages = append(append([]chat.Message(nil), hello.Messages...), chat.Message{Role: chat.RoleAssistant, Parts: []chat.Part{
		{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c1", Name: "query", Args: json.RawMessage(`{"sql":"SELECT 1"}`)}},
	}})
	a := &chatAPI{base{}}
	p, err := a.params(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p.Messages[len(p.Messages)-1])
	if !strings.Contains(string(b), `"content":" "`) || !strings.Contains(string(b), `"tool_calls":[`) {
		t.Errorf("tool-call message = %s, want content \" \"", b)
	}
}

func TestModelsList(t *testing.T) {
	s := newServer(t, 200, `{"object":"list","data":[{"id":"models/gemini-3.8-flash","object":"model","created":0,"owned_by":"google"},{"id":"gemma-4-31b-it","object":"model","created":0,"owned_by":"google"}]}`)
	ms, err := backend(t, s, provider.KindCompatible).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].ID != "gemini-3.8-flash" || ms[1].ID != "gemma-4-31b-it" {
		t.Errorf("models = %+v", ms)
	}
}

// ---- Responses API ----

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

func TestResponsesToolCallAndReasoning(t *testing.T) {
	s := newServer(t, 200, events(
		`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"gAAAA-enc"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"delta":"Checking."}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Checking.","annotations":[]}]}}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"query","arguments":"{\"sql\":\"SELECT 1\"}","status":"completed"}}`,
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":800,"cache_write_tokens":0},"output_tokens":40,"output_tokens_details":{"reasoning_tokens":20},"total_tokens":1040}}}`))
	p := backend(t, s, provider.KindOpenAI)
	evs, err := run(p, hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 3 || ps[0].Thinking == nil || ps[1].Text.Text != "Checking." || ps[2].ToolCall.ID != "call_1" {
		t.Fatalf("parts = %+v", ps)
	}
	if d := last(evs); d.Stop != provider.StopToolUse || *d.Usage != (chat.Usage{Input: 200, Output: 40, CacheRead: 800}) {
		t.Errorf("done = %+v %+v", d, d.Usage)
	}
	var first map[string]any
	json.Unmarshal([]byte(s.lastBody()), &first)
	if first["store"] != false || first["stream"] != true || s.paths[0] != "/v1/responses" {
		t.Errorf("request = %s to %s", s.lastBody(), s.paths[0])
	}

	// The reasoning goes back unchanged before its call.
	next := hello
	next.Messages = append(append([]chat.Message(nil), hello.Messages...),
		chat.Message{Role: chat.RoleAssistant, Parts: ps},
		chat.Message{Role: chat.RoleTool, Parts: []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "call_1", Text: "1"}}}})
	run(p, next)
	var req struct {
		Input []map[string]any `json:"input"`
	}
	json.Unmarshal([]byte(s.lastBody()), &req)
	var types []string
	for _, it := range req.Input {
		types = append(types, it["type"].(string))
	}
	if got := strings.Join(types, ","); got != "message,reasoning,message,function_call,function_call_output" {
		t.Fatalf("input = %s", got)
	}
	if r := req.Input[1]; r["id"] != "rs_1" || r["encrypted_content"] != "gAAAA-enc" || r["summary"] == nil {
		t.Errorf("reasoning item = %v", r)
	}
}

func TestResponsesEnds(t *testing.T) {
	tests := []struct {
		name string
		body string
		stop provider.StopReason
		kind provider.ErrorKind
	}{
		{"max tokens", events(`{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":100}}}`), provider.StopMaxTokens, ""},
		{"cut off", events(`{"type":"response.output_text.delta","delta":"par"}`), "", provider.Transport},
		{"failed", events(`{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"The server had an error"}}}`), "", provider.Overloaded},
		{"rate limit in stream", events(`{"type":"error","code":"rate_limit_exceeded","message":"Rate limit reached"}`), "", provider.RateLimited},
		{"quota in stream", events(`{"type":"error","code":"insufficient_quota","message":"You exceeded your current quota"}`), "", provider.Quota},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, 200, tt.body)
			evs, err := run(backend(t, s, provider.KindOpenAI), hello)
			if tt.kind != "" {
				if errKind(err) != tt.kind {
					t.Fatalf("err = %v, want %s", err, tt.kind)
				}
				return
			}
			if err != nil || last(evs).Stop != tt.stop {
				t.Fatalf("err = %v, last = %+v", err, last(evs))
			}
		})
	}
}

func TestResponsesBadToolJSON(t *testing.T) {
	call := `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"query","arguments":"{\"sql\":","status":"completed"}}`
	for name, end := range map[string]string{
		"completed":  `{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5}}}`,
		"max tokens": `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":100}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newServer(t, 200, events(call, end))
			evs, err := run(backend(t, s, provider.KindOpenAI), hello)
			if err != nil {
				t.Fatal(err)
			}
			stop := provider.StopToolUse
			if name == "max tokens" {
				stop = provider.StopMaxTokens
			}
			checkInvalid(t, evs, `{"sql":`, stop)
		})
	}
}

func TestResponsesHTTPError(t *testing.T) {
	s := newServer(t, 429, `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`)
	_, err := run(backend(t, s, provider.KindOpenAI), hello)
	if errKind(err) != provider.Quota {
		t.Fatalf("err = %v", err)
	}
}
