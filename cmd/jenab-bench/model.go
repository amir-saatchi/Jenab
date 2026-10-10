package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// modelName is the fake model, as the app's settings name it.
const modelName = "bench-model"

// fakeModel is a local Ollama server (/api/tags, /api/show, /api/chat)
// that streams a fixed Markdown reply at a set rate. A local Ollama needs
// no key, so the app runs without the keychain.
type fakeModel struct {
	reply   []string // tokens of a chat answer
	rate    float64  // tokens a second
	srv     *http.Server
	url     string
	streams atomic.Int64 // answers streaming now
	peak    atomic.Int64
	calls   atomic.Int64
}

func startModel(reply string, rate float64) (*fakeModel, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &fakeModel{reply: tokenize(reply), rate: rate, url: "http://" + ln.Addr().String() + "/"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"models": []map[string]string{{"name": modelName}}})
	})
	mux.HandleFunc("POST /api/show", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"capabilities": []string{"completion", "tools"},
			"model_info": map[string]any{"general.architecture": "bench", "bench.context_length": 262144}})
	})
	mux.HandleFunc("POST /api/chat", m.chat)
	m.srv = &http.Server{Handler: mux}
	go m.srv.Serve(ln)
	return m, nil
}

func (m *fakeModel) close() { m.srv.Close() }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// chat streams the reply. A request without tools, such as a chat title,
// gets a short answer at once.
func (m *fakeModel) chat(w http.ResponseWriter, r *http.Request) {
	m.calls.Add(1)
	var req struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The prompt size the app checks against its own estimate: generous,
	// so its truncation guard never fires.
	prompt := 0
	for _, msg := range req.Messages {
		prompt += len(msg.Content)
	}
	w.Header().Set("content-type", "application/x-ndjson")
	fl, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	send := func(content string, done bool) {
		c := map[string]any{"message": map[string]string{"role": "assistant", "content": content}, "done": done}
		if done {
			c["done_reason"] = "stop"
			c["prompt_eval_count"] = prompt
			c["eval_count"] = len(m.reply)
		}
		enc.Encode(c)
		if fl != nil {
			fl.Flush()
		}
	}
	if len(req.Tools) == 0 {
		send("Benchmark chat", false)
		send("", true)
		return
	}
	n := m.streams.Add(1)
	defer m.streams.Add(-1)
	for p := m.peak.Load(); n > p && !m.peak.CompareAndSwap(p, n); p = m.peak.Load() {
	}
	// Paced by the clock, not by sleeps, so the rate holds on a busy machine.
	start := time.Now()
	every := time.Duration(float64(time.Second) / m.rate)
	for i, tok := range m.reply {
		if d := time.Until(start.Add(time.Duration(i) * every)); d > 0 {
			select {
			case <-time.After(d):
			case <-r.Context().Done():
				return
			}
		}
		send(tok, false)
	}
	send("", true)
}

func (m *fakeModel) String() string {
	return fmt.Sprintf("%d tokens at %.0f/s", len(m.reply), m.rate)
}
