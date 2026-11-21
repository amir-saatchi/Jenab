package view

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/store"
)

// Check is what a unit's validator reports through, and the project it
// reads. Paths are JSON pointers relative to the unit's object: "" for the
// object, "/columns/2/field" for a part of it. An issue points at the
// value, or at the key when the value is an object, a list or missing
// (SPEC 10).
//
// The checks of steps 3 to 5 do nothing without an Env.
type Check struct {
	r     *run
	base  string // the unit's object in the config: "" for a view
	view  *View  // the view checked, for a data unit
	block *Block // the block checked, for a page unit
}

// Context is the check's context.
func (c *Check) Context() context.Context { return c.r.ctx }

// View is the common fields of the view checked; nil for a page block.
func (c *Check) View() *View { return c.view }

// Block is the page block checked, its span filled in; nil for a view.
func (c *Check) Block() *Block { return c.block }

// Env reports whether the check has a project to read (steps 3 to 5).
func (c *Check) Env() bool { return c.r.env != nil }

// Errorf reports an error of step at rel.
func (c *Check) Errorf(step Step, rel, format string, a ...any) {
	c.r.add(step, c.base+rel, false, false, fmt.Sprintf(format, a...))
}

// Warnf reports a warning, which doesn't block the save.
func (c *Check) Warnf(step Step, rel, format string, a ...any) {
	c.r.add(step, c.base+rel, false, true, fmt.Sprintf(format, a...))
}

// Columns returns the result columns of the view's query: the SQL check of
// step 5, done once, with its error reported at /query. ok is false when
// the view has no query, there is no Env, or the query failed.
func (c *Check) Columns() (cols []store.QueryColumn, ok bool) {
	if c.view == nil || c.view.Query == "" {
		return nil, false
	}
	return c.Query("/query", c.view.Query)
}

// Query runs the SQL check on another query of the unit, as an options or
// a load query, at rel. Its error is reported once, however often it is
// asked for.
func (c *Check) Query(rel, q string) ([]store.QueryColumn, bool) {
	_, seen := c.r.queries[q]
	p := c.r.prepare(q)
	if p.err == nil {
		return p.cols, true
	}
	if !seen && p.err != errNoEnv {
		c.Errorf(StepSQL, rel, "%s", p.err.Error())
	}
	return nil, false
}

// column returns the result column name of the view's query.
func (c *Check) column(name string) (store.QueryColumn, bool) {
	cols, _ := c.Columns()
	for _, col := range cols {
		if col.Name == name {
			return col, true
		}
	}
	return store.QueryColumn{}, false
}

// given reports whether the config has a value at rel, null too.
func (c *Check) given(rel string) bool {
	_, ok := c.r.doc.val[c.base+rel]
	return ok
}

// Field checks that name, at rel, is a result column of the view's query
// (step 5). An empty name is the schema's to report.
func (c *Check) Field(rel, name string) bool {
	cols, ok := c.Columns()
	if !ok || name == "" {
		return ok
	}
	if _, ok := c.column(name); ok {
		return true
	}
	c.Errorf(StepSQL, rel, "'%s' is not a result column of the query; it has %s%s", name, columnList(cols), suggest(name, names(cols)))
	return false
}

// Format checks a field's format and its options at rel, the object with
// format and format_options: the options fit the format, and an image or
// file format shows a column that holds file keys (step 4).
func (c *Check) Format(rel, field, format string, o *FormatOptions) {
	if o != nil {
		if o.Currency != "" && format != "currency" {
			c.Warnf(StepParse, rel+"/format_options/currency", "only the currency format uses a currency")
		}
		if len(o.Tones) > 0 && format != "badge" {
			c.Warnf(StepParse, rel+"/format_options/tones", "only the badge format uses tones")
		}
		if o.Decimals != nil && format != "number" && format != "currency" && format != "percent" {
			c.Warnf(StepParse, rel+"/format_options/decimals", "only the number, currency and percent formats use decimals")
		}
	}
	if format == "image" || format == "file" {
		c.ObjectColumn(rel+"/format", field, format)
	}
}

