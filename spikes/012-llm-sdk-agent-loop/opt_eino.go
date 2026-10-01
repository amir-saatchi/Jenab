package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/cloudwego/eino-ext/components/model/claude"
	einooai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// Option 3: CloudWeGo Eino. We use only the ChatModel components (no graph, no ReAct agent):
// they return tool calls and never run tools.

var optEino = Option{
	Name: "eino", Module: "cloudwego/eino + eino-ext claude, openai", Version: "v0.9.21 + claude v0.1.26, openai v0.1.13", Released: "2026-09-23 + 2026-09-24, 2026-04-16", License: "Apache-2.0",
	New: func(o Options) (Provider, error) {
		ctx := context.Background()
		if o.Kind == Anthropic {
			base := o.BaseURL
			cm, err := claude.NewChatModel(ctx, &claude.Config{APIKey: o.APIKey, BaseURL: &base, Model: o.Model, MaxTokens: 1024})
			if err != nil {
				return nil, err
			}
			return &einoP{cm: cm, kind: o.Kind}, nil
		}
		cm, err := einooai.NewChatModel(ctx, &einooai.ChatModelConfig{APIKey: o.APIKey, BaseURL: o.BaseURL + "/v1", Model: o.Model})
		if err != nil {
			return nil, err
		}
		return &einoP{cm: cm, kind: o.Kind}, nil
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
		var oe *einooai.APIError
		if errors.As(err, &oe) {
			return &ProviderError{Status: oe.HTTPStatusCode, Type: oe.Type, Msg: oe.Message}
		}
		return nil
	},
}

type einoP struct {
	cm   model.ToolCallingChatModel
	kind Kind
}

func (p *einoP) Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error) {
	var msgs []*schema.Message
	for _, s := range req.System {
		m := schema.SystemMessage(s.Text)
		if s.Cache && p.kind == Anthropic {
			m = claude.SetMessageCacheControl(m, &claude.CacheControl{})
		}
		msgs = append(msgs, m)
	}
	names := map[string]string{}
	for _, m := range req.Messages {
		cache := false
		for _, pt := range m.Parts {
			cache = cache || pt.Cache
		}
		var out []*schema.Message
		switch m.Role {
		case "user":
			sm := &schema.Message{Role: schema.User}
			for _, pt := range m.Parts {
				sm.UserInputMultiContent = append(sm.UserInputMultiContent, einoPart(pt))
			}
			out = append(out, sm)
		case "assistant":
			sm := &schema.Message{Role: schema.Assistant}
			for _, pt := range m.Parts {
				switch pt.Type {
				case PText:
					sm.Content += pt.Text
				case PThinking:
					// No public setter: these are the adapter's private Extra keys.
					sm.Extra = map[string]any{"_eino_claude_thinking": pt.Text, "_eino_claude_thinking_signature": pt.Signature}
					sm.ReasoningContent = pt.Text
				case PToolCall:
					names[pt.CallID] = pt.Name
					sm.ToolCalls = append(sm.ToolCalls, schema.ToolCall{ID: pt.CallID, Type: "function", Function: schema.FunctionCall{Name: pt.Name, Arguments: pt.Args}})
				}
			}
			out = append(out, sm)
		case "tool":
			for _, pt := range m.Parts {
				sm := &schema.Message{Role: schema.Tool, ToolCallID: pt.CallID, ToolName: names[pt.CallID]}
				for _, c := range pt.Content {
					sm.UserInputMultiContent = append(sm.UserInputMultiContent, einoPart(c))
				}
				out = append(out, sm)
			}
		}
		if cache && p.kind == Anthropic && len(out) > 0 {
			out[len(out)-1] = claude.SetMessageCacheControl(out[len(out)-1], &claude.CacheControl{})
		}
		msgs = append(msgs, out...)
	}
	var tools []*schema.ToolInfo
	for _, t := range req.Tools {
		b, _ := json.Marshal(t.Schema)
		var js jsonschema.Schema
		if err := json.Unmarshal(b, &js); err != nil {
			return nil, err
		}
		tools = append(tools, &schema.ToolInfo{Name: t.Name, Desc: t.Description, ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&js)})
	}
	var opts []model.Option
	if len(tools) > 0 {
		opts = append(opts, model.WithTools(tools))
	}
	if req.MaxTokens > 0 {
		opts = append(opts, model.WithMaxTokens(req.MaxTokens))
	}
	if req.ThinkingBudget > 0 && p.kind == Anthropic {
		tc := anthropic.ThinkingConfigParamOfEnabled(int64(req.ThinkingBudget))
		opts = append(opts, claude.WithThinkingConfig(&tc))
	}
	sr, err := p.cm.Stream(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	defer sr.Close()
	var chunks []*schema.Message
	started := map[int]bool{}
	for {
		ch, err := sr.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, ch)
		if ch.Content != "" {
			on(Event{Kind: EvText, Text: ch.Content})
		}
		if ch.ReasoningContent != "" {
			on(Event{Kind: EvThinking, Text: ch.ReasoningContent})
		}
		for _, tc := range ch.ToolCalls {
			i := 0
			if tc.Index != nil {
				i = *tc.Index
			}
			if !started[i] && tc.ID != "" {
				started[i] = true
				on(Event{Kind: EvToolStart, Index: i, CallID: tc.ID, Name: tc.Function.Name})
			}
			if tc.Function.Arguments != "" {
				on(Event{Kind: EvToolDelta, Index: i, Text: tc.Function.Arguments})
			}
		}
	}
	if len(chunks) == 0 {
		return nil, errors.New("empty stream")
	}
	full, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, err
	}
	msg := Message{Role: "assistant"}
	if th, ok := claude.GetThinking(full); ok && th != "" {
		sig, _ := full.Extra["_eino_claude_thinking_signature"].(string)
		msg.Parts = append(msg.Parts, Part{Type: PThinking, Text: th, Signature: sig})
	}
	if full.Content != "" {
		msg.Parts = append(msg.Parts, Part{Type: PText, Text: full.Content})
	}
	for i, tc := range full.ToolCalls {
		args := tc.Function.Arguments
		if args == "" {
			args = "{}"
		}
		on(Event{Kind: EvToolEnd, Index: i})
		msg.Parts = append(msg.Parts, Part{Type: PToolCall, CallID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	var usage Usage
	stop := ""
	if rm := full.ResponseMeta; rm != nil {
		stop = rm.FinishReason
		if u := rm.Usage; u != nil {
			usage = Usage{Input: u.PromptTokens - u.PromptTokenDetails.CachedTokens - u.PromptTokenDetails.CacheWriteTokens, Output: u.CompletionTokens,
				CacheRead: u.PromptTokenDetails.CachedTokens, CacheWrite: u.PromptTokenDetails.CacheWriteTokens}
			if p.kind == OpenAI {
				usage.Input = u.PromptTokens
			}
		}
	}
	on(Event{Kind: EvUsage, Usage: usage})
	on(Event{Kind: EvStop, Stop: stop})
	return &Response{Message: msg, Usage: usage, Stop: stop}, nil
}

func einoPart(pt Part) schema.MessageInputPart {
	if pt.Type == PImage {
		b64 := base64.StdEncoding.EncodeToString(pt.Data)
		return schema.MessageInputPart{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
			MessagePartCommon: schema.MessagePartCommon{Base64Data: &b64, MIMEType: pt.MIME}}}
	}
	return schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: pt.Text}
}

var _ = strings.TrimSpace
