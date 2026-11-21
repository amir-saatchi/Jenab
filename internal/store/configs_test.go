package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// The YAML of a view as an agent writes it, with a comment.
const priceView = `version: 1
id: btc_prices
title: BTC price
type: table
# last 90 days
query: SELECT date, price FROM prices
`

func saveView(t *testing.T, p *ProjectDB, src id.Source, cid, yaml string, expected int) ConfigWrite {
	t.Helper()
	w, err := p.SaveConfig(context.Background(), ConfigSave{Kind: KindView, ID: cid, Type: "table", Title: "BTC price", YAML: yaml, Expected: expected, Source: src})
	if err != nil {
		t.Fatalf("save %s: %v", cid, err)
	}
	return w
}

// configRow is a config's row as it is, deleted or not; zero if none.
func configRow(t *testing.T, p *ProjectDB, kind ConfigKind, cid string) Config {
	t.Helper()
	cs, err := Query(context.Background(), p.DB, "SELECT "+configCols+" FROM "+kind.table()+" WHERE id = ?", []any{cid}, scanConfig(kind))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 {
		return Config{}
	}
	return cs[0]
}

func TestConfigSaveReviseDelete(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)

	w := saveView(t, p, msg("t1"), "btc_prices", priceView, 0)
	if w.Op != "save" || w.Config.Revision != 1 || w.Config.YAML != priceView || w.Config.UpdatedBy != msg("t1") || w.Change == "" {
		t.Fatalf("first save: %+v", w)
	}
	c := lastChangeOf(t, p, w.Change)
	if c.Target != "view:btc_prices" || c.Op != "save" || c.Before.Valid || c.Source != string(msg("t1")) ||
		!strings.Contains(c.After.String, "# last 90 days") {
		t.Fatalf("change %+v", c)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_change_rows WHERE change_id = ? AND table_name = '_jenab_views'
		AND row_key = 'view:btc_prices' AND kind = 'i'`, w.Change); n != 1 {
		t.Fatalf("%d row entries", n)
	}

	// The same again writes nothing.
	if again := saveView(t, p, msg("t2"), "btc_prices", priceView, 0); again.Op != "" || again.Config.Revision != 1 {
		t.Fatalf("same save: %+v", again)
	}

	// A revision.
	revised := strings.Replace(priceView, "90 days", "30 days", 1)
	w = saveView(t, p, msg("t2"), "btc_prices", revised, 1)
	if w.Op != "save" || w.Config.Revision != 2 {
		t.Fatalf("revise: %+v", w)
	}
	c = lastChangeOf(t, p, w.Change)
	if !strings.Contains(c.Before.String, "90 days") || !strings.Contains(c.After.String, "30 days") {
		t.Fatalf("revise change %+v", c)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_change_rows WHERE change_id = ? AND kind = 'u' AND before IS NOT NULL`, w.Change); n != 1 {
		t.Fatalf("%d row entries for the revision", n)
	}

	// A save from revision 1 is stale, with the config as it is now.
	_, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "btc_prices", Type: "table", YAML: priceView + "# x\n", Expected: 1, Source: msg("t3")})
	var ce *ConfigConflictError
	if !errors.As(err, &ce) || !errors.Is(err, ErrConflict) || ce.Current.Revision != 2 || ce.Current.YAML != revised {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "new_one", Type: "table", YAML: priceView, Expected: 3, Source: msg("t3")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected revision of a config that isn't there: %v", err)
	}

	// Views and pages share their IDs.
	page := "version: 1\nid: bitcoin\ntitle: Bitcoin\nrows:\n  - [ { view: btc_prices } ]\n"
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "btc_prices", Title: "x", YAML: page, Source: msg("t3")}); !errors.Is(err, ErrConfigIDTaken) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("a page with a view's ID: %v", err)
	}
	pw, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "bitcoin", Type: "ignored", Title: "Bitcoin", YAML: page, Source: msg("t3")})
	if err != nil || pw.Config.Type != "page" || pw.Config.Revision != 1 {
		t.Fatalf("page: %+v, %v", pw, err)
	}
	saveView(t, p, msg("t3"), "btc_news", "version: 1\nid: btc_news\n", 0)

	// Lists and reads.
	views, err := p.Configs(ctx, KindView)
	if err != nil || len(views) != 2 || views[0].ID != "btc_news" || views[1].ID != "btc_prices" || views[1].YAML != revised {
		t.Fatalf("views %+v, %v", views, err)
	}
	if pages, err := p.Configs(ctx, KindPage); err != nil || len(pages) != 1 || pages[0].Title != "Bitcoin" {
		t.Fatalf("pages %+v, %v", pages, err)
	}
	if got, err := p.Config(ctx, "", "bitcoin"); err != nil || got.Kind != KindPage || got.YAML != page {
		t.Fatalf("read by ID alone: %+v, %v", got, err)
	}
	if _, err := p.Config(ctx, KindView, "bitcoin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read a page as a view: %v", err)
	}

	// Delete, by ID alone.
	if _, err := p.DeleteConfig(ctx, ConfigDelete{ID: "btc_prices", Expected: 1, Source: msg("t4")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	d, err := p.DeleteConfig(ctx, ConfigDelete{ID: "btc_prices", Source: msg("t4")})
	if err != nil || d.Op != "delete" || d.Config.Kind != KindView || d.Config.Revision != 3 || d.Config.Live() {
		t.Fatalf("delete: %+v, %v", d, err)
	}
	c = lastChangeOf(t, p, d.Change)
	if c.Op != "delete" || c.Target != "view:btc_prices" || !strings.Contains(c.Before.String, "30 days") {
		t.Fatalf("delete change %+v", c)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_change_rows WHERE change_id = ? AND kind = 'd'`, d.Change); n != 1 {
		t.Fatalf("%d row entries for the delete", n)
	}
	if _, err := p.Config(ctx, "", "btc_prices"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read a deleted view: %v", err)
	}
	if views, _ := p.Configs(ctx, KindView); len(views) != 1 {
		t.Fatalf("views after the delete: %+v", views)
	}
	if _, err := p.DeleteConfig(ctx, ConfigDelete{ID: "btc_prices", Source: msg("t4")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	// Saved again, it goes on from its revision; and now a page may take
	// a deleted view's ID.
	if w := saveView(t, p, msg("t5"), "btc_prices", priceView, 0); w.Config.Revision != 4 {
		t.Fatalf("saved again: %+v", w)
	}
	if _, err := p.DeleteConfig(ctx, ConfigDelete{Kind: KindView, ID: "btc_news", Source: msg("t5")}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "btc_news", Title: "News", YAML: page, Source: msg("t5")}); err != nil {
		t.Fatalf("a page with a deleted view's ID: %v", err)
	}

	// Bad saves.
	for name, s := range map[string]ConfigSave{
		"no source":    {Kind: KindView, ID: "a", Type: "table", YAML: "x: 1"},
		"no kind":      {ID: "a", Type: "table", YAML: "x: 1", Source: msg("t")},
		"bad kind":     {Kind: "pipeline", ID: "a", Type: "table", YAML: "x: 1", Source: msg("t")},
		"empty ID":     {Kind: KindView, Type: "table", YAML: "x: 1", Source: msg("t")},
		"ID with a /":  {Kind: KindView, ID: "a/b", Type: "table", YAML: "x: 1", Source: msg("t")},
		"ID with a :":  {Kind: KindView, ID: "a:b", Type: "table", YAML: "x: 1", Source: msg("t")},
		"long ID":      {Kind: KindView, ID: strings.Repeat("a", MaxConfigID+1), Type: "table", YAML: "x: 1", Source: msg("t")},
		"empty YAML":   {Kind: KindView, ID: "a", Type: "table", YAML: " \n", Source: msg("t")},
		"no type":      {Kind: KindView, ID: "a", YAML: "x: 1", Source: msg("t")},
		"not UTF-8":    {Kind: KindView, ID: "a", Type: "table", YAML: "x: \xff", Source: msg("t")},
		"negative rev": {Kind: KindView, ID: "a", Type: "table", YAML: "x: 1", Expected: -1, Source: msg("t")},
	} {
		if _, err := p.SaveConfig(ctx, s); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A Persian ID works.
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "قیمت_بیت‌کوین", Type: "stat", YAML: "x: 1", Source: msg("t")}); err != nil {
		t.Errorf("a Persian ID: %v", err)
	}
}

// Undo turn and its redo put configs back by revision; a later save is a
// conflict; undoing a first save deletes, undoing a delete restores.
func TestConfigUndo(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	t1, t2 := []id.Source{msg("t1")}, []id.Source{msg("t2")}
	v1 := saveView(t, p, t1[0], "btc_prices", priceView, 0)
	revised := strings.Replace(priceView, "90 days", "30 days", 1)
	saveView(t, p, t2[0], "btc_prices", revised, 1)

	st, err := p.UndoState(ctx, t2)
	if err != nil || st.Undo.Summary() != "1 view" {
		t.Fatalf("state %+v, %v", st, err)
	}
	r := undoOK(t, p, t2, UndoOptions{})
	if got := configRow(t, p, KindView, "btc_prices"); got.YAML != priceView || got.Revision != 3 || got.UpdatedBy.Kind() != "undo" {
		t.Fatalf("after undo: %+v", got)
	}
	if len(r.Configs) != 1 || r.Configs[0] != (ConfigChange{Kind: KindView, ID: "btc_prices", Op: "save", Revision: 3}) {
		t.Errorf("undo's configs %+v", r.Configs)
	}
	// A writer holding revision 2 is refused.
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "btc_prices", Type: "table", YAML: "x: 1", Expected: 2, Source: msg("t3")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("save from before the undo: %v", err)
	}
	undoOK(t, p, t2, UndoOptions{Redo: true})
	if got := configRow(t, p, KindView, "btc_prices"); got.YAML != revised || got.Revision != 4 {
		t.Fatalf("after redo: %+v", got)
	}

	// Turn 1 under turn 2's later save: a conflict, then skipped.
	r, err = p.Undo(ctx, t1, UndoOptions{})
	// Turn 2's save, its undo and its redo each touched it later.
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 3 {
		t.Fatalf("undo under a later save: %+v, %v", r, err)
	}
	it := UndoItem{"_jenab_views", "view:btc_prices"}
	if x := r.Conflicts[0]; x.Item != it || x.Reason != ConflictChanged || x.Source != msg("t2") {
		t.Fatalf("conflict %+v", x)
	}
	if got := configRow(t, p, KindView, "btc_prices"); got.Revision != 4 {
		t.Fatalf("a refused undo changed the view: %+v", got)
	}
	if r := undoOK(t, p, t1, UndoOptions{Skip: []UndoItem{it}}); len(r.Skipped) != 1 || r.Plan.Changes != 0 || len(r.Configs) != 0 {
		t.Fatalf("all skipped: %+v", r)
	}
	// Once turn 2 is undone, turn 1 undoes: its first save is deleted.
	undoOK(t, p, t2, UndoOptions{})
	undoOK(t, p, t1, UndoOptions{})
	if got := configRow(t, p, KindView, "btc_prices"); got.Live() || got.Revision != 6 {
		t.Fatalf("after undoing the first save: %+v", got)
	}
	if _, err := p.Config(ctx, KindView, "btc_prices"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read after undoing the first save: %v", err)
	}
	// And redone, oldest first.
	undoOK(t, p, t1, UndoOptions{Redo: true})
	undoOK(t, p, t2, UndoOptions{Redo: true})
	if got := configRow(t, p, KindView, "btc_prices"); got.YAML != revised {
		t.Fatalf("after redoing both: %+v", got)
	}

	// A delete undone brings the config back; the chip's undo works by
	// its change.
	t4 := []id.Source{msg("t4")}
	if _, err := p.DeleteConfig(ctx, ConfigDelete{ID: "btc_prices", Source: t4[0]}); err != nil {
		t.Fatal(err)
	}
	r = undoOK(t, p, t4, UndoOptions{})
	if got, err := p.Config(ctx, "", "btc_prices"); err != nil || got.YAML != revised {
		t.Fatalf("after undoing the delete: %+v, %v", got, err)
	}
	if r.Configs[0].Op != "save" {
		t.Errorf("undo of a delete reports %+v", r.Configs)
	}
	if u, err := p.ChangeUndone(ctx, v1.Change); err != nil || u {
		t.Errorf("first save reads undone: %v, %v", u, err)
	}
	w := saveView(t, p, id.SourceUser, "btc_prices", priceView, 0)
	if _, err := p.UndoChange(ctx, w.Change, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := configRow(t, p, KindView, "btc_prices"); got.YAML != revised {
		t.Fatalf("after the chip's undo: %+v", got)
	}
	if u, err := p.ChangeUndone(ctx, w.Change); err != nil || !u {
		t.Errorf("the chip's change reads %v, %v", u, err)
	}
}

// A turn that saved a page and wrote rows is partly undone when one of its
// items is skipped, and the rest is undone later.
func TestConfigUndoPartlyWithRows(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE prices (coin TEXT, date TEXT, price REAL, PRIMARY KEY (coin, date))`)
	turn, later := []id.Source{msg("turn")}, []id.Source{msg("later")}
	if _, err := p.Insert(ctx, limit.Interactive, turn[0], "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	page := "version: 1\nid: bitcoin\ntitle: Bitcoin\nrows: []\n"
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "bitcoin", Title: "Bitcoin", YAML: page, Source: turn[0]}); err != nil {
		t.Fatal(err)
	}
	saveView(t, p, turn[0], "btc_prices", priceView, 0)
	// Another chat changes the view later.
	saveView(t, p, later[0], "btc_prices", priceView+"# later\n", 0)

	r, err := p.Undo(ctx, turn, UndoOptions{Check: true})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Item.Key != "view:btc_prices" {
		t.Fatalf("check: %+v, %v", r, err)
	}
	if got := configRow(t, p, KindPage, "bitcoin"); !got.Live() {
		t.Fatal("a checked undo changed the page")
	}
	r = undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{r.Conflicts[0].Item}})
	if r.Plan.Summary() != "1 row in prices, 1 page" || len(r.Configs) != 1 || r.Configs[0].Op != "delete" || r.Configs[0].Kind != KindPage {
		t.Fatalf("undo with the view skipped: %q, %+v", r.Plan.Summary(), r.Configs)
	}
	if one[int](t, p.DB, `SELECT count(*) FROM prices`) != 0 || configRow(t, p, KindPage, "bitcoin").Live() {
		t.Fatal("the row or the page is still there")
	}
	if got := configRow(t, p, KindView, "btc_prices"); !strings.Contains(got.YAML, "# later") {
		t.Fatalf("the skipped view changed: %+v", got)
	}
	st, err := p.UndoState(ctx, turn)
	if err != nil || st.Undo.Summary() != "1 view" || st.Redo.Summary() != "1 row in prices, 1 page" || st.Last == nil || st.Last.Redo {
		t.Fatalf("partly undone: %+v, %v", st, err)
	}
	// The later save undone, the rest of the turn undoes.
	undoOK(t, p, later, UndoOptions{})
	r = undoOK(t, p, turn, UndoOptions{})
	if r.Plan.Summary() != "1 view" || configRow(t, p, KindView, "btc_prices").Live() {
		t.Fatalf("the rest: %q, %+v", r.Plan.Summary(), configRow(t, p, KindView, "btc_prices"))
	}
	if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 0 || st.Redo.Summary() != "1 row in prices, 1 page, 1 view" {
		t.Fatalf("after the rest: %+v", st)
	}
	// Redo puts all back.
	undoOK(t, p, turn, UndoOptions{Redo: true})
	if one[int](t, p.DB, `SELECT count(*) FROM prices`) != 1 || !configRow(t, p, KindPage, "bitcoin").Live() ||
		configRow(t, p, KindView, "btc_prices").YAML != priceView {
		t.Fatal("redo left something out")
	}
}