// ObjectColumn checks that the result column field comes straight from an
// object_ref column, so it holds file keys (step 4).
func (c *Check) ObjectColumn(rel, field, what string) {
	col, ok := c.column(field)
	if !ok || !c.Env() {
		return
	}
	if col.Table == "" {
		c.Errorf(StepSchema, rel, "the %s format shows file keys, so %s must be an object_ref column selected as it is, not an expression", what, field)
		return
	}
	t := c.r.table(col.Table)
	if t == nil {
		return
	}
	for _, tc := range t.Columns {
		if strings.EqualFold(tc.Name, col.Column) {
			if tc.Kind != "object_ref" {
				c.Errorf(StepSchema, rel, "the %s format shows file keys, but %s.%s is not an object_ref column; mark it object_ref in a migration, or use another format", what, t.Name, tc.Name)
			}
			return
		}
	}
}

// Table returns the agent table name, at rel (step 4). A _jenab_ table, or
// one that isn't there, is an error.
func (c *Check) Table(rel, name string) (*store.Table, bool) {
	if name == "" || !c.Env() {
		return nil, false
	}
	l := strings.ToLower(name)
	if strings.HasPrefix(l, "_jenab_") || strings.HasPrefix(l, "sqlite_") {
		c.Errorf(StepSchema, rel, "%s is one of Jenab's own tables, which configs can't use", name)
		return nil, false
	}
	t := c.r.table(name)
	if t == nil {
		if c.r.err == nil {
			c.Errorf(StepSchema, rel, "no table %s%s", name, suggest(name, c.r.tableNames()))
		}
		return nil, false
	}
	return t, true
}

// Column returns the column name of t, at rel (step 4).
func (c *Check) Column(rel string, t *store.Table, name string) (*store.Column, bool) {
	if t == nil || name == "" {
		return nil, false
	}
	for i := range t.Columns {
		if strings.EqualFold(t.Columns[i].Name, name) {
			return &t.Columns[i], true
		}
	}
	var cols []string
	for _, col := range t.Columns {
		cols = append(cols, col.Name)
	}
	c.Errorf(StepSchema, rel, "%s is not a column of %s; it has %s%s", name, t.Name, strings.Join(cols, ", "), suggest(name, cols))
	return nil, false
}

// Pipeline checks a run_pipeline target at rel (step 3). There are no
// pipelines before Phase 4, so it always fails, saying so.
func (c *Check) Pipeline(rel, id string) {
	c.Errorf(StepReferences, rel, "pipelines can't be saved yet, so there is no pipeline %s to run; remove run_pipeline for now", id)
}

// ViewRef returns the saved view id, at rel (step 3).
func (c *Check) ViewRef(rel, id string) (*View, bool) {
	s := c.ref(rel, store.KindView, id, "view")
	if s == nil {
		return nil, false
	}
	return s.view, s.view != nil
}

// Form checks that id, at rel, is a saved form view (step 3), and returns
// it.
func (c *Check) Form(rel, id string) (*View, bool) {
	s := c.ref(rel, store.KindView, id, "form")
	if s == nil {
		return nil, false
	}
	if s.cfg.Type != "form" {
		c.Errorf(StepReferences, rel, "%s is a %s view, not a form", s.cfg.ID, s.cfg.Type)
		return nil, false
	}
	return s.view, s.view != nil
}

// PageRef checks that id, at rel, is a saved page (step 3).
func (c *Check) PageRef(rel, id string) bool {
	if c.r.kind == store.KindPage && c.r.doc != nil {
		if self, _ := c.r.doc.value.(map[string]any)["id"].(string); store.NormConfigID(self) == store.NormConfigID(id) {
			return true // the page itself
		}
	}
	return c.ref(rel, store.KindPage, id, "page") != nil
}

