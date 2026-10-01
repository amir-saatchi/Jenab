package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// T8: custom base URL. Every mock test already uses one. This adds one real tool-call round trip
// against Ollama's OpenAI-compatible endpoint (http://localhost:11434/v1) with qwen3.5:4b.

const ollamaRoot = "http://localhost:11434"
const ollamaModel = "qwen3.5:4b"

func ollamaUp() bool {
	c := http.Client{Timeout: 2 * time.Second}
	r, err := c.Get(ollamaRoot + "/api/tags")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	var tags struct{ Models []struct{ Name string } }
	_ = json.NewDecoder(r.Body).Decode(&tags)
	for _, m := range tags.Models {
		if m.Name == ollamaModel {
			return true
		}
	}
	return false
}

func testOllama(opt Option) Cell {
	p, err := opt.New(Options{Kind: OpenAI, BaseURL: ollamaRoot, APIKey: "ollama", Model: ollamaModel, MaxRetries: 0})
	if err != nil {
		return errCell(err)
	}
	req := &Request{Model: ollamaModel, MaxTokens: 2048, Tools: []ToolDef{weatherTool},
		System:   []Part{{Type: PText, Text: "You are a helpful assistant. Use tools when they help."}},
		Messages: []Message{{Role: "user", Parts: []Part{{Type: PText, Text: "What is the weather in Berlin right now? Use the get_weather tool, then answer in one short sentence."}}}}}
	var args string
	tools := map[string]ToolFunc{"get_weather": func(a json.RawMessage) []Part {
		args = string(a)
		return []Part{{Type: PText, Text: "18 C, sunny"}}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t0 := time.Now()
	res, err := runLoop(ctx, p, req, tools, false, 3)
	el := time.Since(t0)
	if err != nil {
		return Cell{"fail", fmt.Sprintf("after %.0f s: %s", el.Seconds(), shortErr(err))}
	}
	txt := finalText(res.Final)
	if args == "" {
		return Cell{"fail", fmt.Sprintf("model did not call the tool; answer %q", trunc(txt, 60))}
	}
	c := &check{}
	c.must(strings.Contains(strings.ToLower(args), "berlin"), "tool args %s", args)
	c.must(strings.Contains(txt, "18"), "answer %q", trunc(txt, 60))
	c.should(countEv(res.Events, EvText) > 1, "answer not streamed")
	return c.cell(fmt.Sprintf("%d requests, %.0f s, args %s, answer %q", res.Requests, el.Seconds(), args, trunc(txt, 50)))
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "..."
	}
	return s
}
