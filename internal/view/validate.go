package view

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validator checks views and pages before they are saved (SPEC 10): the
// parse and the JSON Schema with the page checks (step 1), references
// (3), the project's tables (4) and the SQL (5). Steps 2 and 6 to 9 are
// for pipelines. Every check runs and every issue comes back, so the model
// can fix them all in one try. The checks make no network or model call,
// and they are the same for every model.
type Validator struct {
	Reg *Registry // nil for Default
	Env Env       // nil skips steps 3 to 5
}

// View checks a view. The view is nil when the config doesn't load; err
// is for a failure of the project (a closed database, a cancelled ctx), not
// of the config.
func (v *Validator) View(ctx context.Context, src []byte) (*View, Result, error) {
	r := v.newRun(ctx, store.KindView)
	d, ok := r.load(src)
	if !ok {
		return nil, r.result(), nil
	}
	out := r.view(d.value)
	return out, r.result(), r.err
}

// Page checks a page.
func (v *Validator) Page(ctx context.Context, src []byte) (*Page, Result, error) {
	r := v.newRun(ctx, store.KindPage)
	d, ok := r.load(src)
	if !ok {
		return nil, r.result(), nil
	}
	out := r.page(d.value)
	return out, r.result(), r.err
}

// Config checks a view or a page: a config with no type is a page, unless
// it has a query or fields and no rows. parsed is a *View or a *Page; kind is "" when the config doesn't
// load.
func (v *Validator) Config(ctx context.Context, src []byte) (kind store.ConfigKind, parsed any, res Result, err error) {
	r := v.newRun(ctx, "")
	d, ok := r.load(src)
	if !ok {
		return "", nil, r.result(), nil
	}
	// A config without a type is a page when it has rows, or when it has
	// nothing only a view has, as a page with its rows misspelled.
	m := d.value.(map[string]any)
	_, rows := m["rows"]
	_, typ := m["type"]
	_, query := m["query"]
	_, fields := m["fields"]
	if !typ && (rows || !query && !fields) {
		r.kind = store.KindPage
		p := r.page(d.value)
		return store.KindPage, p, r.result(), r.err
	}
	r.kind = store.KindView
	vw := r.view(d.value)
	return store.KindView, vw, r.result(), r.err
}

// run is one check of one config.
type run struct {
	ctx  context.Context
	reg  *Registry
	env  Env
	kind store.ConfigKind
	doc  *doc
	res  Result
	err  error // the first failure of the project; the check goes on without it

	schemaPaths []string // where the schemas found errors; Go's issues under them are dropped

	configs map[string]*saved
	ids     map[store.ConfigKind][]string
	tables  map[string]*store.Table // lower-case name → nil for none
	names   []string
	queries map[string]*prepared

	ps *pageState
}

// saved is a saved config as the checks see it.
type saved struct {
	cfg  store.Config
	view *View // a view's parse; nil when it doesn't parse
}

// prepared is a query's SQL check.
type prepared struct {
	cols []store.QueryColumn
	err  error // nil, or an issue's message as an error
}

func (v *Validator) newRun(ctx context.Context, kind store.ConfigKind) *run {
	reg := v.Reg
	if reg == nil {
		reg = Default
	}
	return &run{ctx: ctx, reg: reg, env: v.Env, kind: kind,
		configs: map[string]*saved{}, ids: map[store.ConfigKind][]string{},
		tables: map[string]*store.Table{}, queries: map[string]*prepared{}}
}

func (r *run) load(src []byte) (*doc, bool) {
	d, err := load(src)
	if err != nil {
		var le *loadError
		if errors.As(err, &le) {
			r.res.Issues = append(r.res.Issues, Issue{Step: StepParse, Line: le.at.line, Col: le.at.col, Msg: le.msg})
		} else {
			r.res.Issues = append(r.res.Issues, Issue{Step: StepParse, Msg: err.Error()})
		}
		return nil, false
	}
	// The ID is checked as it is saved, in NFC (store.NormConfigID), by
	// the schema's maxLength too.
	if m, ok := d.value.(map[string]any); ok {
		if id, ok := m["id"].(string); ok {
			m["id"] = store.NormConfigID(id)
		}
	}
	r.doc = d
	return d, true
}

// result drops the issues Go found where a schema found one already, and
// sorts.
func (r *run) result() Result {
	out := r.res
	out.Issues = nil
	for _, i := range r.res.Issues {
		if !i.schema && r.underSchema(i.Path) {
			continue
		}
		out.Issues = append(out.Issues, i)
	}
	out.sort()
	out.limit()
	for k := range out.Issues {
		out.Issues[k].schema = false
	}
	return out
}

