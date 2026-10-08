// Package anthropic is the backend for Anthropic's Messages API (SPEC 3.8).
//
// The request body is built here rather than with the SDK's param types,
// so its shape is plain to read; the SDK still sends it and reads the
// stream.
package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// defaultMaxTokens is used when the request sets none; the API needs one.
// Thinking counts toward it. A starting value.
const defaultMaxTokens = 32000

// New builds the backend for c.
func New(c provider.Connection) (provider.Provider, error) {
	g, err := provider.NewGuard(c, "x-api-key", "anthropic-version")
	if err != nil {
		return nil, err
	}
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(), // no ANTHROPIC_* variables
		option.WithBaseURL(c.BaseURL),
		option.WithAPIKey(c.Key.Reveal()),
		option.WithMaxRetries(0), // the caller retries (SPEC 3.8)
		option.WithMiddleware(func(r *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			return g.Do(r, next)
		}),
	}
	if c.HTTP != nil {
		opts = append(opts, option.WithHTTPClient(c.HTTP))
	}
	client := sdk.NewClient(opts...)
	return &backend{name: c.Name, guard: g, client: &client}, nil
}

type backend struct {
	name   string
	guard  *provider.Guard
	client *sdk.Client
}

// Models lists the models with their limits.
func (b *backend) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	ctx, capt := provider.WithCapture(ctx)
	pager := b.client.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	var out []provider.ModelInfo
	for pager.Next() {
		m := pager.Current()
		out = append(out, provider.ModelInfo{ID: m.ID, Name: m.DisplayName, Context: int(m.MaxInputTokens), Output: int(m.MaxTokens)})
	}
	if err := pager.Err(); err != nil {
		return nil, b.guard.Fail(b.name, capt, err)
	}
	return out, nil
}

// The request body.
type request struct {
	Model        string       `json:"model"`
	MaxTokens    int          `json:"max_tokens"`
	System       []block      `json:"system,omitempty"`
	Messages     []message    `json:"messages"`
	Tools        []tool       `json:"tools,omitempty"`
	Thinking     *thinking    `json:"thinking,omitempty"`
	CacheControl *cacheMarker `json:"cache_control,omitempty"` // automatic cache point at the end (SPEC 3.1)
}

type thinking struct {
	Type         string `json:"type"` // adaptive or enabled
	Display      string `json:"display,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type cacheMarker struct {
	Type string `json:"type"`
}

var ephemeral = &cacheMarker{Type: "ephemeral"}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

// block is any content block.
type block struct {
	Type         string       `json:"type"`
	Text         string       `json:"text,omitempty"`
	CacheControl *cacheMarker `json:"cache_control,omitempty"`
	// thinking and redacted_thinking; thinking is sent even when empty
	Thinking  *string `json:"thinking,omitempty"`
	Signature string  `json:"signature,omitempty"`
	Data      string  `json:"data,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string  `json:"tool_use_id,omitempty"`
	Content   []block `json:"content,omitempty"`
	IsError   bool    `json:"is_error,omitempty"`
	// image
	Source *source `json:"source,omitempty"`
}

