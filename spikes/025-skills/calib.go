package main

// Size calibration, as in SPIKE-021: each text is measured as the prompt_tokens difference
// against a one-line system prompt, so each model's own tokenizer and chat template count.

import (
	"context"

	"github.com/openai/openai-go/v3"
)

type Calib struct {
	Model    string
	Baseline int
	Tokens   map[string]int // text -> tokens (difference against the baseline)
	Chars    map[string]int
	Prompt   int // all prompt tokens spent on the calibration (budget)
	Output   int
	Err      string `json:",omitempty"`
}

func calibrate(ctx context.Context, m *Model) Calib {
	c := Calib{Model: m.ID, Tokens: map[string]int{}, Chars: map[string]int{}}
	ask := func(sys string, tools []openai.ChatCompletionToolUnionParam) int {
		msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(sys), openai.UserMessage("Reply with the word ok.")}
		r := stream(ctx, m, Req{Label: "calibrate", Messages: msgs, Tools: tools, MaxTokens: 64})
		spend(r.Prompt, r.Output)
		c.Prompt += r.Prompt
		c.Output += r.Output
		if r.Err != nil && c.Err == "" {
			c.Err = short(r.ErrText, 300)
		}
		return r.Prompt
	}
	const base = "You are a test."
	c.Baseline = ask(base, nil)
	texts := []struct{ id, text string }{
		{"guide_v2", guides["v2"]},
		{"intro", intro},
		{"skill_list", skillList(nil)},
	}
	for _, n := range skillOrder {
		texts = append(texts, struct{ id, text string }{"skill:" + n, skillText(n)})
	}
	for _, t := range texts {
		c.Chars[t.id] = len(t.text)
		c.Tokens[t.id] = ask(base+"\n\n"+t.text, nil) - c.Baseline
	}
	c.Tokens["tools_A"] = ask(base, oaiTools) - c.Baseline
	c.Tokens["tools_BC"] = ask(base, oaiToolsBC) - c.Baseline
	return c
}
