package view

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// The JSON Schemas (SPEC 10, SPIKE-013): 2020-12, checked with
// santhosh-tekuri/jsonschema/v6 with formats asserted. Only the leaf errors
// are kept, and a failed anyOf or oneOf is one error.

//go:embed schema/*.json
var schemaFS embed.FS

// schemaBase is where the schemas' $ids live. Nothing is ever fetched from
// it: every schema is added to the compiler before it compiles.
const schemaBase = "https://jenab.local/schema/"

// DefsURL is the schema of the definitions every unit may $ref, as
// "defs.json#/$defs/filters": ids, labels, formats, tones, filters,
// actions, a block's span and rows of blocks.
const DefsURL = schemaBase + "defs.json"

// schemaFile reads one of the embedded schemas.
func schemaFile(name string) []byte {
	b, err := schemaFS.ReadFile("schema/" + name)
	if err != nil {
		panic(err)
	}
	return b
}

// compileSchema compiles a unit's schema, src, under the $id url, with the
// definitions added.
func compileSchema(url string, src []byte) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	defs, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaFile("defs.json")))
	if err != nil {
		return nil, err
	}
	if err := c.AddResource(DefsURL, defs); err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("view: schema %s: %w", url, err)
	}
	rawSchemas.Store(DefsURL, defs)
	if url != DefsURL {
		if err := c.AddResource(url, doc); err != nil {
			return nil, err
		}
		rawSchemas.Store(url, doc)
	}
	return c.Compile(url)
}

var printer = message.NewPrinter(language.English)

// schemaIssue is one leaf error of a schema check: where, and whether it
// points at the key.
type schemaIssue struct {
	ptr string
	key bool
	msg string
}

// checkSchema checks v against s and returns its leaf errors.
func checkSchema(s *jsonschema.Schema, v any) []schemaIssue {
	err := s.Validate(v)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []schemaIssue{{msg: err.Error()}}
	}
	var out []schemaIssue
	flatten(ve, &out)
	return out
}

