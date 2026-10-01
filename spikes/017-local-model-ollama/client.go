package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// The /v1 client, used the way Burrow's provider layer will use it (SPEC 3.8): official SDK, custom
// base URL, streaming with include_usage, own retry count, complete streams only.

var oai openai.Client

func initClient() {
	oai = openai.NewClient(option.WithBaseURL(*host+"/v1"), option.WithAPIKey("ollama"), option.WithMaxRetries(0))
}

type thinkMode int

const (
	thinkDefault thinkMode = iota // no field sent: the model's default (Qwen 3.5 thinks)
	thinkOff                      // reasoning_effort: "none"
)

type req struct {
	Model     string
	Msgs      []openai.ChatCompletionMessageParamUnion
	Tools     []openai.ChatCompletionToolUnionParam
	MaxTokens int
	MaxCompletion bool // send max_completion_tokens instead of max_tokens
	Think     thinkMode
	Effort    string         // explicit reasoning_effort, overrides Think
	Extra     map[string]any // extra top-level JSON fields
	Timeout   time.Duration
}

type call struct{ ID, Name, Args string }

type res struct {
	TTFT         time.Duration // first streamed token of any kind (reasoning, text or tool call)
	FirstContent time.Duration // first text or tool-call token (the answer)
	Total        time.Duration
	Reasoning    string
	Content      string
	Calls        []call
	PromptTok    int
	OutTok       int
	Finish       string
	Err          error
}

func (r *res) timedOut() bool { return errors.Is(r.Err, context.DeadlineExceeded) }

func stream(q req) *res {
	prm := openai.ChatCompletionNewParams{Model: shared.ChatModel(q.Model), Messages: q.Msgs, Tools: q.Tools,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}}
	// Ollama 0.21 ignores max_completion_tokens on /v1 (section 2); only the older max_tokens works.
	if q.MaxTokens > 0 && q.MaxCompletion {
		prm.MaxCompletionTokens = openai.Int(int64(q.MaxTokens))
	} else if q.MaxTokens > 0 {
		prm.MaxTokens = openai.Int(int64(q.MaxTokens))
	}
	if q.Effort != "" {
		prm.ReasoningEffort = shared.ReasoningEffort(q.Effort)
	} else if q.Think == thinkOff {
		prm.ReasoningEffort = shared.ReasoningEffortNone
	}
	var opts []option.RequestOption
	for k, v := range q.Extra {
		opts = append(opts, option.WithJSONSet(k, v))
	}
	to := q.Timeout
	if to == 0 {
		to = 6 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	out := &res{}
	t0 := time.Now()
	s := oai.Chat.Completions.NewStreaming(ctx, prm, opts...)
	defer s.Close()
	var calls []*call
	first := func(content bool) {
		if out.TTFT == 0 {
			out.TTFT = time.Since(t0)
		}
		if content && out.FirstContent == 0 {
			out.FirstContent = time.Since(t0)
		}
	}
	for s.Next() {
		ch := s.Current()
		if ch.JSON.Usage.Valid() && ch.Usage.TotalTokens > 0 {
			out.PromptTok, out.OutTok = int(ch.Usage.PromptTokens), int(ch.Usage.CompletionTokens)
		}
		if len(ch.Choices) == 0 {
			continue
		}
		c := ch.Choices[0]
		if r, ok := c.Delta.JSON.ExtraFields["reasoning"]; ok && r.Raw() != "" && r.Raw() != "null" {
			var txt string
			_ = json.Unmarshal([]byte(r.Raw()), &txt)
			if txt != "" {
				first(false)
				out.Reasoning += txt
			}
		}
		if c.Delta.Content != "" {
			first(true)
			out.Content += c.Delta.Content
		}
		for _, d := range c.Delta.ToolCalls {
			first(true)
			i := int(d.Index)
			for len(calls) <= i {
				calls = append(calls, nil)
			}
			if calls[i] == nil {
				calls[i] = &call{ID: d.ID, Name: d.Function.Name}
			}
			calls[i].Args += d.Function.Arguments
		}
		if c.FinishReason != "" {
			out.Finish = c.FinishReason
		}
	}
	out.Total = time.Since(t0)
	for _, c := range calls {
		if c != nil {
			out.Calls = append(out.Calls, *c)
		}
	}
	if err := s.Err(); err != nil {
		out.Err = err
	} else if ctx.Err() != nil {
		out.Err = ctx.Err()
	} else if out.Finish == "" {
		out.Err = fmt.Errorf("stream ended without finish_reason")
	}
	return out
}

func sys(s string) openai.ChatCompletionMessageParamUnion  { return openai.SystemMessage(s) }
func user(s string) openai.ChatCompletionMessageParamUnion { return openai.UserMessage(s) }
func asst(s string) openai.ChatCompletionMessageParamUnion { return openai.AssistantMessage(s) }
func toolMsg(id, s string) openai.ChatCompletionMessageParamUnion {
	return openai.ToolMessage(s, id)
}

func asstCalls(text string, calls []call) openai.ChatCompletionMessageParamUnion {
	var am openai.ChatCompletionAssistantMessageParam
	if text != "" {
		am.Content.OfString = openai.String(text)
	}
	for _, c := range calls {
		am.ToolCalls = append(am.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: c.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: c.Name, Arguments: c.Args}}})
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &am}
}

func sec(d time.Duration) string {
	if d == 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}

func rate(n int, d time.Duration) string {
	if n == 0 || d <= 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f", float64(n)/d.Seconds())
}
