package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// The methods (SPIKE-030).
const (
	mTool   = "tool"        // a submit tool, the prompt asks for the call
	mForced = "tool-forced" // the same, with tool_choice requiring the call
	mText   = "text"        // JSON in the text, the schema in the prompt
	mNative = "native"      // as text, plus the API's own JSON mode
)

var methods = []string{mTool, mForced, mText, mNative}

// The prompt is the same for every model; only the method's line differs.
const (
	baseSystem = "You are one step of a data pipeline. Do what the instruction says with the input. Use only the input: never make up a value. When the input doesn't give a value, use null."
	toolLine   = "Give your result by calling the submit tool once. Its arguments are the result. Don't write the result as text."
	textLine   = "Reply with only the result: one JSON value that matches this JSON Schema, with no other text.\n\n"
)

const submitName = "submit"

// extraKey carries the fields the inject transport adds to a request body.
type extraKey struct{}

func withExtra(ctx context.Context, fields map[string]any) context.Context {
	if len(fields) == 0 {
		return ctx
	}
	return context.WithValue(ctx, extraKey{}, fields)
}

// inject adds the context's fields to a JSON request body. It sits under
// the app's backends, so they build and read every request as in the app;
// only tool_choice, response_format or format is added.
type inject struct{ next http.RoundTripper }

func (t inject) RoundTrip(r *http.Request) (*http.Response, error) {
	fields, _ := r.Context().Value(extraKey{}).(map[string]any)
	if len(fields) == 0 || r.Body == nil || r.Method != http.MethodPost {
		return t.next.RoundTrip(r)
	}
	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("inject: %w", err)
	}
	for k, v := range fields {
		m[k] = mustJSON(v)
	}
	body, _ = json.Marshal(m)
	r = r.Clone(r.Context())
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	r.ContentLength = int64(len(body))
	r.Header.Set("content-length", fmt.Sprint(len(body)))
	return t.next.RoundTrip(r)
}

// Model is one model under test.
type Model struct {
	Ref     string // provider/model
	Kind    provider.Kind
	Support Support
}

// Support is what the probe found the API accepts.
type Support struct {
	ForcedNamed    bool   `json:"forced_named"`    // tool_choice naming the tool
	ForcedRequired bool   `json:"forced_required"` // tool_choice "required"
	JSONSchema     bool   `json:"json_schema"`     // response_format json_schema, or Ollama's format with a schema
	JSONObject     bool   `json:"json_object"`     // response_format json_object
	Notes          string `json:"notes,omitempty"`
}

// extra is what a method adds to the request on this API; ok is false when
// the API has no way to do it.
func (m *Model) extra(method string, schema json.RawMessage) (map[string]any, bool) {
	ollama := m.Kind == provider.KindOllama
	switch method {
	case mForced:
		switch {
		case ollama:
			return nil, false // Ollama's /api/chat has no tool_choice
		case m.Support.ForcedNamed:
			return map[string]any{"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": submitName}}}, true
		case m.Support.ForcedRequired:
			return map[string]any{"tool_choice": "required"}, true
		}
		return nil, false
	case mNative:
		switch {
		case ollama && m.Support.JSONSchema:
			return map[string]any{"format": schema}, true
		case ollama:
			return nil, false
		case m.Support.JSONSchema:
			return map[string]any{"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "result", "schema": schema}}}, true
		case m.Support.JSONObject:
			return map[string]any{"response_format": map[string]any{"type": "json_object"}}, true
		}
		return nil, false
	}
	return nil, true
}

// Record is one task asked once, with its retry.
type Record struct {
	Model    string   `json:"model"`
	Method   string   `json:"method"`
	Task     string   `json:"task"`
	Kind     string   `json:"kind"`
	Lang     string   `json:"lang"`
	Rep      int      `json:"rep"`
	Thinking bool     `json:"thinking"`
	Skipped  string   `json:"skipped,omitempty"`
	Error    string   `json:"error,omitempty"`
	ErrKind  string   `json:"error_kind,omitempty"`
	Valid1   bool     `json:"valid_first"`
	Valid    bool     `json:"valid"` // first try or after the retry
	Fail1    []string `json:"fail_first,omitempty"`
	Fail2    []string `json:"fail_retry,omitempty"`
	Wrapped  bool     `json:"wrapped,omitempty"`
	Score    *score   `json:"score,omitempty"`
	Requests int      `json:"requests"`
	Prompt   int      `json:"prompt_tokens"`
	Output   int      `json:"output_tokens"`
	Seconds  float64  `json:"seconds"`
	Answer1  string   `json:"answer_first,omitempty"`
	Answer2  string   `json:"answer_retry,omitempty"`
}

type asker struct {
	reg       *provider.Registry
	redact    func(string) string
	maxTokens int
}

