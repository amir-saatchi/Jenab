package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers"
	aanth "github.com/mozilla-ai/any-llm-go/providers/anthropic"
	aoai "github.com/mozilla-ai/any-llm-go/providers/openai"
	openaiv1 "github.com/openai/openai-go"
)

// Option 4: Mozilla any-llm-go. One OpenAI-shaped API over many providers; wraps
// anthropic-sdk-go and openai-go v1. It never runs tools.

var optAnyLLM = Option{
	Name: "any-llm-go", Module: "github.com/mozilla-ai/any-llm-go", Version: "v0.9.0", Released: "2026-03-09", License: "Apache-2.0",
	New: func(o Options) (Provider, error) {
		if o.Kind == Anthropic {
			p, err := aanth.New(anyllm.WithAPIKey(o.APIKey), anyllm.WithBaseURL(o.BaseURL))
			if err != nil {
				return nil, err
			}
			return &anyP{p: p, kind: o.Kind}, nil
		}
		p, err := aoai.NewCompatible(aoai.CompatibleConfig{Name: "local", DefaultBaseURL: o.BaseURL + "/v1", DefaultAPIKey: o.APIKey, RequireAPIKey: false})
		if err != nil {
			return nil, err
		}
		return &anyP{p: p, kind: o.Kind}, nil
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

type anyP struct {
	p    providers.Provider
	kind Kind
}

func (p *anyP) Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error) {
	var msgs []providers.Message
	for _, s := range req.System {
		msgs = append(msgs, providers.Message{Role: "system", Content: s.Text})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			var parts []providers.ContentPart
			for _, pt := range m.Parts {
				if pt.Type == PImage {
					parts = append(parts, providers.ContentPart{Type: "image_url", ImageURL: &providers.ImageURL{URL: dataURL(pt)}})
				} else {
					parts = append(parts, providers.ContentPart{Type: "text", Text: pt.Text})
				}
			}
			msgs = append(msgs, providers.Message{Role: "user", Content: parts})
		case "assistant":
			am := providers.Message{Role: "assistant"}
			var text []string
			for _, pt := range m.Parts {
				switch pt.Type {
				case PText:
					text = append(text, pt.Text)
				case PThinking:
					am.Reasoning = &providers.Reasoning{Content: pt.Text} // no field for the signature
				case PToolCall:
					am.ToolCalls = append(am.ToolCalls, providers.ToolCall{ID: pt.CallID, Type: "function", Function: providers.FunctionCall{Name: pt.Name, Arguments: pt.Args}})
				}
			}
			am.Content = strings.Join(text, "")
			msgs = append(msgs, am)
		case "tool":
			// Tool messages carry a string. Images go into a user message after them.
			var imgs []providers.ContentPart
			for _, pt := range m.Parts {
				var txt []string
				for _, c := range pt.Content {
					if c.Type == PImage {
						imgs = append(imgs, providers.ContentPart{Type: "image_url", ImageURL: &providers.ImageURL{URL: dataURL(c)}})
					} else {
						txt = append(txt, c.Text)
					}
				}
				msgs = append(msgs, providers.Message{Role: "tool", ToolCallID: pt.CallID, Content: strings.Join(txt, "\n")})
			}
			if len(imgs) > 0 {
				msgs = append(msgs, providers.Message{Role: "user", Content: imgs})
			}
		}
	}
	var tools []providers.Tool
	for _, t := range req.Tools {
		b, _ := json.Marshal(t.Schema)
		var sch map[string]any
		_ = json.Unmarshal(b, &sch)
		tools = append(tools, providers.Tool{Type: "function", Function: providers.Function{Name: t.Name, Description: t.Description, Parameters: sch}})
	}
	mt := req.MaxTokens
	prm := providers.CompletionParams{Model: req.Model, Messages: msgs, Tools: tools, MaxTokens: &mt,
		StreamOptions: &providers.StreamOptions{IncludeUsage: true}}
	if req.ThinkingBudget > 0 {
		// Only effort levels: low=1024, medium=4096, high=16384 budget tokens.
		prm.ReasoningEffort = "medium"
		if req.ThinkingBudget <= 1024 {
			prm.ReasoningEffort = "low"
		}
	}
	chunks, errs := p.p.CompletionStream(ctx, prm)
	type tc struct{ id, name, args string }
	var calls []*tc
	var text, think strings.Builder
	var usage Usage
	var stop string
	for ch := range chunks {
		if ch.Usage != nil {
			usage = Usage{Input: ch.Usage.PromptTokens, Output: ch.Usage.CompletionTokens}
		}
		if len(ch.Choices) == 0 {
			continue
		}
		c := ch.Choices[0]
		if c.Delta.Content != "" {
			text.WriteString(c.Delta.Content)
			on(Event{Kind: EvText, Text: c.Delta.Content})
		}
		if c.Delta.Reasoning != nil && c.Delta.Reasoning.Content != "" {
			think.WriteString(c.Delta.Reasoning.Content)
			on(Event{Kind: EvThinking, Text: c.Delta.Reasoning.Content})
		}
		for _, d := range c.Delta.ToolCalls {
			// No index on tool-call deltas: a delta with an ID starts a new call, others continue the last.
			if d.ID != "" && (len(calls) == 0 || calls[len(calls)-1].id != d.ID) {
				calls = append(calls, &tc{id: d.ID, name: d.Function.Name})
				on(Event{Kind: EvToolStart, Index: len(calls) - 1, CallID: d.ID, Name: d.Function.Name})
			}
			if len(calls) == 0 {
				continue
			}
			cur := calls[len(calls)-1]
			if p.kind == Anthropic {
				// The Anthropic adapter sends the accumulated arguments each time.
				if len(d.Function.Arguments) > len(cur.args) {
					on(Event{Kind: EvToolDelta, Index: len(calls) - 1, Text: d.Function.Arguments[len(cur.args):]})
				}
				cur.args = d.Function.Arguments
			} else if d.Function.Arguments != "" {
				cur.args += d.Function.Arguments
				on(Event{Kind: EvToolDelta, Index: len(calls) - 1, Text: d.Function.Arguments})
			}
		}
		if c.FinishReason != "" {
			stop = c.FinishReason
		}
	}
	if err := <-errs; err != nil {
		return nil, err
	}
	msg := Message{Role: "assistant"}
	if think.Len() > 0 {
		msg.Parts = append(msg.Parts, Part{Type: PThinking, Text: think.String()})
	}
	if text.Len() > 0 {
		msg.Parts = append(msg.Parts, Part{Type: PText, Text: text.String()})
	}
	for i, c := range calls {
		args := c.args
		if args == "" {
			args = "{}"
		}
		on(Event{Kind: EvToolEnd, Index: i})
		msg.Parts = append(msg.Parts, Part{Type: PToolCall, CallID: c.id, Name: c.name, Args: args})
	}
	on(Event{Kind: EvUsage, Usage: usage})
	on(Event{Kind: EvStop, Stop: stop})
	return &Response{Message: msg, Usage: usage, Stop: stop}, nil
}
