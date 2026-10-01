package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	ganth "github.com/zendev-sh/goai/provider/anthropic"
	"github.com/zendev-sh/goai/provider/compat"
)

// Option 5: GoAI (zendev-sh/goai), a Go library in the spirit of the Vercel AI SDK. Raw net/http,
// no official SDKs. We call StreamText with tools that have no Execute func, so it never runs them.

var optGoAI = Option{
	Name: "goai", Module: "github.com/zendev-sh/goai", Version: "v0.10.4", Released: "2026-09-22", License: "MIT",
	New: func(o Options) (Provider, error) {
		var m provider.LanguageModel
		if o.Kind == Anthropic {
			m = ganth.Chat(o.Model, ganth.WithAPIKey(o.APIKey), ganth.WithBaseURL(o.BaseURL), ganth.WithSkipEnvResolve())
		} else {
			m = compat.Chat(o.Model, compat.WithAPIKey(o.APIKey), compat.WithBaseURL(o.BaseURL+"/v1"))
		}
		return &goaiP{m: m, kind: o.Kind, retries: o.MaxRetries}, nil
	},
	Classify: func(err error) *ProviderError {
		var ae *goai.APIError
		if !errors.As(err, &ae) {
			return nil
		}
		pe := &ProviderError{Status: ae.StatusCode, Msg: ae.Message}
		for k, v := range ae.ResponseHeaders {
			if strings.EqualFold(k, "retry-after") {
				pe.RetryAfter, pe.HasRetry = parseRetryAfter(v)
			}
		}
		var body struct {
			Error struct{ Type string } `json:"error"`
		}
		_ = json.Unmarshal([]byte(ae.ResponseBody), &body)
		pe.Type = body.Error.Type
		return pe
	},
}

type goaiP struct {
	m       provider.LanguageModel
	kind    Kind
	retries int
}

func (p *goaiP) Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error) {
	var sys []string
	sysCache := false
	for _, s := range req.System {
		sys = append(sys, s.Text)
		sysCache = sysCache || s.Cache
	}
	names := map[string]string{}
	var msgs []provider.Message
	for _, m := range req.Messages {
		var parts []provider.Part
		var imgs []provider.Part
		for _, pt := range m.Parts {
			cc := ""
			if pt.Cache {
				cc = "ephemeral"
			}
			switch pt.Type {
			case PText:
				parts = append(parts, provider.Part{Type: provider.PartText, Text: pt.Text, CacheControl: cc})
			case PImage:
				parts = append(parts, provider.Part{Type: provider.PartImage, URL: dataURL(pt), MediaType: pt.MIME, CacheControl: cc})
			case PThinking:
				parts = append(parts, provider.Part{Type: provider.PartReasoning, Text: pt.Text, ProviderOptions: map[string]any{"signature": pt.Signature}})
			case PToolCall:
				names[pt.CallID] = pt.Name
				parts = append(parts, provider.Part{Type: provider.PartToolCall, ToolCallID: pt.CallID, ToolName: pt.Name, ToolInput: json.RawMessage(pt.Args), CacheControl: cc})
			case PToolResult:
				// A GoAI tool result is a string. Images go into a user message after it.
				var txt []string
				for _, c := range pt.Content {
					if c.Type == PImage {
						imgs = append(imgs, provider.Part{Type: provider.PartImage, URL: dataURL(c), MediaType: c.MIME})
					} else {
						txt = append(txt, c.Text)
					}
				}
				parts = append(parts, provider.Part{Type: provider.PartToolResult, ToolCallID: pt.CallID, ToolName: names[pt.CallID], ToolOutput: strings.Join(txt, "\n"), CacheControl: cc})
			}
		}
		role := provider.Role(m.Role)
		msgs = append(msgs, provider.Message{Role: role, Content: parts})
		if len(imgs) > 0 {
			msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: imgs})
		}
	}
	var tools []goai.Tool
	for _, t := range req.Tools {
		s, _ := json.Marshal(t.Schema)
		tools = append(tools, goai.Tool{Name: t.Name, Description: t.Description, InputSchema: s}) // no Execute
	}
	opts := []goai.Option{goai.WithSystem(strings.Join(sys, "\n\n")), goai.WithMessages(msgs...), goai.WithTools(tools...),
		goai.WithMaxOutputTokens(req.MaxTokens), goai.WithMaxSteps(1), goai.WithPromptCaching(sysCache)}
	if p.retries >= 0 {
		opts = append(opts, goai.WithMaxRetries(p.retries))
	}
	if req.ThinkingBudget > 0 {
		opts = append(opts, goai.WithProviderOptions(map[string]any{"thinking": map[string]any{"type": "enabled", "budgetTokens": req.ThinkingBudget}}))
	}
	ts, err := goai.StreamText(ctx, p.m, opts...)
	if err != nil {
		return nil, err
	}
	msg := Message{Role: "assistant"}
	var text strings.Builder
	var think *Part
	var usage Usage
	var stop string
	idx := map[string]int{}
	for ch := range ts.Stream() {
		switch ch.Type {
		case provider.ChunkText:
			if ch.Text != "" {
				text.WriteString(ch.Text)
				on(Event{Kind: EvText, Text: ch.Text})
			}
		case provider.ChunkReasoning:
			if think == nil {
				think = &Part{Type: PThinking}
			}
			think.Text += ch.Text
			if s, ok := ch.Metadata["signature"].(string); ok {
				think.Signature += s
			}
			if ch.Text != "" {
				on(Event{Kind: EvThinking, Text: ch.Text})
			}
		case provider.ChunkToolCallStreamStart:
			idx[ch.ToolCallID] = len(idx)
			on(Event{Kind: EvToolStart, Index: idx[ch.ToolCallID], CallID: ch.ToolCallID, Name: ch.ToolName})
		case provider.ChunkToolCallDelta:
			on(Event{Kind: EvToolDelta, Index: idx[ch.ToolCallID], Text: ch.ToolInput})
		case provider.ChunkToolCall:
			on(Event{Kind: EvToolEnd, Index: idx[ch.ToolCallID]})
			msg.Parts = append(msg.Parts, Part{Type: PToolCall, CallID: ch.ToolCallID, Name: ch.ToolName, Args: ch.ToolInput})
		case provider.ChunkStepFinish:
			stop = string(ch.FinishReason)
		case provider.ChunkFinish:
			if ch.FinishReason != "" {
				stop = string(ch.FinishReason)
			}
			u := ch.Usage
			usage = Usage{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadTokens, CacheWrite: u.CacheWriteTokens}
		}
	}
	if err := ts.Err(); err != nil {
		return nil, err
	}
	var head []Part
	if think != nil {
		head = append(head, *think)
	}
	if text.Len() > 0 {
		head = append(head, Part{Type: PText, Text: text.String()})
	}
	msg.Parts = append(head, msg.Parts...)
	on(Event{Kind: EvUsage, Usage: usage})
	on(Event{Kind: EvStop, Stop: stop})
	return &Response{Message: msg, Usage: usage, Stop: stop}, nil
}