// ask runs one task with one method on one model: the first try, and one
// retry with the errors if the answer isn't valid.
func (a *asker) ask(ctx context.Context, m *Model, t *Task, method string, thinking bool) (rec Record) {
	rec = Record{Model: m.Ref, Method: method, Task: t.ID, Kind: t.Kind, Lang: t.Lang, Thinking: thinking}
	fields, ok := m.extra(method, t.Schema)
	if !ok {
		rec.Skipped = "the API has no way to do this"
		return rec
	}
	sch, err := compileSchema(t.ID, t.Schema)
	if err != nil {
		rec.Error = err.Error()
		return rec
	}
	tools := method == mTool || method == mForced
	system := baseSystem + "\n\n"
	if tools {
		system += toolLine
	} else {
		system += textLine + string(t.Schema)
	}
	req := provider.Request{Model: m.Ref, Thinking: thinking, MaxTokens: a.maxTokens,
		System: []provider.Block{{Text: system}},
		Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText,
			Text: &chat.Text{Text: "Instruction:\n" + strings.TrimSpace(t.Instruction) + "\n\nInput:\n" + t.Input}}}}}}
	if tools {
		req.Tools = []provider.ToolDef{{Name: submitName, Description: "Submits the result. The arguments are the result.", Schema: t.Schema}}
	}
	ctx = withExtra(ctx, fields)
	start := time.Now()
	defer func() { rec.Seconds = time.Since(start).Seconds() }()

	for try := 1; try <= 2; try++ {
		parts, done, err := a.call(ctx, req)
		rec.Requests++
		if done.Usage != nil {
			rec.Prompt += done.Usage.Input + done.Usage.CacheRead + done.Usage.CacheWrite
			rec.Output += done.Usage.Output
		}
		if err != nil {
			rec.Error = a.redact(err.Error())
			var pe *provider.Error
			if errors.As(err, &pe) {
				rec.ErrKind = string(pe.Kind)
			}
			return rec
		}
		c, answer, call := read(t, sch, parts, tools)
		if try == 1 {
			rec.Answer1, rec.Fail1, rec.Valid1 = clip(answer), c.kinds, c.valid()
		} else {
			rec.Answer2, rec.Fail2 = clip(answer), c.kinds
		}
		if c.valid() {
			rec.Valid, rec.Wrapped = true, c.wrapped
			sc := scoreAnswer(t, c.value)
			rec.Score = &sc
			return rec
		}
		// The retry: the answer goes back with what is wrong.
		req.Messages = append(req.Messages, chat.Message{Role: chat.RoleAssistant, Parts: parts})
		why := strings.Join(c.msgs, "\n")
		switch {
		case tools && call != nil:
			res := chat.Message{Role: chat.RoleTool}
			for _, p := range parts {
				if p.ToolCall == nil {
					continue
				}
				text := "Not run: only one submit call is used."
				if p.ToolCall == call {
					text = "The result doesn't match the schema:\n" + why + "\nCall submit again with the corrected result."
				}
				res.Parts = append(res.Parts, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: p.ToolCall.ID, Text: text, IsError: true}})
			}
			req.Messages = append(req.Messages, res)
		case tools:
			req.Messages = append(req.Messages, userText("You didn't call the submit tool. Call it now with your result."))
		default:
			req.Messages = append(req.Messages, userText("Your reply isn't valid:\n"+why+"\nReply again with only the corrected JSON."))
		}
	}
	return rec
}

// read finds the answer in the parts and checks it.
func read(t *Task, sch *jsonschema.Schema, parts []chat.Part, tools bool) (c checked, answer string, call *chat.ToolCall) {
	var text strings.Builder
	for _, p := range parts {
		switch {
		case p.Text != nil:
			text.WriteString(p.Text.Text)
		case p.ToolCall != nil && call == nil && p.ToolCall.Name == submitName:
			call = p.ToolCall
		}
	}
	if tools {
		if call == nil {
			c.add(failNoCall, "no submit call")
			return c, text.String(), nil
		}
		v, err := parseArgs(call.Args)
		if err != nil {
			c.add(failBadJSON, err.Error())
			return c, string(call.Args), call
		}
		return check(t, sch, v), string(call.Args), call
	}
	if strings.TrimSpace(text.String()) == "" {
		c.add(failEmpty, "the reply is empty")
		return c, "", nil
	}
	v, wrapped, err := parseText(text.String())
	if err != nil {
		c.add(failBadJSON, err.Error())
		return c, text.String(), nil
	}
	c = check(t, sch, v)
	c.wrapped = wrapped
	return c, text.String(), nil
}

func userText(s string) chat.Message {
	return chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}}}
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 20000 {
		return string(r[:20000]) + "…"
	}
	return s
}

// call streams one request through the app's registry.
func (a *asker) call(ctx context.Context, req provider.Request) ([]chat.Part, provider.Event, error) {
	var parts []chat.Part
	var done provider.Event
	for ev, err := range a.reg.Stream(ctx, limit.Background, req) {
		if err != nil {
			return parts, done, err
		}
		switch ev.Kind {
		case provider.EventPart:
			parts = append(parts, *ev.Part)
		case provider.EventDone:
			done = ev
		}
	}
	return parts, done, nil
}
