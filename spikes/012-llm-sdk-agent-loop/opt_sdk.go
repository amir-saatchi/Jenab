package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	aoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	ooption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// Option 1: the official SDKs behind our own Provider interface. Everything in this file is code
// Burrow would own (about 300 lines for both providers).

var optSDK = Option{
	Name: "official SDKs", Module: "anthropic-sdk-go + openai-go/v3", Version: "v1.75.0 + v3.66.0", Released: "2026-09-22 + 2026-09-23", License: "MIT + Apache-2.0",
	New: func(o Options) (Provider, error) {
		if o.Kind == Anthropic {
			opts := []aoption.RequestOption{aoption.WithAPIKey(o.APIKey), aoption.WithBaseURL(o.BaseURL)}
			if o.MaxRetries >= 0 {
				opts = append(opts, aoption.WithMaxRetries(o.MaxRetries))
			}
			c := anthropic.NewClient(opts...)
			return &sdkAnth{c: &c}, nil
		}
		opts := []ooption.RequestOption{ooption.WithAPIKey(o.APIKey), ooption.WithBaseURL(o.BaseURL + "/v1")}
		if o.MaxRetries >= 0 {
			opts = append(opts, ooption.WithMaxRetries(o.MaxRetries))
		}
		c := openai.NewClient(opts...)
		return &sdkOAI{c: &c}, nil
	},
	Classify: func(err error) *ProviderError {
		var ae *anthropic.Error
		if errors.As(err, &ae) {
			pe := &ProviderError{Status: ae.StatusCode, Type: string(ae.Type()), Msg: firstLine(ae.Error())}
			if ae.Response != nil {
				pe.RetryAfter, pe.HasRetry = parseRetryAfter(ae.Response.Header.Get("retry-after"))
			}
			return pe
		}
		var oe *openai.Error
		if errors.As(err, &oe) {
			pe := &ProviderError{Status: oe.StatusCode, Type: oe.Type, Msg: oe.Message}
			if oe.Response != nil {
				pe.RetryAfter, pe.HasRetry = parseRetryAfter(oe.Response.Header.Get("retry-after"))
			}
			return pe
		}
		return nil
	},
}

// ---------------- Anthropic ----------------

type sdkAnth struct{ c *anthropic.Client }

func anthCache(on bool) anthropic.CacheControlEphemeralParam {
	if on {
		return anthropic.NewCacheControlEphemeralParam()
	}
	return anthropic.CacheControlEphemeralParam{}
}

func (p *sdkAnth) params(req *Request) anthropic.MessageNewParams {
	prm := anthropic.MessageNewParams{Model: anthropic.Model(req.Model), MaxTokens: int64(req.MaxTokens)}
	for _, s := range req.System {
		prm.System = append(prm.System, anthropic.TextBlockParam{Text: s.Text, CacheControl: anthCache(s.Cache)})
	}
	for _, t := range req.Tools {
		props, _ := t.Schema["properties"]
		var reqd []string
		if r, ok := t.Schema["required"].([]string); ok {
			reqd = r
		}
		prm.Tools = append(prm.Tools, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name: t.Name, Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: props, Required: reqd}}})
	}
	if req.ThinkingBudget > 0 {
		prm.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(req.ThinkingBudget))
	}
	for _, m := range req.Messages {
		var blocks []anthropic.ContentBlockParamUnion
		for _, pt := range m.Parts {
			cc := anthCache(pt.Cache)
			switch pt.Type {
			case PText:
				blocks = append(blocks, anthropic.ContentBlockParamUnion{OfText: &anthropic.TextBlockParam{Text: pt.Text, CacheControl: cc}})
			case PImage:
				b := anthropic.NewImageBlockBase64(pt.MIME, base64.StdEncoding.EncodeToString(pt.Data))
				b.OfImage.CacheControl = cc
				blocks = append(blocks, b)
			case PThinking:
				blocks = append(blocks, anthropic.NewThinkingBlock(pt.Signature, pt.Text))
			case PToolCall:
				blocks = append(blocks, anthropic.ContentBlockParamUnion{OfToolUse: &anthropic.ToolUseBlockParam{
					ID: pt.CallID, Name: pt.Name, Input: json.RawMessage(pt.Args), CacheControl: cc}})
			case PToolResult:
				tr := &anthropic.ToolResultBlockParam{ToolUseID: pt.CallID, CacheControl: cc}
				if pt.IsError {
					tr.IsError = anthropic.Bool(true)
				}
				for _, c := range pt.Content {
					if c.Type == PImage {
						tr.Content = append(tr.Content, anthropic.ToolResultBlockParamContentUnion{OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{OfBase64: &anthropic.Base64ImageSourceParam{
								MediaType: anthropic.Base64ImageSourceMediaType(c.MIME), Data: base64.StdEncoding.EncodeToString(c.Data)}}}})
					} else {
						tr.Content = append(tr.Content, anthropic.ToolResultBlockParamContentUnion{OfText: &anthropic.TextBlockParam{Text: c.Text}})
					}
				}
				blocks = append(blocks, anthropic.ContentBlockParamUnion{OfToolResult: tr})
			}
		}
		if m.Role == "assistant" {
			prm.Messages = append(prm.Messages, anthropic.NewAssistantMessage(blocks...))
		} else {
			prm.Messages = append(prm.Messages, anthropic.NewUserMessage(blocks...))
		}
	}
	return prm
}

