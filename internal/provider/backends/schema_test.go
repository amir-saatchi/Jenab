package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// A schema with keywords some APIs ignore (minLength), to see it is sent
// as given.
const answerSchema = `{"type":"object","properties":{"a":{"type":"string","minLength":1}},"required":["a"]}`

const answerText = `{"a":"x"}`

// schemaKind is one kind's protocol, as far as the test needs it.
type schemaKind struct {
	kind provider.Kind
	path string // the base URL's path
	// field digs the JSON mode out of a request body: the schema it
	// carries, and nil when the body has none.
	field func(t *testing.T, body map[string]json.RawMessage) json.RawMessage
	// answer is a complete stream whose text is answerText.
	answer   string
	reject   string // a 400 body
	noSchema bool   // the backend sends no JSON mode
}

func dig(t *testing.T, raw json.RawMessage, keys ...string) json.RawMessage {
	t.Helper()
	for _, k := range keys {
		if raw == nil {
			return nil
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: %v in %s", k, err, raw)
		}
		raw = m[k]
	}
	return raw
}

func object(t *testing.T, body map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func want(t *testing.T, raw json.RawMessage, key, value string) {
	t.Helper()
	if got := string(dig(t, raw, key)); got != value {
		t.Errorf("%s = %s, want %s", key, got, value)
	}
}

var schemaKinds = []schemaKind{
	{
		kind: provider.KindCompatible, path: "/v1/",
		field: func(t *testing.T, body map[string]json.RawMessage) json.RawMessage {
			rf := body["response_format"]
			if rf == nil {
				return nil
			}
			want(t, rf, "type", `"json_schema"`)
			js := dig(t, rf, "json_schema")
			want(t, js, "name", `"answer"`)
			want(t, js, "strict", `false`)
			return dig(t, js, "schema")
		},
		answer: `data: {"choices":[{"index":0,"delta":{"content":"{\"a\":\"x\"}"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
		reject: `{"error":{"message":"response_format json_schema is not supported","type":"invalid_request_error"}}`,
	},
	{
		kind: provider.KindOpenAI, path: "/v1/",
		field: func(t *testing.T, body map[string]json.RawMessage) json.RawMessage {
			f := dig(t, object(t, body), "text", "format")
			if f == nil {
				return nil
			}
			want(t, f, "type", `"json_schema"`)
			want(t, f, "name", `"answer"`)
			want(t, f, "strict", `false`)
			return dig(t, f, "schema")
		},
		answer: "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"a\\\":\\\"x\\\"}\"}\n\n" +
			"event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"{\\\"a\\\":\\\"x\\\"}\",\"annotations\":[]}]}}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5}}}\n\n",
		reject: `{"error":{"message":"Invalid schema for response_format 'answer'","type":"invalid_request_error","param":"text.format.schema"}}`,
	},
	{
		kind: provider.KindGemini, path: "/v1beta/",
		field: func(t *testing.T, body map[string]json.RawMessage) json.RawMessage {
			rf := body["response_format"]
			if rf == nil {
				return nil
			}
			want(t, rf, "type", `"text"`)
			want(t, rf, "mime_type", `"application/json"`)
			return dig(t, rf, "schema")
		},
		answer: "event: step.start\ndata: {\"index\":0,\"step\":{\"type\":\"model_output\"},\"event_type\":\"step.start\"}\n\n" +
			"event: step.delta\ndata: {\"index\":0,\"delta\":{\"text\":\"{\\\"a\\\":\\\"x\\\"}\",\"type\":\"text\"},\"event_type\":\"step.delta\"}\n\n" +
			"event: step.stop\ndata: {\"index\":0,\"event_type\":\"step.stop\"}\n\n" +
			"event: interaction.completed\ndata: {\"interaction\":{\"status\":\"completed\",\"usage\":{\"total_input_tokens\":10,\"total_output_tokens\":5}},\"event_type\":\"interaction.completed\"}\n\n",
		reject: `{"error":{"code":400,"message":"Invalid JSON payload received. Unknown name \"response_format\"","status":"INVALID_ARGUMENT"}}`,
	},
	{
		kind: provider.KindOllama, path: "/",
		field: func(t *testing.T, body map[string]json.RawMessage) json.RawMessage {
			return body["format"]
		},
		answer: `{"message":{"role":"assistant","content":"{\"a\":\"x\"}"},"done":false}` + "\n" +
			`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":5}` + "\n",
		reject: `{"error":"invalid format: json schema not supported"}`,
	},
	{
		kind: provider.KindAnthropic, path: "/", noSchema: true,
		field: func(t *testing.T, body map[string]json.RawMessage) json.RawMessage {
			for _, k := range []string{"output_config", "output_format", "response_format", "format"} {
				if body[k] != nil {
					return body[k]
				}
			}
			return nil
		},
		answer: "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n" +
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"{\\\"a\\\":\\\"x\\\"}\"}}\n\n" +
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	},
}

// schemaServer answers every model request with sk.answer, or with a 400
// when reject is set and the body carries a JSON mode. It keeps the
// bodies.
type schemaServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]json.RawMessage
}