// flatten keeps the leaf errors of e. A failed anyOf or oneOf becomes one
// error that lists what each choice wanted.
func flatten(e *jsonschema.ValidationError, out *[]schemaIssue) {
	ptr := pointer(e.InstanceLocation)
	switch k := e.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		var alts []schemaIssue
		for _, c := range e.Causes {
			flatten(c, &alts)
		}
		if len(alts) == 0 { // oneOf matched more than one
			*out = append(*out, schemaIssue{ptr, false, k.LocalizedString(printer)})
			return
		}
		var parts []string
		for _, a := range alts {
			if rel := strings.TrimPrefix(a.ptr, ptr); rel != "" {
				parts = append(parts, specPath(rel)+" "+a.msg)
			} else {
				parts = append(parts, a.msg)
			}
		}
		*out = append(*out, schemaIssue{ptr, false, "must be one of these: " + strings.Join(dedupe(parts), "; or ")})
		return
	case *kind.AdditionalProperties:
		known := knownFields(e.SchemaURL)
		for _, p := range k.Properties {
			*out = append(*out, schemaIssue{ptr + "/" + escape(p), true, "unknown field" + suggest(p, known)})
		}
		return
	case *kind.FalseSchema: // unevaluatedProperties
		*out = append(*out, schemaIssue{ptr, true, "unknown field"})
		return
	case *kind.Required:
		for _, p := range k.Missing {
			*out = append(*out, schemaIssue{ptr + "/" + escape(p), true, "is required"})
		}
		return
	case *kind.Enum:
		want := make([]string, len(k.Want))
		for i, w := range k.Want {
			want[i] = jsonText(w)
		}
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("%s is not one of %s", jsonText(k.Got), strings.Join(want, ", "))})
		return
	case *kind.Const:
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("must be %s, not %s", jsonText(k.Want), jsonText(k.Got))})
		return
	case *kind.Pattern:
		msg := fmt.Sprintf("%q doesn't have the form %s", k.Got, k.Want)
		if k.Want == "^[A-Z]{3}$" {
			msg = fmt.Sprintf("%q is not a currency code: three capital letters, as USD", k.Got)
		}
		*out = append(*out, schemaIssue{ptr, false, msg})
		return
	case *kind.Type:
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("must be %s, not %s", strings.Join(article(k.Want), " or "), article([]string{k.Got})[0])})
		return
	case *kind.MinItems:
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("needs at least %d %s, not %d", k.Want, plural(k.Want, "item", "items"), k.Got)})
		return
	case *kind.MinProperties:
		*out = append(*out, schemaIssue{ptr, true, fmt.Sprintf("needs at least %d %s", k.Want, plural(k.Want, "field", "fields"))})
		return
	case *kind.MaxProperties:
		*out = append(*out, schemaIssue{ptr, true, fmt.Sprintf("has at most %d %s, not %d", k.Want, plural(k.Want, "field", "fields"), k.Got)})
		return
	case *kind.MaxItems:
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("has at most %d %s, not %d", k.Want, plural(k.Want, "item", "items"), k.Got)})
		return
	case *kind.MinLength:
		msg := fmt.Sprintf("needs at least %d characters", k.Want)
		if k.Want == 1 {
			msg = "must not be empty"
		}
		*out = append(*out, schemaIssue{ptr, false, msg})
		return
	case *kind.MaxLength:
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("has %d characters; at most %d are allowed", k.Got, k.Want)})
		return
	case *kind.Minimum:
		got, _ := k.Got.Float64()
		want, _ := k.Want.Float64()
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("%v is too small; the least is %v", got, want)})
		return
	case *kind.Maximum:
		got, _ := k.Got.Float64()
		want, _ := k.Want.Float64()
		*out = append(*out, schemaIssue{ptr, false, fmt.Sprintf("%v is too large; the most is %v", got, want)})
		return
	}
	if len(e.Causes) == 0 {
		*out = append(*out, schemaIssue{ptr, false, e.ErrorKind.LocalizedString(printer)})
		return
	}
	for _, c := range e.Causes {
		flatten(c, out)
	}
}

// jsonText writes a value as a config would: null, not <nil>.
func jsonText(v any) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprint(v)
}

// rawSchemas holds each compiled schema's JSON by its URL, for the field
// names an unknown field is compared with.
var rawSchemas sync.Map

// knownFields returns the fields of the schema object whose
// additionalProperties failed at url, none when it can't tell.
func knownFields(url string) []string {
	base, frag, _ := strings.Cut(url, "#")
	x, ok := rawSchemas.Load(base)
	if !ok {
		return nil
	}
	v := x
	for _, t := range strings.Split(strings.Trim(frag, "/"), "/") {
		if t == "" || t == "additionalProperties" || t == "unevaluatedProperties" {
			continue
		}
		t = unescape(t)
		switch c := v.(type) {
		case map[string]any:
			v = c[t]
		case []any:
			i, err := strconv.Atoi(t)
			if err != nil || i < 0 || i >= len(c) {
				return nil
			}
			v = c[i]
		default:
			return nil
		}
	}
	m, _ := v.(map[string]any)
	props, _ := m["properties"].(map[string]any)
	return slices.Sorted(maps.Keys(props))
}

func pointer(loc []string) string {
	var b strings.Builder
	for _, t := range loc {
		b.WriteString("/" + escape(t))
	}
	return b.String()
}

// article names JSON types as a sentence does: a string, an object.
func article(types []string) []string {
	out := make([]string, len(types))
	for i, t := range types {
		switch t {
		case "object", "array", "integer":
			out[i] = "an " + map[string]string{"object": "object (fields)", "array": "array (a list)", "integer": "integer"}[t]
		case "null":
			out[i] = "null"
		default:
			out[i] = "a " + t
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	out := s[:0]
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// mustCompile is compileSchema for the package's own schemas.
func mustCompile(url string, src []byte) *jsonschema.Schema {
	s, err := compileSchema(url, src)
	if err != nil {
		panic(err)
	}
	return s
}
