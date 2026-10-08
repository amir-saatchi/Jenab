// Package openai is the backend for the APIs the OpenAI SDK speaks: OpenAI
// itself through the Responses API, and Gemini and any OpenAI-compatible
// provider through Chat Completions (SPEC 3.8).
package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// New builds the backend for c: the Responses API for OpenAI, Chat
// Completions for the other kinds.
func New(c provider.Connection) (provider.Provider, error) {
	g, err := provider.NewGuard(c)
	if err != nil {
		return nil, err
	}
	opts := []option.RequestOption{
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
	b := base{name: c.Name, kind: c.Kind, guard: g, client: &client}
	if c.Kind == provider.KindOpenAI {
		return &responsesAPI{b}, nil
	}
	return &chatAPI{b}, nil
}

type base struct {
	name   string
	kind   provider.Kind
	guard  *provider.Guard
	client *sdk.Client
}

// toolCallsOnly is the content of an assistant message with only tool
// calls (SPEC 3.8).
const toolCallsOnly = " "

// Models lists the models the key can use. Gemini's IDs come as
// "models/<id>".
func (b *base) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	ctx, capt := provider.WithCapture(ctx)
	pager := b.client.Models.ListAutoPaging(ctx)
	var out []provider.ModelInfo
	for pager.Next() {
		m := pager.Current()
		out = append(out, provider.ModelInfo{ID: strings.TrimPrefix(m.ID, "models/")})
	}
	if err := pager.Err(); err != nil {
		return nil, b.guard.Fail(b.name, capt, err)
	}
	return out, nil
}

// chatAPI speaks Chat Completions.
type chatAPI struct{ base }

func (a *chatAPI) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		params, err := a.params(ctx, req)
		if err != nil {
			yield(provider.Event{}, err)
			return
		}
		ctx, capt := provider.WithCapture(ctx)
		st := a.client.Chat.Completions.NewStreaming(ctx, params)
		defer st.Close()

		// Tool calls are keyed by index, but a new id at a used index
		// starts a new call (Gemini, SPIKE-018).
		type call struct {
			id, name, extra string
			args            strings.Builder
		}
		var (
			calls          []*call
			slot           = map[int64]int{}
			text, thinking strings.Builder
			finish         string
			usage          *chat.Usage
		)
		for st.Next() {
			ch := st.Current()
			if ch.JSON.Usage.Valid() && (ch.Usage.PromptTokens > 0 || ch.Usage.CompletionTokens > 0) {
				cached := int(ch.Usage.PromptTokensDetails.CachedTokens)
				usage = &chat.Usage{Input: int(ch.Usage.PromptTokens) - cached, Output: int(ch.Usage.CompletionTokens), CacheRead: cached}
			}
			if len(ch.Choices) == 0 {
				continue
			}
			c := ch.Choices[0]
			for _, f := range []string{"reasoning_content", "reasoning"} {
				if s := extraString(c.Delta.JSON.ExtraFields, f); s != "" {
					thinking.WriteString(s)
					if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartThinking, Text: s}, nil) {
						return
					}
					break
				}
			}
			if s := c.Delta.Content; s != "" {
				text.WriteString(s)
				if !yield(provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: s}, nil) {
					return
				}
			}
			for _, d := range c.Delta.ToolCalls {
				i, ok := slot[d.Index]
				if ok && d.ID != "" && calls[i].id != "" && d.ID != calls[i].id {
					ok = false
				}
				if !ok {
					calls = append(calls, &call{id: d.ID})
					i = len(calls) - 1
					slot[d.Index] = i
				}
				cl := calls[i]
				if cl.id == "" {
					cl.id = d.ID
				}
				if cl.name == "" {
					cl.name = d.Function.Name
				}
				cl.args.WriteString(d.Function.Arguments)
				if x, ok := d.JSON.ExtraFields["extra_content"]; ok && x.Raw() != "" && x.Raw() != "null" {
					cl.extra = x.Raw() // Gemini's thought signature, sent back unchanged
				}
			}
			if c.FinishReason != "" {
				finish = c.FinishReason
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
		if finish == "" {
			yield(provider.Event{}, provider.TransportError(a.name, "the stream ended without a finish_reason", provider.ErrCutOff))
			return
		}

		var parts []chat.Part
		if thinking.Len() > 0 {
			parts = append(parts, chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: thinking.String()}})
		}
		if text.Len() > 0 {
			parts = append(parts, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: text.String()}})
		}
		for i, c := range calls {
			if c.id == "" {
				c.id = fmt.Sprintf("call_%d", i)
			}
			parts = append(parts, chat.Part{Kind: chat.PartToolCall, ToolCall: provider.ToolCall(c.id, c.name, c.args.String(), c.extra)})
		}
		for i := range parts {
			if !yield(provider.Event{Kind: provider.EventPart, Part: &parts[i]}, nil) {
				return
			}
		}
		if usage == nil {
			usage = &chat.Usage{}
		}
		yield(provider.Event{Kind: provider.EventDone, Usage: usage, Stop: stopOf(finish, len(calls) > 0)}, nil)
	}
}

func stopOf(finish string, calls bool) provider.StopReason {
	switch {
	case finish == "length": // also with calls: the last one may be cut off
		return provider.StopMaxTokens
	case calls || finish == "tool_calls" || finish == "function_call":
		return provider.StopToolUse
	case finish == "content_filter":
		return provider.StopRefused
	}
	return provider.StopEnd
}

