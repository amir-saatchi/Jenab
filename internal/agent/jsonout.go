package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// JSON answers by text (SPIKE-030): the schema is in the prompt, the
// answer is read leniently, checked against the schema, and a wrong
// answer gets one retry that lists what was wrong. The same for every
// model (1); no provider's JSON mode is used.

// jsonSchema is a compiled schema with its text for the prompt.
type jsonSchema struct {
	text string
	sch  *jsonschema.Schema
}

// mustSchema compiles a schema of the app's own. It panics on a bad one,
// which is a bug.
func mustSchema(name, text string) jsonSchema {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
	if err != nil {
		panic(fmt.Sprintf("agent: schema %s: %v", name, err))
	}
	c := jsonschema.NewCompiler()
	url := "agent:" + name
	if err := c.AddResource(url, doc); err != nil {
		panic(fmt.Sprintf("agent: schema %s: %v", name, err))
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(text)); err != nil {
		panic(fmt.Sprintf("agent: schema %s: %v", name, err))
	}
	return jsonSchema{text: compact.String(), sch: c.MustCompile(url)}
}

// errNoJSON: the answer has no JSON object.
var errNoJSON = errors.New("the answer has no JSON object")

// maxSchemaErrors is how many errors the retry lists (SPIKE-030).
const maxSchemaErrors = 8

// readJSON finds the first JSON object in a model's answer, past any
// text or code fence around it, checks it against s and decodes it into
// v. Its error lists what was wrong, for the retry.
func readJSON(text string, s jsonSchema, v any) error {
	var doc any
	found := false
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		d := json.NewDecoder(strings.NewReader(text[i:]))
		d.UseNumber()
		var x any
		if d.Decode(&x) == nil {
			if _, ok := x.(map[string]any); ok {
				doc, found = x, true
				break
			}
		}
	}
	if !found {
		return errNoJSON
	}
	if err := s.sch.Validate(doc); err != nil {
		return &schemaError{lines: schemaLines(err)}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// schemaError is an answer that doesn't match its schema.
type schemaError struct{ lines []string }

func (e *schemaError) Error() string {
	return "the answer doesn't match the schema:\n" + strings.Join(e.lines, "\n")
}

var schemaPrinter = message.NewPrinter(language.English)

// schemaLines are a validation error's leaves, at most maxSchemaErrors,
// each with its path.
func schemaLines(err error) []string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []string{"- " + err.Error()}
	}
	var lines []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			lines = append(lines, fmt.Sprintf("- at /%s: %s", strings.Join(e.InstanceLocation, "/"), e.ErrorKind.LocalizedString(schemaPrinter)))
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(lines) > maxSchemaErrors {
		n := len(lines) - maxSchemaErrors
		lines = append(lines[:maxSchemaErrors], fmt.Sprintf("- and %d more", n))
	}
	return lines
}

// fixRequest is the retry's message: what was wrong, and the ask to
// answer again.
func fixRequest(problems []string) string {
	return "Your answer can't be used:\n" + strings.Join(problems, "\n") + "\n\nAnswer again with the whole JSON object only, fixed."
}
