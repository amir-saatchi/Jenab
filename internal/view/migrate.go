package view

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Views and forms as a migration's dependents (SPEC 7.5, 8.2, P3-04): the
// schema agent gets each saved view with the columns it uses, sends the
// views a migration breaks with their whole new YAML, and the migration
// saves and checks them in its own transaction against the new schema.

// Kinds of a migration's dependents that are views. A form is a view of
// type form; the schema agent may name it either way.
const (
	DependentView = "view"
	DependentForm = "form"
)

// AddDependents makes views and forms dependents of db's migrations.
func AddDependents(db *store.ProjectDB) {
	db.AddDependent(&dependent{kind: DependentView})
	db.AddDependent(&dependent{kind: DependentForm})
}

// dependent is the views as a store.Dependent. The form kind only saves
// updates sent as kind form; the view kind checks and lists every view,
// forms too.
type dependent struct {
	kind string
	reg  *Registry // nil for Default
}

var (
	_ store.Dependent       = (*dependent)(nil)
	_ store.DependentLister = (*dependent)(nil)
)

func (d *dependent) Kind() string { return d.kind }

func (d *dependent) registry() *Registry {
	if d.reg == nil {
		return Default
	}
	return d.reg
}

// Update validates u's config against the schema after the migration's
// steps and saves it as the view u.ID. A config that fails is a problem
// at updates[i], with every issue, and the migration fails. An update of
// a view that doesn't exist fails with that alone, before validation.
func (d *dependent) Update(t *store.DependentTx, _ id.Source, u store.DependentUpdate) error {
	fail := func(path, msg string) error {
		return &store.MigrationError{Problems: []store.Problem{{Path: path, Msg: msg}}}
	}
	if cur, err := t.Config(store.KindView, u.ID); err != nil {
		return err
	} else if cur == nil {
		msg := fmt.Sprintf("there is no view %s to update; a migration only updates saved views", u.ID)
		if p, err := t.Config(store.KindPage, u.ID); err != nil {
			return err
		} else if p != nil {
			msg = fmt.Sprintf("%s is a page; pages only show views, so they need no update", u.ID)
		}
		return fail("id", msg)
	}
	src, err := configText(u.Config)
	if err != nil {
		return fail("config", err.Error())
	}
	v := &Validator{Reg: d.reg, Env: TxEnv{T: t}}
	pv, res, err := v.View(t.Context(), []byte(src))
	if err != nil {
		return err
	}
	var probs []store.Problem
	if pv != nil && pv.ID != store.NormConfigID(u.ID) {
		probs = append(probs, store.Problem{Path: "id", Msg: fmt.Sprintf("the config's id is %q, not %q; an update keeps the view's id", pv.ID, u.ID)})
	}
	if d.kind == DependentForm && pv != nil && pv.Type != "" && pv.Type != "form" {
		probs = append(probs, store.Problem{Path: "kind", Msg: fmt.Sprintf("%s is a %s view, not a form; send it as kind view", u.ID, pv.Type)})
	}
	for _, is := range res.Errors() {
		probs = append(probs, store.Problem{Path: "config", Msg: is.String()})
	}
	if res.MoreErrors > 0 {
		probs = append(probs, store.Problem{Path: "config", Msg: fmt.Sprintf("and %d more errors; fix these first", res.MoreErrors)})
	}
	if pv == nil && len(probs) == 0 {
		probs = append(probs, store.Problem{Path: "config", Msg: "the config doesn't load as a view"})
	}
	if len(probs) > 0 {
		return &store.MigrationError{Problems: probs}
	}
	_, err = t.SaveConfig(store.ConfigSave{Kind: store.KindView, ID: pv.ID, Type: pv.Type, Title: pv.Title, YAML: src})
	return err
}

// configText is an update's config as YAML: a JSON string is the YAML
// itself, as read_config shows it; an object is written as JSON, which is
// YAML too.
func configText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", errors.New("the config is missing: send the view's whole YAML, as read_config shows it, with your changes")
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		if strings.TrimSpace(s) == "" {
			return "", errors.New("the config is empty: send the view's whole YAML, as read_config shows it, with your changes")
		}
		return s, nil
	}
	if raw[0] != '{' {
		return "", errors.New("the config is the view's whole YAML as a string, as read_config shows it")
	}
	var b bytes.Buffer
	if err := json.Indent(&b, raw, "", "  "); err != nil {
		return "", err
	}
	return b.String() + "\n", nil
}

// Reached lists the saved views, forms too, that use any of tables: in a
// query, a form's submit table or a table's rows_from. A view with a
// query whose tables can't be read counts too, as it may use any. The
// form kind lists none, as the view kind lists forms too.
func (d *dependent) Reached(t *store.DependentTx, tables []string) ([]string, error) {
	if d.kind != DependentView {
		return nil, nil
	}
	vs, err := t.Configs(store.KindView)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range vs {
		if tables == nil || reaches(d.registry(), c, tables) {
			out = append(out, c.Target())
		}
	}
	return out, nil
}

