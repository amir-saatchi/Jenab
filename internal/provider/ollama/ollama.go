// Package ollama is the backend for a local Ollama through its native API
// (SPEC 3.8, SPIKE-017): /api/chat takes num_ctx and keep_alive per
// request, which /v1 ignores. The stream is one JSON object per line.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// keepAlive keeps the model loaded between turns; loading it again takes
// longer than a turn (SPIKE-017). A starting value.
const keepAlive = "30m"

// New builds the backend for c. A local Ollama needs no key; one is sent
// as a bearer token if set.
func New(c provider.Connection) (provider.Provider, error) {
	g, err := provider.NewGuard(c)
	if err != nil {
		return nil, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	return &backend{name: c.Name, base: strings.TrimSuffix(c.BaseURL, "/"), key: c.Key, guard: g, http: hc, shown: map[string]*shown{}}, nil
}

type backend struct {
	name, base string
	key        secret.Value
	guard      *provider.Guard
	http       *http.Client

	mu    sync.Mutex
	shown map[string]*shown // /api/show answers by model
}

// shown is what /api/show tells about a model.
type shown struct {
	Capabilities []string                   `json:"capabilities"`
	ModelInfo    map[string]json.RawMessage `json:"model_info"`
}

func (s *shown) has(c string) bool { return slices.Contains(s.Capabilities, c) }

// context returns the trained context length, 0 if not reported.
func (s *shown) context() int {
	var arch string
	json.Unmarshal(s.ModelInfo["general.architecture"], &arch)
	var n int
	json.Unmarshal(s.ModelInfo[arch+".context_length"], &n)
	return n
}

// post sends a JSON body to path through the Guard. An error answer is
// returned as an *provider.Error.
func (b *backend) post(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	ctx, capt := provider.WithCapture(ctx)
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	if k := b.key.Reveal(); k != "" {
		req.Header.Set("authorization", "Bearer "+k)
	}
	resp, err := b.guard.Do(req, b.http.Do)
	if err != nil {
		return nil, b.guard.Fail(b.name, capt, err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, b.guard.Fail(b.name, capt, fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	return resp, nil
}

func (b *backend) show(ctx context.Context, model string) (*shown, error) {
	b.mu.Lock()
	s := b.shown[model]
	b.mu.Unlock()
	if s != nil {
		return s, nil
	}
	resp, err := b.post(ctx, "POST", "/api/show", map[string]string{"model": model})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	s = &shown{}
	if err := json.NewDecoder(resp.Body).Decode(s); err != nil {
		return nil, provider.TransportError(b.name, "an unreadable /api/show answer", err)
	}
	b.mu.Lock()
	b.shown[model] = s
	b.mu.Unlock()
	return s, nil
}

func (b *backend) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	resp, err := b.post(ctx, "GET", "/api/tags", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, provider.TransportError(b.name, "an unreadable /api/tags answer", err)
	}
	var out []provider.ModelInfo
	for _, m := range tags.Models {
		info := provider.ModelInfo{ID: m.Name, Name: m.Name}
		if s, err := b.show(ctx, m.Name); err == nil {
			info.Context = s.context()
		}
		out = append(out, info)
	}
	return out, nil
}

type request struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	Tools     []tool    `json:"tools,omitempty"`
	Stream    bool      `json:"stream"`
	Think     *bool     `json:"think,omitempty"`
	Options   options   `json:"options"`
	KeepAlive string    `json:"keep_alive"`
}

type options struct {
	NumCtx     int `json:"num_ctx,omitempty"`
	NumPredict int `json:"num_predict,omitempty"`
}

type tool struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type function struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

type message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking,omitempty"`
	Images    []string   `json:"images,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

type toolCall struct {
	ID       string   `json:"id,omitempty"`
	Function function `json:"function"`
}

type chunk struct {
	Message         message `json:"message"`
	Done            bool    `json:"done"`
	DoneReason      string  `json:"done_reason"`
	PromptEvalCount int     `json:"prompt_eval_count"`
	EvalCount       int     `json:"eval_count"`
	Error           string  `json:"error"`
}

func (b *backend) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		body, err := b.body(ctx, req)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		resp, err := b.post(ctx, "POST", "/api/chat", body)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		defer resp.Body.Close()

		var text, think strings.Builder
		var calls []chat.Part
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			var c chunk
			if err := json.Unmarshal(line, &c); err != nil {
				yield(provider.Event{}, provider.TransportError(b.name, "an unreadable stream line", err))
				return
			}
			if c.Error != "" {
				yield(provider.Event{}, provider.TransportError(b.name, b.guard.Redact(c.Error), nil))
				return
			}
			if t := c.Message.Thinking; t != "" {
				think.WriteString(t)
				if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartThinking, Text: t}, nil) {
					return
				}
			}
			if t := c.Message.Content; t != "" {
				text.WriteString(t)
				if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: t}, nil) {
					return
				}
			}
			for _, tc := range c.Message.ToolCalls {
				args := tc.Function.Arguments
				if string(args) == "null" {
					args = nil
				}
				id := tc.ID
				if id == "" {
					id = newCallID()
				}
				calls = append(calls, chat.Part{Kind: chat.PartToolCall, ToolCall: provider.ToolCall(id, tc.Function.Name, string(args), "")})
			}
			if !c.Done {
				continue
			}
			var parts []chat.Part
			if think.Len() > 0 {
				parts = append(parts, chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: think.String()}})
			}
			if text.Len() > 0 {
				parts = append(parts, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: text.String()}})
			}
			parts = append(parts, calls...)
			for i := range parts {
				if !yield(provider.Event{Kind: provider.EventPart, Part: &parts[i]}, nil) {
					return
				}
			}
			stop := provider.StopEnd
			switch {
			case c.DoneReason == "length": // also with calls: the last one may be cut off
				stop = provider.StopMaxTokens
			case len(calls) > 0:
				stop = provider.StopToolUse
			}
			yield(provider.Event{Kind: provider.EventDone, Stop: stop, Usage: &chat.Usage{Input: c.PromptEvalCount, Output: c.EvalCount}}, nil)
			return
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			yield(provider.Event{}, provider.TransportError(b.name, b.guard.Redact(err.Error()), err))
			return
		}
		if err := ctx.Err(); err != nil {
			yield(provider.Event{}, provider.TransportError(b.name, "cancelled", err))
			return
		}
		yield(provider.Event{}, provider.TransportError(b.name, "the stream ended without done", provider.ErrCutOff))
	}
}

func newCallID() string {
	var b [8]byte
	rand.Read(b[:])
	return "call_" + hex.EncodeToString(b[:])
}

func (b *backend) body(ctx context.Context, req provider.Request) (*request, error) {
	r := &request{Model: req.Model, Stream: true, KeepAlive: keepAlive,
		Options: options{NumCtx: req.Context, NumPredict: req.MaxTokens}}
	// Thinking is set only for models that report it: others reject
	// think: true. Unknown means the model's default.
	if s, err := b.show(ctx, req.Model); err == nil && s.has("thinking") {
		r.Think = &req.Thinking
	}
	for _, t := range req.Tools {
		r.Tools = append(r.Tools, tool{Type: "function", Function: function{Name: t.Name, Description: t.Description, Parameters: t.Schema}})
	}
	var sys []string
	for _, s := range req.System {
		sys = append(sys, s.Text)
	}
	if len(sys) > 0 {
		r.Messages = append(r.Messages, message{Role: "system", Content: strings.Join(sys, "\n\n")})
	}

	names := map[string]string{} // tool call ID → name, for tool_name
	for _, m := range req.Messages {
		switch m.Role {
		case chat.RoleUser:
			um, err := userMessage(ctx, req, m.Parts)
			if err != nil {
				return nil, err
			}
			r.Messages = append(r.Messages, um)
		case chat.RoleAssistant:
			am := message{Role: "assistant"}
			var text strings.Builder
			for _, p := range m.Parts {
				switch {
				case p.Thinking != nil && p.Thinking.Signature == "" && p.Thinking.Redacted == "":
					am.Thinking += p.Thinking.Text // plain thinking text; signed or hidden thinking is another provider's
				case p.Text != nil:
					text.WriteString(p.Text.Text)
				case p.ToolCall != nil:
					names[p.ToolCall.ID] = p.ToolCall.Name
					am.ToolCalls = append(am.ToolCalls, toolCall{Function: function{Name: p.ToolCall.Name, Arguments: p.ToolCall.Args}})
				}
			}
			am.Content = text.String()
			r.Messages = append(r.Messages, am)
		case chat.RoleTool:
			var images []chat.Part
			for _, p := range m.Parts {
				switch {
				case p.ToolResult != nil:
					r.Messages = append(r.Messages, message{Role: "tool", Content: p.ToolResult.Text, ToolName: names[p.ToolResult.CallID]})
				case p.Image != nil:
					images = append(images, p)
				}
			}
			// A tool's images follow the results in one user message.
			if len(images) > 0 {
				um, err := userMessage(ctx, req, images)
				if err != nil {
					return nil, err
				}
				r.Messages = append(r.Messages, um)
			}
		}
	}
	return r, nil
}

func userMessage(ctx context.Context, req provider.Request, ps []chat.Part) (message, error) {
	m := message{Role: "user"}
	var text []string
	for _, p := range ps {
		switch {
		case p.Text != nil:
			text = append(text, p.Text.Text)
		case p.Notice != nil:
			text = append(text, p.Notice.Text)
		case p.Image != nil:
			if req.Image == nil {
				return m, fmt.Errorf("image %s: no image reader", p.Image.Ref)
			}
			data, err := req.Image(ctx, p.Image.Ref)
			if err != nil {
				return m, fmt.Errorf("image %s: %w", p.Image.Ref, err)
			}
			m.Images = append(m.Images, base64.StdEncoding.EncodeToString(data))
		}
	}
	m.Content = strings.Join(text, "\n\n")
	return m, nil
}
