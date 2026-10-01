package main

// JSON Schema validation (SPEC 10 step 1, SPIKE-013 approach): santhosh-tekuri/jsonschema/v6,
// AssertFormat on, custom formats, leaf errors only, a failed anyOf/oneOf as one error, errors
// mapped to line:column and SPEC paths. Every issue carries an error category for the report.

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	sjs "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Error categories used in results.md.
const (
	CatYAML     = "yaml"
	CatSchema   = "schema"
	CatUnknown  = "unknown field"
	CatStepType = "wrong step type"
	CatExpr     = "bad expression"
	CatSQL      = "bad SQL"
	CatRefs     = "bad refs"
	CatSemantic = "semantic"
	CatDepend   = "dependent not updated"
)

var allCats = []string{CatYAML, CatSchema, CatUnknown, CatStepType, CatExpr, CatSQL, CatRefs, CatSemantic, CatDepend}

type Issue struct {
	Ptr    string // JSON pointer inside the config
	Pos    Pos    // line:column, zero when the config came as JSON tool arguments
	Cat    string
	Msg    string
	Warn   bool
	Prefix string // e.g. "views[1]: " for dependents sent with a migration
}

func (i Issue) String() string {
	var sb strings.Builder
	sb.WriteString(i.Prefix)
	if i.Warn {
		sb.WriteString("warning: ")
	}
	if i.Pos.Line > 0 {
		fmt.Fprintf(&sb, "line %s ", i.Pos)
	}
	sb.WriteString(specPath(i.Ptr))
	sb.WriteString(": ")
	sb.WriteString(i.Msg)
	return sb.String()
}

// issues collects issues for one config document.
type issues struct {
	doc  *Doc
	list []Issue
}

func (is *issues) add(ptr, cat, format string, a ...any) {
	is.list = append(is.list, Issue{Ptr: ptr, Pos: is.doc.posFor(ptr, false), Cat: cat, Msg: fmt.Sprintf(format, a...)})
}

func (is *issues) addKey(ptr, cat, format string, a ...any) {
	is.list = append(is.list, Issue{Ptr: ptr, Pos: is.doc.posFor(ptr, true), Cat: cat, Msg: fmt.Sprintf(format, a...)})
}

func (is *issues) warn(ptr, format string, a ...any) {
	is.list = append(is.list, Issue{Ptr: ptr, Pos: is.doc.posFor(ptr, false), Cat: CatSemantic, Msg: fmt.Sprintf(format, a...), Warn: true})
}

func (is *issues) errCount() int {
	n := 0
	for _, i := range is.list {
		if !i.Warn {
			n++
		}
	}
	return n
}

//go:embed schemas/*.json
var schemaFS embed.FS

var (
	pipelineSchema, viewSchema, migrationSchema *sjs.Schema
	enPrinter                                   = message.NewPrinter(language.English)
)

func compileSchema(file string) *sjs.Schema {
	b, err := schemaFS.ReadFile("schemas/" + file)
	if err != nil {
		panic(err)
	}
	doc, err := sjs.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		panic(file + ": " + err.Error())
	}
	c := sjs.NewCompiler()
	c.AssertFormat()
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
	url := "https://burrow.local/schema/" + file
	if err := c.AddResource(url, doc); err != nil {
		panic(err)
	}
	s, err := c.Compile(url)
	if err != nil {
		panic(file + ": " + err.Error())
	}
	return s
}

func initSchemas() {
	pipelineSchema = compileSchema("pipeline.schema.json")
	viewSchema = compileSchema("view.schema.json")
	migrationSchema = compileSchema("migration.schema.json")
}

// jsonAny converts through encoding/json, so the validator sees float64/json.Number consistently.
func jsonAny(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out any
	d.Decode(&out)
	return out
}

type vErr struct{ Ptr, Keyword, Msg string }

// schemaIssues validates v and returns issues. typeField names the discriminator whose enum
// error counts as "wrong step type" (use, type, op).
func schemaIssues(sch *sjs.Schema, v any, is *issues) {
	err := sch.Validate(jsonAny(v))
	if err == nil {
		return
	}
	var ve *sjs.ValidationError
	if !errors.As(err, &ve) {
		is.add("", CatSchema, "%s", err.Error())
		return
	}
	var out []vErr
	flatSanthosh(ve, &out)
	seen := map[vErr]bool{}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ptr < out[j].Ptr })
	for _, e := range out {
		if seen[e] {
			continue
		}
		seen[e] = true
		cat := CatSchema
		last := e.Ptr[strings.LastIndex(e.Ptr, "/")+1:]
		switch {
		case e.Keyword == "additionalProperties":
			cat = CatUnknown
		case e.Keyword == "enum" && (last == "use" || last == "op" || (last == "type" && strings.Count(e.Ptr, "/") == 1)):
			cat = CatStepType
		}
		atKey := e.Keyword == "additionalProperties" || e.Keyword == "required"
		is.list = append(is.list, Issue{Ptr: e.Ptr, Pos: is.doc.posFor(e.Ptr, atKey), Cat: cat, Msg: e.Msg})
	}
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

func flatSanthosh(e *sjs.ValidationError, out *[]vErr) {
	ptr := ptrOf(e.InstanceLocation)
	switch k := e.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		var alts []vErr
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
		msg := "must match one of: " + strings.Join(parts, " | ")
		if _, ok := k.(*kind.OneOf); ok && len(alts) == 0 {
			msg = "must match exactly one of the allowed forms, but matches more than one"
		}
		*out = append(*out, vErr{ptr, kw(e.ErrorKind), msg})
		return
	case *kind.AdditionalProperties:
		for _, p := range k.Properties {
			*out = append(*out, vErr{ptr + "/" + esc(p), "additionalProperties", fmt.Sprintf("unknown field %q", p)})
		}
		return
	case *kind.Required:
		for _, p := range k.Missing {
			*out = append(*out, vErr{ptr, "required", fmt.Sprintf("missing required field %q", p)})
		}
		return
	}
	if len(e.Causes) == 0 {
		*out = append(*out, vErr{ptr, kw(e.ErrorKind), e.ErrorKind.LocalizedString(enPrinter)})
		return
	}
	for _, c := range e.Causes {
		flatSanthosh(c, out)
	}
}

// ---- small helpers for walking loosely typed values ----

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asStr(v any) string {
	s, _ := v.(string)
	return s
}

func strList(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// didYouMean suggests the closest name.
func didYouMean(name string, cands []string) string {
	best, bd := "", 1<<30
	for _, c := range cands {
		d := lev(strings.ToLower(name), strings.ToLower(c))
		if d < bd {
			best, bd = c, d
		}
	}
	if best != "" && (bd <= 3 || strings.Contains(best, name) || strings.Contains(name, best)) {
		return fmt.Sprintf(" (did you mean %q?)", best)
	}
	return ""
}

func lev(a, b string) int {
	d := make([]int, len(b)+1)
	for j := range d {
		d[j] = j
	}
	for i := 1; i <= len(a); i++ {
		prev := d[0]
		d[0] = i
		for j := 1; j <= len(b); j++ {
			cur := d[j]
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[j] = min(d[j]+1, d[j-1]+1, prev+cost)
			prev = cur
		}
	}
	return d[len(b)]
}
