package ollama

import (
	"context"
	"encoding/json"
	"errors"
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

const qwenShow = `{"capabilities":["completion","tools","thinking","vision"],"model_info":{"general.architecture":"qwen35","qwen35.context_length":262144}}`

type server struct {
	*httptest.Server
	mu    sync.Mutex
	chats []string
	shows int
	auth  []string
}

// newServer answers /api/chat with chat, /api/show with show and
// /api/tags with two models.
func newServer(t *testing.T, status int, chat, show string) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		s.mu.Unlock()
		switch r.URL.Path {
		case "/api/show":
			s.mu.Lock()
			s.shows++
			s.mu.Unlock()
			if strings.Contains(string(b), "gemma") {
				io.WriteString(w, `{"capabilities":["completion"],"model_info":{"general.architecture":"gemma3","gemma3.context_length":131072}}`)
				return
			}
			io.WriteString(w, show)
		case "/api/tags":
			io.WriteString(w, `{"models":[{"name":"qwen3.5:4b","model":"qwen3.5:4b","size":3400000000},{"name":"gemma3:1b","model":"gemma3:1b"}]}`)
		case "/api/chat":
			s.mu.Lock()
			s.chats = append(s.chats, string(b))
			s.mu.Unlock()
			w.Header().Set("content-type", "application/x-ndjson")
			w.WriteHeader(status)
			io.WriteString(w, chat)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) lastChat(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var m map[string]any
	if err := json.Unmarshal([]byte(s.chats[len(s.chats)-1]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func newBackend(t *testing.T, s *server, key string) provider.Provider {
	t.Helper()
	p, err := New(provider.Connection{Name: "local", Kind: provider.KindOllama, BaseURL: s.URL + "/", Key: secret.NewValue("provider:local", key), HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return p
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

var hello = provider.Request{Model: "qwen3.5:4b", Thinking: true, Context: 12288, MaxTokens: 400,
	System:   []provider.Block{{Text: "You are Jenab.", Cache: true}, {Text: "Today is Friday."}},
	Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Count the BTC rows."}}}}}}

const answer = `{"model":"qwen3.5:4b","message":{"role":"assistant","content":"","thinking":"The user wants"},"done":false}
{"model":"qwen3.5:4b","message":{"role":"assistant","content":"","thinking":" a count."},"done":false}
{"model":"qwen3.5:4b","message":{"role":"assistant","content":"There are "},"done":false}
{"model":"qwen3.5:4b","message":{"role":"assistant","content":"42."},"done":false}
{"model":"qwen3.5:4b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":1080,"eval_count":31}
`

func TestComplete(t *testing.T) {
	s := newServer(t, 200, answer, qwenShow)
	p := newBackend(t, s, "")
	evs, err := run(p, hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 2 || ps[0].Thinking.Text != "The user wants a count." || ps[1].Text.Text != "There are 42." {
		t.Fatalf("parts = %+v", ps)
	}
	d := evs[len(evs)-1]
	if d.Kind != provider.EventDone || d.Stop != provider.StopEnd || *d.Usage != (chat.Usage{Input: 1080, Output: 31}) {
		t.Fatalf("done = %+v", d)
	}
	b := s.lastChat(t)
	opts := b["options"].(map[string]any)
	if b["think"] != true || opts["num_ctx"] != float64(12288) || opts["num_predict"] != float64(400) || b["keep_alive"] != keepAlive || b["stream"] != true {
		t.Errorf("body = %v", b)
	}
	if m := b["messages"].([]any)[0].(map[string]any); m["role"] != "system" || m["content"] != "You are Jenab.\n\nToday is Friday." {
		t.Errorf("system = %v", m)
	}
	if s.auth[0] != "" {
		t.Errorf("a local Ollama got Authorization %q", s.auth[0])
	}

	// /api/show is asked once per model.
	run(p, hello)
	if s.shows != 1 {
		t.Errorf("%d /api/show calls", s.shows)
	}
}

func TestThink(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		thinking bool
		want     any // the think field; nil for none
	}{
		{"on", "qwen3.5:4b", true, true},
		{"off", "qwen3.5:4b", false, false},
		{"a model without thinking", "gemma3:1b", true, nil},
	}
	for _, tt := range tests {
		s := newServer(t, 200, answer, qwenShow)
		req := hello
		req.Model, req.Thinking = tt.model, tt.thinking
		if _, err := run(newBackend(t, s, ""), req); err != nil {
			t.Fatal(err)
		}
		if got := s.lastChat(t)["think"]; got != tt.want {
			t.Errorf("%s: think = %v, want %v", tt.name, got, tt.want)
		}
	}
}

const calls = `{"model":"qwen3.5:4b","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"query","arguments":{"sql":"SELECT count(*) FROM btc"}}},{"function":{"name":"describe_table","arguments":{"table":"btc"}}}]},"done":false}
{"model":"qwen3.5:4b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":2668,"eval_count":40}
`

func TestToolCallsAndSendBack(t *testing.T) {
	s := newServer(t, 200, calls, qwenShow)
	p := newBackend(t, s, "")
	evs, err := run(p, hello)
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(evs)
	if len(ps) != 2 || ps[0].ToolCall.Name != "query" || string(ps[0].ToolCall.Args) != `{"sql":"SELECT count(*) FROM btc"}` ||
		ps[0].ToolCall.ID == "" || ps[0].ToolCall.ID == ps[1].ToolCall.ID {
		t.Fatalf("parts = %+v", ps)
	}
	if evs[len(evs)-1].Stop != provider.StopToolUse {
		t.Errorf("stop = %s", evs[len(evs)-1].Stop)
	}

	next := hello
	next.Image = func(context.Context, string) ([]byte, error) { return []byte("PNG"), nil }
	next.Messages = append(append([]chat.Message(nil), hello.Messages...),
		chat.Message{Role: chat.RoleAssistant, Parts: append([]chat.Part{
			{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "local thought"}},
			{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "signed thought", Signature: "EqQB"}},
		}, ps...)},
		chat.Message{Role: chat.RoleTool, Parts: []chat.Part{
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: ps[0].ToolCall.ID, Text: "42"}},
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: ps[1].ToolCall.ID, Text: "chart"}},
			{Kind: chat.PartImage, Image: &chat.Image{Ref: "img", MIME: "image/png"}},
		}})
	if _, err := run(p, next); err != nil {
		t.Fatal(err)
	}
	var req struct {
		Messages []message `json:"messages"`
	}
	s.mu.Lock()
	json.Unmarshal([]byte(s.chats[1]), &req)
	s.mu.Unlock()
	m := req.Messages
	if len(m) != 6 {
		t.Fatalf("%d messages, want system, user, assistant, tool, tool, user: %+v", len(m), m)
	}
	a := m[2]
	if a.Thinking != "local thought" || len(a.ToolCalls) != 2 || string(a.ToolCalls[0].Function.Arguments) != `{"sql":"SELECT count(*) FROM btc"}` {
		t.Errorf("assistant = %+v", a)
	}
	if m[3].Role != "tool" || m[3].ToolName != "query" || m[3].Content != "42" || m[4].ToolName != "describe_table" {
		t.Errorf("results = %+v %+v", m[3], m[4])
	}
	if m[5].Role != "user" || len(m[5].Images) != 1 || m[5].Images[0] != "UE5H" {
		t.Errorf("image message = %+v", m[5])
	}
}

func TestEnds(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		stop   provider.StopReason
		kind   provider.ErrorKind
	}{
		{"length", 200, `{"message":{"role":"assistant","content":"par"},"done":false}` + "\n" + `{"message":{"role":"assistant","content":""},"done":true,"done_reason":"length","prompt_eval_count":10,"eval_count":400}`, provider.StopMaxTokens, ""},
		{"cut off", 200, `{"message":{"role":"assistant","content":"par"},"done":false}` + "\n", "", provider.Transport},
		{"broken line", 200, `{"message":{"role":"assistant","content":"par"` + "\n", "", provider.Transport},
		{"error in stream", 200, `{"message":{"role":"assistant","content":"par"},"done":false}` + "\n" + `{"error":"llama runner process has terminated"}`, "", provider.Transport},
		{"model not found", 404, `{"error":"model \"qwen9\" not found, try pulling it first"}`, "", provider.BadRequest},
		{"too large", 413, `{"error":"request entity too large"}`, "", provider.TooLarge},
		{"cloud quota", 429, `{"error":"you've reached your weekly usage limit, upgrade for higher limits"}`, "", provider.Quota},
		{"busy", 503, `{"error":"server busy, please try again"}`, "", provider.Overloaded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, tt.status, tt.body, qwenShow)
			evs, err := run(newBackend(t, s, ""), hello)
			if tt.kind != "" {
				if errKind(err) != tt.kind {
					t.Fatalf("err = %v, want %s", err, tt.kind)
				}
				if tt.name == "error in stream" && !strings.Contains(err.Error(), "runner process has terminated") {
					t.Errorf("err = %v, want Ollama's message", err)
				}
				return
			}
			if err != nil || evs[len(evs)-1].Stop != tt.stop {
				t.Fatalf("err = %v, events %+v", err, evs)
			}
		})
	}
}

func TestKey(t *testing.T) {
	const key = "ollama-test-SECRET-0123456789"
	s := newServer(t, 401, `{"error":"unauthorized: key `+key+` is not valid"}`, qwenShow)
	_, err := run(newBackend(t, s, key), hello)
	if errKind(err) != provider.BadRequest || strings.Contains(err.Error(), key) {
		t.Fatalf("err = %v", err)
	}
	for _, a := range s.auth {
		if a != "Bearer "+key {
			t.Errorf("Authorization = %q", a)
		}
	}
}

func TestModels(t *testing.T) {
	s := newServer(t, 200, answer, qwenShow)
	ms, err := newBackend(t, s, "").Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.ModelInfo{{ID: "qwen3.5:4b", Name: "qwen3.5:4b", Context: 262144}, {ID: "gemma3:1b", Name: "gemma3:1b", Context: 131072}}
	if len(ms) != 2 || ms[0] != want[0] || ms[1] != want[1] {
		t.Errorf("models = %+v", ms)
	}
}
