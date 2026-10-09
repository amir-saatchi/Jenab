package openai

import (
	"context"
	"encoding/json"
	"iter"

	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// responsesAPI speaks OpenAI's Responses API, the only one through which
// current OpenAI models call tools. Nothing is stored at OpenAI
// (store: false): the reasoning comes back encrypted and is sent back with
// its tool calls, like Anthropic's thinking signatures.
//
// The request body is built here rather than with the SDK's param types,
// so its shape is plain to read; the SDK still sends it and reads the
// stream.
type responsesAPI struct{ base }

// The request body.
type rRequest struct {
	Model           string     `json:"model"`
	Instructions    string     `json:"instructions,omitempty"`
	Input           []rItem    `json:"input"`
	Tools           []rTool    `json:"tools,omitempty"`
	MaxOutputTokens int        `json:"max_output_tokens,omitempty"`
	Store           bool       `json:"store"`
	Include         []string   `json:"include"`
	Reasoning       *rReasonCf `json:"reasoning,omitempty"`
}

type rReasonCf struct {
	Summary string `json:"summary,omitempty"`
}

type rTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

// rItem is an input or output item: a message, a reasoning item, a function
// call or a function call's output.
type rItem struct {
	Type string `json:"type"`
	// message
	Role    string     `json:"role,omitempty"`
	Content []rContent `json:"content,omitempty"`
	// reasoning
	ID               string      `json:"id,omitempty"`
	Summary          *[]rContent `json:"summary,omitempty"` // always sent for reasoning, even empty
	EncryptedContent string      `json:"encrypted_content,omitempty"`
	// function_call and function_call_output
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type rContent struct {
	Type     string `json:"type"` // input_text, input_image, output_text, refusal, summary_text
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Refusal  string `json:"refusal,omitempty"`
}

// reasoning is what a Thinking part's Signature holds for OpenAI.
type reasoning struct {
	ID               string `json:"id"`
	EncryptedContent string `json:"encrypted_content"`
}

// rEvent is the part of a stream event used here.
type rEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Item     rItem  `json:"item"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Response struct {
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"input_tokens_details"`
			OutputTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
}

func (a *responsesAPI) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		body, err := a.body(ctx, req)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		ctx, capt := provider.WithCapture(ctx)
		st := a.client.Responses.NewStreaming(ctx, responses.ResponseNewParams{}, option.WithRequestBody("application/json", body))
		defer st.Close()

		calls := false
		for st.Next() {
			var ev rEvent
			if err := json.Unmarshal([]byte(st.Current().RawJSON()), &ev); err != nil {
				yield(provider.Event{}, provider.TransportError(a.name, "an unreadable stream event", err))
				return
			}
			switch ev.Type {
			case "response.output_text.delta":
				if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: ev.Delta}, nil) {
					return
				}
			case "response.reasoning_summary_text.delta":
				if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartThinking, Text: ev.Delta}, nil) {
					return
				}
			case "response.output_item.done":
				part, err := a.part(ev.Item)
				if err != nil {
					yield(provider.Event{}, err)
					return
				}
				if part == nil {
					continue
				}
				calls = calls || part.ToolCall != nil
				if !yield(provider.Event{Kind: provider.EventPart, Part: part}, nil) {
					return
				}
			case "response.completed", "response.incomplete":
				yield(a.done(ev, calls), nil)
				return
			case "response.failed":
				code, msg := "", "the response failed"
				if e := ev.Response.Error; e != nil {
					code, msg = e.Code, e.Message
				}
				yield(provider.Event{}, a.failed(code, msg))
				return
			case "error":
				yield(provider.Event{}, a.failed(ev.Code, ev.Message))
				return
			}
		}
		if err := st.Err(); err != nil {
			yield(provider.Event{}, a.guard.Fail(a.name, capt, err))
			return
		}
		if err := ctx.Err(); err != nil {
			yield(provider.Event{}, provider.TransportError(a.name, "cancelled", err))
			return
		}
		yield(provider.Event{}, provider.TransportError(a.name, "the stream ended without response.completed", provider.ErrCutOff))
	}
}

// part converts a finished output item; nil for items Jenab doesn't keep.
func (a *responsesAPI) part(it rItem) (*chat.Part, error) {
	switch it.Type {
	case "message":
		var text string
		for _, c := range it.Content {
			switch c.Type {
			case "output_text":
				text += c.Text
			case "refusal":
				text += c.Refusal
			}
		}
		if text == "" {
			return nil, nil
		}
		return &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: text}}, nil
	case "reasoning":
		sig, _ := json.Marshal(reasoning{ID: it.ID, EncryptedContent: it.EncryptedContent})
		var text string
		for _, s := range deref(it.Summary) {
			text += s.Text
		}
		return &chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: text, Signature: string(sig)}}, nil
	case "function_call":
		return &chat.Part{Kind: chat.PartToolCall, ToolCall: provider.ToolCall(it.CallID, it.Name, it.Arguments, "")}, nil
	}
	return nil, nil
}

