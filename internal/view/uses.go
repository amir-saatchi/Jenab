package view

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/store"
)

// What saved configs use, for the project card (SPEC 3.2) and
// delete_config (8.1, P3-03): the views a page shows, the forms and pages
// buttons and actions open, and the tables a view reads or writes.

// Uses is what one saved config refers to. Each list has a name once, in
// the order it first comes.
type Uses struct {
	Views  []string // a page's view blocks, in cards too
	Forms  []string // open_form targets: a page's actions and buttons, a table's row actions
	Pages  []string // open_page targets: a page's actions and buttons
	Tables []string // a view's tables: in its queries, rows_from and a form's submit, as the project names them
}

// UsesOf reads what c uses. tables are the project's tables: a query's
// names are matched against them, without case, so a column or an alias
// with a table's name counts too, which the card can bear. A config that
// doesn't parse uses nothing.
func UsesOf(reg *Registry, c store.Config, tables []string) Uses {
	if reg == nil {
		reg = Default
	}
	var u Uses
	if c.Kind == store.KindPage {
		p, err := ParsePage(reg, []byte(c.YAML))
		if p == nil && err != nil {
			return u
		}
		for _, a := range p.Actions {
			u.action(a.OpenForm, a.OpenPage)
		}
		u.rows(p.Rows)
		return u
	}
	v, _ := ParseView(reg, []byte(c.YAML))
	if v == nil {
		return u
	}
	t := tableNames(tables)
	u.query(t, v.Query)
	u.filters(t, v.Filters)
	switch s := v.Spec.(type) {
	case *Table:
		if s.RowsFrom != nil {
			u.table(t, s.RowsFrom.Table)
		}
		for _, a := range s.RowActions {
			u.action(a.OpenForm, "")
		}
	case *Form:
		u.table(t, s.Submit.Table)
		u.query(t, s.Load)
		for _, f := range s.Fields {
			if f.Options != nil {
				u.query(t, f.Options.Query)
			}
		}
	}
	return u
}

// tableNames maps the lower-case name of each table to its name.
func tableNames(tables []string) map[string]string {
	m := make(map[string]string, len(tables))
	for _, n := range tables {
		m[strings.ToLower(n)] = n
	}
	return m
}

func (u *Uses) rows(rows [][]Block) {
	for _, row := range rows {
		for _, b := range row {
			switch s := b.Spec.(type) {
			case *ViewBlock:
				add(&u.Views, store.NormConfigID(s.View))
			case *ButtonBlock:
				u.action(s.OpenForm, s.OpenPage)
			}
			u.rows(b.Rows)
		}
	}
}

func (u *Uses) action(form, page string) {
	if form != "" {
		add(&u.Forms, store.NormConfigID(form))
	}
	if page != "" {
		add(&u.Pages, store.NormConfigID(page))
	}
}

func (u *Uses) filters(t map[string]string, fs []Filter) {
	for _, f := range fs {
		if f.Options != nil {
			u.query(t, f.Options.Query)
		}
	}
}

func (u *Uses) query(t map[string]string, q string) {
	if q == "" {
		return
	}
	names, err := sqlguard.Names(q)
	if err != nil {
		return
	}
	for _, n := range names {
		u.table(t, n)
	}
}

func (u *Uses) table(t map[string]string, name string) {
	if n, ok := t[strings.ToLower(name)]; ok {
		add(&u.Tables, n)
	}
}

func add(list *[]string, s string) {
	if s != "" && !slices.Contains(*list, s) {
		*list = append(*list, s)
	}
}

// Users are the saved configs that use one config: what delete_config
// names when it refuses (P3-03).
type Users struct {
	Pages []string // pages that show the view, or open the form or page
	Views []string // table views whose row actions open the form
}

// Any reports whether something uses the config.
func (u Users) Any() bool { return len(u.Pages) > 0 || len(u.Views) > 0 }

// UsersOf finds the configs among all that use the config of kind and id:
// a page that shows a view or opens it as a form, a table view whose row
// action opens it as a form, and, for a page, a page that opens it. The
// config itself is left out, as is any deleted one.
func UsersOf(reg *Registry, all []store.Config, kind store.ConfigKind, id string) Users {
	id = store.NormConfigID(id)
	var out Users
	for _, c := range all {
		if !c.Live() || c.Kind == kind && c.ID == id {
			continue
		}
		u := UsesOf(reg, c, nil)
		uses := slices.Contains(u.Forms, id)
		if kind == store.KindView {
			uses = uses || slices.Contains(u.Views, id)
		} else {
			uses = slices.Contains(u.Pages, id)
		}
		if !uses {
			continue
		}
		if c.Kind == store.KindPage {
			add(&out.Pages, c.ID)
		} else {
			add(&out.Views, c.ID)
		}
	}
	return out
}

// CardRefs is the store's RefsFunc for the project card (SPEC 3.2): a
// page's views, and a view's tables, forms aside from views by type.
func CardRefs(c store.Config, tables []string) store.ConfigRefs {
	u := UsesOf(Default, c, tables)
	return store.ConfigRefs{Views: u.Views, Tables: u.Tables}
}

// Card builds db's project card as of at, with what each saved view and
// page uses.
func Card(ctx context.Context, db *store.ProjectDB, at time.Time) (string, error) {
	return db.CardWith(ctx, at, CardRefs)
}

// MissingRefs checks what c refers to against all, the live views and
// pages: the views a page shows, the forms it and a table's row actions
// open, and the pages it opens. It gives an issue for each one that isn't
// there, worded as validation words it (step 3), for a save whose targets
// went after it was validated (P3-03). A page may open itself.
func MissingRefs(reg *Registry, c store.Config, all []store.Config) []string {
	find := func(kind store.ConfigKind, id string) *store.Config {
		for i := range all {
			if all[i].Live() && all[i].Kind == kind && all[i].ID == id {
				return &all[i]
			}
		}
		return nil
	}
	ids := func(kind store.ConfigKind) []string {
		var out []string
		for _, s := range all {
			if s.Live() && s.Kind == kind {
				out = append(out, s.ID)
			}
		}
		return out
	}
	missing := func(kind store.ConfigKind, id, what string) string {
		msg := fmt.Sprintf("no %s %s", what, id)
		if other := find(otherKind(kind), id); other != nil {
			msg += fmt.Sprintf("; %s is a %s", other.ID, other.Kind)
		} else {
			msg += suggest(id, ids(kind))
		}
		if kind == store.KindView {
			msg += "; save the view first"
		}
		return msg
	}
	u := UsesOf(reg, c, nil)
	var out []string
	for _, v := range u.Views {
		if find(store.KindView, v) == nil {
			out = append(out, missing(store.KindView, v, "view"))
		}
	}
	for _, f := range u.Forms {
		switch s := find(store.KindView, f); {
		case s == nil:
			out = append(out, missing(store.KindView, f, "form"))
		case s.Type != "form":
			out = append(out, fmt.Sprintf("%s is a %s view, not a form", s.ID, s.Type))
		}
	}
	for _, p := range u.Pages {
		if (c.Kind != store.KindPage || p != store.NormConfigID(c.ID)) && find(store.KindPage, p) == nil {
			out = append(out, missing(store.KindPage, p, "page"))
		}
	}
	return out
}