// reaches reports whether the saved view c uses any of tables, or has a
// query whose tables can't be read. A view that doesn't parse reaches
// none: its issues don't depend on the schema.
func reaches(reg *Registry, c store.Config, tables []string) bool {
	if len(UsesOf(reg, c, tables).Tables) > 0 {
		return true
	}
	v, _ := ParseView(reg, []byte(c.YAML))
	if v == nil {
		return false
	}
	for _, q := range viewQueries(v) {
		if q == "" {
			continue
		}
		if _, err := sqlguard.Names(q); err != nil {
			return true
		}
	}
	return false
}

// usedBefore is what a view that failed its check before a migration's
// steps used then, to find what the steps took away from it.
type usedBefore struct {
	yaml string
	uses []string
}

const memoUsed = DependentView + ":used"

// Check validates the views targets against the schema as t has it. The
// form kind checks nothing, as the view kind checks forms too.
//
// Before the steps, it keeps what each failing view uses. After them, a
// view that still fails as it was saved then gets an issue for each table
// or column of those that is gone: SQLite names only the first unknown
// name of a query, so a column the steps drop from a view that already
// named a missing one would leave its issues as they were.
func (d *dependent) Check(t *store.DependentTx, targets []string) ([]store.Problem, error) {
	if d.kind != DependentView {
		return nil, nil
	}
	used, _ := t.Memo()[memoUsed].(map[string]usedBefore)
	if used == nil {
		used = map[string]usedBefore{}
		t.Memo()[memoUsed] = used
	}
	var tables []string
	cols := txColumns(t)
	v := &Validator{Reg: d.reg, Env: TxEnv{T: t}}
	var out []store.Problem
	for _, target := range targets {
		kind, vid, _ := strings.Cut(target, ":")
		if kind != DependentView {
			continue
		}
		c, err := t.Config(store.KindView, vid)
		if err != nil {
			return nil, err
		}
		if c == nil {
			continue
		}
		issues, err := configIssues(t.Context(), v, *c)
		if err != nil {
			return nil, err
		}
		if len(issues) > 0 && tables == nil {
			if tables, err = t.Tables(); err != nil {
				return nil, err
			}
		}
		switch {
		case len(issues) == 0:
		case !t.AfterSteps():
			uses, err := ColumnsUsed(d.registry(), *c, tables, cols)
			if err != nil {
				return nil, err
			}
			used[target] = usedBefore{yaml: c.YAML, uses: uses}
		default:
			b, ok := used[target]
			if !ok || b.yaml != c.YAML {
				break
			}
			gone, err := goneUses(t, b.uses)
			if err != nil {
				return nil, err
			}
			for _, g := range gone {
				issues = append(issues, fmt.Sprintf("uses %s, which the migration renamed or dropped", g))
			}
		}
		for _, is := range issues {
			out = append(out, store.Problem{Path: c.Target(), Msg: is})
		}
	}
	return out, nil
}

// goneUses are the uses, table.column or a table, that t no longer has.
func goneUses(t *store.DependentTx, uses []string) ([]string, error) {
	shapes := map[string]*store.Table{}
	var out []string
	for _, u := range uses {
		table, col, _ := strings.Cut(u, ".")
		k := strings.ToLower(table)
		tb, ok := shapes[k]
		if !ok {
			s, err := t.TableShape(table)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
			if err == nil {
				tb = &s
			}
			shapes[k] = tb
		}
		switch {
		case tb == nil:
			out = append(out, u)
		case col != "" && !slices.ContainsFunc(tb.Columns, func(c store.Column) bool { return strings.EqualFold(c.Name, col) }):
			out = append(out, u)
		}
	}
	return out, nil
}

// validations counts the views configIssues validated, for tests.
var validations atomic.Int64

// configIssues validates the saved view c and returns its errors as
// text, with "and N more errors" when they were capped.
func configIssues(ctx context.Context, v *Validator, c store.Config) ([]string, error) {
	validations.Add(1)
	_, res, err := v.View(ctx, []byte(c.YAML))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, is := range res.Errors() {
		out = append(out, is.String())
	}
	if res.MoreErrors > 0 {
		out = append(out, fmt.Sprintf("and %d more errors", res.MoreErrors))
	}
	return out, nil
}

// List gives every saved view with the table columns it uses, for the
// schema agent's input and the approval card (SPEC 8.2). The form kind
// lists none, as the view kind lists forms too.
func (d *dependent) List(t *store.DependentTx) ([]store.DependentInfo, error) {
	if d.kind != DependentView {
		return nil, nil
	}
	vs, err := t.Configs(store.KindView)
	if err != nil {
		return nil, err
	}
	tables, err := t.Tables()
	if err != nil {
		return nil, err
	}
	cols := txColumns(t)
	out := make([]store.DependentInfo, 0, len(vs))
	for _, c := range vs {
		uses, err := ColumnsUsed(d.registry(), c, tables, cols)
		if err != nil {
			return nil, err
		}
		out = append(out, store.DependentInfo{Kind: DependentView, ID: c.ID, Type: c.Type, Title: c.Title, Uses: uses})
	}
	return out, nil
}