func (a *responsesAPI) done(ev rEvent, calls bool) provider.Event {
	stop := provider.StopEnd
	if calls {
		stop = provider.StopToolUse
	}
	if d := ev.Response.IncompleteDetails; d != nil {
		switch d.Reason {
		case "max_output_tokens":
			stop = provider.StopMaxTokens
		case "content_filter":
			stop = provider.StopRefused
		}
	}
	u := &chat.Usage{}
	if r := ev.Response.Usage; r != nil {
		d := r.InputTokensDetails
		u = &chat.Usage{Input: max(r.InputTokens-d.CachedTokens-d.CacheWriteTokens, 0), Output: r.OutputTokens, CacheRead: d.CachedTokens, CacheWrite: d.CacheWriteTokens,
			Thought: r.OutputTokensDetails.ReasoningTokens}
	}
	return provider.Event{Kind: provider.EventDone, Usage: u, Stop: stop}
}

// failed maps an error sent inside the stream, which has a code but no
// HTTP status.
func (a *responsesAPI) failed(code, msg string) *provider.Error {
	status := 400
	switch code {
	case "rate_limit_exceeded":
		status = 429
	case "server_error", "server_is_overloaded", "slow_down":
		status = 503
	case "insufficient_quota":
		status = 429
		msg = code + ": " + msg
	}
	return provider.Classify(a.name, status, nil, a.guard.Redact(msg))
}

func (a *responsesAPI) body(ctx context.Context, req provider.Request) ([]byte, error) {
	r := rRequest{
		Model:           req.Model,
		Instructions:    systemText(req.System),
		MaxOutputTokens: req.MaxTokens,
		Store:           false,
		Include:         []string{"reasoning.encrypted_content"},
	}
	for _, t := range req.Tools {
		r.Tools = append(r.Tools, rTool{Type: "function", Name: t.Name, Description: t.Description, Parameters: t.Schema})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case chat.RoleUser:
			c, err := inputContent(ctx, req, m.Parts)
			if err != nil {
				return nil, err
			}
			if len(c) > 0 {
				r.Input = append(r.Input, rItem{Type: "message", Role: "user", Content: c})
			}
		case chat.RoleAssistant:
			for _, p := range m.Parts {
				switch {
				case p.Thinking != nil && p.Thinking.Signature != "":
					var rs reasoning
					if json.Unmarshal([]byte(p.Thinking.Signature), &rs) != nil || rs.ID == "" {
						continue // thinking from another provider
					}
					summary := []rContent{}
					if p.Thinking.Text != "" {
						summary = append(summary, rContent{Type: "summary_text", Text: p.Thinking.Text})
					}
					it := rItem{Type: "reasoning", ID: rs.ID, EncryptedContent: rs.EncryptedContent, Summary: &summary}
					r.Input = append(r.Input, it)
				case p.Text != nil:
					r.Input = append(r.Input, rItem{Type: "message", Role: "assistant", Content: []rContent{{Type: "output_text", Text: p.Text.Text}}})
				case p.ToolCall != nil:
					r.Input = append(r.Input, rItem{Type: "function_call", CallID: p.ToolCall.ID, Name: p.ToolCall.Name, Arguments: string(p.ToolCall.Args)})
				}
			}
		case chat.RoleTool:
			var images []chat.Part
			for _, p := range m.Parts {
				switch {
				case p.ToolResult != nil:
					r.Input = append(r.Input, rItem{Type: "function_call_output", CallID: p.ToolResult.CallID, Output: p.ToolResult.Text})
				case p.Image != nil:
					images = append(images, p)
				}
			}
			if len(images) > 0 {
				c, err := inputContent(ctx, req, images)
				if err != nil {
					return nil, err
				}
				r.Input = append(r.Input, rItem{Type: "message", Role: "user", Content: c})
			}
		}
	}
	if r.Input == nil {
		r.Input = []rItem{}
	}
	return json.Marshal(r)
}

func inputContent(ctx context.Context, req provider.Request, ps []chat.Part) ([]rContent, error) {
	var out []rContent
	for _, p := range ps {
		switch {
		case p.Text != nil:
			out = append(out, rContent{Type: "input_text", Text: p.Text.Text})
		case p.Notice != nil:
			out = append(out, rContent{Type: "input_text", Text: p.Notice.Text})
		case p.Image != nil:
			url, err := dataURL(ctx, req, p.Image)
			if err != nil {
				return nil, err
			}
			out = append(out, rContent{Type: "input_image", ImageURL: url})
		}
	}
	return out, nil
}

func deref[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}
