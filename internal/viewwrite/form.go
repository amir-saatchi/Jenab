package viewwrite

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// Form is what a form block shows when it opens: its fields with their
// select options, and the values to fill them with.
type Form struct {
	View   string `json:"view"`
	Title  string `json:"title"`
	Action string `json:"action"` // insert, update or run_pipeline
	Table  string `json:"table,omitempty"`
	// Key is the row an update form edits, as the row action gave it.
	Key    any     `json:"key,omitempty"`
	Fields []Field `json:"fields"`
	// Values are an insert form's defaults, or the row an update form's
	// load read, by field.
	Values map[string]Value `json:"values"`
	// Missing: no row has Key any more ("This row is no longer there.").
	Missing bool `json:"missing,omitempty"`
	// Error is why the form can't be filled, as its load failing; the
	// form shows it in place of its fields.
	Error string `json:"error,omitempty"`
}

// Field is one field of a form.
type Field struct {
	Field    string   `json:"field"`
	Label    string   `json:"label"`   // the field's name when the config has none
	Control  string   `json:"control"` // text, textarea, number, date, select, checkbox, file
	Required bool     `json:"required,omitempty"`
	Options  []Option `json:"options,omitempty"` // a select's choices
	// Problem is why a select has no choices, as its options query
	// failing.
	Problem string `json:"problem,omitempty"`
	Prefix  string `json:"prefix,omitempty"` // a file field's bucket prefix
}

// formOf returns v's form, or a request error.
func formOf(v *view.View) (*view.Form, error) {
	f, ok := v.Spec.(*view.Form)
	if !ok || f == nil {
		return nil, badRequest("%s is a %s view, not a form", v.ID, v.Type)
	}
	return f, nil
}

// options returns the choices of the select fields of f, by field, and
// why a field has none.
func (w *Writer) options(ctx context.Context, f *view.Form) (map[string][]Option, map[string]string) {
	opts, problems := map[string][]Option{}, map[string]string{}
	r := w.runner()
	for _, fd := range f.Fields {
		if fd.Control != "select" || fd.Options == nil {
			continue
		}
		if fd.Options.Query == "" {
			opts[fd.Field] = optionsOf(fd.Options.List)
			continue
		}
		vals, problem := r.Options(ctx, fd.Options.Query)
		opts[fd.Field] = optionsOf(vals)
		if problem != "" {
			problems[fd.Field] = problem
		}
	}
	return opts, problems
}

// Open reads the form v for the page screen: its fields and their
// options, with an insert form's defaults, or for an update form the row
// with key, which its load reads on the read-only path.
func (w *Writer) Open(ctx context.Context, v *view.View, key any) (Form, error) {
	f, err := formOf(v)
	if err != nil {
		return Form{}, err
	}
	out := Form{View: v.ID, Title: v.Title, Action: f.Submit.Action, Table: f.Submit.Table, Values: map[string]Value{}}
	opts, problems := w.options(ctx, f)
	if err := ctx.Err(); err != nil {
		return Form{}, err
	}
	today := w.now()
	for _, fd := range f.Fields {
		out.Fields = append(out.Fields, Field{Field: fd.Field, Label: cmp.Or(fd.Label, fd.Field), Control: fd.Control,
			Required: fd.Required, Options: opts[fd.Field], Problem: problems[fd.Field], Prefix: fd.Prefix})
		if f.Submit.Action != "update" && fd.Default != nil {
			out.Values[fd.Field] = valueOf(fd, fd.Default, opts[fd.Field], today)
		}
	}
	if f.Submit.Action != "update" {
		return out, nil
	}
	if key == nil {
		out.Error = "This form edits a row: open it from the row's action."
		return out, nil
	}
	shape, err := w.DB.TableShape(ctx, f.Submit.Table)
	if errors.Is(err, store.ErrNotFound) {
		out.Error = fmt.Sprintf("There is no table %s any more.", f.Submit.Table)
		return out, nil
	}
	if err != nil {
		return Form{}, err
	}
	if key, err = keyOf(shape, cmp.Or(f.Submit.Key, first(shape.Key)), key); err != nil {
		return Form{}, err
	}
	out.Key = key
	rows, err := w.DB.ViewQuery(ctx, f.Load, []any{sql.Named("key", key)}, 2)
	if err != nil {
		if ctx.Err() != nil {
			return Form{}, ctx.Err()
		}
		out.Error = "Can't read the row: " + message(err)
		return out, nil
	}
	if len(rows.Values) == 0 {
		out.Missing = true
		return out, nil
	}
	row := rows.Values[0]
	for _, fd := range f.Fields {
		for j, c := range rows.Columns {
			if c == fd.Field { // matched as written (P3-02)
				out.Values[fd.Field] = valueOf(fd, row[j], opts[fd.Field], today)
				break
			}
		}
	}
	return out, nil
}

