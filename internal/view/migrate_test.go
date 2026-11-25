package view

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The migration tests run on projects of their own, as they change the
// schema: btc_news, a table view and a form over it, and a chart over
// btc_prices, which no rename of btc_news reaches.

const newsTable = `version: 1
id: news_table
title: News
type: table
query: SELECT date, title, reason, url FROM btc_news
default_sort: { field: date, direction: desc }
columns:
  - { field: date, format: date }
  - { field: title }
  - { field: reason, label: "Why it matters" }
  - { field: url, format: url }
`

const editNews = `version: 1
id: edit_news
title: Edit news
type: form
load: "SELECT title, reason FROM btc_news WHERE url = :key"
fields:
  - { field: title, control: text, required: true }
  - { field: reason, control: textarea }
submit: { action: update, table: btc_news, key: url }
`

const priceChart = `version: 1
id: price_chart
title: Price
type: chart
chart_type: line
query: SELECT date, price FROM btc_prices
x: { field: date, format: date }
y:
  - { field: price }
`

// migrateProject opens a project with views and forms as its migrations'
// dependents, as the project manager adds them, and the configs above.
func migrateProject(t *testing.T) *store.ProjectDB {
	t.Helper()
	ctx := context.Background()
	p, err := store.OpenProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(ctx) })
	_, err = store.Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for _, s := range []string{
			`CREATE TABLE btc_prices(date TEXT PRIMARY KEY, price REAL NOT NULL) WITHOUT ROWID`,
			`CREATE TABLE btc_news(url TEXT PRIMARY KEY, date TEXT NOT NULL, title TEXT NOT NULL, reason TEXT) WITHOUT ROWID`,
			`INSERT INTO btc_news VALUES ('https://example.com/a', '2026-11-20', 'ETF inflows', 'demand')`,
		} {
			if _, err := tx.Exec(s); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	AddDependents(p)
	for _, y := range []string{newsTable, editNews, priceChart} {
		saveView(t, p, y, id.SourceUser)
	}
	return p
}

