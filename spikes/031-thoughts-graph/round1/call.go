package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var printer = message.NewPrinter(language.English)

// client calls one model through the app's provider registry, with thinking
// off, as pipeline steps send it.
type client struct {
	reg   *provider.Registry
	model string // provider/id
}

type response struct {
	parts []chat.Part
	usage chat.Usage
	stop  provider.StopReason
}

// errQuota stops every later call to the model.
var errQuota = errors.New("quota used up")

// send sends one request; rate limits, overloads and transport errors are
// tried again, up to 3 tries.
func (c *client) send(ctx context.Context, system string, msgs []chat.Message, tools []provider.ToolDef, maxTokens int) (response, error) {
	req := provider.Request{Model: c.model, System: []provider.Block{{Text: system}}, Messages: msgs, Tools: tools, MaxTokens: maxTokens}
	for try := 0; ; try++ {
		r, err := c.once(ctx, req)
		var pe *provider.Error
		if err == nil || !errors.As(err, &pe) {
			return r, err
		}
		if pe.Kind == provider.Quota {
			return r, fmt.Errorf("%w: %v", errQuota, err)
		}
		if !pe.Retryable() || try == 2 {
			return r, err
		}
		wait := max(pe.RetryAfter, provider.Backoff(try))
		c.logf("%v; next try in %s", err, wait.Round(time.Second))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return r, ctx.Err()
		}
	}
}

func (c *client) once(ctx context.Context, req provider.Request) (response, error) {
	var r response
	for ev, err := range c.reg.Stream(ctx, limit.Background, req) {
		if err != nil {
			return r, err
		}
		switch ev.Kind {
		case provider.EventPart:
			r.parts = append(r.parts, *ev.Part)
		case provider.EventDone:
			if ev.Usage != nil {
				r.usage = *ev.Usage
			}
			r.stop = ev.Stop
		}
	}
	return r, nil
}

// callRec is one call as stored: a define or expand call with its one
// retry, or a conclude or baseline call.
type callRec struct {
	Purpose    string // define, expand, conclude, baseline
	Focus      int
	Tries      int
	Valid      bool
	Failures   []string // no_tool_call, text_json, bad_json, schema, check, max_tokens
	Split      bool     // the thoughts came in more than one tool call
	Err        string
	Usage      chat.Usage
	Ms         int64
	IndexChars int
	Prompt     string
	Reply      string
}

func (c *callRec) add(u chat.Usage) {
	c.Usage.Input += u.Input
	c.Usage.Output += u.Output
	c.Usage.CacheRead += u.CacheRead
	c.Usage.CacheWrite += u.CacheWrite
}

// args are record_thoughts' arguments.
type args struct {
	Thoughts []struct {
		Kind      string  `json:"kind"`
		Title     string  `json:"title"`
		Content   string  `json:"content"`
		Weight    float64 `json:"weight"`
		Rationale string  `json:"rationale"`
		Status    string  `json:"status"`
		MergeWith []int   `json:"merge_with"`
	} `json:"thoughts"`
	Reweight []struct {
		ID     int     `json:"id"`
		Weight float64 `json:"weight"`
	} `json:"reweight"`
	Next string `json:"next"`
}

// thoughts asks for record_thoughts with up to k thoughts. A reply that
// doesn't fit the schema, or that check rejects, gets one retry with the
// error. JSON in the text instead of a call is taken, and counted.
func (c *client) thoughts(ctx context.Context, prompt string, k int, check func(*args) error, rec *callRec) (*args, error) {
	schema := toolSchema(k)
	sch, err := compileSchema(schema)
	if err != nil {
		return nil, err
	}
	tools := []provider.ToolDef{{Name: toolName, Description: toolDescription, Schema: schema}}
	msgs := []chat.Message{userMsg(prompt)}
	rec.Prompt = prompt
	start := time.Now()
	defer func() { rec.Ms = time.Since(start).Milliseconds() }()
	for try := 1; try <= 2; try++ {
		rec.Tries = try
		r, err := c.send(ctx, systemPrompt, msgs, tools, 4000)
		rec.add(r.usage)
		if err != nil {
			rec.Err = err.Error()
			return nil, err
		}
		if r.stop == provider.StopMaxTokens {
			rec.Failures = append(rec.Failures, "max_tokens")
		}
		raw, calls, text := pick(r.parts)
		rec.Reply = text
		if len(calls) > 1 {
			rec.Split = true
		}
		if raw != nil {
			rec.Reply = string(raw)
		}
		if raw == nil {
			if j := jsonIn(text); j != nil {
				raw = j
				rec.Failures = append(rec.Failures, "text_json")
			}
		}
		var fix string
		var a *args
		if raw == nil {
			rec.Failures = append(rec.Failures, "no_tool_call")
			fix = "Answer by calling record_thoughts, with arguments as its schema says."
		} else if a, err = decode(sch, raw); err != nil {
			rec.Failures = append(rec.Failures, kindOf(err))
			fix = "Not recorded: " + err.Error() + "\nCall record_thoughts again with all the thoughts, fixed."
		} else if err = check(a); err != nil {
			rec.Failures = append(rec.Failures, "check")
			fix = "Not recorded: " + err.Error() + "\nCall record_thoughts again with all the thoughts, fixed."
		} else {
			rec.Valid = true
			return a, nil
		}
		if try == 2 {
			rec.Err = fix
			return nil, errors.New(fix)
		}
		if len(r.parts) == 0 {
			continue // nothing to answer: the same request again
		}
		msgs = append(msgs, chat.Message{Role: chat.RoleAssistant, Parts: r.parts})
		if len(calls) > 0 {
			var parts []chat.Part
			for _, id := range calls {
				parts = append(parts, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: id, Text: fix, IsError: true}})
			}
			msgs = append(msgs, chat.Message{Role: chat.RoleTool, Parts: parts})
		} else {
			msgs = append(msgs, userMsg(fix))
		}
	}
	panic("unreachable")
}