func (p *sdkAnth) Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error) {
	stream := p.c.Messages.NewStreaming(ctx, p.params(req))
	defer stream.Close()
	var acc anthropic.Message
	var usage Usage
	var stop string
	sawStop := false
	kinds := map[int64]string{}
	for stream.Next() {
		ev := stream.Current()
		if err := acc.Accumulate(ev); err != nil {
			return nil, fmt.Errorf("accumulate: %w", err)
		}
		switch e := ev.AsAny().(type) {
		case anthropic.MessageStartEvent:
			u := e.Message.Usage
			usage = Usage{Input: int(u.InputTokens), Output: int(u.OutputTokens), CacheWrite: int(u.CacheCreationInputTokens), CacheRead: int(u.CacheReadInputTokens)}
		case anthropic.ContentBlockStartEvent:
			kinds[e.Index] = e.ContentBlock.Type
			if e.ContentBlock.Type == "tool_use" {
				on(Event{Kind: EvToolStart, Index: int(e.Index), CallID: e.ContentBlock.ID, Name: e.ContentBlock.Name})
			}
		case anthropic.ContentBlockDeltaEvent:
			switch d := e.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				on(Event{Kind: EvText, Index: int(e.Index), Text: d.Text})
			case anthropic.InputJSONDelta:
				on(Event{Kind: EvToolDelta, Index: int(e.Index), Text: d.PartialJSON})
			case anthropic.ThinkingDelta:
				on(Event{Kind: EvThinking, Index: int(e.Index), Text: d.Thinking})
			}
		case anthropic.ContentBlockStopEvent:
			if kinds[e.Index] == "tool_use" {
				on(Event{Kind: EvToolEnd, Index: int(e.Index)})
			}
		case anthropic.MessageDeltaEvent:
			stop = string(e.Delta.StopReason)
			usage.Output = int(e.Usage.OutputTokens)
			if e.Usage.JSON.CacheReadInputTokens.Valid() {
				usage.CacheRead = int(e.Usage.CacheReadInputTokens)
			}
			if e.Usage.JSON.CacheCreationInputTokens.Valid() {
				usage.CacheWrite = int(e.Usage.CacheCreationInputTokens)
			}
		case anthropic.MessageStopEvent:
			sawStop = true
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	// Rule: a stream that ends without message_stop is truncated. The SDK does not check this.
	if !sawStop && !sdkRawMode {
		return nil, errTruncated
	}
	on(Event{Kind: EvUsage, Usage: usage})
	on(Event{Kind: EvStop, Stop: stop})
	msg := Message{Role: "assistant"}
	for _, b := range acc.Content {
		switch v := b.AsAny().(type) {
		case anthropic.TextBlock:
			msg.Parts = append(msg.Parts, Part{Type: PText, Text: v.Text})
		case anthropic.ThinkingBlock:
			msg.Parts = append(msg.Parts, Part{Type: PThinking, Text: v.Thinking, Signature: v.Signature})
		case anthropic.ToolUseBlock:
			msg.Parts = append(msg.Parts, Part{Type: PToolCall, CallID: v.ID, Name: v.Name, Args: string(v.Input)})
		}
	}
	return &Response{Message: msg, Usage: usage, Stop: stop}, nil
}

// sdkRawMode turns off our end-of-stream check, to show what the SDK alone does (T7).
var sdkRawMode bool

var errTruncated = errors.New("stream ended before message_stop / finish_reason")

// ---------------- OpenAI Chat Completions ----------------

type sdkOAI struct{ c *openai.Client }

func (p *sdkOAI) params(req *Request) openai.ChatCompletionNewParams {
	prm := openai.ChatCompletionNewParams{Model: shared.ChatModel(req.Model),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}}
	if req.MaxTokens > 0 {
		prm.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}
	var sys []string
	for _, s := range req.System {
		sys = append(sys, s.Text)
	}
	if len(sys) > 0 {
		prm.Messages = append(prm.Messages, openai.SystemMessage(strings.Join(sys, "\n\n")))
	}
	for _, t := range req.Tools {
		prm.Tools = append(prm.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Description), Parameters: shared.FunctionParameters(t.Schema)}))
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			var parts []openai.ChatCompletionContentPartUnionParam
			for _, pt := range m.Parts {
				if pt.Type == PImage {
					parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: dataURL(pt)}))
				} else {
					parts = append(parts, openai.TextContentPart(pt.Text))
				}
			}
			prm.Messages = append(prm.Messages, openai.UserMessage(parts))
		case "assistant":
			var am openai.ChatCompletionAssistantMessageParam
			var text []string
			for _, pt := range m.Parts {
				switch pt.Type {
				case PText:
					text = append(text, pt.Text)
				case PToolCall:
					am.ToolCalls = append(am.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: pt.CallID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: pt.Name, Arguments: pt.Args}}})
				}
			}
			if len(text) > 0 {
				am.Content.OfString = openai.String(strings.Join(text, ""))
			}
			prm.Messages = append(prm.Messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &am})
		case "tool":
			// Chat Completions tool messages carry text only. Images go into one user message after
			// the tool messages (our rule, not the SDK's).
			var imgs []openai.ChatCompletionContentPartUnionParam
			for _, pt := range m.Parts {
				var txt []string
				for _, c := range pt.Content {
					if c.Type == PImage {
						txt = append(txt, fmt.Sprintf("[image %d attached below]", len(imgs)+1))
						imgs = append(imgs, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: dataURL(c)}))
					} else {
						txt = append(txt, c.Text)
					}
				}
				prm.Messages = append(prm.Messages, openai.ToolMessage(strings.Join(txt, "\n"), pt.CallID))
			}
			if len(imgs) > 0 {
				parts := append([]openai.ChatCompletionContentPartUnionParam{openai.TextContentPart("Images returned by the tool calls above:")}, imgs...)
				prm.Messages = append(prm.Messages, openai.UserMessage(parts))
			}
		}
	}
	return prm
}

