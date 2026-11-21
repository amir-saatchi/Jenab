package view

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Step is one of SPEC 10's checks. Views and pages go through 1, 3, 4 and
// 5; 2 and 6–9 are for pipelines (Phase 4).
type Step int

const (
	StepParse      Step = 1 // YAML or JSON, the JSON Schema, and the page checks
	StepReferences Step = 3 // open_form, open_page, view and run_pipeline targets
	StepSchema     Step = 4 // tables and columns of the project database
	StepSQL        Step = 5 // the SQL guard, and fields against result columns
)

// Issue is one error or warning about a config, with where it is: Path is
// a JSON pointer (/columns/2/field), Line and Col its place in the YAML
// (0 when the config has no lines).
type Issue struct {
	Step    Step
	Path    string
	Line    int
	Col     int
	Msg     string
	Warning bool

	schema bool // found by a JSON Schema
}

// Field is the issue's path as SPEC 1 writes it: columns[2].field.
func (i Issue) Field() string { return specPath(i.Path) }

// String is SPEC 10's form: line 4:13 columns[2].field: 'avg' is not a
// result column of the query; it has date, price. A warning starts with
// "warning: ".
func (i Issue) String() string {
	var b strings.Builder
	if i.Warning {
		b.WriteString("warning: ")
	}
	switch {
	case i.Line > 0 && i.Path == "": // a parse error
		fmt.Fprintf(&b, "line %d:%d: ", i.Line, i.Col)
	case i.Line > 0:
		fmt.Fprintf(&b, "line %d:%d %s: ", i.Line, i.Col, i.Field())
	default:
		b.WriteString(i.Field() + ": ")
	}
	b.WriteString(i.Msg)
	return b.String()
}

// Result is what a check found: every error and warning, in the order of
// the steps, and within a step in the order of the config. Warnings don't
// block a save.
type Result struct {
	Issues []Issue
	// MoreErrors and MoreWarnings count the issues left out of Issues:
	// it holds at most MaxIssues of each kind.
	MoreErrors, MoreWarnings int
}

// MaxIssues is the most errors, and apart from them the most warnings, a
// Result lists; the first ones, in SPEC 10's order, are the ones to fix
// first.
const MaxIssues = 50

// limit keeps the first MaxIssues errors and warnings of the sorted
// issues, counting the others.
func (r *Result) limit() {
	var errs, warns int
	r.Issues = slices.DeleteFunc(r.Issues, func(i Issue) bool {
		n := &errs
		if i.Warning {
			n = &warns
		}
		*n++
		return *n > MaxIssues
	})
	r.MoreErrors, r.MoreWarnings = max(errs-MaxIssues, 0), max(warns-MaxIssues, 0)
}

// OK reports whether the config has no errors, so it may be saved.
func (r Result) OK() bool { return len(r.Errors()) == 0 }

// Errors are the issues that block a save.
func (r Result) Errors() []Issue {
	return slices.DeleteFunc(slices.Clone(r.Issues), func(i Issue) bool { return i.Warning })
}

// Warnings are the issues that don't.
func (r Result) Warnings() []Issue {
	return slices.DeleteFunc(slices.Clone(r.Issues), func(i Issue) bool { return !i.Warning })
}

// String lists the issues one a line, errors first, as the tools return
// them to the model.
func (r Result) String() string {
	var b strings.Builder
	errs, warns := r.Errors(), r.Warnings()
	switch n := len(errs) + r.MoreErrors; n {
	case 0:
	case 1:
		b.WriteString("1 error:\n")
	default:
		fmt.Fprintf(&b, "%d errors:\n", n)
	}
	for _, i := range errs {
		b.WriteString(i.String())
		b.WriteByte('\n')
	}
	if r.MoreErrors > 0 {
		fmt.Fprintf(&b, "and %d more %s; fix these first\n", r.MoreErrors, plural(r.MoreErrors, "error", "errors"))
	}
	for _, i := range warns {
		b.WriteString(i.String())
		b.WriteByte('\n')
	}
	if r.MoreWarnings > 0 {
		fmt.Fprintf(&b, "and %d more %s\n", r.MoreWarnings, plural(r.MoreWarnings, "warning", "warnings"))
	}
	return b.String()
}

// sort orders the issues by step, then by line and column, keeping the
// order they were found in otherwise, and drops repeats.
func (r *Result) sort() {
	slices.SortStableFunc(r.Issues, func(a, b Issue) int {
		if c := cmp.Compare(a.Step, b.Step); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		return cmp.Compare(a.Col, b.Col)
	})
	r.Issues = slices.CompactFunc(r.Issues, func(a, b Issue) bool { return a == b })
}

// specPath turns /columns/2/field into columns[2].field (SPEC 1).
func specPath(ptr string) string {
	if ptr == "" {
		return "(top level)"
	}
	var b strings.Builder
	for _, t := range strings.Split(ptr[1:], "/") {
		t = unescape(t)
		if _, err := strconv.Atoi(t); err == nil {
			b.WriteString("[" + t + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(t)
	}
	return b.String()
}

// Ptr joins parts into a JSON pointer: Ptr("columns", 2, "field") is
// /columns/2/field.
func Ptr(parts ...any) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteByte('/')
		switch p := p.(type) {
		case int:
			b.WriteString(strconv.Itoa(p))
		case string:
			b.WriteString(escape(p))
		default:
			b.WriteString(escape(fmt.Sprint(p)))
		}
	}
	return b.String()
}