// Submitted is what a form's submit did. When it wrote, Change is its
// change-log entry, which the notice's Undo undoes; otherwise Errors,
// Problem or Missing say why it didn't.
type Submitted struct {
	Change id.Change `json:"change,omitempty"`
	Table  string    `json:"table,omitempty"` // the table it wrote, so the page re-runs its blocks
	// Errors are on their fields, by field: "a row with this value already
	// exists".
	Errors map[string]string `json:"errors,omitempty"`
	// Problem is an error on no field, as "already running since 08:00".
	Problem string `json:"problem,omitempty"`
	// Missing: the row an update form edits is no longer there.
	Missing bool `json:"missing,omitempty"`
	// Run is the run a run_pipeline form started.
	Run id.Run `json:"run,omitempty"`
	// Notice is the line the page shows with Undo, as "Saved to
	// watch_keywords.".
	Notice string `json:"notice,omitempty"`
	// Removed are the file fields whose upload was moved to the bucket's
	// trash because the submit failed (app.FormService); the page clears
	// them, and the user uploads again.
	Removed []string `json:"removed,omitempty"`
}

// OK reports whether the submit did what it should.
func (s Submitted) OK() bool { return len(s.Errors) == 0 && s.Problem == "" && !s.Missing }

func (s *Submitted) fieldError(field, msg string) {
	if s.Errors == nil {
		s.Errors = map[string]string{}
	}
	if _, ok := s.Errors[field]; !ok {
		s.Errors[field] = msg
	}
}

// Submit writes the form v with values, by field: an insert, the update of
// the row with key, or the start of its pipeline. Each value is read as
// its control gives it and checked against its column's type before
// anything is written; a constraint the write breaks comes back on its
// field (SPEC 5.5). The write is one interactive request with one
// change-log entry, its source form:<view_id>.
//
// The error is for what the user can't fix: the project closed, ctx
// cancelled, a view that isn't a form.
func (w *Writer) Submit(ctx context.Context, v *view.View, key any, values map[string]Value) (Submitted, error) {
	f, err := formOf(v)
	if err != nil {
		return Submitted{}, err
	}
	var out Submitted
	var opts map[string][]Option
	for _, fd := range f.Fields {
		if fd.Control == "select" {
			var problems map[string]string
			opts, problems = w.options(ctx, f)
			if err := ctx.Err(); err != nil {
				return Submitted{}, err
			}
			for field, p := range problems {
				out.fieldError(field, p)
			}
			break
		}
	}
	vals := readAll(f, values, opts, &out)
	if f.Submit.Action == "run_pipeline" {
		if !out.OK() {
			return out, nil
		}
		for field, val := range vals {
			if d, ok := val.(dateText); ok {
				vals[field] = string(d)
			}
		}
		run, problem, err := w.runPipeline(ctx, f.Submit.Pipeline, vals, Source(v.ID))
		out.Run, out.Problem = run, problem
		if err == nil && problem == "" {
			out.Notice = fmt.Sprintf("Started %s.", f.Submit.Pipeline)
		}
		return out, err
	}
	out.Table = f.Submit.Table
	shape, err := w.DB.TableShape(ctx, f.Submit.Table)
	if errors.Is(err, store.ErrNotFound) {
		out.Problem = fmt.Sprintf("Not saved: there is no table %s any more.", f.Submit.Table)
		return out, nil
	}
	if err != nil {
		return Submitted{}, err
	}
	out.Table = shape.Name
	update := f.Submit.Action == "update"
	keyCol := cmp.Or(f.Submit.Key, first(shape.Key))
	if update {
		if key == nil {
			return Submitted{}, badRequest("the update form %s needs the row's key", v.ID)
		}
		if key, err = keyOf(shape, keyCol, key); err != nil {
			return Submitted{}, err
		}
		// The page sends every field; an update missing one would set it
		// to NULL or false without the user seeing it.
		for _, fd := range f.Fields {
			if _, ok := values[fd.Field]; !ok {
				out.Problem = fmt.Sprintf("Not saved: missing value for %s.", fd.Field)
				return out, nil
			}
		}
	}
	row := store.Row{}
	cols := map[string]string{} // field → column
	kept := map[string]string{} // a file field's key Upload didn't hand out, by field: the row's own on an update
	for _, fd := range f.Fields {
		col, ok := shape.Column(fd.Field)
		if !ok {
			out.fieldError(fd.Field, fmt.Sprintf("has no column in %s any more", shape.Name))
			continue
		}
		cols[fd.Field] = col.Name
		val, given := vals[fd.Field]
		if !given {
			continue // it couldn't be read
		}
		val, problem := fit(val, col.Type, shape.Strict)
		if problem != "" {
			out.fieldError(fd.Field, problem)
			continue
		}
		if k, ok := val.(string); ok && fd.Control == "file" && !w.issued(v.ID, fd.Field, k) {
			if !update {
				out.fieldError(fd.Field, "is not a file uploaded to this field")
				continue
			}
			kept[fd.Field] = k
		}
		if update && slices.ContainsFunc(shape.Key, func(k string) bool { return strings.EqualFold(k, col.Name) }) {
			// The row's key: shown, but an update can't change it.
			if val != nil && text(val) != text(key) {
				out.fieldError(fd.Field, "is the row's key, which can't be changed")
			}
			continue
		}
		if val == nil {
			if col.NotNull && (f.Submit.Action == "update" || col.Default == nil) {
				out.fieldError(fd.Field, "needs a value")
				continue
			}
			if f.Submit.Action == "insert" {
				continue // its default, or NULL
			}
		}
		row[col.Name] = val
	}
	if len(kept) > 0 && out.OK() {
		if err := w.keepsFiles(ctx, shape, keyCol, key, cols, kept, &out); err != nil {
			return Submitted{}, err
		}
	}
	if !out.OK() {
		return out, nil
	}
	src := Source(v.ID)
	var res store.WriteResult
	switch f.Submit.Action {
	case "insert":
		if len(row) == 0 {
			out.Problem = "Not saved: fill in at least one field."
			return out, nil
		}
		res, err = w.DB.Insert(ctx, limit.Interactive, src, shape.Name, []store.Row{row}, store.ConflictFail)
	case "update":
		if len(row) == 0 {
			out.Problem = "Not saved: the form has nothing to change."
			return out, nil
		}
		res, err = w.DB.Update(ctx, limit.Interactive, src, store.UpdateReq{Table: shape.Name, Set: row,
			Keys: []store.Row{{keyCol: key}}})
	default:
		return Submitted{}, badRequest("the form %s has the submit action %q", v.ID, f.Submit.Action)
	}
	if err != nil {
		if placed := w.place(ctx, err, f, cols, row, shape, &out); placed {
			return out, nil
		}
		return Submitted{}, err
	}
	out.Change = res.Change
	switch {
	case f.Submit.Action == "update" && res.Updated == 0:
		out.Missing = true
		out.Problem = "This row is no longer there."
	case f.Submit.Action == "update":
		out.Notice = fmt.Sprintf("Saved the row in %s.", shape.Name)
	default:
		out.Notice = fmt.Sprintf("Added a row to %s.", shape.Name)
	}
	return out, nil
}

