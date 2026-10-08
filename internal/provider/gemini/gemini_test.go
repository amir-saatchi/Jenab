package gemini

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

const testKey = "AIza-test-SECRET-0123456789"

// reply is one answer of the test server.
type reply struct {
	status int
	body   string
}

type server struct {
	*httptest.Server
	mu      sync.Mutex
	replies []reply // for /interactions, in order; the last one repeats
	bodies  []string
	keys    []string
	queries []string
}

func newServer(t *testing.T, replies ...reply) *server {
	t.Helper()
	s := &server{replies: replies}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.keys = append(s.keys, r.Header.Get("x-goog-api-key"))
		s.queries = append(s.queries, r.URL.RawQuery)
		s.mu.Unlock()
		switch r.URL.Path {
		case "/v1beta/interactions":
			s.mu.Lock()
			s.bodies = append(s.bodies, string(b))
			rp := s.replies[min(len(s.bodies), len(s.replies))-1]
			s.mu.Unlock()
			if rp.status != 200 {
				w.Header().Set("content-type", "application/json")
				w.WriteHeader(rp.status)
				io.WriteString(w, rp.body)
				return
			}
			w.Header().Set("content-type", "text/event-stream")
			io.WriteString(w, rp.body)
		case "/v1beta/models":
			if r.URL.Query().Get("pageToken") == "" {
				io.WriteString(w, `{"models":[{"name":"models/gemini-3.5-flash-lite","displayName":"Gemini 3.5 Flash-Lite","inputTokenLimit":1048576,"outputTokenLimit":65536,"supportedGenerationMethods":["generateContent","countTokens"]},{"name":"models/text-embedding-005","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"p2"}`)
				return
			}
			io.WriteString(w, `{"models":[{"name":"models/gemma-4-31b-it","displayName":"Gemma 4 31B","inputTokenLimit":131072,"outputTokenLimit":8192,"supportedGenerationMethods":["generateContent"]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) body(t *testing.T, i int) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var m map[string]any
	if err := json.Unmarshal([]byte(s.bodies[i]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func newBackend(t *testing.T, s *server) provider.Provider {
	t.Helper()
	p, err := New(provider.Connection{Name: "gemini", Kind: provider.KindGemini, BaseURL: s.URL + "/v1beta/",
		Key: secret.NewValue("provider:gemini", testKey), HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// sse makes a stream from event data, as the API sends it.
func sse(data ...string) string {
	var b strings.Builder
	for _, d := range data {
		var e struct {
			Type string `json:"event_type"`
		}
		json.Unmarshal([]byte(d), &e)
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", e.Type, d)
	}
	b.WriteString("event: done\ndata: [DONE]\n\n")
	return b.String()
}

func completed(status string) string {
	return `{"interaction":{"status":"` + status + `","usage":{"total_tokens":1231,"total_input_tokens":1000,"total_cached_tokens":800,"total_output_tokens":27,"total_thought_tokens":204}},"event_type":"interaction.completed"}`
}

const start = `{"interaction":{"object":"interaction","model":"gemini-3.5-flash-lite"},"event_type":"interaction.created"}`

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

func text(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }

var hello = provider.Request{Model: "gemini-3.5-flash-lite", Thinking: true, MaxTokens: 400,
	System:   []provider.Block{{Text: "You are Jenab.", Cache: true}, {Text: "Today is Friday."}},
	Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{text("Count the BTC rows.")}}}}

func TestComplete(t *testing.T) {
	s := newServer(t, reply{200, sse(start,
		`{"status":"in_progress","event_type":"interaction.status_update"}`,
		`{"index":0,"step":{"type":"thought"},"event_type":"step.start"}`,
		`{"index":0,"delta":{"content":{"type":"text","text":"The user wants"},"type":"thought_summary"},"event_type":"step.delta"}`,
		`{"index":0,"delta":{"content":{"type":"text","text":" a count."},"type":"thought_summary"},"event_type":"step.delta"}`,
		`{"index":0,"delta":{"signature":"EsQICsEI","type":"thought_signature"},"event_type":"step.delta"}`,
		`{"index":0,"event_type":"step.stop"}`,
		`{"index":1,"step":{"type":"model_output"},"event_type":"step.start"}`,
		`{"index":1,"delta":{"text":"There are ","type":"text"},"event_type":"step.delta"}`,
		`{"index":1,"delta":{"text":"42.","type":"text"},"event_type":"step.delta"}`,
		`{"index":1,"event_type":"step.stop"}`,
		completed("completed"))})
	evs, err := run(newBackend(t, s), hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 2 || ps[0].Thinking == nil || ps[0].Thinking.Text != "The user wants a count." || ps[1].Text.Text != "There are 42." {
		t.Fatalf("parts = %+v", ps)
	}
	if got := ps[0].Thinking.Signature; got != `{"google":{"thought_signature":"EsQICsEI"}}` {
		t.Errorf("signature = %s", got)
	}
	if d := last(evs); d.Stop != provider.StopEnd || *d.Usage != (chat.Usage{Input: 200, Output: 231, CacheRead: 800}) {
		t.Errorf("done = %+v %+v", d, d.Usage)
	}
	deltas := 0
	for _, e := range evs {
		if e.Kind == provider.EventDelta {
			deltas++
		}
	}
	if deltas != 4 {
		t.Errorf("%d deltas, want 4", deltas)
	}

	b := s.body(t, 0)
	if b["store"] != false || b["stream"] != true || b["model"] != "gemini-3.5-flash-lite" || b["system_instruction"] != "You are Jenab.\n\nToday is Friday." {
		t.Errorf("body = %v", b)
	}
	gc := b["generation_config"].(map[string]any)
	if gc["max_output_tokens"] != 400.0 || gc["thinking_summaries"] != "auto" || gc["thinking_level"] != nil {
		t.Errorf("generation_config = %v, want the model's own level with summaries", gc)
	}
	if s.keys[0] != testKey || strings.Contains(s.queries[0], testKey) {
		t.Errorf("the key must go only in x-goog-api-key: header %q, query %q", s.keys[0], s.queries[0])
	}
}

// TestToolCallAndReplay: a call keeps its ID and signature; the next
// request sends the whole history back as steps.
func TestToolCallAndReplay(t *testing.T) {
	s := newServer(t, reply{200, sse(start,
		`{"index":0,"step":{"type":"thought"},"event_type":"step.start"}`,
		`{"index":0,"delta":{"signature":"sig-thought","type":"thought_signature"},"event_type":"step.delta"}`,
		`{"index":0,"event_type":"step.stop"}`,
		`{"index":1,"step":{"id":"call_343015","signature":"sig-call","type":"function_call","name":"query","arguments":{}},"event_type":"step.start"}`,
		`{"index":1,"delta":{"arguments":"{\"sql\":","type":"arguments_delta"},"event_type":"step.delta"}`,
		`{"index":1,"delta":{"arguments":"\"SELECT 1\"}","type":"arguments_delta"},"event_type":"step.delta"}`,
		`{"index":1,"event_type":"step.stop"}`,
		`{"index":2,"step":{"type":"function_call","name":"describe","arguments":{"table":"btc"}},"event_type":"step.start"}`,
		`{"index":2,"event_type":"step.stop"}`,
		completed("requires_action"))})
	p := newBackend(t, s)
	evs, err := run(p, hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 3 || ps[1].ToolCall == nil || ps[2].ToolCall == nil {
		t.Fatalf("parts = %+v", ps)
	}
	c := ps[1].ToolCall
	if c.ID != "call_343015" || c.Name != "query" || string(c.Args) != `{"sql":"SELECT 1"}` || c.Extra != `{"google":{"thought_signature":"sig-call"}}` {
		t.Errorf("call = %+v", c)
	}
	if c2 := ps[2].ToolCall; !strings.HasPrefix(c2.ID, "call_") || string(c2.Args) != `{"table":"btc"}` || c2.Extra != "" {
		t.Errorf("second call = %+v, want a made ID and the arguments from step.start", c2)
	}
	if last(evs).Stop != provider.StopToolUse {
		t.Errorf("stop = %s", last(evs).Stop)
	}

	next := hello
	next.Messages = append(append([]chat.Message{}, hello.Messages...),
		chat.Message{Role: chat.RoleAssistant, Parts: append([]chat.Part{
			{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "Anthropic's", Signature: "EqQBCkYIBxgC"}},
			{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "OpenAI's", Signature: `{"id":"rs_1","encrypted_content":"gAAAA"}`}},
			text("Checking."),
		}, ps...)},
		chat.Message{Role: chat.RoleTool, Parts: []chat.Part{
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "call_343015", Text: "1"}},
			{Kind: chat.PartImage, Image: &chat.Image{Ref: "img1", MIME: "image/png"}},
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: ps[2].ToolCall.ID, Text: "no such table", IsError: true}},
		}},
		chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticePageOpened, Text: "[page opened]"}}, text("go on")}})
	next.Image = func(context.Context, string) ([]byte, error) { return []byte("PNG"), nil }
	run(p, next)

	var b struct {
		Input []struct {
			Type      string            `json:"type"`
			Signature string            `json:"signature"`
			ID        string            `json:"id"`
			Name      string            `json:"name"`
			Arguments json.RawMessage   `json:"arguments"`
			CallID    string            `json:"call_id"`
			Result    json.RawMessage   `json:"result"`
			IsError   bool              `json:"is_error"`
			Content   []json.RawMessage `json:"content"`
		} `json:"input"`
	}
	s.mu.Lock()
	json.Unmarshal([]byte(s.bodies[1]), &b)
	s.mu.Unlock()
	var types []string
	for _, st := range b.Input {
		types = append(types, st.Type)
	}
	if got := strings.Join(types, ","); got != "user_input,model_output,thought,function_call,function_call,function_result,function_result,user_input" {
		t.Fatalf("steps = %s, want other providers' thinking left out", got)
	}
	in := b.Input
	if in[2].Signature != "sig-thought" || in[3].ID != "call_343015" || in[3].Signature != "sig-call" || string(in[3].Arguments) != `{"sql":"SELECT 1"}` || in[4].Signature != "" {
		t.Errorf("history = %+v", in[2:5])
	}
	if in[5].CallID != "call_343015" || in[5].Name != "query" || string(in[5].Result) != `[{"type":"text","text":"1"},{"type":"image","data":"UE5H","mime_type":"image/png"}]` {
		t.Errorf("result with image = %+v %s", in[5], in[5].Result)
	}
	if in[6].Name != "describe" || !in[6].IsError || string(in[6].Result) != `"no such table"` {
		t.Errorf("error result = %+v %s", in[6], in[6].Result)
	}
	if len(in[7].Content) != 2 {
		t.Errorf("user step = %s", in[7].Content)
	}
}

func TestEnds(t *testing.T) {
	brokenCall := []string{start,
		`{"index":0,"step":{"id":"c1","type":"function_call","name":"query","arguments":{}},"event_type":"step.start"}`,
		`{"index":0,"delta":{"arguments":"{\"sql\":","type":"arguments_delta"},"event_type":"step.delta"}`,
		`{"index":0,"event_type":"step.stop"}`}
	tests := []struct {
		name string
		r    reply
		stop provider.StopReason
		kind provider.ErrorKind
	}{
		{"max tokens", reply{200, sse(start, `{"index":0,"step":{"type":"model_output"},"event_type":"step.start"}`, `{"index":0,"delta":{"text":"1 2 3","type":"text"},"event_type":"step.delta"}`, `{"index":0,"event_type":"step.stop"}`, completed("incomplete"))}, provider.StopMaxTokens, ""},
		{"max tokens in a call", reply{200, sse(append(brokenCall, completed("incomplete"))...)}, provider.StopMaxTokens, ""},
		{"bad call JSON", reply{200, sse(append(brokenCall, completed("requires_action"))...)}, provider.StopToolUse, ""},
		{"no completed", reply{200, "event: step.start\ndata: {\"index\":0,\"step\":{\"type\":\"model_output\"},\"event_type\":\"step.start\"}\n\n"}, "", provider.Transport},
		{"failed", reply{200, sse(start, `{"interaction":{"status":"failed","errors":[{"code":"internal","message":"An internal error occurred"}]},"event_type":"interaction.completed"}`)}, "", provider.Overloaded},
		{"error in stream", reply{200, sse(start, `{"error":{"code":"resource_exhausted","message":"Resource has been exhausted"},"event_type":"error"}`)}, "", provider.RateLimited},
		{"rate limit", reply{429, `{"error":{"code":"resource_exhausted","message":"Too many requests","details":[{"retryDelay":"7s"}]}}`}, "", provider.RateLimited},
		{"daily quota", reply{429, `{"error":{"code":"resource_exhausted","message":"Quota exceeded for metric generate_content_free_tier_requests, limit: GenerateRequestsPerDayPerProjectPerModel-FreeTier"}}`}, "", provider.Quota},
		{"no such model", reply{404, `{"error":{"message":"Model 'gemini-9' not found.","code":"not_found"}}`}, "", provider.BadRequest},
		{"too large", reply{413, `{"error":{"message":"Request payload size exceeds the limit","code":"invalid_request"}}`}, "", provider.TooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs, err := run(newBackend(t, newServer(t, tt.r)), hello)
			if tt.kind != "" {
				if errKind(err) != tt.kind {
					t.Fatalf("err = %v, want %s", err, tt.kind)
				}
				if tt.name == "no completed" && !errors.Is(err, provider.ErrCutOff) {
					t.Errorf("err = %v, want a cut-off stream", err)
				}
				var pe *provider.Error
				if tt.name == "rate limit" && (!errors.As(err, &pe) || pe.RetryAfter != 7*time.Second) {
					t.Errorf("err = %v, want the retryDelay", err)
				}
				return
			}
			if err != nil || last(evs).Stop != tt.stop {
				t.Fatalf("err = %v, last = %+v", err, last(evs))
			}
			if strings.Contains(tt.name, "call") {
				c := parts(evs)[0].ToolCall
				if c == nil || c.Invalid != `{"sql":` || string(c.Args) != "{}" {
					t.Errorf("call = %+v, want it marked invalid", c)
				}
			}
		})
	}
}

// TestThinkingOff: thinking off asks for the lowest level; a level the
// model rejects is replaced by the next one and remembered for the model.
func TestThinkingOff(t *testing.T) {
	rejected := reply{400, `{"error":{"message":"Thinking level THINKING_LEVEL_MINIMAL is not supported for this model. Please retry with other thinking level.","code":"invalid_request"}}`}
	ok := reply{200, sse(start, completed("completed"))}
	s := newServer(t, rejected, ok)
	p := newBackend(t, s)
	off := hello
	off.Thinking = false
	off.Model = "gemini-3.8-flash"
	if _, err := run(p, off); err != nil {
		t.Fatal(err)
	}
	if _, err := run(p, off); err != nil {
		t.Fatal(err)
	}
	other := off
	other.Model = "gemini-3.5-flash-lite"
	run(p, other)
	var levels []string
	for i := range 4 {
		gc := s.body(t, i)["generation_config"].(map[string]any)
		levels = append(levels, fmt.Sprint(gc["thinking_level"], "/", gc["thinking_summaries"]))
	}
	if got := strings.Join(levels, " "); got != "minimal/none low/none low/none minimal/none" {
		t.Errorf("levels = %s", got)
	}
}

// TestThinkingOffNoLevel: past the last level, no level is sent.
func TestThinkingOffNoLevel(t *testing.T) {
	low := reply{400, `{"error":{"message":"Thinking level THINKING_LEVEL_LOW is not supported for this model.","code":"invalid_request"}}`}
	s := newServer(t, reply{400, strings.Replace(low.body, "LOW", "MINIMAL", 1)}, low, reply{200, sse(start, completed("completed"))})
	off := hello
	off.Thinking = false
	if _, err := run(newBackend(t, s), off); err != nil {
		t.Fatal(err)
	}
	if gc := s.body(t, 2)["generation_config"].(map[string]any); gc["thinking_level"] != nil {
		t.Errorf("generation_config = %v, want no level", gc)
	}
}

// TestOtherBadRequest: a 400 about something else is not retried.
func TestOtherBadRequest(t *testing.T) {
	s := newServer(t, reply{400, `{"error":{"message":"Invalid JSON payload","code":"invalid_request"}}`})
	off := hello
	off.Thinking = false
	if _, err := run(newBackend(t, s), off); errKind(err) != provider.BadRequest || len(s.bodies) != 1 {
		t.Fatalf("err = %v after %d requests", err, len(s.bodies))
	}
}

func TestModels(t *testing.T) {
	s := newServer(t, reply{200, ""})
	ms, err := newBackend(t, s).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.ModelInfo{
		{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", Context: 1048576, Output: 65536},
		{ID: "gemma-4-31b-it", Name: "Gemma 4 31B", Context: 131072, Output: 8192},
	}
	if fmt.Sprint(ms) != fmt.Sprint(want) {
		t.Errorf("models = %+v", ms)
	}
	for _, q := range s.queries {
		if strings.Contains(q, testKey) {
			t.Errorf("the key is in the query: %s", q)
		}
	}
}

// TestKeyNotInErrors: an error answer that quotes the key is redacted.
func TestKeyNotInErrors(t *testing.T) {
	s := newServer(t, reply{400, `{"error":{"message":"API key not valid: ` + testKey + `","code":"invalid_argument"}}`})
	_, err := run(newBackend(t, s), hello)
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.BadRequest {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, pe), testKey) {
		t.Errorf("the key is in the error: %v", err)
	}
}
