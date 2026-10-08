package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

var probeSchema = json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string","enum":["yes","no"]}},"required":["answer"],"additionalProperties":false}`)

// probeAll finds what each model's API accepts: a required tool call and a
// JSON mode. Results are kept in file; force probes again.
func probeAll(ctx context.Context, a *asker, models []*Model, file string, force bool) error {
	known := map[string]Support{}
	if b, err := os.ReadFile(file); err == nil && !force {
		if err := json.Unmarshal(b, &known); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	for _, m := range models {
		if s, ok := known[m.Ref]; ok {
			m.Support = s
			continue
		}
		m.Support = probe(ctx, a, m)
		known[m.Ref] = m.Support
		log.Printf("probe %-32s %+v", m.Ref, m.Support)
	}
	b, _ := json.MarshalIndent(known, "", "  ")
	return os.WriteFile(file, append(b, '\n'), 0o644)
}

func probe(ctx context.Context, a *asker, m *Model) Support {
	var s Support
	var notes []string
	try := func(name string, fields map[string]any, tools bool) bool {
		req := provider.Request{Model: m.Ref, MaxTokens: 512,
			System:   []provider.Block{{Text: baseSystem}},
			Messages: []chat.Message{userText("Is the sky blue on a clear day? Answer yes or no, as JSON: {\"answer\": \"yes\"} or {\"answer\": \"no\"}.")}}
		if tools {
			req.Tools = []provider.ToolDef{{Name: submitName, Description: "Submits the result.", Schema: probeSchema}}
		}
		var parts []chat.Part
		var err error
		for attempt := range 3 {
			cctx, cancel := context.WithTimeout(withExtra(ctx, fields), 2*time.Minute)
			parts, _, err = a.call(cctx, req)
			cancel()
			var pe *provider.Error
			if err == nil || !errors.As(err, &pe) || (pe.Kind != provider.RateLimited && pe.Kind != provider.Overloaded && pe.Kind != provider.Transport) {
				break
			}
			time.Sleep(time.Duration(10*(attempt+1)) * time.Second)
		}
		if err != nil {
			notes = append(notes, name+" refused: "+clipLine(a.redact(err.Error()), 200))
			return false
		}
		var text string
		called := false
		for _, p := range parts {
			if p.Text != nil {
				text += p.Text.Text
			}
			if p.ToolCall != nil && p.ToolCall.Name == submitName {
				called = true
			}
		}
		if tools {
			if !called {
				notes = append(notes, name+" accepted, but no call")
			}
			return true
		}
		if _, wrapped, err := parseText(text); err != nil || wrapped {
			notes = append(notes, fmt.Sprintf("%s accepted, answer %q", name, clipLine(text, 80)))
		}
		return true
	}
	if m.Kind == provider.KindOllama {
		s.JSONSchema = try("format", map[string]any{"format": probeSchema}, false)
		notes = append(notes, "no tool_choice in Ollama's /api/chat")
	} else {
		s.ForcedNamed = try("tool_choice named", map[string]any{"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": submitName}}}, true)
		s.ForcedRequired = try("tool_choice required", map[string]any{"tool_choice": "required"}, true)
		s.JSONSchema = try("json_schema", map[string]any{"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "result", "schema": probeSchema}}}, false)
		s.JSONObject = try("json_object", map[string]any{"response_format": map[string]any{"type": "json_object"}}, false)
	}
	s.Notes = strings.Join(notes, "; ")
	return s
}