func (a *chatAPI) params(ctx context.Context, req provider.Request) (sdk.ChatCompletionNewParams, error) {
	p := sdk.ChatCompletionNewParams{
		Model:         shared.ChatModel(req.Model),
		StreamOptions: sdk.ChatCompletionStreamOptionsParam{IncludeUsage: sdk.Bool(true)},
	}
	if s := systemText(req.System); s != "" {
		p.Messages = append(p.Messages, sdk.SystemMessage(s))
	}
	msgs, err := a.messages(ctx, req)
	if err != nil {
		return p, err
	}
	p.Messages = append(p.Messages, msgs...)
	for _, t := range req.Tools {
		var schema shared.FunctionParameters
		if err := json.Unmarshal(t.Schema, &schema); err != nil {
			return p, fmt.Errorf("tool %s: schema: %w", t.Name, err)
		}
		p.Tools = append(p.Tools, sdk.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: sdk.String(t.Description), Parameters: schema,
		}))
	}
	if req.MaxTokens > 0 {
		p.MaxCompletionTokens = sdk.Int(int64(req.MaxTokens))
	}
	return p, nil
}

func systemText(bs []provider.Block) string {
	var parts []string
	for _, b := range bs {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// messages converts the chat. Tool messages take text only, so a tool
// message's images follow in one user message (SPEC 3.8). Thinking is not
// sent back: Chat Completions has no field for it.
func (a *chatAPI) messages(ctx context.Context, req provider.Request) ([]sdk.ChatCompletionMessageParamUnion, error) {
	var out []sdk.ChatCompletionMessageParamUnion
	for _, m := range req.Messages {
		switch m.Role {
		case chat.RoleUser:
			parts, err := userParts(ctx, req, m.Parts)
			if err != nil {
				return nil, err
			}
			if len(parts) > 0 {
				out = append(out, sdk.UserMessage(parts))
			}
		case chat.RoleAssistant:
			am := sdk.ChatCompletionAssistantMessageParam{}
			var text strings.Builder
			for _, p := range m.Parts {
				switch {
				case p.Text != nil:
					text.WriteString(p.Text.Text)
				case p.ToolCall != nil:
					fc := &sdk.ChatCompletionMessageFunctionToolCallParam{
						ID:       p.ToolCall.ID,
						Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{Name: p.ToolCall.Name, Arguments: string(p.ToolCall.Args)},
					}
					if p.ToolCall.Extra != "" {
						fc.SetExtraFields(map[string]any{"extra_content": json.RawMessage(p.ToolCall.Extra)})
					}
					am.ToolCalls = append(am.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{OfFunction: fc})
				}
			}
			if text.Len() > 0 || len(am.ToolCalls) > 0 {
				if text.Len() > 0 {
					am.Content.OfString = sdk.String(text.String())
				} else {
					// Tool calls only: a single space, the same for every
					// provider. Every empty form breaks one of them:
					// Cloudflare Workers AI refuses a missing content, Z.ai
					// refuses "" and null (with a 429 "overloaded"), and
					// Ollama turns [] into no message, so the model loses
					// every call after the first and repeats it.
					am.Content.OfString = sdk.String(toolCallsOnly)
				}
				out = append(out, sdk.ChatCompletionMessageParamUnion{OfAssistant: &am})
			}
		case chat.RoleTool:
			var images []chat.Part
			for _, p := range m.Parts {
				switch {
				case p.ToolResult != nil:
					out = append(out, sdk.ToolMessage(p.ToolResult.Text, p.ToolResult.CallID))
				case p.Image != nil:
					images = append(images, p)
				}
			}
			if len(images) > 0 {
				parts, err := userParts(ctx, req, images)
				if err != nil {
					return nil, err
				}
				out = append(out, sdk.UserMessage(parts))
			}
		}
	}
	return out, nil
}

// userParts converts text, notices and images; other parts have no shape
// in the API and are left to the caller.
func userParts(ctx context.Context, req provider.Request, ps []chat.Part) ([]sdk.ChatCompletionContentPartUnionParam, error) {
	var out []sdk.ChatCompletionContentPartUnionParam
	for _, p := range ps {
		switch {
		case p.Text != nil:
			out = append(out, sdk.TextContentPart(p.Text.Text))
		case p.Notice != nil:
			out = append(out, sdk.TextContentPart(p.Notice.Text))
		case p.Image != nil:
			url, err := dataURL(ctx, req, p.Image)
			if err != nil {
				return nil, err
			}
			out = append(out, sdk.ImageContentPart(sdk.ChatCompletionContentPartImageImageURLParam{URL: url}))
		}
	}
	return out, nil
}

func dataURL(ctx context.Context, req provider.Request, img *chat.Image) (string, error) {
	if req.Image == nil {
		return "", fmt.Errorf("image %s: no image reader", img.Ref)
	}
	b, err := req.Image(ctx, img.Ref)
	if err != nil {
		return "", fmt.Errorf("image %s: %w", img.Ref, err)
	}
	return "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// extraString reads a string field the SDK doesn't know, e.g. reasoning.
func extraString(fields map[string]respjson.Field, name string) string {
	f, ok := fields[name]
	if !ok || f.Raw() == "" || f.Raw() == "null" {
		return ""
	}
	var s string
	if json.Unmarshal([]byte(f.Raw()), &s) != nil {
		return ""
	}
	return s
}