// A delete undone while the other kind took its ID is a conflict.
func TestConfigUndoIDTaken(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	saveView(t, p, msg("t1"), "btc", priceView, 0)
	if _, err := p.DeleteConfig(ctx, ConfigDelete{ID: "btc", Source: msg("t2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "btc", Title: "BTC", YAML: "rows: []", Source: msg("t3")}); err != nil {
		t.Fatal(err)
	}
	r, err := p.Undo(ctx, []id.Source{msg("t2")}, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 {
		t.Fatalf("undo of the delete: %+v, %v", r, err)
	}
	x := r.Conflicts[0]
	if x.Reason != ConflictConstraint || x.Reached == nil || *x.Reached != (UndoItem{"_jenab_pages", "page:btc"}) {
		t.Fatalf("conflict %+v", x)
	}
	if configRow(t, p, KindView, "btc").Live() {
		t.Fatal("the view came back")
	}
}

// Saves, deletes and undos send their changes once committed; checks,
// refused undos and saves that change nothing send none.
func TestConfigEvents(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	var mu sync.Mutex
	var got [][]ConfigChange
	p.OnConfigs(func(cs []ConfigChange) {
		mu.Lock()
		got = append(got, slices.Clone(cs))
		mu.Unlock()
	})
	take := func() [][]ConfigChange {
		mu.Lock()
		defer mu.Unlock()
		g := got
		got = nil
		return g
	}
	saveView(t, p, msg("t1"), "a", priceView, 0)
	if g := take(); len(g) != 1 || g[0][0] != (ConfigChange{Kind: KindView, ID: "a", Op: "save", Revision: 1}) {
		t.Fatalf("save sent %+v", g)
	}
	saveView(t, p, msg("t1"), "a", priceView, 0)
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "a", Type: "table", YAML: "x", Expected: 9, Source: msg("t1")}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if g := take(); len(g) != 0 {
		t.Fatalf("a save of the same, and a refused one, sent %+v", g)
	}
	if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "b", YAML: "rows: []", Source: msg("t1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DeleteConfig(ctx, ConfigDelete{ID: "a", Source: msg("t2")}); err != nil {
		t.Fatal(err)
	}
	if g := take(); len(g) != 2 || g[1][0] != (ConfigChange{Kind: KindView, ID: "a", Op: "delete", Revision: 2}) {
		t.Fatalf("page save and view delete sent %+v", g)
	}
	if _, err := p.Undo(ctx, []id.Source{msg("t1")}, UndoOptions{Check: true}); !errors.Is(err, ErrUndoConflict) {
		t.Fatal(err)
	}
	if _, err := p.Undo(ctx, []id.Source{msg("t2")}, UndoOptions{Check: true}); err != nil {
		t.Fatal(err)
	}
	if g := take(); len(g) != 0 {
		t.Fatalf("checks and a refused undo sent %+v", g)
	}
	undoOK(t, p, []id.Source{msg("t2")}, UndoOptions{})
	undoOK(t, p, []id.Source{msg("t1")}, UndoOptions{})
	g := take()
	if len(g) != 2 || len(g[0]) != 1 || g[0][0].Op != "save" || len(g[1]) != 2 {
		t.Fatalf("undos sent %+v", g)
	}
	for _, c := range g[1] {
		if c.Op != "delete" {
			t.Errorf("undo of turn 1 sent %+v", c)
		}
	}
	// Row writes send nothing.
	mustExec(t, p.DB, `CREATE TABLE notes (id INTEGER PRIMARY KEY, text TEXT)`)
	if _, err := p.Insert(ctx, limit.Interactive, msg("t3"), "notes", []Row{{"id": 1, "text": "x"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, []id.Source{msg("t3")}, UndoOptions{})
	if g := take(); len(g) != 0 {
		t.Fatalf("rows sent %+v", g)
	}
}

// A project.db from before views and pages is backed up, then gets their
// tables, its data kept; an app from before them refuses the new file.
func TestConfigFormatUpdate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "project.db")
	before := Format{Name: "project", Steps: ProjectFormat.Steps[:5]} // before step 6, views and pages
	old, err := Open(ctx, path, Options{Format: before, Snapshots: filepath.Join(dir, "snapshots")})
	if err != nil {
		t.Fatal(err)
	}
	op := newProjectDB(old, dir)
	if _, err := op.UpdateMemory(ctx, MemoryEdit{Section: "goals", Content: "Track BTC.", Source: msg("t1")}); err != nil {
		t.Fatal(err)
	}
	if n := one[int](t, op.DB, `SELECT count(*) FROM sqlite_schema WHERE name = '_jenab_views'`); n != 0 {
		t.Fatal("the old format has views")
	}
	if err := op.Close(ctx); err != nil {
		t.Fatal(err)
	}

	p, err := OpenProject(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if v := one[int](t, p.DB, "PRAGMA user_version"); v != len(ProjectFormat.Steps) {
		t.Fatalf("user_version %d", v)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "snapshots", "project-pre-update-*-v5.db"))
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	if projectMemory(t, p, "goals") != "Track BTC." {
		t.Fatal("memory lost")
	}
	saveView(t, p, msg("t2"), "btc_prices", priceView, 0)
	undoOK(t, p, []id.Source{msg("t2")}, UndoOptions{})
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// The backup is the old file as it was.
	b, err := Open(ctx, backups[0], Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if v := one[int](t, b, "PRAGMA user_version"); v != len(before.Steps) {
		t.Errorf("backup's user_version %d", v)
	}
	b.Close(ctx)

	if _, err := Open(ctx, path, Options{Format: before, Snapshots: filepath.Join(dir, "snapshots")}); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("the old app opened the new file: %v", err)
	}
}

// Writers racing on one view see their events in commit order: each
// revision once, rising. The event goes out on the writer, right after
// the commit, not after Do returns to each caller.
func TestConfigEventsInCommitOrder(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	var mu sync.Mutex
	var revs []int
	p.OnConfigs(func(cs []ConfigChange) {
		if cs[0].Revision == 1 {
			// Hold the first event, as a busy UI thread might: an event
			// sent after Do returned would let later revisions pass it.
			time.Sleep(50 * time.Millisecond)
		}
		mu.Lock()
		for _, c := range cs {
			revs = append(revs, c.Revision)
		}
		mu.Unlock()
	})
	const n = 40
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			yaml := priceView + "# writer " + strconv.Itoa(i) + "\n"
			if _, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "racing", Type: "table", YAML: yaml, Source: msg("t1")}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(revs) != n {
		t.Fatalf("%d events for %d saves", len(revs), n)
	}
	for i, r := range revs {
		if r != i+1 {
			t.Fatalf("events out of commit order: %v", revs)
		}
	}
	// A hook that panics loses its event, not the write's answer.
	p.OnConfigs(func([]ConfigChange) { panic("hook") })
	if w, err := p.SaveConfig(ctx, ConfigSave{Kind: KindView, ID: "racing", Type: "table", YAML: priceView, Source: msg("t2")}); err != nil || w.Config.Revision != n+1 {
		t.Fatalf("save with a panicking hook %+v, %v", w, err)
	}
	if c := configRow(t, p, KindView, "racing"); c.Revision != n+1 {
		t.Fatalf("after the panic %+v", c)
	}
}

// An ID is one config however it is spelled: the decomposed Persian آ (ا
// with a madda mark) and the composed one are stored, read, taken and
// deleted as the composed (NFC) form.
func TestConfigIDNormalized(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	const nfc = "\u0622\u0645\u0627\u0631_\u0631\u0648\u0632"       // the composed form
	const nfd = "\u0627\u0653\u0645\u0627\u0631_\u0631\u0648\u0632" // the same, with \u0622 as \u0627 and the mark U+0653
	if nfc == nfd || NormConfigID(nfd) != nfc {
		t.Fatal("the test IDs aren't two spellings of one ID")
	}
	w := saveView(t, p, msg("t1"), nfd, priceView, 0)
	if w.Config.ID != nfc || w.Op != "save" {
		t.Fatalf("saved %+v", w)
	}
	if c := configRow(t, p, KindView, nfc); c.Revision != 1 {
		t.Fatalf("not stored as NFC: %+v", c)
	}
	if c := configRow(t, p, KindView, nfd); c.Revision != 0 {
		t.Fatalf("an NFD row too: %+v", c)
	}
	// The other spelling saves the same config, a new revision.
	if w := saveView(t, p, msg("t1"), nfc, priceView+"# again\n", 1); w.Config.Revision != 2 {
		t.Fatalf("second spelling %+v", w)
	}
	for _, cid := range []string{nfc, nfd} {
		if c, err := p.Config(ctx, "", cid); err != nil || c.ID != nfc || c.Revision != 2 {
			t.Fatalf("read %q: %+v, %v", cid, c, err)
		}
	}
	if cs, err := p.Configs(ctx, KindView); err != nil || len(cs) != 1 {
		t.Fatalf("list %+v, %v", cs, err)
	}
	// A page can't take it in either spelling.
	_, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: nfd, Title: "x", YAML: "rows: []\n", Source: msg("t1")})
	if !errors.Is(err, ErrConfigIDTaken) {
		t.Fatalf("page with the view's NFD ID: %v", err)
	}
	// Deleting by the decomposed spelling deletes it.
	if w, err := p.DeleteConfig(ctx, ConfigDelete{ID: nfd, Source: msg("t2")}); err != nil || w.Config.ID != nfc || w.Op != "delete" {
		t.Fatalf("delete %+v, %v", w, err)
	}
	if _, err := p.Config(ctx, KindView, nfc); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	// Targets in the change log use the stored form.
	n, err := readTx(ctx, p.DB, func(tx *sql.Tx) (n int, err error) {
		err = tx.QueryRow(`SELECT count(*) FROM _jenab_changes WHERE target = ?`, ConfigTarget(KindView, nfc)).Scan(&n)
		return n, err
	})
	if err != nil || n != 3 {
		t.Fatalf("%d changes for the NFC target, %v", n, err)
	}
}