// txColumns gives a table's column names as t has it, each table read
// once; none for a table that isn't there.
func txColumns(t *store.DependentTx) func(table string) ([]string, error) {
	shapes := map[string][]string{}
	return func(table string) ([]string, error) {
		k := strings.ToLower(table)
		if c, ok := shapes[k]; ok {
			return c, nil
		}
		tb, err := t.TableShape(table)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		var c []string
		for _, col := range tb.Columns {
			c = append(c, col.Name)
		}
		shapes[k] = c
		return c, nil
	}
}

// viewQueries are v's queries: its own, its filters' options, and a
// form's load and its fields' options; some may be empty.
func viewQueries(v *View) []string {
	qs := []string{v.Query}
	for _, f := range v.Filters {
		if f.Options != nil {
			qs = append(qs, f.Options.Query)
		}
	}
	if s, ok := v.Spec.(*Form); ok {
		qs = append(qs, s.Load)
		for _, f := range s.Fields {
			if f.Options != nil {
				qs = append(qs, f.Options.Query)
			}
		}
	}
	return qs
}

// ColumnsUsed lists what the saved view c uses, as table.column, or a
// table alone when no column of it is named (SELECT *). A query's names
// are matched against the columns of the tables it reads, without case,
// so a name that is a column of two of them counts for both; a form's
// fields and key are columns of its submit table, and a table's
// rows_from key and row actions' set columns of its rows_from table.
// cols gives a table's columns. A view that doesn't parse uses nothing.
func ColumnsUsed(reg *Registry, c store.Config, tables []string, cols func(table string) ([]string, error)) ([]string, error) {
	if reg == nil {
		reg = Default
	}
	v, _ := ParseView(reg, []byte(c.YAML))
	if v == nil {
		return nil, nil
	}
	t := tableNames(tables)
	var out []string
	uses := func(table string, names []string) error {
		n, ok := t[strings.ToLower(table)]
		if !ok {
			return nil
		}
		have, err := cols(n)
		if err != nil {
			return err
		}
		named := false
		for _, name := range names {
			if i := slices.IndexFunc(have, func(h string) bool { return strings.EqualFold(h, name) }); i >= 0 {
				add(&out, n+"."+have[i])
				named = true
			}
		}
		if !named && !slices.ContainsFunc(out, func(s string) bool { return strings.HasPrefix(strings.ToLower(s), strings.ToLower(n)+".") }) {
			add(&out, n)
		}
		return nil
	}
	query := func(q string) error {
		if q == "" {
			return nil
		}
		names, err := sqlguard.Names(q)
		if err != nil {
			return nil
		}
		var u Uses
		u.query(t, q)
		for _, tb := range u.Tables {
			if err := uses(tb, names); err != nil {
				return err
			}
		}
		return nil
	}
	switch s := v.Spec.(type) {
	case *Table:
		if s.RowsFrom != nil {
			names := []string{s.RowsFrom.Key}
			for _, a := range s.RowActions {
				for k := range a.Set {
					names = append(names, k)
				}
			}
			slices.Sort(names[1:])
			if err := uses(s.RowsFrom.Table, names); err != nil {
				return nil, err
			}
		}
	case *Form:
		names := []string{}
		for _, f := range s.Fields {
			names = append(names, f.Field)
		}
		if s.Submit.Key != "" {
			names = append(names, s.Submit.Key)
		}
		if s.Submit.Table != "" {
			if err := uses(s.Submit.Table, names); err != nil {
				return nil, err
			}
		}
	}
	for _, q := range viewQueries(v) {
		if err := query(q); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// TxEnv is the Env of a migration's transaction: the schema after its
// steps, the configs as it saved them, and the SQL check on its own
// connection (store.DependentTx).
type TxEnv struct{ T *store.DependentTx }

var _ Env = TxEnv{}

func (e TxEnv) Config(_ context.Context, kind store.ConfigKind, cid string) (store.Config, bool, error) {
	c, err := e.T.Config(kind, cid)
	if err != nil || c == nil {
		return store.Config{}, false, err
	}
	return *c, true, nil
}

func (e TxEnv) IDs(_ context.Context, kind store.ConfigKind) ([]string, error) {
	cs, err := e.T.Configs(kind)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out, nil
}

func (e TxEnv) Tables(context.Context) ([]string, error) { return e.T.Tables() }

func (e TxEnv) Table(_ context.Context, name string) (store.Table, bool, error) {
	t, err := e.T.TableShape(name)
	if errors.Is(err, store.ErrNotFound) {
		return store.Table{}, false, nil
	}
	return t, err == nil, err
}

func (e TxEnv) QueryColumns(_ context.Context, q string) ([]store.QueryColumn, error) {
	return e.T.QueryColumns(q)
}

func (e TxEnv) Object(_ context.Context, key string) (bool, error) { return e.T.Object(key) }
