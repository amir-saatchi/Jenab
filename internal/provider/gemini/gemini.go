// Package gemini is the backend for Gemini through its native Interactions
// API (SPEC 3.8, TASK-004), Google's API for new projects. It is a small
// REST client: Google's Go SDK adds about 13 MB, mostly for Vertex AI.
//
// Every request is stateless (store: false) and sends the whole history as
// steps: the user's input, the model's thoughts with their signatures, its
// text and function calls, and the results. The stream is server-sent
// events, each with an event_type; a complete one ends with
// interaction.completed.
package gemini

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// New builds the backend for c. The key goes in the x-goog-api-key header,
// never in the URL.
func New(c provider.Connection) (provider.Provider, error) {
	g, err := provider.NewGuard(c, "x-goog-api-key")
	if err != nil {
		return nil, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	return &backend{name: c.Name, base: strings.TrimSuffix(c.BaseURL, "/"), key: c.Key, guard: g, http: hc, low: map[string]int{}}, nil
}

type backend struct {
	name, base string
	key        secret.Value
	guard      *provider.Guard
	http       *http.Client

	mu  sync.Mutex
	low map[string]int // model → index in lowLevels of the lowest level it takes
}

// lowLevels are the thinking levels tried, lowest first, when thinking is
// off. Not every model has every level, and the API says so with a 400;
// the next level is then tried and remembered for the model. Past the
// last one, no level is sent and the model uses its default.
var lowLevels = []string{"minimal", "low"}

// do sends a request through the Guard. An error answer is returned as an
// *provider.Error.
func (b *backend) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
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
		req.Header.Set("x-goog-api-key", k)
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

// Models lists the models that generate content, with their limits.
func (b *backend) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	var out []provider.ModelInfo
	token := ""
	for {
		path := "/models?pageSize=1000"
		if token != "" {
			path += "&pageToken=" + url.QueryEscape(token)
		}
		resp, err := b.do(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Models []struct {
				Name             string   `json:"name"`
				DisplayName      string   `json:"displayName"`
				InputTokenLimit  int      `json:"inputTokenLimit"`
				OutputTokenLimit int      `json:"outputTokenLimit"`
				Methods          []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, provider.TransportError(b.name, "an unreadable model list", err)
		}
		for _, m := range page.Models {
			if !slices.Contains(m.Methods, "generateContent") {
				continue // embeddings and the like
			}
			out = append(out, provider.ModelInfo{ID: strings.TrimPrefix(m.Name, "models/"), Name: m.DisplayName,
				Context: m.InputTokenLimit, Output: m.OutputTokenLimit})
		}
		if page.NextPageToken == "" || page.NextPageToken == token {
			return out, nil
		}
		token = page.NextPageToken
	}
}

type request struct {
	Model             string    `json:"model"`
	Input             []step    `json:"input"`
	SystemInstruction string    `json:"system_instruction,omitempty"`
	Tools             []tool    `json:"tools,omitempty"`
	GenerationConfig  genConfig `json:"generation_config"`
	Store             bool      `json:"store"`
	Stream            bool      `json:"stream"`
}

type genConfig struct {
	MaxOutputTokens   int    `json:"max_output_tokens,omitempty"`
	ThinkingLevel     string `json:"thinking_level,omitempty"`
	ThinkingSummaries string `json:"thinking_summaries,omitempty"`
}

type tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// step is one step of the history or of the answer. Type says which
// fields are used.
type step struct {
	Type      string          `json:"type"`
	Content   []content       `json:"content,omitempty"`   // user_input, model_output
	Signature string          `json:"signature,omitempty"` // thought, function_call
	Summary   []content       `json:"summary,omitempty"`   // thought
	ID        string          `json:"id,omitempty"`        // function_call
	Name      string          `json:"name,omitempty"`      // function_call, function_result
	Arguments json.RawMessage `json:"arguments,omitempty"` // function_call: an object
	CallID    string          `json:"call_id,omitempty"`   // function_result
	Result    any             `json:"result,omitempty"`    // function_result: a string, or text and images
	IsError   bool            `json:"is_error,omitempty"`  // function_result
}

type content struct {
	Type     string `json:"type"` // text, image
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// event is the part of a stream event used here.
type event struct {
	EventType string `json:"event_type"`
	Index     int    `json:"index"`
	Step      step   `json:"step"`
	Delta     struct {
		Type      string  `json:"type"`
		Text      string  `json:"text"`
		Signature string  `json:"signature"`
		Arguments string  `json:"arguments"`
		Content   content `json:"content"`
	} `json:"delta"`
	Interaction struct {
		Status string     `json:"status"`
		Usage  *usage     `json:"usage"`
		Errors []apiError `json:"errors"`
	} `json:"interaction"`
	Error *apiError `json:"error"`
}

type usage struct {
	Input   int `json:"total_input_tokens"`
	Cached  int `json:"total_cached_tokens"`
	Output  int `json:"total_output_tokens"`
	Thought int `json:"total_thought_tokens"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// signature is what a Thinking part's Signature and a ToolCall's Extra
// hold for Gemini: the same shape as the extra_content that Gemini's
// OpenAI-compatible API sends, so a chat can move between the two. Other
// providers skip it: it is JSON without OpenAI's id.
type signature struct {
	Google struct {
		ThoughtSignature string `json:"thought_signature"`
	} `json:"google"`
}

func signatureJSON(sig string) string {
	if sig == "" {
		return ""
	}
	var s signature
	s.Google.ThoughtSignature = sig
	b, _ := json.Marshal(s)
	return string(b)
}

// ownSignature reads a signature written by signatureJSON; "" for none or
// another provider's.
func ownSignature(raw string) string {
	var s signature
	if raw == "" || json.Unmarshal([]byte(raw), &s) != nil {
		return ""
	}
	return s.Google.ThoughtSignature
}

// open is the step being streamed.
type open struct {
	step
	text, args strings.Builder
}

func (b *backend) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		body, err := b.body(ctx, req)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		resp, err := b.send(ctx, req, body)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		defer resp.Body.Close()

		var cur *open
		calls := false
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			data, ok := bytes.CutPrefix(sc.Bytes(), []byte("data:"))
			data = bytes.TrimSpace(data)
			if !ok || len(data) == 0 || string(data) == "[DONE]" {
				continue
			}
			var ev event
			if err := json.Unmarshal(data, &ev); err != nil {
				yield(provider.Event{}, provider.TransportError(b.name, "an unreadable stream event", err))
				return
			}
			switch ev.EventType {
			case "step.start":
				cur = &open{step: ev.Step}
			case "step.delta":
				if cur == nil {
					continue
				}
				switch ev.Delta.Type {
				case "text":
					cur.text.WriteString(ev.Delta.Text)
					if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: ev.Delta.Text}, nil) {
						return
					}
				case "thought_summary":
					t := ev.Delta.Content.Text
					cur.text.WriteString(t)
					if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartThinking, Text: t}, nil) {
						return
					}
				case "thought_signature":
					cur.Signature = ev.Delta.Signature
				case "arguments_delta":
					cur.args.WriteString(ev.Delta.Arguments)
				}
			case "step.stop":
				if cur == nil {
					continue
				}
				p := b.part(cur)
				cur = nil
				if p == nil {
					continue
				}
				calls = calls || p.ToolCall != nil
				if !yield(provider.Event{Kind: provider.EventPart, Part: p}, nil) {
					return
				}
			case "interaction.completed":
				done, err := b.done(ev, calls)
				yield(done, err)
				return
			case "error":
				e := apiError{}
				if ev.Error != nil {
					e = *ev.Error
				}
				yield(provider.Event{}, b.failed(e.Code, e.Message))
				return
			}
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			yield(provider.Event{}, provider.TransportError(b.name, b.guard.Redact(err.Error()), err))
			return
		}
		if err := ctx.Err(); err != nil {
			yield(provider.Event{}, provider.TransportError(b.name, "cancelled", err))
			return
		}
		yield(provider.Event{}, provider.TransportError(b.name, "the stream ended without interaction.completed", provider.ErrCutOff))
	}
}

// send posts the request. With thinking off, a thinking level the model
// rejects is replaced by the next one, which is kept for the model.
func (b *backend) send(ctx context.Context, req provider.Request, body *request) (*http.Response, error) {
	for {
		resp, err := b.do(ctx, "POST", "/interactions", body)
		if err == nil || body.GenerationConfig.ThinkingLevel == "" || !levelRejected(err) {
			return resp, err
		}
		b.mu.Lock()
		b.low[req.Model] = max(b.low[req.Model], slices.Index(lowLevels, body.GenerationConfig.ThinkingLevel)+1)
		body.GenerationConfig.ThinkingLevel = level(b.low[req.Model])
		b.mu.Unlock()
	}
}

func levelRejected(err error) bool {
	var pe *provider.Error
	return errors.As(err, &pe) && pe.Status == http.StatusBadRequest && strings.Contains(strings.ToLower(pe.Message), "thinking level")
}

func level(i int) string {
	if i < len(lowLevels) {
		return lowLevels[i]
	}
	return ""
}

// part turns a finished step into a part; nil for a step without one.
func (b *backend) part(o *open) *chat.Part {
	switch o.Type {
	case "thought":
		if o.Signature == "" && o.text.Len() == 0 {
			return nil
		}
		return &chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: o.text.String(), Signature: signatureJSON(o.Signature)}}
	case "model_output":
		if o.text.Len() == 0 {
			return nil
		}
		return &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: o.text.String()}}
	case "function_call":
		args := o.args.String()
		if args == "" && len(o.Arguments) > 0 && string(o.Arguments) != "null" {
			args = string(o.Arguments) // sent whole in step.start
		}
		id := o.ID
		if id == "" {
			id = newCallID()
		}
		return &chat.Part{Kind: chat.PartToolCall, ToolCall: provider.ToolCall(id, o.Name, args, signatureJSON(o.Signature))}
	}
	return nil
}

func (b *backend) done(ev event, calls bool) (provider.Event, error) {
	in := ev.Interaction
	stop := provider.StopEnd
	switch in.Status {
	case "completed", "requires_action":
		if calls {
			stop = provider.StopToolUse
		}
	case "incomplete": // also with calls: the last one may be cut off
		stop = provider.StopMaxTokens
	case "failed":
		e := apiError{Code: "failed", Message: "the interaction failed"}
		if len(in.Errors) > 0 {
			e = in.Errors[0]
		}
		return provider.Event{}, b.failed(e.Code, e.Message)
	default:
		return provider.Event{}, provider.TransportError(b.name, fmt.Sprintf("the interaction ended as %q", in.Status), provider.ErrCutOff)
	}
	u := &chat.Usage{}
	if r := in.Usage; r != nil {
		// Thought tokens are billed as output but counted apart.
		*u = chat.Usage{Input: r.Input - r.Cached, Output: r.Output + r.Thought, CacheRead: r.Cached}
	}
	return provider.Event{Kind: provider.EventDone, Stop: stop, Usage: u}, nil
}

// failed maps an error sent inside the stream, which has a code but no
// HTTP status.
func (b *backend) failed(code, msg string) *provider.Error {
	status := http.StatusBadRequest
	switch code {
	case "resource_exhausted", "rate_limit_exceeded", "too_many_requests":
		status = http.StatusTooManyRequests
	case "unavailable", "internal", "overloaded", "deadline_exceeded", "failed":
		status = http.StatusServiceUnavailable
	}
	if code != "" {
		msg = code + ": " + msg
	}
	return provider.Classify(b.name, status, nil, b.guard.Redact(msg))
}

func newCallID() string {
	var b [8]byte
	rand.Read(b[:])
	return "call_" + hex.EncodeToString(b[:])
}

func (b *backend) body(ctx context.Context, req provider.Request) (*request, error) {
	r := &request{Model: req.Model, Store: false, Stream: true,
		GenerationConfig: genConfig{MaxOutputTokens: req.MaxTokens, ThinkingSummaries: "none"}}
	if req.Thinking {
		r.GenerationConfig.ThinkingSummaries = "auto"
	} else {
		b.mu.Lock()
		r.GenerationConfig.ThinkingLevel = level(b.low[req.Model])
		b.mu.Unlock()
	}
	var sys []string
	for _, s := range req.System {
		sys = append(sys, s.Text)
	}
	r.SystemInstruction = strings.Join(sys, "\n\n")
	for _, t := range req.Tools {
		r.Tools = append(r.Tools, tool{Type: "function", Name: t.Name, Description: t.Description, Parameters: t.Schema})
	}
	names := map[string]string{} // call ID → tool name, for function_result
	for _, m := range req.Messages {
		switch m.Role {
		case chat.RoleUser:
			c, err := userContent(ctx, req, m.Parts)
			if err != nil {
				return nil, err
			}
			if len(c) > 0 {
				r.Input = append(r.Input, step{Type: "user_input", Content: c})
			}
		case chat.RoleAssistant:
			for _, p := range m.Parts {
				switch {
				case p.Thinking != nil:
					sig := ownSignature(p.Thinking.Signature)
					if sig == "" {
						continue // thinking from another provider, or none
					}
					s := step{Type: "thought", Signature: sig}
					if p.Thinking.Text != "" {
						s.Summary = []content{{Type: "text", Text: p.Thinking.Text}}
					}
					r.Input = append(r.Input, s)
				case p.Text != nil && p.Text.Text != "":
					r.Input = append(r.Input, step{Type: "model_output", Content: []content{{Type: "text", Text: p.Text.Text}}})
				case p.ToolCall != nil:
					c := p.ToolCall
					names[c.ID] = c.Name
					args := c.Args
					if !isObject(args) {
						args = json.RawMessage(`{}`)
					}
					r.Input = append(r.Input, step{Type: "function_call", ID: c.ID, Name: c.Name, Arguments: args, Signature: ownSignature(c.Extra)})
				}
			}
		case chat.RoleTool:
			// A tool's images follow its result; they go into that result.
			for i := 0; i < len(m.Parts); i++ {
				res := m.Parts[i].ToolResult
				if res == nil {
					continue
				}
				s := step{Type: "function_result", CallID: res.CallID, Name: names[res.CallID], IsError: res.IsError, Result: res.Text}
				var subs []content
				for i+1 < len(m.Parts) && m.Parts[i+1].Image != nil {
					i++
					img, err := image(ctx, req, m.Parts[i].Image)
					if err != nil {
						return nil, err
					}
					subs = append(subs, img)
				}
				if len(subs) > 0 {
					s.Result = append([]content{{Type: "text", Text: res.Text}}, subs...)
				}
				r.Input = append(r.Input, s)
			}
		}
	}
	return r, nil
}

func isObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{' && json.Valid(t)
}

func userContent(ctx context.Context, req provider.Request, ps []chat.Part) ([]content, error) {
	var out []content
	for _, p := range ps {
		switch {
		case p.Text != nil && strings.TrimSpace(p.Text.Text) != "":
			out = append(out, content{Type: "text", Text: p.Text.Text})
		case p.Notice != nil:
			out = append(out, content{Type: "text", Text: p.Notice.Text})
		case p.Image != nil:
			img, err := image(ctx, req, p.Image)
			if err != nil {
				return nil, err
			}
			out = append(out, img)
		}
	}
	return out, nil
}

func image(ctx context.Context, req provider.Request, img *chat.Image) (content, error) {
	if req.Image == nil {
		return content{}, fmt.Errorf("image %s: no image reader", img.Ref)
	}
	data, err := req.Image(ctx, img.Ref)
	if err != nil {
		return content{}, fmt.Errorf("image %s: %w", img.Ref, err)
	}
	return content{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: img.MIME}, nil
}