// saveView saves the view y as it is, without validating it.
func saveView(t *testing.T, p *store.ProjectDB, y string, src id.Source) {
	t.Helper()
	pv, err := ParseView(Default, []byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveConfig(context.Background(), store.ConfigSave{Kind: store.KindView, ID: pv.ID, Type: pv.Type, Title: pv.Title, YAML: y, Source: src}); err != nil {
		t.Fatal(err)
	}
}

const renameReason = `{"op": "rename_column", "table": "btc_news", "from": "reason", "to": "topic"}`

// update is a dependent update with the YAML y as a string, as the
// schema agent sends it.
func update(kind, vid, y string) store.DependentUpdate {
	b, _ := json.Marshal(y)
	return store.DependentUpdate{Kind: kind, ID: vid, Config: b}
}

func renamed(y string) string { return strings.ReplaceAll(y, "reason", "topic") }

func migration(steps string, us ...store.DependentUpdate) store.Migration {
	return migrationOf([]string{steps}, us...)
}

func migrationOf(steps []string, us ...store.DependentUpdate) store.Migration {
	m := store.Migration{Updates: us}
	for _, s := range steps {
		m.Steps = append(m.Steps, json.RawMessage(s))
	}
	return m
}

// addVolume reaches price_chart and no view of btc_news.
const addVolume = `{"op": "add_column", "table": "btc_prices", "column": {"name": "volume", "type": "REAL"}}`

// count runs a count query on p.
func count(t *testing.T, p *store.ProjectDB, q string) int {
	t.Helper()
	n, err := store.Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		var n int
		return n, tx.QueryRow(q).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// waitSent waits up to d for want project:configs events, as they go
// out after the commit.
func waitSent(mu *sync.Mutex, sent *[][]store.ConfigChange, want int, d time.Duration) {
	deadline := time.Now().Add(d)
	for {
		mu.Lock()
		n := len(*sent)
		mu.Unlock()
		if n >= want || time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func columns(t *testing.T, p *store.ProjectDB, table string) []string {
	t.Helper()
	tb, err := p.TableShape(context.Background(), table)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range tb.Columns {
		out = append(out, c.Name)
	}
	return out
}

func config(t *testing.T, p *store.ProjectDB, vid string) store.Config {
	t.Helper()
	c, err := p.Config(context.Background(), store.KindView, vid)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// problems are a failed migration's problems as path: msg.
func problems(t *testing.T, err error) []string {
	t.Helper()
	var me *store.MigrationError
	if !errors.As(err, &me) {
		t.Fatalf("err %v, want a MigrationError", err)
	}
	var out []string
	for _, p := range me.Problems {
		out = append(out, p.Path+": "+p.Msg)
	}
	return out
}

// The schema agent gets each view and form with the columns it uses.
func TestDependentsListViewsAndForms(t *testing.T) {
	p := migrateProject(t)
	ds, err := p.Dependents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]store.DependentInfo{}
	for _, d := range ds {
		got[d.ID] = d
	}
	uses := func(vid string) []string {
		u := slices.Clone(got[vid].Uses)
		slices.Sort(u)
		return u
	}
	if u := uses("news_table"); !slices.Equal(u, []string{"btc_news.date", "btc_news.reason", "btc_news.title", "btc_news.url"}) {
		t.Errorf("news_table uses %v", u)
	}
	if u := uses("edit_news"); !slices.Equal(u, []string{"btc_news.reason", "btc_news.title", "btc_news.url"}) || got["edit_news"].Type != "form" {
		t.Errorf("edit_news %+v", got["edit_news"])
	}
	if u := uses("price_chart"); !slices.Equal(u, []string{"btc_prices.date", "btc_prices.price"}) || got["price_chart"].Kind != DependentView {
		t.Errorf("price_chart %+v", got["price_chart"])
	}
}

// A rename with the view and the form that use the column updated
// commits all of it: the column, both configs under the migration's
// source, and project:configs after the commit. A try saves nothing.
func TestMigrationUpdatesDependents(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	var mu sync.Mutex
	var sent [][]store.ConfigChange
	p.OnConfigs(func(cs []store.ConfigChange) {
		mu.Lock()
		sent = append(sent, cs)
		mu.Unlock()
	})
	m := migration(renameReason, update(DependentView, "news_table", renamed(newsTable)), update(DependentForm, "edit_news", renamed(editNews)))
	src := id.SourceOf("message", "m1")

	try, err := p.Migrate(ctx, src, m, store.MigrateOptions{Try: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(try.Configs) != 0 || try.Change != "" || !slices.Contains(columns(t, p, "btc_news"), "reason") || config(t, p, "news_table").YAML != newsTable {
		t.Fatalf("a try changed something: %+v", try)
	}

	rep, err := p.Migrate(ctx, src, m, store.MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Updated, []string{"view:news_table", "view:edit_news"}) || len(rep.Broken) != 0 {
		t.Errorf("updated %v, broken %v", rep.Updated, rep.Broken)
	}
	if cols := columns(t, p, "btc_news"); !slices.Contains(cols, "topic") || slices.Contains(cols, "reason") {
		t.Errorf("columns %v", cols)
	}
	want := id.SourceOf("migration", string(rep.Change))
	for _, vid := range []string{"news_table", "edit_news"} {
		c := config(t, p, vid)
		if strings.Contains(c.YAML, "reason") || c.UpdatedBy != want || c.Revision != 2 {
			t.Errorf("%s: by %s, revision %d:\n%s", vid, c.UpdatedBy, c.Revision, c.YAML)
		}
	}
	if c := config(t, p, "price_chart"); c.Revision != 1 {
		t.Errorf("price_chart revision %d", c.Revision)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(sent)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || len(sent[0]) != 2 {
		t.Fatalf("project:configs %+v", sent)
	}
	if broken, err := BrokenViews(ctx, p); err != nil || len(broken) != 0 {
		t.Errorf("broken after the migration: %v, %v", broken, err)
	}
}

// All or nothing: a dependent left out, or one whose update fails its
// check, stops the migration with its errors, and neither the schema nor
// any config changed, the update that passed included.
func TestMigrationDependentFails(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	src := id.SourceOf("message", "m1")
	unchanged := func() {
		t.Helper()
		if !slices.Contains(columns(t, p, "btc_news"), "reason") {
			t.Error("the column was renamed")
		}
		for vid, y := range map[string]string{"news_table": newsTable, "edit_news": editNews} {
			if c := config(t, p, vid); c.YAML != y || c.Revision != 1 {
				t.Errorf("%s changed: revision %d", vid, c.Revision)
			}
		}
	}

	// The form is left out.
	_, err := p.Migrate(ctx, src, migration(renameReason, update(DependentView, "news_table", renamed(newsTable))), store.MigrateOptions{})
	ps := problems(t, err)
	if len(ps) == 0 || !slices.ContainsFunc(ps, func(s string) bool {
		return strings.HasPrefix(s, "view:edit_news: ") && strings.Contains(s, "reason")
	}) || slices.ContainsFunc(ps, func(s string) bool { return strings.HasPrefix(s, "view:news_table") }) {
		t.Errorf("problems %q", ps)
	}
	unchanged()

	// The form's update still uses the old column.
	_, err = p.Migrate(ctx, src, migration(renameReason,
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "edit_news", editNews)), store.MigrateOptions{})
	ps = problems(t, err)
	if len(ps) == 0 || !strings.HasPrefix(ps[0], "updates[1].config: ") || !strings.Contains(ps[0], "reason") {
		t.Errorf("problems %q", ps)
	}
	unchanged()

	// An update of a view that isn't there, or with another id.
	_, err = p.Migrate(ctx, src, migration(renameReason,
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "nope", renamed(editNews))), store.MigrateOptions{})
	if ps = problems(t, err); !slices.ContainsFunc(ps, func(s string) bool { return strings.HasPrefix(s, "updates[1].id: ") }) {
		t.Errorf("problems %q", ps)
	}
	unchanged()
}

// A view that failed its check before a migration and isn't updated
// doesn't stop it: the report lists it, the card marks it, and it gives
// an error on its next load.
func TestMigrationLeavesBrokenView(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	bad := strings.ReplaceAll(priceChart, "price_chart", "old_chart")
	bad = strings.ReplaceAll(bad, "SELECT date, price", "SELECT date, close AS price")
	saveView(t, p, bad, id.SourceUser)

	// The migration reaches old_chart, which keeps the issue it had.
	rep, err := p.Migrate(ctx, id.SourceOf("message", "m1"), migrationOf([]string{renameReason, addVolume},
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "edit_news", renamed(editNews))), store.MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Broken, []string{"view:old_chart"}) {
		t.Errorf("broken %v", rep.Broken)
	}
	broken, err := BrokenViews(ctx, p)
	if err != nil || len(broken) != 1 || len(broken["old_chart"]) == 0 || !strings.Contains(broken["old_chart"][0], "close") {
		t.Fatalf("broken %v, %v", broken, err)
	}
	card, err := Card(ctx, p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(card, `- old_chart (chart): "Price" — broken: `) || strings.Contains(card, `- news_table (table): "News" — broken`) {
		t.Errorf("card:\n%s", card)
	}
	err = Invalid(ctx, p, config(t, p, "old_chart"))
	var ie *InvalidError
	if !errors.As(err, &ie) || ie.Type != "chart" || len(ie.Issues) == 0 || !strings.HasPrefix(err.Error(), "The view old_chart no longer works with the project: ") {
		t.Errorf("invalid %v", err)
	}
	if err := Invalid(ctx, p, config(t, p, "news_table")); err != nil {
		t.Errorf("news_table: %v", err)
	}

	// Fixed, it isn't marked any more.
	saveView(t, p, strings.ReplaceAll(priceChart, "price_chart", "old_chart"), id.SourceUser)
	if card, err := Card(ctx, p, time.Now()); err != nil || strings.Contains(card, "broken") {
		t.Errorf("card after the fix: %v\n%s", err, card)
	}

	// A view the migration broke is a problem, not a broken view.
	_, err = p.Migrate(ctx, id.SourceOf("message", "m2"), migration(`{"op": "rename_column", "table": "btc_prices", "from": "price", "to": "close"}`), store.MigrateOptions{})
	ps := problems(t, err)
	if !slices.ContainsFunc(ps, func(s string) bool { return strings.HasPrefix(s, "view:price_chart: ") }) ||
		!slices.ContainsFunc(ps, func(s string) bool { return strings.HasPrefix(s, "view:old_chart: ") }) {
		t.Errorf("problems %q", ps)
	}
}

// Undo turn reverts what the turn's messages saved, and leaves a
// migration and the configs it saved as they are (SPEC 2.6).
func TestMigrationSavesSurviveUndoTurn(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	msg := id.SourceOf("message", "m1")
	// The turn saves a view of its own, then migrates.
	saveView(t, p, strings.ReplaceAll(priceChart, "price_chart", "turn_chart"), msg)
	rep, err := p.Migrate(ctx, msg, migration(renameReason,
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "edit_news", renamed(editNews))), store.MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	res, err := p.Undo(ctx, []id.Source{msg}, store.UndoOptions{})
	if err != nil {
		t.Fatalf("undo: %v (%+v)", err, res.Conflicts)
	}
	if !slices.Contains(res.Migrations, rep.Change) {
		t.Errorf("migrations kept %v, want %s", res.Migrations, rep.Change)
	}
	if _, err := p.Config(ctx, store.KindView, "turn_chart"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("turn_chart after the undo: %v", err)
	}
	want := id.SourceOf("migration", string(rep.Change))
	for _, vid := range []string{"news_table", "edit_news"} {
		if c := config(t, p, vid); strings.Contains(c.YAML, "reason") || c.UpdatedBy != want {
			t.Errorf("%s after the undo: by %s\n%s", vid, c.UpdatedBy, c.YAML)
		}
	}
	if cols := columns(t, p, "btc_news"); !slices.Contains(cols, "topic") {
		t.Errorf("columns after the undo %v", cols)
	}
	if broken, err := BrokenViews(ctx, p); err != nil || len(broken) != 0 {
		t.Errorf("broken after the undo: %v, %v", broken, err)
	}
}

// The baseline excuses only the issues a view already had: a view that
// failed before and that the migration breaks again, here by renaming a
// column it uses while SQLite still names only the first missing one,
// stops the migration with all its issues, the old ones marked.
func TestMigrationBrokenViewNewIssue(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	bad := strings.ReplaceAll(priceChart, "price_chart", "old_chart")
	bad = strings.ReplaceAll(bad, "SELECT date, price", "SELECT date, close, price")
	saveView(t, p, bad, id.SourceUser)
	before, err := BrokenViews(ctx, p)
	if err != nil || len(before["old_chart"]) == 0 || !strings.Contains(before["old_chart"][0], "close") {
		t.Fatalf("before %v, %v", before, err)
	}

	m := migration(`{"op": "rename_column", "table": "btc_prices", "from": "price", "to": "value"}`,
		update(DependentView, "price_chart", strings.ReplaceAll(priceChart, "price FROM", "value AS price FROM")))
	_, err = p.Migrate(ctx, id.SourceOf("message", "m1"), m, store.MigrateOptions{})
	ps := problems(t, err)
	if !slices.Contains(ps, "view:old_chart: uses btc_prices.price, which the migration renamed or dropped") ||
		!slices.ContainsFunc(ps, func(s string) bool {
			return strings.HasPrefix(s, "view:old_chart: already before the migration: ") && strings.Contains(s, "close")
		}) || slices.ContainsFunc(ps, func(s string) bool { return strings.HasPrefix(s, "view:price_chart") }) {
		t.Errorf("problems %q", ps)
	}
	if cols := columns(t, p, "btc_prices"); !slices.Contains(cols, "price") {
		t.Errorf("columns %v", cols)
	}

	// The same reason alone still doesn't stop a migration.
	rep, err := p.Migrate(ctx, id.SourceOf("message", "m2"), migration(addVolume), store.MigrateOptions{})
	if err != nil || !slices.Equal(rep.Broken, []string{"view:old_chart"}) {
		t.Errorf("same reason: broken %v, %v", rep.Broken, err)
	}
}

// A migration validates only the views that use a table its steps
// change, and the ones it updates, before and after them.
func TestMigrationChecksReachedViews(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	for i := range 5 {
		saveView(t, p, strings.ReplaceAll(priceChart, "price_chart", fmt.Sprintf("chart_%d", i)), id.SourceUser)
	}
	n := validations.Load()
	_, err := p.Migrate(ctx, id.SourceOf("message", "m1"), migration(renameReason,
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "edit_news", renamed(editNews))), store.MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// news_table and edit_news, before and after; no chart.
	if got := validations.Load() - n; got != 4 {
		t.Errorf("a rename in btc_news validated %d views, want 4", got)
	}
	n = validations.Load()
	if _, err := p.Migrate(ctx, id.SourceOf("message", "m2"), migration(addVolume), store.MigrateOptions{Try: true}); err != nil {
		t.Fatal(err)
	}
	// The six charts, before and after.
	if got := validations.Load() - n; got != 12 {
		t.Errorf("a column added to btc_prices validated %d views, want 12", got)
	}
	n = validations.Load()
	if _, err := p.Migrate(ctx, id.SourceOf("message", "m3"), migration(`{"op": "create_index", "table": "btc_prices", "columns": ["price"]}`), store.MigrateOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := validations.Load() - n; got != 0 {
		t.Errorf("an index validated %d views, want 0", got)
	}
}

// With about 200 views, a migration that reaches two of them holds the
// writer for a short time; one that reaches them all for longer. The
// times are logged for the report.
func TestMigrationScopeTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	ctx := context.Background()
	p := migrateProject(t)
	for i := range 200 {
		saveView(t, p, strings.ReplaceAll(priceChart, "price_chart", fmt.Sprintf("chart_%d", i)), id.SourceUser)
	}
	// The same kind of step on each table, each one run a few times:
	// only what it reaches differs.
	timed := func(table string) time.Duration {
		var best time.Duration
		for i := range 3 {
			step := fmt.Sprintf(`{"op": "add_column", "table": %q, "column": {"name": "extra_%d", "type": "REAL"}, "value": "NULL"}`, table, i)
			start := time.Now()
			if _, err := p.Migrate(ctx, id.SourceOf("message", "m"), migration(step), store.MigrateOptions{}); err != nil {
				t.Fatal(err)
			}
			if d := time.Since(start); i == 0 || d < best {
				best = d
			}
		}
		return best
	}
	few, all := timed("btc_news"), timed("btc_prices")
	t.Logf("203 views: a migration reaching 2 took %v, one reaching 201 took %v (best of 3)", few, all)
}

// The broken-view cache is keyed by the bucket too: adding or deleting a
// file validates the views again.
func TestBrokenViewsObjectStamp(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	if _, err := BrokenViews(ctx, p); err != nil {
		t.Fatal(err)
	}
	n := validations.Load()
	if _, err := BrokenViews(ctx, p); err != nil || validations.Load() != n {
		t.Fatalf("a second call validated again: %v", err)
	}
	if _, err := p.PutObject(ctx, limit.Interactive, id.SourceUser, "images/logo.png", strings.NewReader("png"), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	if _, err := BrokenViews(ctx, p); err != nil || validations.Load() == n {
		t.Errorf("adding a file didn't validate again: %v", err)
	}
	n = validations.Load()
	if err := p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "images/logo.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := BrokenViews(ctx, p); err != nil || validations.Load() == n {
		t.Errorf("deleting a file didn't validate again: %v", err)
	}
}

// An update of a view that isn't there fails with that alone, before its
// config is read; a config that doesn't load fails with its issues.
func TestMigrationUpdateMissingView(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	src := id.SourceOf("message", "m1")
	_, err := p.Migrate(ctx, src, migration(renameReason, update(DependentView, "news_table", renamed(newsTable)),
		update(DependentView, "edit_news", renamed(editNews)), update(DependentView, "nope", "not: [yaml")), store.MigrateOptions{})
	if ps := problems(t, err); !slices.Equal(ps, []string{"updates[2].id: there is no view nope to update; a migration only updates saved views"}) {
		t.Errorf("problems %q", ps)
	}
	_, err = p.Migrate(ctx, src, migration(renameReason, update(DependentView, "news_table", "not: [yaml"),
		update(DependentView, "edit_news", renamed(editNews))), store.MigrateOptions{})
	if ps := problems(t, err); len(ps) == 0 || !strings.HasPrefix(ps[0], "updates[0].config: ") {
		t.Errorf("problems %q", ps)
	}
}

// One view updated twice in a migration is listed once, with both
// changes in order, at the last revision. A save that changes nothing is
// listed too, and sends no project:configs.
func TestMigrationUpdatesViewTwice(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	var mu sync.Mutex
	var sent [][]store.ConfigChange
	p.OnConfigs(func(cs []store.ConfigChange) {
		mu.Lock()
		sent = append(sent, cs)
		mu.Unlock()
	})
	second := strings.ReplaceAll(renamed(newsTable), "title: News", "title: Bitcoin news")
	rep, err := p.Migrate(ctx, id.SourceOf("message", "m1"), migration(renameReason,
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "edit_news", renamed(editNews)),
		update(DependentView, "news_table", second), update(DependentView, "price_chart", priceChart)), store.MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Updated, []string{"view:news_table", "view:edit_news", "view:price_chart"}) {
		t.Errorf("updated %v", rep.Updated)
	}
	var got []string
	for _, c := range rep.Configs {
		got = append(got, fmt.Sprintf("%s:%s@%d", c.Kind, c.ID, c.Revision))
	}
	if want := []string{"view:news_table@2", "view:edit_news@2", "view:news_table@3"}; !slices.Equal(got, want) {
		t.Errorf("configs %v, want %v", got, want)
	}
	if c := config(t, p, "news_table"); c.Revision != 3 || c.YAML != second {
		t.Errorf("news_table revision %d:\n%s", c.Revision, c.YAML)
	}
	if c := config(t, p, "price_chart"); c.Revision != 1 {
		t.Errorf("price_chart revision %d", c.Revision)
	}
	waitSent(&mu, &sent, 1, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || len(sent[0]) != 3 {
		t.Errorf("project:configs %+v", sent)
	}
}

// A migration with updates that rolls back sends no project:configs and
// leaves no config revision or change-log row behind.
func TestMigrationRollbackLeavesNothing(t *testing.T) {
	ctx := context.Background()
	p := migrateProject(t)
	var mu sync.Mutex
	var sent [][]store.ConfigChange
	p.OnConfigs(func(cs []store.ConfigChange) {
		mu.Lock()
		sent = append(sent, cs)
		mu.Unlock()
	})
	const revisions = `SELECT coalesce(sum(revision), 0) FROM _jenab_views`
	const changes = `SELECT (SELECT count(*) FROM _jenab_changes) + (SELECT count(*) FROM _jenab_change_rows)`
	revs, chs := count(t, p, revisions), count(t, p, changes)
	// news_table's update passes; edit_news still uses the old column.
	_, err := p.Migrate(ctx, id.SourceOf("message", "m1"), migration(renameReason,
		update(DependentView, "news_table", renamed(newsTable)), update(DependentView, "edit_news", editNews)), store.MigrateOptions{})
	problems(t, err)
	if got := count(t, p, revisions); got != revs {
		t.Errorf("config revisions %d, want %d", got, revs)
	}
	if got := count(t, p, changes); got != chs {
		t.Errorf("change-log rows %d, want %d", got, chs)
	}
	waitSent(&mu, &sent, 1, 300*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 0 {
		t.Errorf("project:configs after a rollback: %+v", sent)
	}
}