func (r *run) underSchema(p string) bool {
	for _, s := range r.schemaPaths {
		if p == s || strings.HasPrefix(p, s+"/") {
			return true
		}
	}
	return false
}

func (r *run) add(step Step, ptr string, key, warning bool, msg string) {
	at := r.doc.at(ptr, key)
	r.res.Issues = append(r.res.Issues, Issue{Step: step, Path: ptr, Line: at.line, Col: at.col, Msg: msg, Warning: warning})
}

// schema checks v, at base in the config, against s.
func (r *run) schema(s *jsonschema.Schema, v any, base string) bool {
	issues := checkSchema(s, v)
	for _, i := range issues {
		p := base + i.ptr
		at := r.doc.at(p, i.key)
		r.res.Issues = append(r.res.Issues, Issue{Step: StepParse, Path: p, Line: at.line, Col: at.col, Msg: i.msg, schema: true})
		r.schemaPaths = append(r.schemaPaths, p)
	}
	return len(issues) == 0
}

// fail keeps the first failure of the project.
func (r *run) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// view checks the loaded view v.
func (r *run) view(v any) *View {
	m := v.(map[string]any)
	typ, _ := m["type"].(string)
	u := r.reg.unit(Data, typ)
	if u == nil {
		r.schema(viewSchema(), v, "")
		if _, ok := m["type"]; !ok {
			msg := "is required: a view has a type, one of " + strings.Join(r.reg.Types(Data), ", ")
			if _, ok := m["rows"]; ok {
				msg += "; this config has rows, so it looks like a page: save it as a page"
			}
			for i := range r.res.Issues {
				if is := &r.res.Issues[i]; is.Path == "/type" && is.Msg == "is required" {
					is.Msg = msg
				}
			}
		}
		if _, ok := m["type"].(string); ok {
			r.add(StepParse, "/type", false, false, fmt.Sprintf("unknown view type %q; use one of %s", typ, strings.Join(r.reg.Types(Data), ", ")))
		}
		out, _ := parseView(r.reg, v)
		return out
	}
	r.schema(u.schema, v, "")
	out, _ := parseView(r.reg, v)
	c := &Check{r: r, view: out}
	r.commonView(c, out)
	if out.Spec != nil {
		u.check(c, out.Spec)
	}
	return out
}

// viewSchema is the schema of the common fields, for a view whose type
// isn't known.
var viewSchema = sync.OnceValue(func() *jsonschema.Schema {
	return mustCompile(schemaBase+"any-view.json", []byte(`{"$ref": "defs.json#/$defs/view"}`))
})

// commonView checks the fields every view has (SPEC 5.1, 5.2, 5.6).
func (r *run) commonView(c *Check, v *View) {
	r.checkID(v.ID)
	params := map[string]bool{}
	for name := range v.Params {
		params[name] = true
		if !paramName(name) {
			c.Errorf(StepParse, Ptr("params", name), "%q is not a parameter name: a letter, then letters, digits or _", name)
		}
	}
	r.checkFilters(c, "/filters", v.Filters, params)
	for i, a := range v.Actions {
		if a.RunPipeline != "" {
			c.Pipeline(Ptr("actions", i, "run_pipeline"), a.RunPipeline)
		}
	}
	if v.Query == "" {
		return
	}
	c.Columns()
	r.bindings(c, v)
}

// checkID checks the config's id, and that the other kind doesn't have it.
func (r *run) checkID(id string) {
	if id == "" {
		return
	}
	if err := store.CheckConfigID(id); err != nil {
		r.add(StepParse, "/id", false, false, fmt.Sprintf("an ID is 1 to %d letters, digits, _ or -, starting with a letter or digit", store.MaxConfigID))
		return
	}
	other := store.KindPage
	if r.kind == store.KindPage {
		other = store.KindView
	}
	if s := r.saved(other, id); s != nil {
		r.add(StepReferences, "/id", false, false, fmt.Sprintf("a %s has the ID %s; views and pages share their IDs, so choose another", other, s.cfg.ID))
	}
}

