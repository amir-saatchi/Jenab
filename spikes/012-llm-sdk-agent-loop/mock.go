package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Mock is a local server that speaks the Anthropic Messages API (POST .../v1/messages) and the
// OpenAI Chat Completions API (POST .../v1/chat/completions). A script decides each response.
type Mock struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
	raw    []string
	script func(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any)
	// closed receives the time the handler saw the client go away (cancel test).
	closed chan stamp
}

func newMock(script func(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any)) *Mock {
	m := &Mock{script: script, closed: make(chan stamp, 4)}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		m.mu.Lock()
		n := len(m.bodies)
		m.bodies = append(m.bodies, body)
		m.raw = append(m.raw, string(b))
		m.mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/messages") && !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.Error(w, `{"error":{"message":"unknown path `+r.URL.Path+`"}}`, 404)
			return
		}
		m.script(m, n, w, r, body)
	}))
	return m
}

func (m *Mock) URL() string { return m.srv.URL }
func (m *Mock) Close()      { m.srv.CloseClientConnections(); m.srv.Close() }
func (m *Mock) Count() int  { m.mu.Lock(); defer m.mu.Unlock(); return len(m.bodies) }
func (m *Mock) Body(i int) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 {
		i += len(m.bodies)
	}
	if i < 0 || i >= len(m.bodies) {
		return nil
	}
	return m.bodies[i]
}
func (m *Mock) Raw(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.raw) {
		return ""
	}
	return m.raw[i]
}

// ---- SSE writers ----

type sse struct {
	w http.ResponseWriter
	f http.Flusher
}

func startSSE(w http.ResponseWriter) *sse {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	f, _ := w.(http.Flusher)
	s := &sse{w, f}
	s.flush()
	return s
}
func (s *sse) flush() {
	if s.f != nil {
		s.f.Flush()
	}
}

// ev writes one Anthropic event: "event: name\ndata: json\n\n".
func (s *sse) ev(name string, data any) {
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, js(data))
	s.flush()
}

// data writes one OpenAI chunk: "data: json\n\n".
func (s *sse) data(d any) {
	if str, ok := d.(string); ok {
		fmt.Fprintf(s.w, "data: %s\n\n", str)
	} else {
		fmt.Fprintf(s.w, "data: %s\n\n", js(d))
	}
	s.flush()
}