func dataURL(p Part) string {
	return "data:" + p.MIME + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
}

func (p *sdkOAI) Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error) {
	stream := p.c.Chat.Completions.NewStreaming(ctx, p.params(req))
	defer stream.Close()
	type tc struct{ id, name, args string }
	var calls []*tc
	var text strings.Builder
	var usage Usage
	var finish string
	for stream.Next() {
		ch := stream.Current()
		if ch.JSON.Usage.Valid() && ch.Usage.TotalTokens > 0 {
			usage = Usage{Input: int(ch.Usage.PromptTokens), Output: int(ch.Usage.CompletionTokens), CacheRead: int(ch.Usage.PromptTokensDetails.CachedTokens)}
		}
		if len(ch.Choices) == 0 {
			continue
		}
		c := ch.Choices[0]
		if r, ok := c.Delta.JSON.ExtraFields["reasoning"]; ok && r.Raw() != "" && r.Raw() != "null" {
			var s string
			_ = json.Unmarshal([]byte(r.Raw()), &s)
			on(Event{Kind: EvThinking, Text: s})
		}
		if c.Delta.Content != "" {
			text.WriteString(c.Delta.Content)
			on(Event{Kind: EvText, Text: c.Delta.Content})
		}
		for _, d := range c.Delta.ToolCalls {
			i := int(d.Index)
			for len(calls) <= i {
				calls = append(calls, nil)
			}
			if calls[i] == nil {
				calls[i] = &tc{id: d.ID, name: d.Function.Name}
				on(Event{Kind: EvToolStart, Index: i, CallID: d.ID, Name: d.Function.Name})
			}
			if d.Function.Arguments != "" {
				calls[i].args += d.Function.Arguments
				on(Event{Kind: EvToolDelta, Index: i, Text: d.Function.Arguments})
			}
		}
		if c.FinishReason != "" {
			finish = c.FinishReason
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if finish == "" && !sdkRawMode {
		return nil, errTruncated
	}
	msg := Message{Role: "assistant"}
	if text.Len() > 0 {
		msg.Parts = append(msg.Parts, Part{Type: PText, Text: text.String()})
	}
	for i, c := range calls {
		if c == nil {
			continue
		}
		if !json.Valid([]byte(c.args)) {
			return nil, fmt.Errorf("tool call %s: arguments are not valid JSON", c.id)
		}
		on(Event{Kind: EvToolEnd, Index: i})
		msg.Parts = append(msg.Parts, Part{Type: PToolCall, CallID: c.id, Name: c.name, Args: c.args})
	}
	on(Event{Kind: EvUsage, Usage: usage})
	on(Event{Kind: EvStop, Stop: finish})
	return &Response{Message: msg, Usage: usage, Stop: finish}, nil
}
