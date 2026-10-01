package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	gjs "github.com/google/jsonschema-go/jsonschema"
	kjs "github.com/kaptinlin/jsonschema"
	sjs "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// VErr is one schema error: JSON pointer of the value, keyword, library message.
type VErr struct {
	Ptr, Keyword, Msg string
}

type validator struct {
	name     string
	validate func(v any) []VErr // v is the JSON-like value from the loader
}

const schemaURL = "https://burrow.local/schema/pipeline.json"

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return b
}

// jsonAny converts through encoding/json, so every library sees float64 numbers.
func jsonAny(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out any
	json.Unmarshal(b, &out)
	return out
}

// ---- santhosh-tekuri/jsonschema/v6 ----

var enPrinter = message.NewPrinter(language.English)

func newSanthosh(schema []byte, assertFormat bool) *sjs.Schema {
	doc, err := sjs.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		panic(err)
	}
	c := sjs.NewCompiler()
	if assertFormat {
		c.AssertFormat()
	}
	for name, check := range customFormats {
		check := check
		c.RegisterFormat(&sjs.Format{Name: name, Validate: func(v any) error {
			s, ok := v.(string)
			if !ok {
				return nil
			}
			if why := check(s); why != "" {
				return errors.New(why)
			}
			return nil
		}})
	}
	if err := c.AddResource(schemaURL, doc); err != nil {
		panic(err)
	}
	return c.MustCompile(schemaURL)
}

func santhoshValidator(schema []byte) validator {
	sch := newSanthosh(schema, true)
	return validator{"santhosh v6", func(v any) []VErr {
		err := sch.Validate(v)
		if err == nil {
			return nil
		}
		var ve *sjs.ValidationError
		if !errors.As(err, &ve) {
			return []VErr{{"", "?", err.Error()}}
		}
		var out []VErr
		flatSanthosh(ve, &out)
		return dedupe(out)
	}}
}

func ptrOf(loc []string) string {
	var sb strings.Builder
	for _, t := range loc {
		sb.WriteString("/" + esc(t))
	}
	return sb.String()
}

func kw(k sjs.ErrorKind) string {
	p := k.KeywordPath()
	if len(p) == 0 {
		return ""
	}
	return p[len(p)-1]
}

// flatSanthosh keeps leaf errors. anyOf/oneOf become one error whose message lists the options.
func flatSanthosh(e *sjs.ValidationError, out *[]VErr) {
	ptr := ptrOf(e.InstanceLocation)
	switch k := e.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		var alts []VErr
		for _, c := range e.Causes {
			flatSanthosh(c, &alts)
		}
		var parts []string
		for _, a := range alts {
			rel := strings.TrimPrefix(a.Ptr, ptr)
			if rel != "" {
				parts = append(parts, specPath(rel)+": "+a.Msg)
			} else {
				parts = append(parts, a.Msg)
			}
		}
		*out = append(*out, VErr{ptr, kw(e.ErrorKind), "must match one of: " + strings.Join(parts, " | ")})
		return
	case *kind.AdditionalProperties:
		for _, p := range k.Properties {
			*out = append(*out, VErr{ptr + "/" + esc(p), "additionalProperties", fmt.Sprintf("unknown field %q", p)})
		}
		return
	case *kind.PropertyNames:
		var why []string
		for _, c := range e.Causes {
			var sub []VErr
			flatSanthosh(c, &sub)
			for _, s := range sub {
				why = append(why, s.Msg)
			}
		}
		*out = append(*out, VErr{ptr + "/" + esc(k.Property), "propertyNames", fmt.Sprintf("invalid name %q: %s", k.Property, strings.Join(why, "; "))})
		return
	}
	if len(e.Causes) == 0 {
		*out = append(*out, VErr{ptr, kw(e.ErrorKind), e.ErrorKind.LocalizedString(enPrinter)})
		return
	}
	for _, c := range e.Causes {
		flatSanthosh(c, out)
	}
}

func dedupe(in []VErr) []VErr {
	seen := map[VErr]bool{}
	var out []VErr
	for _, e := range in {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ptr < out[j].Ptr })
	return out
}