func newSchemaServer(t *testing.T, sk schemaKind, reject bool) *schemaServer {
	s := &schemaServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/show") { // Ollama asks what the model can do
			io.WriteString(w, `{"capabilities":["completion"]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("%s: body: %v", sk.kind, err)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		if reject && sk.field(t, body) != nil {
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, sk.reject)
			return
		}
		if sk.kind == provider.KindOllama {
			w.Header().Set("content-type", "application/x-ndjson")
		} else {
			w.Header().Set("content-type", "text/event-stream")
		}
		io.WriteString(w, sk.answer)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *schemaServer) sent() []map[string]json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]json.RawMessage(nil), s.bodies...)
}

type keyring map[string]string

func (k keyring) Get(name string) (string, error) {
	v, ok := k[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}
func (k keyring) Set(name, v string) error { k[name] = v; return nil }
func (k keyring) Delete(name string) error { delete(k, name); return nil }

// schemaRegistry is a Registry with one provider "p" of sk's kind at srv.
func schemaRegistry(sk schemaKind, srv *schemaServer) *provider.Registry {
	f := All()[sk.kind]
	return provider.NewRegistry(provider.Deps{
		Settings: config.LLMSettings{
			MaxParallelCalls: 4,
			Providers: map[string]config.ProviderSettings{
				"p": {Kind: string(sk.kind), BaseURL: srv.URL + sk.path},
			},
		},
		Secrets: secret.New(keyring{provider.KeyName("p"): "test-key-0123456789"}),
		Backends: map[provider.Kind]provider.Factory{sk.kind: func(c provider.Connection) (provider.Provider, error) {
			c.HTTP = srv.Client()
			return f(c)
		}},
	})
}

func ask(t *testing.T, reg *provider.Registry, model string, schema bool) string {
	t.Helper()
	req := provider.Request{Model: "p/" + model, MaxTokens: 100,
		System: []provider.Block{{Text: "Answer in JSON."}}}
	if schema {
		req.Schema = json.RawMessage(answerSchema)
	}
	var text strings.Builder
	for ev, err := range reg.Stream(context.Background(), limit.Background, req) {
		if err != nil {
			t.Fatal(err)
		}
		if ev.Kind == provider.EventPart && ev.Part.Text != nil {
			text.WriteString(ev.Part.Text.Text)
		}
	}
	return text.String()
}

// TestSchemaPerKind: each kind sends the schema in its API's JSON mode,
// as given; sends none without one; and drops it for the model after the
// API rejects it (TASK-029).
func TestSchemaPerKind(t *testing.T) {
	for _, sk := range schemaKinds {
		t.Run(string(sk.kind), func(t *testing.T) {
			srv := newSchemaServer(t, sk, false)
			reg := schemaRegistry(sk, srv)
			if got := ask(t, reg, "m1", false); got != answerText {
				t.Errorf("answer %q", got)
			}
			if got := ask(t, reg, "m1", true); got != answerText {
				t.Errorf("answer %q", got)
			}
			bodies := srv.sent()
			if len(bodies) != 2 {
				t.Fatalf("%d requests, want 2", len(bodies))
			}
			if f := sk.field(t, bodies[0]); f != nil {
				t.Errorf("a request without a schema sent %s", f)
			}
			f := sk.field(t, bodies[1])
			switch {
			case sk.noSchema && f != nil:
				t.Errorf("sent %s, want no JSON mode", f)
			case !sk.noSchema && string(f) != answerSchema:
				t.Errorf("schema sent as %s, want it as given: %s", f, answerSchema)
			}
			// The JSON mode is the only difference.
			if sk.noSchema && !bytes.Equal(object(t, bodies[0]), object(t, bodies[1])) {
				t.Errorf("the schema changed the body:\n%s\n%s", object(t, bodies[0]), object(t, bodies[1]))
			}
		})
		if sk.noSchema {
			continue
		}
		t.Run(string(sk.kind)+"/rejected", func(t *testing.T) {
			srv := newSchemaServer(t, sk, true)
			reg := schemaRegistry(sk, srv)
			if got := ask(t, reg, "m1", true); got != answerText {
				t.Errorf("answer %q", got)
			}
			if got := ask(t, reg, "m1", true); got != answerText {
				t.Errorf("answer %q", got)
			}
			ask(t, reg, "m2", true) // another model still tries it
			var with []bool
			for _, b := range srv.sent() {
				with = append(with, sk.field(t, b) != nil)
			}
			if want := []bool{true, false, false, true, false}; !equalBools(with, want) {
				t.Errorf("JSON mode per request = %v, want %v", with, want)
			}
		})
	}
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
