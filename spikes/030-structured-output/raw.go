package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// rawCheck sends a task's tool request to an OpenAI-compatible API over
// plain HTTP, outside the app's backends, and prints what comes back: the
// status, the finish reason, the usage and whether the arguments are JSON.
// It tries the schema as it is and plain (no null in type lists, no
// patterns), each with and without tool_choice.
func rawCheck(baseURL, key, model string, t *Task, maxTokens int) {
	plain := plainSchema(t.Schema)
	for _, v := range []struct {
		name   string
		schema json.RawMessage
		forced bool
	}{
		{"as is", t.Schema, false},
		{"as is, forced", t.Schema, true},
		{"plain", plain, false},
		{"plain, forced", plain, true},
	} {
		body := map[string]any{
			"model":                 model,
			"stream":                true,
			"stream_options":        map[string]any{"include_usage": true},
			"max_completion_tokens": maxTokens,
			"messages": []any{
				map[string]any{"role": "system", "content": baseSystem + "\n\n" + toolLine},
				map[string]any{"role": "user", "content": "Instruction:\n" + strings.TrimSpace(t.Instruction) + "\n\nInput:\n" + t.Input},
			},
			"tools": []any{map[string]any{"type": "function", "function": map[string]any{
				"name": submitName, "description": "Submits the result. The arguments are the result.", "parameters": v.schema}}},
		}
		if v.forced {
			body["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": submitName}}
		}
		fmt.Printf("%-14s %s\n", v.name, rawSend(baseURL, key, body))
	}
}

func rawSend(baseURL, key string, body map[string]any) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(baseURL, "/")+"/chat/completions", bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "error: " + strings.ReplaceAll(err.Error(), key, "***")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Sprintf("%d %s", resp.StatusCode, clipLine(string(b), 300))
	}
	var finish, args, text string
	var reasoning int
	var usage json.RawMessage
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok || strings.TrimSpace(line) == "[DONE]" {
			continue
		}
		var c struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Function struct {
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal([]byte(line), &c) != nil {
			continue
		}
		for _, ch := range c.Choices {
			text += ch.Delta.Content
			reasoning += len([]rune(ch.Delta.ReasoningContent))
			for _, tc := range ch.Delta.ToolCalls {
				args += tc.Function.Arguments
			}
			if ch.FinishReason != "" {
				finish = ch.FinishReason
			}
		}
		if len(c.Usage) > 0 && string(c.Usage) != "null" {
			usage = c.Usage
		}
	}
	return fmt.Sprintf("%.0fs finish=%s args=%d valid=%v reasoning_chars=%d text_chars=%d usage=%s args_end=%q",
		time.Since(start).Seconds(), finish, len(args), json.Valid([]byte(args)), reasoning, len(text), usage, tail(args, 80))
}

func tail(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return "…" + string(r[len(r)-n:])
	}
	return s
}

// plainSchema drops null from type lists and removes patterns.
func plainSchema(s json.RawMessage) json.RawMessage {
	var v any
	json.Unmarshal(s, &v)
	var walk func(any)
	walk = func(x any) {
		switch m := x.(type) {
		case map[string]any:
			if ts, ok := m["type"].([]any); ok {
				for _, t := range ts {
					if t != "null" {
						m["type"] = t
						break
					}
				}
			}
			delete(m, "pattern")
			for _, c := range m {
				walk(c)
			}
		case []any:
			for _, c := range m {
				walk(c)
			}
		}
	}
	walk(v)
	return mustJSON(v)
}