// ref reads the saved config id of kind, and reports a missing one as a
// what.
func (c *Check) ref(rel string, kind store.ConfigKind, id, what string) *saved {
	if id == "" || !c.Env() {
		return nil
	}
	s := c.r.saved(kind, id)
	if s != nil {
		return s
	}
	if c.r.err != nil {
		return nil
	}
	msg := fmt.Sprintf("no %s %s", what, id)
	if other := c.r.saved(otherKind(kind), id); other != nil {
		msg += fmt.Sprintf("; %s is a %s", other.cfg.ID, other.cfg.Kind)
	} else {
		msg += suggest(id, c.r.idsOf(kind))
	}
	if kind == store.KindView {
		msg += "; save the view first"
	}
	c.Errorf(StepReferences, rel, "%s", msg)
	return nil
}

func otherKind(k store.ConfigKind) store.ConfigKind {
	if k == store.KindPage {
		return store.KindView
	}
	return store.KindPage
}

// Object checks a bucket key at rel: a key that isn't valid is an error,
// and one with no file is a warning, since a pipeline may store it later
// (SPEC 10).
func (c *Check) Object(rel, key string) {
	if key == "" {
		return
	}
	if err := validKey(key); err != nil {
		c.Errorf(StepParse, rel, "is not a bucket key: %s", strings.TrimPrefix(err.Error(), "bucket: "))
		return
	}
	if !c.Env() {
		return
	}
	ok, err := c.r.env.Object(c.r.ctx, key)
	if err != nil {
		c.r.fail(err)
		return
	}
	if !ok {
		c.Warnf(StepSchema, rel, "the bucket has no file %s yet; the image shows as missing until one is stored there", key)
	}
}

// Binds records that the page block binds the query parameters names, so
// a page filter of that name binds to it (SPEC 5.9).
func (c *Check) Binds(names ...string) {
	if c.r.ps == nil {
		return
	}
	for _, n := range names {
		c.r.ps.bound[n] = true
	}
}

// Unbound records that the block binds parameters the check can't know,
// as a view that couldn't be read, so unbound page filters aren't
// reported.
func (c *Check) Unbound() {
	if c.r.ps != nil {
		c.r.ps.unknown = true
	}
}

// PageFilter returns the page's filter of param.
func (c *Check) PageFilter(param string) (Filter, bool) {
	if c.r.ps == nil {
		return Filter{}, false
	}
	i, ok := c.r.ps.filters[param]
	if !ok {
		return Filter{}, false
	}
	p, _ := parsePage(c.r.reg, c.r.doc.value)
	if p == nil || i >= len(p.Filters) {
		return Filter{}, false
	}
	return p.Filters[i], true
}

// ShowFilter records that the block, at rel, shows the page filter param:
// it must be a page filter, shown once (SPEC 5.9).
func (c *Check) ShowFilter(rel, param string) {
	if c.r.ps == nil || param == "" {
		return
	}
	if _, ok := c.r.ps.filters[param]; !ok {
		var have []string
		for p := range c.r.ps.filters {
			have = append(have, p)
		}
		slices.Sort(have)
		msg := fmt.Sprintf("%s is not a page filter", param)
		if len(have) == 0 {
			msg += "; add it to the page's filters first"
		} else {
			msg += "; the page has " + strings.Join(have, ", ") + suggest(param, have)
		}
		c.Errorf(StepParse, rel, "%s", msg)
		return
	}
	if at, dup := c.r.ps.shown[param]; dup {
		c.Errorf(StepParse, rel, "the page filter %s is shown already, at %s; a filter is shown once", param, specPath(at))
		return
	}
	c.r.ps.shown[param] = c.base
}

// Target checks a button's target at rel, the object: one of
// run_pipeline, open_page and open_form, and that it exists.
func (c *Check) Target(rel, what, pipeline, page, form string) {
	c.r.target(c, rel, what, pipeline, page, form)
}

// queryParams are the named parameters of q, without their prefix; nil
// when q doesn't pass the text check.
func queryParams(q string) []string {
	ps, err := paramsOf(q)
	if err != nil {
		return nil
	}
	return ps
}

func names(cols []store.QueryColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
	}
	return out
}

func columnList(cols []store.QueryColumn) string {
	if len(cols) == 0 {
		return "none"
	}
	return strings.Join(names(cols), ", ")
}
