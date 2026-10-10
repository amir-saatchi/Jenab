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

// client calls one model through the app's provider registry.
type client struct {
	reg   *provider.Registry
	model string // provider/id
}

type response struct {
	text     string
	thinking int // characters of thinking that came back
	usage    chat.Usage
	stop     provider.StopReason
	parts    []chat.Part
}

// errQuota stops every later call to the model.
var errQuota = errors.New("quota used up")

// send sends one request; rate limits, overloads and transport errors are
// tried again, up to 3 tries.
func (c *client) send(ctx context.Context, system string, msgs []chat.Message, thinking bool, maxTokens int) (response, error) {
	req := provider.Request{Model: c.model, System: []provider.Block{{Text: system}}, Messages: msgs, MaxTokens: maxTokens, Thinking: thinking}
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
	var text strings.Builder
	for ev, err := range c.reg.Stream(ctx, limit.Background, req) {
		if err != nil {
			return r, err
		}
		switch ev.Kind {
		case provider.EventPart:
			r.parts = append(r.parts, *ev.Part)
			switch ev.Part.Kind {
			case chat.PartText:
				text.WriteString(ev.Part.Text.Text)
			case chat.PartThinking:
				r.thinking += len(ev.Part.Thinking.Text)
			}
		case provider.EventDone:
			if ev.Usage != nil {
				r.usage = *ev.Usage
			}
			r.stop = ev.Stop
		}
	}
	r.text = text.String()
	return r, nil
}

// callRec is one call as stored, with its one retry.
type callRec struct {
	Purpose    string // define, expand, conclude
	Focus      int
	Tries      int
	Valid      bool
	Failures   []string // no_json, schema, check, max_tokens
	Err        string
	Usage      chat.Usage
	Thinking   int
	Ms         int64
	IndexChars int
	Prompt     string
	Reply      string
}

func (c *callRec) add(r response) {
	c.Usage.Input += r.usage.Input
	c.Usage.Output += r.usage.Output
	c.Usage.CacheRead += r.usage.CacheRead
	c.Usage.CacheWrite += r.usage.CacheWrite
	c.Usage.Thought += r.usage.Thought
	c.Thinking += r.thinking
}

// askJSON asks for one JSON object that matches schema. accept decodes it
// and makes the checks the schema can't; its error, like a schema error,
// goes back to the model once.
func (c *client) askJSON(ctx context.Context, system, prompt string, schema json.RawMessage, thinking bool, maxTokens int, accept func(json.RawMessage) error, rec *callRec) error {
	sch, err := compileSchema(schema)
	if err != nil {
		return err
	}
	msgs := []chat.Message{userMsg(prompt)}
	rec.Prompt = prompt
	start := time.Now()
	defer func() { rec.Ms = time.Since(start).Milliseconds() }()
	for try := 1; try <= 2; try++ {
		rec.Tries = try
		r, err := c.send(ctx, system, msgs, thinking, maxTokens)
		rec.add(r)
		if err != nil {
			rec.Err = err.Error()
			return err
		}
		rec.Reply = r.text
		if r.stop == provider.StopMaxTokens {
			rec.Failures = append(rec.Failures, "max_tokens")
		}
		var fix string
		raw := jsonIn(r.text)
		switch {
		case raw == nil:
			rec.Failures = append(rec.Failures, "no_json")
			fix = "Your answer had no JSON object. Answer with exactly one JSON object that matches the schema, and nothing else."
		case validate(sch, raw) != nil:
			rec.Failures = append(rec.Failures, "schema")
			fix = "The JSON doesn't match the schema:" + validate(sch, raw).Error() + "\nAnswer again with the whole JSON object, fixed."
		default:
			if err := accept(raw); err != nil {
				rec.Failures = append(rec.Failures, "check")
				fix = "Not accepted: " + err.Error() + "\nAnswer again with the whole JSON object, fixed."
			} else {
				rec.Valid = true
				return nil
			}
		}
		if try == 2 {
			rec.Err = fix
			return errors.New(fix)
		}
		if strings.TrimSpace(r.text) != "" {
			msgs = append(msgs, chat.Message{Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: r.text}}}}, userMsg(fix))
		}
	}
	panic("unreachable")
}

// text asks for a plain answer.
func (c *client) text(ctx context.Context, system, prompt string, thinking bool, maxTokens int, rec *callRec) (string, error) {
	rec.Prompt, rec.Tries = prompt, 1
	start := time.Now()
	r, err := c.send(ctx, system, []chat.Message{userMsg(prompt)}, thinking, maxTokens)
	rec.Ms = time.Since(start).Milliseconds()
	rec.add(r)
	if err != nil {
		rec.Err = err.Error()
		return "", err
	}
	if r.stop == provider.StopMaxTokens {
		rec.Failures = append(rec.Failures, "max_tokens")
	}
	rec.Reply, rec.Valid = r.text, strings.TrimSpace(r.text) != ""
	if !rec.Valid {
		rec.Err = "empty answer"
		return "", errors.New("empty answer")
	}
	return r.text, nil
}

func userMsg(s string) chat.Message {
	return chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}}}
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
	if err := c.AddResource("answer.json", doc); err != nil {
		return nil, err
	}
	return c.Compile("answer.json")
}

// validate lists at most 8 schema errors, with their paths.
func validate(sch *jsonschema.Schema, raw json.RawMessage) error {
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf(" %v", err)
	}
	err = sch.Validate(v)
	var ve *jsonschema.ValidationError
	if err == nil || !errors.As(err, &ve) {
		return err
	}
	var lines []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 && len(lines) < 8 {
			lines = append(lines, fmt.Sprintf("\n- at /%s: %s", strings.Join(e.InstanceLocation, "/"), e.ErrorKind.LocalizedString(printer)))
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return errors.New(strings.Join(lines, ""))
}