func first(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// keepsFiles checks the file keys of an update that Upload didn't hand
// out: each must be the one the row has now, which the user kept. A row
// that is gone is Missing.
func (w *Writer) keepsFiles(ctx context.Context, shape store.Table, keyCol string, key any, cols, kept map[string]string, out *Submitted) error {
	fields := slices.Sorted(maps.Keys(kept))
	sel := make([]string, len(fields))
	for i, f := range fields {
		sel[i] = quoteName(cols[f])
	}
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s = :key", strings.Join(sel, ", "), quoteName(shape.Name), quoteName(keyCol))
	rows, err := w.DB.ViewQuery(ctx, q, []any{sql.Named("key", key)}, 2)
	if err != nil {
		return err
	}
	if len(rows.Values) == 0 {
		out.Missing, out.Problem = true, "This row is no longer there."
		return nil
	}
	for i, f := range fields {
		if text(rows.Values[0][i]) != kept[f] {
			out.fieldError(f, "is not a file uploaded to this field")
		}
	}
	return nil
}

// quoteName quotes a table or column name for SQL.
func quoteName(n string) string { return `"` + strings.ReplaceAll(n, `"`, `""`) + `"` }

// readAll reads every field's value, as its control gives it: the values
// by field, nil for an empty one; a field that can't be read gets its
// error and isn't in the map.
func readAll(f *view.Form, values map[string]Value, opts map[string][]Option, out *Submitted) map[string]any {
	got := map[string]any{}
	for _, fd := range f.Fields {
		val, problem := read(fd, values[fd.Field], opts[fd.Field])
		if problem != "" {
			out.fieldError(fd.Field, problem)
			continue
		}
		got[fd.Field] = val
	}
	return got
}

// place puts a failed write's error where the form shows it: on the field
// of the column it names, or as the form's problem. It reports false for
// an error that isn't the row's, such as the project closing.
func (w *Writer) place(ctx context.Context, err error, f *view.Form, cols map[string]string, row store.Row, shape store.Table, out *Submitted) bool {
	fieldOf := func(col string) string {
		for _, fd := range f.Fields {
			if c, ok := cols[fd.Field]; ok && strings.EqualFold(c, col) {
				return fd.Field
			}
		}
		return ""
	}
	if key, ok := store.MissingObject(err); ok {
		for field, col := range cols {
			if s, ok := row[col].(string); ok && s == key {
				out.fieldError(field, "the file is no longer there; upload it again")
				return true
			}
		}
		out.Problem = fmt.Sprintf("Not saved: there is no file %s.", key)
		return true
	}
	var ce *store.ConstraintError
	if errors.As(err, &ce) {
		field, msg := constraintText(ce, shape, row, slices.Collect(maps.Values(cols)), fieldOf, "")
		if field != "" {
			out.fieldError(field, msg)
		} else {
			out.Problem = "Not saved: " + msg + "."
		}
		return true
	}
	if errors.Is(err, store.ErrInvalid) && ctx.Err() == nil {
		out.Problem = "Not saved: " + message(err)
		return true
	}
	return false
}

// constraintText words a broken constraint for a field, or, with field
// "", for the form or the action as a whole. written are the columns
// the write sets, columns those the form or the set action has, and
// fieldOf finds the field that writes a column. op is "delete" for a
// delete, whose foreign-key error is about the rows that refer to it;
// referrers then names their tables, when the schema says.
func constraintText(ce *store.ConstraintError, shape store.Table, written store.Row, columns []string, fieldOf func(string) string, op string, referrers ...string) (field, msg string) {
	switch ce.Kind {
	case store.ConstraintPrimaryKey, store.ConstraintUnique:
		for _, c := range ce.Columns {
			if field = fieldOf(c); field != "" {
				break
			}
		}
		if len(ce.Columns) > 1 {
			msg = fmt.Sprintf("a row with these values of %s already exists", strings.Join(ce.Columns, ", "))
		} else {
			msg = "a row with this value already exists"
		}
		if field == "" {
			msg = fmt.Sprintf("a row with this %s already exists", strings.Join(ce.Columns, ", "))
		}
		return field, msg
	case store.ConstraintNotNull:
		for _, c := range ce.Columns {
			if field = fieldOf(c); field != "" {
				return field, "needs a value"
			}
		}
		return "", fmt.Sprintf("%s needs a value, and the form has no field for it", strings.Join(ce.Columns, ", "))
	case store.ConstraintDatatype:
		for _, c := range ce.Columns {
			if field = fieldOf(c); field != "" {
				col, _ := shape.Column(c)
				return field, "must fit the column's type " + col.Type
			}
		}
	case store.ConstraintCheck:
		// On a field only when the check names exactly one of the
		// columns, as written in its text; a check on several, or one
		// SQLite reports by its constraint name, is the whole row's.
		var named []string
		for _, c := range columns {
			if containsWord(ce.Name, c) {
				named = append(named, c)
			}
		}
		if len(named) == 1 {
			if field = fieldOf(named[0]); field != "" {
				return field, "doesn't pass the table's check: " + ce.Name
			}
		}
		return "", "the row doesn't pass the table's check " + ce.Name
	case store.ConstraintForeignKey:
		if op == "delete" {
			if len(referrers) > 0 {
				return "", fmt.Sprintf("rows of %s still refer to it", strings.Join(referrers, ", "))
			}
			return "", "other rows still refer to it"
		}
		// The field of the one reference the row sets, if only one.
		var hit string
		n := 0
		for _, fk := range shape.ForeignKeys {
			for _, c := range fk.Columns {
				if _, ok := written[c]; ok && fieldOf(c) != "" {
					hit = fieldOf(c)
					n++
					break
				}
			}
		}
		if n == 1 {
			return hit, "refers to a row that isn't there"
		}
		return "", "the row refers to a row of another table that isn't there"
	}
	return "", message(ce)
}

// containsWord reports whether s has name as a whole word, ignoring case.
func containsWord(s, name string) bool {
	ls, ln := strings.ToLower(s), strings.ToLower(name)
	for i := 0; ; {
		j := strings.Index(ls[i:], ln)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !wordByte(ls[j-1])
		after := j+len(ln) == len(ls) || !wordByte(ls[j+len(ln)])
		if before && after {
			return true
		}
		i = j + 1
	}
}

func wordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c >= 0x80
}

// message words a store or query error for the user: without the
// package's prefix, and closed with a full stop.
func message(err error) string {
	msg := err.Error()
	for _, p := range []string{"store: ", "SQL logic error: "} {
		msg = strings.ReplaceAll(msg, p, "")
	}
	if i := strings.LastIndex(msg, " ("); i > 0 && strings.HasSuffix(msg, ")") && isCode(msg[i+2:len(msg)-1]) {
		msg = msg[:i]
	}
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return msg
}

func isCode(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