// bindings checks the query's parameters against params and filters.
func (r *run) bindings(c *Check, v *View) {
	ps, ok := namedParams(c, "/query", v.Query, "use named parameters such as :from_date, not ?, so params and filters can bind them")
	if !ok {
		return // the SQL check reported it
	}
	used := map[string]bool{}
	bound := map[string]bool{}
	for name := range v.Params {
		bound[name] = true
	}
	for _, f := range v.Filters {
		bound[f.Param] = true
	}
	for _, name := range ps {
		used[name] = true
		if !bound[name] {
			c.Warnf(StepSQL, "/query", ":%s is not in params or filters, so it is NULL unless a page filter binds it", name)
		}
	}
	for name := range v.Params {
		if !used[name] {
			c.Warnf(StepSQL, Ptr("params", name), "the query has no :%s", name)
		}
	}
	for i, f := range v.Filters {
		if f.Param != "" && !used[f.Param] {
			c.Warnf(StepSQL, Ptr("filters", i, "param"), "the query has no :%s", f.Param)
		}
	}
}

// checkFilters checks a view's or a page's filters (SPEC 5.2). params are
// the names in params.
func (r *run) checkFilters(c *Check, base string, fs []Filter, params map[string]bool) {
	seen := map[string]int{}
	for i, f := range fs {
		at := func(field string) string { return base + Ptr(i, field) }
		if f.Param != "" {
			switch {
			case !paramName(f.Param):
				c.Errorf(StepParse, at("param"), "%q is not a parameter name: a letter, then letters, digits or _", f.Param)
			case params[f.Param]:
				c.Errorf(StepParse, at("param"), "%s is in params too; a parameter is in params or in filters, not both", f.Param)
			}
			if j, dup := seen[f.Param]; dup {
				c.Errorf(StepParse, at("param"), "filters[%d] has this parameter already; a parameter has one filter", j)
			} else {
				seen[f.Param] = i
			}
		}
		checkOptions(c, base+Ptr(i), f.Control, f.Options)
		if f.Default != nil {
			checkDefault(c, at("default"), f.Control, f.Default, f.Options)
		}
	}
}

// checkOptions checks a select's options: only a select has them, and it
// must. An options query returns one column and takes no parameters.
func checkOptions(c *Check, base, control string, o *Options) {
	switch {
	case control == "select" && o == nil:
		c.Errorf(StepParse, base+"/control", "a select needs options: a list, or { query: \"SELECT ...\" } that returns one column")
	case control != "select" && control != "" && o != nil:
		c.Errorf(StepParse, base+"/options", "only a select has options; this control is %s", control)
	case o != nil && o.Query != "":
		cols, ok := c.Query(base+"/options/query", o.Query)
		if ok && len(cols) != 1 {
			c.Errorf(StepSQL, base+"/options/query", "an options query returns one column; this one returns %d (%s)", len(cols), columnList(cols))
		}
		// Nothing binds an options query's parameters: they would be NULL.
		if ps, err := sqlguard.Params(o.Query); err == nil && len(ps) > 0 {
			name := ps[0].Name
			if name == "" {
				name = "?"
			}
			c.Errorf(StepSQL, base+"/options/query", "an options query takes no parameters, so %s would always be NULL; write the values into the query", name)
		}
	}
}

// checkDefault checks a filter's or a form field's default against its
// control.
func checkDefault(c *Check, ptr, control string, def any, o *Options) {
	switch control {
	case "date":
		s, ok := def.(string)
		if !ok || !DateDefault(s) {
			c.Errorf(StepParse, ptr, "is not a date: use an ISO date (2026-11-20) or today, today-7d, today-3m, today-1y, start_of_month or start_of_year")
		}
	case "number":
		switch def.(type) {
		case int64, float64:
		default:
			c.Errorf(StepParse, ptr, "must be a number")
		}
	case "checkbox":
		if _, ok := def.(bool); !ok {
			c.Errorf(StepParse, ptr, "must be true or false")
		}
	case "select":
		if o != nil && o.Query == "" && len(o.List) > 0 {
			for _, x := range o.List {
				if fmt.Sprint(x) == fmt.Sprint(def) {
					return
				}
			}
			c.Errorf(StepParse, ptr, "%v is not one of the options", def)
		}
	case "file":
		c.Errorf(StepParse, ptr, "a file field has no default")
	}
}