// text asks for a plain answer.
func (c *client) text(ctx context.Context, system, prompt string, rec *callRec) (string, error) {
	rec.Prompt, rec.Tries = prompt, 1
	start := time.Now()
	r, err := c.send(ctx, system, []chat.Message{userMsg(prompt)}, nil, 4000)
	rec.Ms = time.Since(start).Milliseconds()
	rec.add(r.usage)
	if err != nil {
		rec.Err = err.Error()
		return "", err
	}
	if r.stop == provider.StopMaxTokens {
		rec.Failures = append(rec.Failures, "max_tokens")
	}
	_, _, text := pick(r.parts)
	rec.Reply, rec.Valid = text, strings.TrimSpace(text) != ""
	return text, nil
}

func userMsg(s string) chat.Message {
	return chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}}}
}

// pick returns the record_thoughts arguments, merged when the model split
// them over several calls, the IDs of all tool calls, and the text.
func pick(parts []chat.Part) (json.RawMessage, []string, string) {
	var text strings.Builder
	var ids []string
	var all []json.RawMessage
	for _, p := range parts {
		switch p.Kind {
		case chat.PartText:
			text.WriteString(p.Text.Text)
		case chat.PartToolCall:
			ids = append(ids, p.ToolCall.ID)
			if p.ToolCall.Name == toolName {
				all = append(all, p.ToolCall.Args)
			}
		}
	}
	switch len(all) {
	case 0:
		return nil, ids, text.String()
	case 1:
		return all[0], ids, text.String()
	}
	merged := map[string]any{"thoughts": []any{}, "reweight": []any{}}
	for _, raw := range all {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			return raw, ids, text.String() // the schema check reports it
		}
		for _, key := range []string{"thoughts", "reweight"} {
			if xs, ok := m[key].([]any); ok {
				merged[key] = append(merged[key].([]any), xs...)
			}
		}
		if n, ok := m["next"]; ok {
			merged["next"] = n
		}
	}
	b, _ := json.Marshal(merged)
	return b, ids, text.String()
}

// jsonIn is the first JSON object in s, also inside a code fence or after
// a sentence.
func jsonIn(s string) json.RawMessage {
	for i := strings.IndexByte(s, '{'); i >= 0; {
		var v json.RawMessage
		if json.NewDecoder(strings.NewReader(s[i:])).Decode(&v) == nil {
			return v
		}
		j := strings.IndexByte(s[i+1:], '{')
		if j < 0 {
			break
		}
		i += 1 + j
	}
	return nil
}

func compileSchema(s json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(s))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("tool:"+toolName, doc); err != nil {
		return nil, err
	}
	return c.Compile("tool:" + toolName)
}

type decodeError struct {
	kind string
	msg  string
}

func (e *decodeError) Error() string { return e.msg }

func kindOf(err error) string {
	var de *decodeError
	if errors.As(err, &de) {
		return de.kind
	}
	return "schema"
}

func decode(sch *jsonschema.Schema, raw json.RawMessage) (*args, error) {
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, &decodeError{"bad_json", "the arguments are not valid JSON: " + err.Error()}
	}
	if err := sch.Validate(v); err != nil {
		return nil, &decodeError{"schema", "the arguments don't match the schema:" + schemaErrors(err)}
	}
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, &decodeError{"schema", "the arguments don't match the schema: " + err.Error()}
	}
	return &a, nil
}

func schemaErrors(err error) string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return " " + err.Error()
	}
	var lines []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			lines = append(lines, fmt.Sprintf("\n- at /%s: %s", strings.Join(e.InstanceLocation, "/"), e.ErrorKind.LocalizedString(printer)))
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return strings.Join(lines, "")
}