type source struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// event is the part of a stream event used here.
type event struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		Usage usage `json:"usage"`
	} `json:"message"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Data string `json:"data"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *usage `json:"usage"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type usage struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

// add takes the fields u reports; message_delta repeats some of them.
func (u usage) add(to *chat.Usage) {
	if u.InputTokens != nil {
		to.Input = *u.InputTokens
	}
	if u.OutputTokens != nil {
		to.Output = *u.OutputTokens
	}
	if u.CacheCreationInputTokens != nil {
		to.CacheWrite = *u.CacheCreationInputTokens
	}
	if u.CacheReadInputTokens != nil {
		to.CacheRead = *u.CacheReadInputTokens
	}
}

// open is a content block being streamed.
type open struct {
	kind                  string
	text, sig, args, data strings.Builder
	id, name              string
}

func (b *backend) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		body, err := b.body(ctx, req)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		ctx, capt := provider.WithCapture(ctx)
		st := b.client.Messages.NewStreaming(ctx, sdk.MessageNewParams{}, option.WithRequestBody("application/json", body))
		defer st.Close()

		blocks := map[int]*open{}
		u := chat.Usage{}
		stop := ""
		for st.Next() {
			var ev event
			if err := json.Unmarshal([]byte(st.Current().RawJSON()), &ev); err != nil {
				yield(provider.Event{}, provider.TransportError(b.name, "an unreadable stream event", err))
				return
			}
			switch ev.Type {
			case "message_start":
				ev.Message.Usage.add(&u)
			case "content_block_start":
				o := &open{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				o.data.WriteString(ev.ContentBlock.Data)
				blocks[ev.Index] = o
			case "content_block_delta":
				o := blocks[ev.Index]
				if o == nil {
					continue
				}
				switch ev.Delta.Type {
				case "text_delta":
					o.text.WriteString(ev.Delta.Text)
					if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: ev.Delta.Text}, nil) {
						return
					}
				case "thinking_delta":
					o.text.WriteString(ev.Delta.Thinking)
					if ev.Delta.Thinking != "" && !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartThinking, Text: ev.Delta.Thinking}, nil) {
						return
					}
				case "signature_delta":
					o.sig.WriteString(ev.Delta.Signature)
				case "input_json_delta":
					o.args.WriteString(ev.Delta.PartialJSON)
				}
			case "content_block_stop":
				o := blocks[ev.Index]
				delete(blocks, ev.Index)
				if o == nil {
					continue
				}
				part, err := b.part(o)
				if err != nil {
					yield(provider.Event{}, err)
					return
				}
				if part != nil && !yield(provider.Event{Kind: provider.EventPart, Part: part}, nil) {
					return
				}
			case "message_delta":
				if ev.Delta.StopReason != "" {
					stop = ev.Delta.StopReason
				}
				if ev.Usage != nil {
					ev.Usage.add(&u)
				}
			case "message_stop":
				yield(provider.Event{Kind: provider.EventDone, Usage: &u, Stop: stopOf(stop)}, nil)
				return
			}
		}
		if err := st.Err(); err != nil {
			// The SDK ends the stream on an error event and returns it as
			// an *sdk.Error carrying the answer's 200 status.
			var ae *sdk.Error
			if errors.As(err, &ae) && ae.StatusCode < 400 {
				var e event
				json.Unmarshal([]byte(ae.RawJSON()), &e)
				yield(provider.Event{}, b.streamError(e.Error.Type, e.Error.Message))
				return
			}
			yield(provider.Event{}, b.guard.Fail(b.name, capt, err))
			return
		}
		if err := ctx.Err(); err != nil {
			yield(provider.Event{}, provider.TransportError(b.name, "cancelled", err))
			return
		}
		yield(provider.Event{}, provider.TransportError(b.name, "the stream ended without message_stop", provider.ErrCutOff))
	}
}

func (b *backend) part(o *open) (*chat.Part, error) {
	switch o.kind {
	case "text":
		if o.text.Len() == 0 {
			return nil, nil
		}
		return &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: o.text.String()}}, nil
	case "thinking":
		return &chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: o.text.String(), Signature: o.sig.String()}}, nil
	case "redacted_thinking":
		return &chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Redacted: o.data.String()}}, nil
	case "tool_use":
		return &chat.Part{Kind: chat.PartToolCall, ToolCall: provider.ToolCall(o.id, o.name, o.args.String(), "")}, nil
	}
	return nil, nil // server tools and other blocks Jenab doesn't use
}

func stopOf(s string) provider.StopReason {
	switch s {
	case "tool_use":
		return provider.StopToolUse
	case "max_tokens", "model_context_window_exceeded":
		return provider.StopMaxTokens
	case "refusal":
		return provider.StopRefused
	}
	return provider.StopEnd
}

// streamError maps an error event sent inside the stream.
func (b *backend) streamError(typ, msg string) *provider.Error {
	status := 400
	switch typ {
	case "overloaded_error":
		status = 529
	case "rate_limit_error":
		status = 429
	case "api_error":
		status = 500
	case "request_too_large":
		status = 413
	}
	return provider.Classify(b.name, status, nil, b.guard.Redact(typ+": "+msg))
}

func (b *backend) body(ctx context.Context, req provider.Request) ([]byte, error) {
	r := request{Model: req.Model, MaxTokens: req.MaxTokens, CacheControl: ephemeral}
	if r.MaxTokens <= 0 {
		r.MaxTokens = defaultMaxTokens
	}
	for _, s := range req.System {
		blk := block{Type: "text", Text: s.Text}
		if s.Cache {
			blk.CacheControl = ephemeral
		}
		r.System = append(r.System, blk)
	}
	for _, t := range req.Tools {
		r.Tools = append(r.Tools, tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}
	// Thinking: models the catalog knows get it on with its text shown.
	// Unknown models get the API's default, which never fails.
	if k := req.Known; req.Thinking && k != nil && k.Thinking {
		if k.ThinkingBudget {
			r.Thinking = &thinking{Type: "enabled", BudgetTokens: max(1024, r.MaxTokens/2)}
		} else {
			r.Thinking = &thinking{Type: "adaptive", Display: "summarized"}
		}
	}

	add := func(role string, bs ...block) {
		if len(bs) == 0 {
			return
		}
		// All tool results of a turn go in one user message (SPEC 3.8).
		if n := len(r.Messages); n > 0 && r.Messages[n-1].Role == role {
			r.Messages[n-1].Content = append(r.Messages[n-1].Content, bs...)
			return
		}
		r.Messages = append(r.Messages, message{Role: role, Content: bs})
	}
	for _, m := range req.Messages {
		var bs []block
		switch m.Role {
		case chat.RoleUser:
			for _, p := range m.Parts {
				blk, err := userBlock(ctx, req, p)
				if err != nil {
					return nil, err
				}
				if blk != nil {
					bs = append(bs, *blk)
				}
			}
			add("user", bs...)
		case chat.RoleAssistant:
			for _, p := range m.Parts {
				switch {
				case p.Thinking != nil && p.Thinking.Redacted != "":
					bs = append(bs, block{Type: "redacted_thinking", Data: p.Thinking.Redacted})
				case p.Thinking != nil:
					if !ownSignature(p.Thinking.Signature) {
						continue // thinking from another provider
					}
					t := p.Thinking.Text
					bs = append(bs, block{Type: "thinking", Thinking: &t, Signature: p.Thinking.Signature})
				case p.Text != nil && strings.TrimSpace(p.Text.Text) != "": // the API refuses blank text
					bs = append(bs, block{Type: "text", Text: p.Text.Text})
				case p.ToolCall != nil:
					bs = append(bs, block{Type: "tool_use", ID: p.ToolCall.ID, Name: p.ToolCall.Name, Input: p.ToolCall.Args})
				}
			}
			add("assistant", bs...)
		case chat.RoleTool:
			for _, p := range m.Parts {
				switch {
				case p.ToolResult != nil:
					res := block{Type: "tool_result", ToolUseID: p.ToolResult.CallID, IsError: p.ToolResult.IsError}
					if strings.TrimSpace(p.ToolResult.Text) != "" {
						res.Content = []block{{Type: "text", Text: p.ToolResult.Text}}
					}
					bs = append(bs, res)
				case p.Image != nil && len(bs) > 0:
					// A tool's image goes inside its result.
					img, err := imageBlock(ctx, req, p.Image)
					if err != nil {
						return nil, err
					}
					last := &bs[len(bs)-1]
					last.Content = append(last.Content, img)
				}
			}
			add("user", bs...)
		}
	}
	if r.Messages == nil {
		r.Messages = []message{}
	}
	return json.Marshal(r)
}

// ownSignature tells an Anthropic thinking signature from another
// provider's data (OpenAI's is JSON) or none.
func ownSignature(sig string) bool {
	return sig != "" && !json.Valid([]byte(sig))
}

func userBlock(ctx context.Context, req provider.Request, p chat.Part) (*block, error) {
	switch {
	case p.Text != nil && strings.TrimSpace(p.Text.Text) != "":
		return &block{Type: "text", Text: p.Text.Text}, nil
	case p.Notice != nil:
		return &block{Type: "text", Text: p.Notice.Text}, nil
	case p.Image != nil:
		img, err := imageBlock(ctx, req, p.Image)
		return &img, err
	}
	return nil, nil
}

func imageBlock(ctx context.Context, req provider.Request, img *chat.Image) (block, error) {
	if req.Image == nil {
		return block{}, fmt.Errorf("image %s: no image reader", img.Ref)
	}
	data, err := req.Image(ctx, img.Ref)
	if err != nil {
		return block{}, fmt.Errorf("image %s: %w", img.Ref, err)
	}
	return block{Type: "image", Source: &source{Type: "base64", MediaType: img.MIME, Data: base64.StdEncoding.EncodeToString(data)}}, nil
}