// page checks the loaded page v.
func (r *run) page(v any) *Page {
	r.schema(pageSchema(), v, "")
	out, _ := parsePage(r.reg, v)
	r.ps = &pageState{filters: map[string]int{}, shown: map[string]string{}, bound: map[string]bool{}}
	c := &Check{r: r}
	r.checkID(out.ID)
	for i, f := range out.Filters {
		if _, dup := r.ps.filters[f.Param]; !dup && f.Param != "" {
			r.ps.filters[f.Param] = i
		}
	}
	r.checkFilters(c, "/filters", out.Filters, nil)
	for i, a := range out.Actions {
		r.target(c, Ptr("actions", i), "a page action", a.RunPipeline, a.OpenPage, a.OpenForm)
	}
	m := v.(map[string]any)
	r.rows(m["rows"], "", "")
	// Without rows, the schema's error is the one to fix first.
	if _, rows := m["rows"].([]any); rows && r.env != nil && !r.ps.unknown {
		for i, f := range out.Filters {
			if f.Param != "" && !r.ps.bound[f.Param] {
				c.Errorf(StepParse, Ptr("filters", i, "param"), "binds to no block: no view on this page has the parameter :%s", f.Param)
			}
		}
	}
	return out
}

var pageSchema = sync.OnceValue(func() *jsonschema.Schema {
	return mustCompile(schemaBase+"page.json", schemaFile("page.json"))
})

// pageState is what the page checks gather across its blocks.
type pageState struct {
	filters map[string]int    // page filter param → its index
	shown   map[string]string // page filter param → the filter block that shows it
	bound   map[string]bool   // parameters some block binds
	unknown bool              // a view that couldn't be read: binding can't be checked
}

// rows checks rows of blocks at base; in is the container they are in,
// "" for the page.
func (r *run) rows(v any, base, in string) {
	rows, _ := v.([]any)
	for i, row := range rows {
		blocks, _ := row.([]any)
		rp := base + Ptr("rows", i)
		given := make([]int, len(blocks))
		sum, free, spans := 0, 0, true
		for j, b := range blocks {
			m, _ := b.(map[string]any)
			if s, ok := m["span"].(int64); ok && s >= 1 && s <= 12 {
				given[j] = int(s)
				sum += int(s)
			} else if _, ok := m["span"]; ok {
				spans = false // the schema reported it
			} else {
				free++
			}
		}
		filled := shareSpans(given)
		for j, b := range blocks {
			if m, ok := b.(map[string]any); ok { // else the schema reported it
				r.block(m, rp+Ptr(j), in, filled[j])
			}
		}
		if !spans || len(blocks) > 4 {
			continue
		}
		switch {
		case sum > 12:
			r.add(StepParse, rp, true, false, fmt.Sprintf("the spans of this row add up to %d; a row has 12 columns", sum))
		case free > 0 && 12-sum < free:
			r.add(StepParse, rp, true, false, fmt.Sprintf("the spans given in this row add up to %d, which leaves %d of the 12 columns for %d %s without span; "+
				"a block without span shares what is left, at least 1 column each, so give spans that leave room or none at all", sum, 12-sum, free, plural(free, "block", "blocks")))
		}
	}
}

// block checks one block, whose span with the default filled in is span.
func (r *run) block(m map[string]any, ptr, in string, span int) {
	types := blockType(r.reg, m)
	switch len(types) {
	case 0:
		r.add(StepParse, ptr, true, false, fmt.Sprintf("a block needs one of these fields, which says what it is: %s", strings.Join(r.reg.Types(PageBlock), ", ")))
		return
	case 1:
	default:
		r.add(StepParse, ptr, true, false, fmt.Sprintf("a block is one of %s; this one has %s", strings.Join(r.reg.Types(PageBlock), ", "), strings.Join(types, " and ")))
		return
	}
	u := r.reg.unit(PageBlock, types[0])
	if u.container && in != "" {
		r.add(StepParse, ptr+Ptr(u.typ), true, false, fmt.Sprintf("a %s can't be inside a %s", u.typ, in))
		return
	}
	r.schema(u.schema, m, ptr)
	spec, _ := u.decode(m)
	blk := &Block{Type: u.typ, Spec: spec, Span: span}
	c := &Check{r: r, base: ptr, block: blk}
	u.check(c, spec)
	if u.container {
		r.rows(m["rows"], ptr, u.typ)
	}
}

// target checks a button's target: one of run_pipeline, open_page and
// open_form.
func (r *run) target(c *Check, base, what, pipeline, page, form string) {
	var given []string
	if pipeline != "" {
		given = append(given, "run_pipeline")
	}
	if page != "" {
		given = append(given, "open_page")
	}
	if form != "" {
		given = append(given, "open_form")
	}
	switch {
	case len(given) == 0:
		c.Errorf(StepParse, base, "%s needs one of run_pipeline, open_page or open_form", what)
		return
	case len(given) > 1:
		c.Errorf(StepParse, base, "%s has one target; this one has %s", what, strings.Join(given, " and "))
		return
	}
	switch {
	case pipeline != "":
		c.Pipeline(base+"/run_pipeline", pipeline)
	case page != "":
		c.PageRef(base+"/open_page", page)
	case form != "":
		c.Form(base+"/open_form", form)
	}
}