func js(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

type obj = map[string]any

// ---- Anthropic stream builder (format from the Messages streaming docs) ----

type aStream struct {
	s   *sse
	idx int
}

func anthStart(w http.ResponseWriter, u Usage) *aStream {
	s := startSSE(w)
	s.ev("message_start", obj{"type": "message_start", "message": obj{
		"id": "msg_01mock", "type": "message", "role": "assistant", "content": []any{}, "model": "claude-mock",
		"stop_reason": nil, "stop_sequence": nil,
		"usage": obj{"input_tokens": u.Input, "cache_creation_input_tokens": u.CacheWrite, "cache_read_input_tokens": u.CacheRead, "output_tokens": 1},
	}})
	return &aStream{s: s}
}

func (a *aStream) text(pieces ...string) {
	a.s.ev("content_block_start", obj{"type": "content_block_start", "index": a.idx, "content_block": obj{"type": "text", "text": ""}})
	a.s.ev("ping", obj{"type": "ping"})
	for _, p := range pieces {
		a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": a.idx, "delta": obj{"type": "text_delta", "text": p}})
	}
	a.s.ev("content_block_stop", obj{"type": "content_block_stop", "index": a.idx})
	a.idx++
}

func (a *aStream) thinking(sig string, pieces ...string) {
	a.s.ev("content_block_start", obj{"type": "content_block_start", "index": a.idx, "content_block": obj{"type": "thinking", "thinking": "", "signature": ""}})
	for _, p := range pieces {
		a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": a.idx, "delta": obj{"type": "thinking_delta", "thinking": p}})
	}
	a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": a.idx, "delta": obj{"type": "signature_delta", "signature": sig}})
	a.s.ev("content_block_stop", obj{"type": "content_block_stop", "index": a.idx})
	a.idx++
}

func (a *aStream) tool(id, name string, pieces ...string) {
	a.s.ev("content_block_start", obj{"type": "content_block_start", "index": a.idx, "content_block": obj{"type": "tool_use", "id": id, "name": name, "input": obj{}}})
	a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": a.idx, "delta": obj{"type": "input_json_delta", "partial_json": ""}})
	for _, p := range pieces {
		a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": a.idx, "delta": obj{"type": "input_json_delta", "partial_json": p}})
	}
	a.s.ev("content_block_stop", obj{"type": "content_block_stop", "index": a.idx})
	a.idx++
}

func (a *aStream) end(stop string, out int) {
	a.s.ev("message_delta", obj{"type": "message_delta", "delta": obj{"stop_reason": stop, "stop_sequence": nil}, "usage": obj{"output_tokens": out}})
	a.s.ev("message_stop", obj{"type": "message_stop"})
}

func anthError(w http.ResponseWriter, status int, typ, msg string, hdr map[string]string) {
	for k, v := range hdr {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("request-id", "req_mock01")
	w.WriteHeader(status)
	fmt.Fprint(w, js(obj{"type": "error", "error": obj{"type": typ, "message": msg}, "request_id": "req_mock01"}))
}

// ---- OpenAI stream builder (chat.completion.chunk) ----

type oStream struct {
	s     *sse
	usage bool
}

func oaiStart(w http.ResponseWriter, body map[string]any) *oStream {
	s := startSSE(w)
	inc := false
	if so, ok := body["stream_options"].(map[string]any); ok {
		inc, _ = so["include_usage"].(bool)
	}
	o := &oStream{s: s, usage: inc}
	o.chunk(obj{"role": "assistant", "content": ""}, nil)
	return o
}

func (o *oStream) chunk(delta obj, finish any) {
	c := obj{"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": 1790000000, "model": "gpt-mock",
		"system_fingerprint": "fp_mock", "choices": []any{obj{"index": 0, "delta": delta, "logprobs": nil, "finish_reason": finish}}}
	if o.usage {
		c["usage"] = nil
	}
	o.s.data(c)
}

func (o *oStream) text(pieces ...string) {
	for _, p := range pieces {
		o.chunk(obj{"content": p}, nil)
	}
}

// tool streams one tool call: first chunk with id and name, then argument pieces.
func (o *oStream) tool(index int, id, name string, pieces ...string) {
	o.chunk(obj{"tool_calls": []any{obj{"index": index, "id": id, "type": "function", "function": obj{"name": name, "arguments": ""}}}}, nil)
	for _, p := range pieces {
		o.chunk(obj{"tool_calls": []any{obj{"index": index, "function": obj{"arguments": p}}}}, nil)
	}
}

func (o *oStream) end(finish string, u Usage) {
	o.chunk(obj{}, finish)
	if o.usage {
		o.s.data(obj{"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": 1790000000, "model": "gpt-mock",
			"system_fingerprint": "fp_mock", "choices": []any{},
			"usage": obj{"prompt_tokens": u.Input, "completion_tokens": u.Output, "total_tokens": u.Input + u.Output,
				"prompt_tokens_details": obj{"cached_tokens": u.CacheRead, "audio_tokens": 0},
				"completion_tokens_details": obj{"reasoning_tokens": 0, "audio_tokens": 0, "accepted_prediction_tokens": 0, "rejected_prediction_tokens": 0}}})
	}
	o.s.data("[DONE]")
}

func oaiError(w http.ResponseWriter, status int, typ, code, msg string, hdr map[string]string) {
	for k, v := range hdr {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-request-id", "req_mock01")
	w.WriteHeader(status)
	fmt.Fprint(w, js(obj{"error": obj{"message": msg, "type": typ, "param": nil, "code": code}}))
}

// ---- JSON helpers for checks ----

func arr(v any) []any       { a, _ := v.([]any); return a }
func mp(v any) map[string]any { m, _ := v.(map[string]any); return m }
func str(v any) string       { s, _ := v.(string); return s }

// isAnth reports whether the request went to the Anthropic endpoint.
func isAnth(r *http.Request) bool { return strings.HasSuffix(r.URL.Path, "/messages") }
