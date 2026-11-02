package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// OpenAI is a chat-completions client for any OpenAI-compatible API (Mistral here). The key goes
// only into the Authorization header. Requests are counted per UTC day in
// out/requests-<name>-<date>.txt, and calls are spaced by Gap.
type OpenAI struct {
	BaseURL string
	Key     string
	Model   string
	Name    string // for the request count file and messages
	Gap     time.Duration
	OutDir  string
	http    http.Client
	lastReq time.Time
}

func (o *OpenAI) countFile() string {
	return filepath.Join(o.OutDir, "requests-"+o.Name+"-"+time.Now().UTC().Format("2006-01-02")+".txt")
}

func (o *OpenAI) used() int {
	b, _ := os.ReadFile(o.countFile())
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func (o *OpenAI) bump() {
	os.MkdirAll(o.OutDir, 0o755)
	os.WriteFile(o.countFile(), []byte(strconv.Itoa(o.used()+1)), 0o644)
}

// post sends one request, retrying 429 and 5xx up to 4 times. Every attempt counts.
func (o *OpenAI) post(path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := strings.TrimSuffix(o.BaseURL, "/") + "/" + path
	for attempt := 0; ; attempt++ {
		if gap := o.Gap - time.Since(o.lastReq); gap > 0 {
			time.Sleep(gap)
		}
		o.lastReq = time.Now()
		o.bump()
		req, _ := http.NewRequest("POST", u, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.Key)
		o.http.Timeout = 120 * time.Second
		resp, err := o.http.Do(req)
		if err != nil {
			if attempt < 4 {
				time.Sleep(5 * time.Second)
				continue
			}
			return nil, fmt.Errorf("%s", redact(err.Error()))
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return rb, nil
		}
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < 4 {
			wait := time.Duration(10*(attempt+1)) * time.Second
			fmt.Fprintf(os.Stderr, "  %s %d, retry in %s\n", o.Name, resp.StatusCode, wait)
			time.Sleep(wait)
			continue
		}
		return nil, fmt.Errorf("%s %d: %s", o.Name, resp.StatusCode, redact(clip(string(rb), 600)))
	}
}

// openaiConv is one conversation. The assistant's messages go back as they came.
type openaiConv struct {
	o        *OpenAI
	messages []json.RawMessage
	tools    []any
}

func newOpenAIConv(o *OpenAI, system, goal string, decls []any) *openaiConv {
	c := &openaiConv{o: o}
	for _, d := range decls {
		c.tools = append(c.tools, map[string]any{"type": "function", "function": d})
	}
	c.add(map[string]any{"role": "system", "content": system})
	c.add(map[string]any{"role": "user", "content": goal})
	return c
}

func (c *openaiConv) add(m any) {
	b, _ := json.Marshal(m)
	c.messages = append(c.messages, b)
}

func (c *openaiConv) Next() (*GenResult, error) {
	rb, err := c.o.post("chat/completions", map[string]any{"model": c.o.Model, "messages": c.messages, "tools": c.tools})
	if err != nil {
		return nil, err
	}
	var r struct {
		Choices []struct {
			Message      json.RawMessage `json:"message"`
			FinishReason string          `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rb, &r); err != nil {
		return nil, err
	}
	if len(r.Choices) == 0 {
		return nil, fmt.Errorf("no choices: %s", clip(string(rb), 300))
	}
	ch := r.Choices[0]
	var m struct {
		Content   any `json:"content"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments any    `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	json.Unmarshal(ch.Message, &m)
	res := &GenResult{Finish: ch.FinishReason, Usage: Usage{Prompt: r.Usage.Prompt, Output: r.Usage.Completion, Total: r.Usage.Total}}
	if s, ok := m.Content.(string); ok {
		res.Text = s
	}
	for _, tc := range m.ToolCalls {
		var args json.RawMessage
		switch a := tc.Function.Arguments.(type) {
		case string:
			args = json.RawMessage(a)
		default:
			args, _ = json.Marshal(a)
		}
		if !json.Valid(args) {
			args = json.RawMessage("{}")
		}
		res.Calls = append(res.Calls, FunctionCall{Name: tc.Function.Name, Args: args, ID: tc.ID})
	}
	c.messages = append(c.messages, ch.Message)
	return res, nil
}

func (c *openaiConv) Reply(calls []FunctionCall, outs []string) {
	for i, call := range calls {
		c.add(map[string]any{"role": "tool", "tool_call_id": call.ID, "name": call.Name, "content": outs[i]})
	}
}