// saved returns the live config id of kind, read once; nil for none.
func (r *run) saved(kind store.ConfigKind, id string) *saved {
	if r.env == nil {
		return nil
	}
	id = store.NormConfigID(id)
	k := string(kind) + ":" + id
	if s, ok := r.configs[k]; ok {
		return s
	}
	cfg, ok, err := r.env.Config(r.ctx, kind, id)
	if err != nil {
		r.fail(err)
		r.configs[k] = nil
		return nil
	}
	if !ok {
		r.configs[k] = nil
		return nil
	}
	s := &saved{cfg: cfg}
	if cfg.Kind == store.KindView {
		s.view, _ = ParseView(r.reg, []byte(cfg.YAML))
	}
	r.configs[k] = s
	return s
}

func (r *run) idsOf(kind store.ConfigKind) []string {
	if ids, ok := r.ids[kind]; ok || r.env == nil {
		return ids
	}
	ids, err := r.env.IDs(r.ctx, kind)
	if err != nil {
		r.fail(err)
	}
	r.ids[kind] = ids
	return ids
}

// table describes an agent table, read once; nil for none.
func (r *run) table(name string) *store.Table {
	k := strings.ToLower(name)
	if t, ok := r.tables[k]; ok {
		return t
	}
	t, ok, err := r.env.Table(r.ctx, name)
	if err != nil {
		r.fail(err)
	}
	if !ok || err != nil {
		r.tables[k] = nil
		return nil
	}
	r.tables[k] = &t
	return &t
}

func (r *run) tableNames() []string {
	if r.names == nil && r.env != nil {
		names, err := r.env.Tables(r.ctx)
		if err != nil {
			r.fail(err)
		}
		r.names = append([]string{}, names...)
	}
	return r.names
}

// prepare runs the SQL check on q once. An error that is the query's
// fault comes back as the issue's message; a failure of the project goes
// to r.err.
func (r *run) prepare(q string) *prepared {
	if p, ok := r.queries[q]; ok {
		return p
	}
	p := &prepared{}
	r.queries[q] = p
	if err := sqlguard.Check(q); err != nil {
		p.err = queryError(err)
		return p
	}
	if r.env == nil {
		p.err = errNoEnv
		return p
	}
	cols, err := r.env.QueryColumns(r.ctx, q)
	var re *sqlguard.RejectError
	switch {
	case err == nil:
		p.cols = cols
	case errors.As(err, &re), sqlworker.IsSQLite(err):
		p.err = queryError(err)
	case r.ctx.Err() != nil, errors.Is(err, store.ErrClosed):
		r.fail(err)
		p.err = errNoEnv
	case errors.Is(err, context.DeadlineExceeded):
		p.err = errors.New("preparing the query took too long; make it simpler")
	default:
		r.fail(err)
		p.err = errNoEnv
	}
	return p
}

// errNoEnv: the query couldn't be prepared, and that is no issue of the
// config.
var errNoEnv = errors.New("no project to prepare the query in")

// queryError words a refused or failed query for the model, with its
// place in the query.
func queryError(err error) error {
	var re *sqlguard.RejectError
	if errors.As(err, &re) {
		if re.Pos >= 0 {
			return fmt.Errorf("line %d:%d of the query: %s", re.Line, re.Col, re.Reason)
		}
		return errors.New(re.Reason)
	}
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "SQL logic error: ")
	if i := strings.LastIndex(msg, " ("); i > 0 && strings.HasSuffix(msg, ")") {
		msg = msg[:i]
	}
	return errors.New(msg)
}

// paramName reports whether s can be a named parameter (2.2): a letter,
// then letters, digits or _.
func paramName(s string) bool {
	for i, c := range s {
		switch {
		case i == 0 && unicode.IsLetter(c):
		case i > 0 && (c == '_' || c >= 0x80 && c != 0xFEFF || c < 0x80 && (unicode.IsLetter(c) || unicode.IsDigit(c))):
		default:
			return false
		}
	}
	return s != ""
}

// key reports whether key is a valid bucket key.
func validKey(key string) error { return bucket.CheckKey(key) }
