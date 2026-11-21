package view

import (
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The data units: table, chart, stat and form views (SPEC 5.3–5.5, 5.10).

func registerData(r *Registry) {
	MustRegister(r, Unit[Table]{Type: "table", Kind: Data, Schema: schemaFile("table.json"), Check: checkTable})
	MustRegister(r, Unit[Chart]{Type: "chart", Kind: Data, Schema: schemaFile("chart.json"), Check: checkChart})
	MustRegister(r, Unit[Stat]{Type: "stat", Kind: Data, Schema: schemaFile("stat.json"), Check: checkStat})
	MustRegister(r, Unit[Form]{Type: "form", Kind: Data, Schema: schemaFile("form.json"), Check: checkForm})
}

// pipelineColumns are written by pipelines (SPEC 6), never by a form or a
// row action.
var pipelineColumns = []string{"_run_id", "_fetched_at"}

func pipelineColumn(name string) bool {
	return slices.ContainsFunc(pipelineColumns, func(p string) bool { return strings.EqualFold(p, name) })
}

func checkTable(c *Check, t *Table) {
	for i, col := range t.Columns {
		c.Field(Ptr("columns", i, "field"), col.Field)
		c.Format(Ptr("columns", i), col.Field, col.Format, col.FormatOptions)
	}
	c.Field("/default_sort/field", t.DefaultSort.Field)

	// The runtime sorts and pages the rows itself (SPEC 5.3).
	if v := c.View(); v != nil && v.Query != "" {
		if cls, err := sqlguard.OuterClauses(v.Query); err == nil {
			for _, cl := range cls {
				if cl.Name == "ORDER BY" {
					c.Warnf(StepSQL, "/query", "line %d:%d of the query: a table sorts its rows itself, by default_sort and the user's clicks, so this ORDER BY is ignored; remove it", cl.Line, cl.Col)
				} else {
					c.Errorf(StepSQL, "/query", "line %d:%d of the query: a table pages its rows itself, by page_size, so it can't have %s; remove it", cl.Line, cl.Col, cl.Name)
				}
			}
		}
	}

	if len(t.RowActions) > 0 && t.RowsFrom == nil {
		c.Errorf(StepParse, "/row_actions", "row actions need rows_from: { table, key }, the table the rows come from and its key column")
	}
	var base *store.Table
	if t.RowsFrom != nil {
		base = checkRowsFrom(c, t.RowsFrom)
	}
	for i, a := range t.RowActions {
		checkRowAction(c, Ptr("row_actions", i), a, t.RowsFrom, base)
	}
}

// checkRowsFrom checks the table and key of rows_from, and that the query
// selects the key.
func checkRowsFrom(c *Check, rf *RowsFrom) *store.Table {
	if cols, ok := c.Columns(); ok && rf.Key != "" {
		if _, ok := c.column(rf.Key); !ok {
			c.Errorf(StepSQL, "/rows_from/key", "the query must select the key column %s, so row actions know their row; it has %s", rf.Key, columnList(cols))
		}
	}
	t, ok := c.Table("/rows_from/table", rf.Table)
	if !ok {
		return nil
	}
	if _, ok := c.Column("/rows_from/key", t, rf.Key); !ok {
		return t
	}
	if !isKey(t, rf.Key) {
		c.Errorf(StepSchema, "/rows_from/key", "%s is not the key of %s: %s", rf.Key, t.Name, keyText(t))
	}
	return t
}

// isKey reports whether name is t's one-column primary key.
func isKey(t *store.Table, name string) bool {
	return len(t.Key) == 1 && strings.EqualFold(t.Key[0], name)
}

func keyText(t *store.Table) string {
	switch len(t.Key) {
	case 0:
		return "it has no primary key"
	case 1:
		return "its key is " + t.Key[0]
	}
	return "its key is (" + strings.Join(t.Key, ", ") + "); row actions and update forms need a one-column key"
}

func checkRowAction(c *Check, ptr string, a RowAction, rf *RowsFrom, t *store.Table) {
	var given []string
	// A field given as null is given: the schema says what is wrong
	// with it.
	if a.OpenForm != "" || c.given(ptr+"/open_form") {
		given = append(given, "open_form")
	}
	if a.Set != nil || c.given(ptr+"/set") {
		given = append(given, "set")
	}
	if a.Delete {
		given = append(given, "delete")
	}
	switch len(given) {
	case 0:
		c.Errorf(StepParse, ptr, "a row action needs one of open_form, set or delete")
		return
	case 1:
	default:
		c.Errorf(StepParse, ptr, "a row action does one thing; this one has %s", strings.Join(given, " and "))
		return
	}
	if a.Bulk && a.OpenForm != "" {
		c.Errorf(StepParse, ptr+"/bulk", "bulk works with set and delete, not open_form")
	}
	if a.OpenForm != "" {
		if f, ok := c.Form(ptr+"/open_form", a.OpenForm); ok {
			if form, _ := f.Spec.(*Form); form != nil && form.Submit.Action != "update" {
				c.Warnf(StepReferences, ptr+"/open_form", "a row action opens its form with the row's key, but %s is an %s form; it uses an update form", f.ID, form.Submit.Action)
			}
		}
	}
	if t == nil {
		return
	}
	for _, field := range slices.Sorted(mapsKeys(a.Set)) {
		fp := ptr + Ptr("set", field)
		col, ok := c.Column(fp, t, field)
		if !ok {
			continue
		}
		switch {
		case rf != nil && strings.EqualFold(field, rf.Key):
			c.Errorf(StepSchema, fp, "%s is the key of %s, which set can't change", field, t.Name)
		case pipelineColumn(field):
			c.Errorf(StepSchema, fp, "%s is written by pipelines, which set can't change", field)
		case col.Generated:
			c.Errorf(StepSchema, fp, "%s.%s is generated by SQLite, so it can't be set", t.Name, col.Name)
		default:
			if why := valueFits(a.Set[field], t.Name+"."+col.Name, col.Type, col.NotNull); why != "" {
				c.Errorf(StepSchema, fp, "%s", why)
			}
		}
	}
}

func mapsKeys(m map[string]any) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func checkChart(c *Check, ch *Chart) {
	c.Field("/x/field", ch.X.Field)
	if ch.X.Format == "image" || ch.X.Format == "file" {
		c.Errorf(StepParse, "/x/format", "a chart's axis can't show %ss", ch.X.Format)
	}
	for i, y := range ch.Y {
		c.Field(Ptr("y", i, "field"), y.Field)
	}
	if ch.SeriesBy != "" {
		c.Field("/series_by", ch.SeriesBy)
	}
	if ch.ChartType == "pie" {
		if ch.SeriesBy != "" {
			c.Errorf(StepParse, "/series_by", "a pie chart can't have series_by")
		}
		if len(ch.Y) > 1 {
			c.Errorf(StepParse, "/y", "a pie chart has one y, the value of each slice; this one has %d", len(ch.Y))
		}
	} else if ch.SeriesBy != "" && len(ch.Y) > 1 {
		c.Errorf(StepParse, "/y", "with series_by, y has exactly one entry; this one has %d", len(ch.Y))
	}
}

func checkStat(c *Check, s *Stat) {
	c.Field("/value/field", s.Value.Field)
	c.Format("/value", s.Value.Field, s.Value.Format, s.Value.FormatOptions)
	if s.Change != nil {
		c.Field("/change/field", s.Change.Field)
		if s.Change.Format == "image" || s.Change.Format == "file" {
			c.Errorf(StepParse, "/change/format", "a change is a number, so it can't be shown as %s", s.Change.Format)
		}
	}
}

func checkForm(c *Check, f *Form) {
	seen := map[string]int{}
	for i, fd := range f.Fields {
		fp := Ptr("fields", i)
		if fd.Field != "" {
			if j, dup := seen[strings.ToLower(fd.Field)]; dup {
				c.Errorf(StepParse, fp+"/field", "fields[%d] is %s already; each field is one column", j, fd.Field)
			} else {
				seen[strings.ToLower(fd.Field)] = i
			}
		}
		checkOptions(c, fp, fd.Control, fd.Options)
		if fd.Default != nil {
			checkDefault(c, fp+"/default", fd.Control, fd.Default, fd.Options)
		}
		switch {
		case fd.Control == "file" && fd.Prefix == "":
			c.Errorf(StepParse, fp+"/control", "a file field needs a prefix, the bucket folder its uploads go to, as uploads/")
		case fd.Control != "file" && fd.Prefix != "":
			c.Errorf(StepParse, fp+"/prefix", "only a file field has a prefix")
		case fd.Prefix != "":
			if err := bucket.CheckPrefix(fd.Prefix); err != nil {
				c.Errorf(StepParse, fp+"/prefix", "is not a bucket prefix: %s", strings.TrimPrefix(err.Error(), "bucket: "))
			}
		}
	}

	s := f.Submit
	switch s.Action {
	case "run_pipeline":
		c.Pipeline("/submit/pipeline", s.Pipeline)
		return
	case "update":
		if f.Load == "" {
			c.Errorf(StepParse, "/submit/action", "an update form needs load, a query that fills its fields from the row's :key")
		}
	case "insert":
		if f.Load != "" {
			c.Warnf(StepParse, "/load", "only an update form uses load")
		}
	}
	if f.Load != "" && s.Action == "update" {
		checkLoad(c, f)
	}
	t, ok := c.Table("/submit/table", s.Table)
	if !ok {
		return
	}
	if s.Action == "update" && s.Key != "" {
		if _, ok := c.Column("/submit/key", t, s.Key); ok && !isKey(t, s.Key) {
			c.Errorf(StepSchema, "/submit/key", "%s is not the key of %s: %s", s.Key, t.Name, keyText(t))
		}
	}
	covered := map[string]FormField{}
	for i, fd := range f.Fields {
		fp := Ptr("fields", i, "field")
		col, ok := c.Column(fp, t, fd.Field)
		if !ok {
			continue
		}
		covered[strings.ToLower(col.Name)] = fd
		switch {
		case pipelineColumn(col.Name):
			c.Errorf(StepSchema, fp, "%s is written by pipelines, not forms", col.Name)
		case col.Generated:
			c.Errorf(StepSchema, fp, "%s.%s is generated by SQLite, so a form can't write it", t.Name, col.Name)
		case s.Action == "update" && (strings.EqualFold(col.Name, s.Key) || isKey(t, col.Name)):
			c.Errorf(StepSchema, fp, "%s is the key of the row this form updates, so the form can't change it; leave the field out", col.Name)
		case fd.Control == "file" && col.Kind != "object_ref":
			c.Errorf(StepSchema, Ptr("fields", i, "control"), "a file field stores a file key, but %s.%s is not an object_ref column; mark it object_ref in a migration", t.Name, col.Name)
		}
	}
	if s.Action != "insert" {
		return
	}
	// Every NOT NULL column without a default needs a value (SPEC 10,
	// step 4). A checkbox always has one.
	for _, col := range t.Columns {
		if !needsValue(t, col) {
			continue
		}
		fd, ok := covered[strings.ToLower(col.Name)]
		switch {
		case !ok:
			c.Errorf(StepSchema, "/fields", "%s.%s is NOT NULL and has no default, so the form needs a required field %s", t.Name, col.Name, col.Name)
		case !fd.Required && fd.Control != "checkbox":
			i := slices.IndexFunc(f.Fields, func(x FormField) bool { return strings.EqualFold(x.Field, col.Name) })
			c.Errorf(StepSchema, Ptr("fields", i, "required"), "must be true: %s.%s is NOT NULL and has no default", t.Name, col.Name)
		}
	}
}

// needsValue reports whether an insert must give col a value.
func needsValue(t *store.Table, col store.Column) bool {
	if col.Default != nil || col.Generated {
		return false
	}
	key := slices.ContainsFunc(t.Key, func(k string) bool { return strings.EqualFold(k, col.Name) })
	if key && t.AutoKey {
		return false
	}
	return col.NotNull || key && t.WithoutRowid
}

// checkLoad checks an update form's load query: it takes only :key, and
// selects the fields.
func checkLoad(c *Check, f *Form) {
	cols, ok := c.Query("/load", f.Load)
	if !ok {
		return
	}
	ps, _ := namedParams(c, "/load", f.Load, "use :key for the row's key, not ?")
	for _, p := range ps {
		if p != "key" {
			c.Errorf(StepSQL, "/load", "load gets only the row's :key, not :%s", p)
		}
	}
	have := map[string]bool{}
	for _, col := range cols {
		have[col.Name] = true
	}
	for i, fd := range f.Fields {
		if fd.Field != "" && !have[fd.Field] {
			c.Warnf(StepSQL, Ptr("fields", i, "field"), "load doesn't select %s, so the field starts empty%s", fd.Field, suggest(fd.Field, names(cols)))
		}
	}
}
