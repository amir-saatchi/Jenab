package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	aoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	ganthropic "github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	openaiv1 "github.com/openai/openai-go"
	ov1option "github.com/openai/openai-go/option"
)

// Option 2: Firebase Genkit for Go. The high-level genkit.Generate runs tools itself; we stop it
// with WithReturnToolRequests(true). Tools must be registered as Go functions, so the stubs
// count any call (libraryRanTool) - it must stay 0.

var optGenkit = Option{
	Name: "genkit", Module: "github.com/firebase/genkit/go", Version: "v1.13.1", Released: "2026-09-03", License: "Apache-2.0",
	New: func(o Options) (Provider, error) {
		ctx := context.Background()
		if o.Kind == Anthropic {
			var opts []aoption.RequestOption
			if o.MaxRetries >= 0 {
				opts = append(opts, aoption.WithMaxRetries(o.MaxRetries))
			}
			g := genkit.Init(ctx, genkit.WithPlugins(&ganthropic.Anthropic{APIKey: o.APIKey, BaseURL: o.BaseURL, Opts: opts}))
			return &genkitP{g: g, kind: o.Kind, model: "anthropic/" + o.Model, tools: map[string]bool{}}, nil
		}
		var opts []ov1option.RequestOption
		if o.MaxRetries >= 0 {
			opts = append(opts, ov1option.WithMaxRetries(o.MaxRetries))
		}
		g := genkit.Init(ctx, genkit.WithPlugins(&compat_oai.OpenAICompatible{Provider: "local", APIKey: o.APIKey, BaseURL: o.BaseURL + "/v1", Opts: opts}))
		return &genkitP{g: g, kind: o.Kind, model: "local/" + o.Model, tools: map[string]bool{}}, nil
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
		var oe *openaiv1.Error
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

// genkitDefaultLoop drops WithReturnToolRequests(true) to show what genkit.Generate does by default.
var genkitDefaultLoop bool

type genkitP struct {
	g     *genkit.Genkit
	kind  Kind
	model string
	tools map[string]bool
}

func (p *genkitP) Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error) {
	var msgs []*ai.Message
	for _, s := range req.System {
		msgs = append(msgs, ai.NewSystemMessage(ai.NewTextPart(s.Text))) // no cache_control support
	}
	names := map[string]string{}
	for _, m := range req.Messages {
		var parts []*ai.Part
		for _, pt := range m.Parts {
			switch pt.Type {
			case PText:
				parts = append(parts, ai.NewTextPart(pt.Text))
			case PImage:
				parts = append(parts, ai.NewMediaPart(pt.MIME, dataURL(pt)))
			case PThinking:
				parts = append(parts, ai.NewReasoningPart(pt.Text, []byte(pt.Signature)))
			case PToolCall:
				names[pt.CallID] = pt.Name
				var in any
				_ = json.Unmarshal([]byte(pt.Args), &in)
				parts = append(parts, ai.NewToolRequestPart(&ai.ToolRequest{Name: pt.Name, Ref: pt.CallID, Input: in}))
			case PToolResult:
				tr := &ai.ToolResponse{Name: names[pt.CallID], Ref: pt.CallID}
				var txt []string
				for _, c := range pt.Content {
					if c.Type == PImage {
						tr.Content = append(tr.Content, ai.NewMediaPart(c.MIME, dataURL(c)))
					} else {
						txt = append(txt, c.Text)
					}
				}
				tr.Output = strings.Join(txt, "\n")
				parts = append(parts, ai.NewToolResponsePart(tr))
			}
		}
		role := map[string]ai.Role{"user": ai.RoleUser, "assistant": ai.RoleModel, "tool": ai.RoleTool}[m.Role]
		msgs = append(msgs, &ai.Message{Role: role, Content: parts})
	}
	var refs []ai.ToolRef
	for _, t := range req.Tools {
		if !p.tools[t.Name] {
			p.tools[t.Name] = true
			genkit.RegisterAction(p.g, ai.NewToolWithInputSchema(t.Name, t.Description, t.Schema, func(tc *ai.ToolContext, in any) (string, error) {
				libraryRanTool.Add(1)
				return "stub", nil
			}))
		}
		refs = append(refs, ai.ToolName(t.Name))
	}
	started := map[string]bool{}
	cb := func(ctx context.Context, ch *ai.ModelResponseChunk) error {
		for _, pt := range ch.Content {
			switch {
			case pt.IsReasoning():
				if pt.Text != "" {
					on(Event{Kind: EvThinking, Text: pt.Text})
				}
			case pt.IsText():
				if pt.Text != "" {
					on(Event{Kind: EvText, Text: pt.Text})
				}
			case pt.IsToolRequest():
				tr := pt.ToolRequest
				if tr.Ref != "" && !started[tr.Ref] {
					started[tr.Ref] = true
					on(Event{Kind: EvToolStart, CallID: tr.Ref, Name: tr.Name})
				}
				if s, ok := tr.Input.(string); ok && s != "" {
					on(Event{Kind: EvToolDelta, Text: s})
				}
			}
		}
		return nil
	}
	opts := []ai.GenerateOption{ai.WithModelName(p.model), ai.WithMessages(msgs...), ai.WithReturnToolRequests(!genkitDefaultLoop), ai.WithStreaming(cb)}
	if len(refs) > 0 {
		opts = append(opts, ai.WithTools(refs...))
	}
	if p.kind == Anthropic {
		cfg := &anthropic.MessageNewParams{MaxTokens: int64(req.MaxTokens)}
		if req.ThinkingBudget > 0 {
			cfg.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(req.ThinkingBudget))
		}
		opts = append(opts, ai.WithConfig(cfg))
	}
	resp, err := genkit.Generate(ctx, p.g, opts...)
	if err != nil {
		return nil, err
	}
	msg := Message{Role: "assistant"}
	i := 0
	for _, pt := range resp.Message.Content {
		switch {
		case pt.IsReasoning():
			sig := ""
			switch s := pt.Metadata["signature"].(type) {
			case []byte:
				sig = string(s)
			case string:
				sig = s
			}
			msg.Parts = append(msg.Parts, Part{Type: PThinking, Text: pt.Text, Signature: sig})
		case pt.IsText():
			if pt.Text != "" {
				msg.Parts = append(msg.Parts, Part{Type: PText, Text: pt.Text})
			}
		case pt.IsToolRequest():
			b, _ := json.Marshal(pt.ToolRequest.Input)
			if s, ok := pt.ToolRequest.Input.(string); ok {
				b = []byte(s)
			}
			on(Event{Kind: EvToolEnd, Index: i})
			i++
			msg.Parts = append(msg.Parts, Part{Type: PToolCall, CallID: pt.ToolRequest.Ref, Name: pt.ToolRequest.Name, Args: string(b)})
		}
	}
	var usage Usage
	if u := resp.Usage; u != nil {
		usage = Usage{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CachedContentTokens}
	}
	on(Event{Kind: EvUsage, Usage: usage})
	on(Event{Kind: EvStop, Stop: string(resp.FinishReason)})
	return &Response{Message: msg, Usage: usage, Stop: string(resp.FinishReason)}, nil
}

var _ = fmt.Sprint
