package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// compileSchema compiles an output schema with the library the app uses for
// tool arguments.
func compileSchema(name string, s json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(s))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	url := "result:" + name
	if err := c.AddResource(url, doc); err != nil {
		return nil, err
	}
	return c.Compile(url)
}

// Failure kinds.
const (
	failNoCall   = "no_call"  // a tool method got no submit call
	failBadJSON  = "bad_json" // no JSON value could be read
	failMissing  = "missing_field"
	failType     = "wrong_type"
	failEnum     = "enum"
	failRange    = "out_of_range"
	failExtra    = "extra_field"
	failCount    = "item_count"
	failFormat   = "format" // a pattern, such as a date
	failRepeated = "repeated_index"
	failGap      = "missing_index"
	failEmpty    = "empty_answer"
	failOther    = "other"
)

// checked is one answer after the checks.
type checked struct {
	value   any
	kinds   []string // failure kinds; none means valid
	msgs    []string // for the retry message
	wrapped bool     // text methods: the JSON had text or a fence around it
}

func (c checked) valid() bool { return len(c.kinds) == 0 }

// check reads a value against the task's schema and the checks the schema
// can't express.
func check(t *Task, sch *jsonschema.Schema, v any) checked {
	c := checked{value: v}
	if err := sch.Validate(v); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			for _, leaf := range leaves(ve) {
				c.add(classify(leaf.ErrorKind), leafMessage(leaf))
			}
		} else {
			c.add(failOther, err.Error())
		}
	}
	if !c.valid() {
		return c
	}
	switch t.Kind {
	case "select":
		var seen []int
		for _, p := range v.(map[string]any)["picks"].([]any) {
			i := toInt(p.(map[string]any)["index"])
			if slices.Contains(seen, i) {
				c.add(failRepeated, fmt.Sprintf("index %d is picked twice", i))
			}
			seen = append(seen, i)
		}
	case "decide":
		seen := map[int]bool{}
		for _, a := range v.(map[string]any)["answers"].([]any) {
			i := toInt(a.(map[string]any)["index"])
			if seen[i] {
				c.add(failRepeated, fmt.Sprintf("index %d is answered twice", i))
			}
			seen[i] = true
		}
		for i := range t.Items {
			if !seen[i] {
				c.add(failGap, fmt.Sprintf("message %d has no answer", i))
			}
		}
	}
	return c
}

func (c *checked) add(k, msg string) {
	if !slices.Contains(c.kinds, k) {
		c.kinds = append(c.kinds, k)
	}
	if len(c.msgs) < 8 {
		c.msgs = append(c.msgs, msg)
	}
}

func leaves(e *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return []*jsonschema.ValidationError{e}
	}
	var out []*jsonschema.ValidationError
	for _, c := range e.Causes {
		out = append(out, leaves(c)...)
	}
	return out
}

func leafMessage(e *jsonschema.ValidationError) string {
	return strings.TrimSpace(e.Error()) // "at '/picks/0': missing property 'reason'"
}

func classify(k jsonschema.ErrorKind) string {
	switch k.(type) {
	case *kind.Required:
		return failMissing
	case *kind.Type:
		return failType
	case *kind.Enum, *kind.Const:
		return failEnum
	case *kind.Minimum, *kind.Maximum, *kind.ExclusiveMinimum, *kind.ExclusiveMaximum:
		return failRange
	case *kind.AdditionalProperties, *kind.FalseSchema:
		return failExtra
	case *kind.MinItems, *kind.MaxItems:
		return failCount
	case *kind.Pattern, *kind.Format:
		return failFormat
	}
	return failOther
}

func toInt(v any) int {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case float64:
		return int(n)
	case int:
		return n
	}
	return -1
}

// parseText reads the first JSON value in a model's text answer: plain, in
// a code fence or after a sentence. It must start at the first { or [.
func parseText(s string) (v any, wrapped bool, err error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil, false, errors.New("the answer is empty")
	}
	i := strings.IndexAny(t, "{[")
	if i < 0 {
		return nil, false, errors.New("the answer has no JSON object")
	}
	d := json.NewDecoder(strings.NewReader(t[i:]))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, false, fmt.Errorf("the answer is not valid JSON: %v", err)
	}
	rest := strings.TrimSpace(t[i+int(d.InputOffset()):])
	return v, i > 0 || rest != "", nil
}

// parseArgs reads a tool call's arguments.
func parseArgs(raw json.RawMessage) (any, error) {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}