// ---- kaptinlin/jsonschema (v0.7.7, the last release that builds on Go 1.26.0) ----

func newKaptinlin(schema []byte, assertFormat bool) *kjs.Schema {
	c := kjs.NewCompiler()
	c.SetAssertFormat(assertFormat)
	for name, check := range customFormats {
		check := check
		c.RegisterFormat(name, func(v any) bool {
			s, ok := v.(string)
			return !ok || check(s) == ""
		}, "string")
	}
	sch, err := c.Compile(schema)
	if err != nil {
		panic(err)
	}
	return sch
}

var summaryKw = map[string]bool{"properties": true, "items": true, "allOf": true, "$ref": true,
	"then": true, "else": true, "if": true, "prefixItems": true, "additionalProperties": false}

func kaptinlinValidator(schema []byte) validator {
	sch := newKaptinlin(schema, true)
	return validator{"kaptinlin v0.7.7", func(v any) []VErr {
		r := sch.Validate(jsonAny(v))
		if r.IsValid() {
			return nil
		}
		var out []VErr
		flatKaptinlin(r, "", &out)
		return dedupe(out)
	}}
}

// flatKaptinlin walks the result tree. Locations in details are relative to the parent, so they
// are joined. Valid branches and `if` sub-results are skipped; anyOf/oneOf are not expanded.
func flatKaptinlin(r *kjs.EvaluationResult, base string, out *[]VErr) {
	loc := base + r.InstanceLocation
	stop := false
	for k, e := range r.Errors {
		if k == "anyOf" || k == "oneOf" {
			stop = true
		}
		if summaryKw[k] {
			continue
		}
		*out = append(*out, VErr{loc, k, e.Error()})
	}
	if stop {
		return
	}
	for _, d := range r.Details {
		if d.Valid || strings.HasPrefix(d.EvaluationPath, "/if") {
			continue
		}
		flatKaptinlin(d, loc, out)
	}
}

// ---- google/jsonschema-go ----

func newGoogle(schema []byte) *gjs.Resolved {
	var s gjs.Schema
	if err := json.Unmarshal(schema, &s); err != nil {
		panic(err)
	}
	rs, err := s.Resolve(nil)
	if err != nil {
		panic(err)
	}
	return rs
}

func googleValidator(schema []byte) validator {
	rs := newGoogle(schema)
	return validator{"google v0.4.3", func(v any) []VErr {
		err := rs.Validate(jsonAny(v))
		if err == nil {
			return nil
		}
		return []VErr{{"?", "?", err.Error()}}
	}}
}

// ---- positions and messages ----

// posFor picks where an error points: the key for unknown fields, missing fields and errors on
// whole objects or lists; the value otherwise. Unknown pointers fall back to the parent.
func (d *Doc) posFor(ptr, keyword string) Pos {
	for {
		if keyword == "additionalProperties" || keyword == "propertyNames" || keyword == "required" || d.Container[ptr] {
			if p, ok := d.Key[ptr]; ok {
				return p
			}
		}
		if p, ok := d.Val[ptr]; ok {
			return p
		}
		if ptr == "" {
			return Pos{1, 1}
		}
		ptr = ptr[:strings.LastIndex(ptr, "/")]
		keyword = ""
	}
}

// specPath turns /steps/2/with/table into steps[2].with.table (SPEC 1).
func specPath(ptr string) string {
	if ptr == "" {
		return "(top level)"
	}
	var sb strings.Builder
	for _, t := range strings.Split(ptr[1:], "/") {
		t = strings.ReplaceAll(strings.ReplaceAll(t, "~1", "/"), "~0", "~")
		if _, err := strconv.Atoi(t); err == nil {
			sb.WriteString("[" + t + "]")
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString(".")
		}
		sb.WriteString(t)
	}
	return sb.String()
}

// llmLine is the line Burrow would send back to the LLM.
func llmLine(d *Doc, e VErr) string {
	return fmt.Sprintf("line %s %s: %s", d.posFor(e.Ptr, e.Keyword), specPath(e.Ptr), e.Msg)
}
