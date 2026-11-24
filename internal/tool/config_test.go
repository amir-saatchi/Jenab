package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// configFixture is a small Bitcoin project (SPEC 9) for the config tools.
var configFixture = []string{
	`CREATE TABLE btc_prices(date TEXT PRIMARY KEY, price REAL NOT NULL) WITHOUT ROWID`,
	`CREATE TABLE watch_keywords(id INTEGER PRIMARY KEY, keyword TEXT NOT NULL UNIQUE)`,
	`INSERT INTO btc_prices VALUES ('2026-11-19', 91000), ('2026-11-20', 92500)`,
}

const chartYAML = `version: 1
id: btc_price_chart
title: Bitcoin price chart
type: chart
chart_type: line
query: SELECT date, price FROM btc_prices WHERE date >= :from_date
params: { from_date: "2026-01-01" }
x: { field: date, format: date }
y:
  - { field: price, label: "Price (USD)" }
`

const keywordListYAML = `version: 1
id: keyword_list
title: News keywords
type: table
query: SELECT id, keyword FROM watch_keywords
default_sort: { field: keyword, direction: asc }
columns:
  - { field: keyword, label: "Keyword", format: text }
rows_from: { table: watch_keywords, key: id }
row_actions:
  - { label: "Edit", open_form: edit_keyword }
`

const editKeywordYAML = `version: 1
id: edit_keyword
title: Edit keyword
type: form
load: "SELECT keyword FROM watch_keywords WHERE id = :key"
fields:
  - { field: keyword, control: text, required: true }
submit: { action: update, table: watch_keywords, key: id }
`

const pageYAML = `version: 1
id: bitcoin
title: Bitcoin
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d" }
rows:
  - [ { view: btc_price_chart } ]
`

// badChart has three errors: an unknown column, a missing y and an
// unknown chart type.
const badChart = `version: 1
id: daily_btc
title: Daily
type: chart
chart_type: lines
query: SELECT date, price FROM btc_prices
x: { field: day }
`

// configEnv is a test env on configFixture with a turn: BeforeAnswer
// checks and AtTurnEnd functions are kept for the test to run.
type configTurn struct {
	env    *Env
	tools  map[string]Tool
	checks []func() string
	kinds  []chat.NoticeKind
	ends   []func()
}

func newConfigTurn(t *testing.T) *configTurn {
	t.Helper()
	env := testEnv(t)
	execAll(t, env, configFixture...)
	ct := &configTurn{env: env, tools: map[string]Tool{}}
	ct.begin(1)
	for _, tl := range ConfigTools() {
		ct.tools[tl.Spec().Name] = tl
	}
	return ct
}

// begin starts turn n, ending the one before.
func (ct *configTurn) begin(n int) {
	for _, f := range ct.ends {
		f()
	}
	ct.ends, ct.checks, ct.kinds = nil, nil, nil
	ct.env.Turn = n
	m := id.Message(id.New())
	ct.env.Message, ct.env.Source = m, id.SourceOf("message", string(m))
	ct.env.AtTurnEnd = func(f func()) { ct.ends = append(ct.ends, f) }
	ct.env.BeforeAnswer = func(k chat.NoticeKind, f func() string) {
		ct.kinds = append(ct.kinds, k)
		ct.checks = append(ct.checks, f)
	}
}

func (ct *configTurn) run(t *testing.T, name string, args any) (Result, error) {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return Run(context.Background(), ct.tools[name], Call{ID: "c1", Args: b, Env: ct.env})
}

func (ct *configTurn) ok(t *testing.T, name string, args any) Result {
	t.Helper()
	r, err := ct.run(t, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r
}

func (ct *configTurn) fail(t *testing.T, name string, args any) string {
	t.Helper()
	_, err := ct.run(t, name, args)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("%s: got %v, want a *tool.Error", name, err)
	}
	return te.Msg
}

// answer runs the turn's checks as the agent does when the model answers
// without a call: the first text, and its kind.
func (ct *configTurn) answer() (chat.NoticeKind, string) {
	for i, c := range ct.checks {
		if s := c(); s != "" {
			return ct.kinds[i], s
		}
	}
	return "", ""
}

// testNow is the card's "as of".
var testNow = time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)

func cfg(y string) map[string]string { return map[string]string{"config": y} }
func cid(s string) map[string]string { return map[string]string{"id": s} }

func TestConfigToolSpecs(t *testing.T) {
	for _, tl := range ConfigTools() {
		s := tl.Spec()
		if s.Areas != Views || s.When != WhenTables {
			t.Errorf("%s: areas %v, when %v", s.Name, s.Areas, s.When)
		}
		wantWrite := s.Name == "save_view" || s.Name == "save_page" || s.Name == "delete_config"
		if (s.Effects == WritesDB) != wantWrite || (!wantWrite && s.Effects != ReadsDB) {
			t.Errorf("%s: effects %v", s.Name, s.Effects)
		}
		if _, ok := tl.(Preflighter); ok {
			t.Errorf("%s asks for approval; config tools don't (8.8)", s.Name)
		}
	}
}

func TestSaveView(t *testing.T) {
	ct := newConfigTurn(t)
	r := ct.ok(t, "save_view", cfg(chartYAML))
	contains(t, r.Text, "Saved view btc_price_chart, revision 1.")
	if len(r.Notices) != 1 || r.Notices[0].Kind != chat.NoticeConfigSaved || r.Notices[0].Target != "view:btc_price_chart" ||
		r.Notices[0].Change == "" || r.Notices[0].Text != "View saved: btc_price_chart" {
		t.Errorf("chip: %+v", r.Notices)
	}
	c, err := ct.env.Project.DB.Config(context.Background(), store.KindView, "btc_price_chart")
	if err != nil || c.Type != "chart" || c.Title != "Bitcoin price chart" || c.YAML != chartYAML || c.UpdatedBy != ct.env.Source {
		t.Fatalf("stored: %+v, %v", c, err)
	}
	// The same again changes nothing and has no chip.
	r = ct.ok(t, "save_view", cfg(chartYAML))
	contains(t, r.Text, "No change", "revision 1")
	if len(r.Notices) != 0 {
		t.Errorf("chip for no change: %+v", r.Notices)
	}
	// A changed one is revision 2.
	r = ct.ok(t, "save_view", cfg(strings.Replace(chartYAML, "Bitcoin price chart", "BTC", 1)))
	contains(t, r.Text, "revision 2")
	// A page through save_view fails validation, with its errors.
	msg := ct.fail(t, "save_view", cfg(pageYAML))
	contains(t, msg, "Not saved:", "error")
}

func TestSaveViewErrors(t *testing.T) {
	ct := newConfigTurn(t)
	msg := ct.fail(t, "save_view", cfg(badChart))
	contains(t, msg, "Not saved: 3 errors:", "line 5:", "line 7:", "day")
	if _, err := ct.env.Project.DB.Config(context.Background(), store.KindView, "daily_btc"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a failed save was saved: %v", err)
	}
	// Warnings come with a save.
	r := ct.ok(t, "save_view", cfg(strings.Replace(chartYAML, "chart_type: line", "chart_type: line\ndescription: \"\"", 1)))
	contains(t, r.Text, "Saved view")
}

func TestSavePage(t *testing.T) {
	ct := newConfigTurn(t)
	// The view the page shows isn't saved yet.
	msg := ct.fail(t, "save_page", cfg(pageYAML))
	contains(t, msg, "btc_price_chart")
	ct.ok(t, "save_view", cfg(chartYAML))
	r := ct.ok(t, "save_page", cfg(pageYAML))
	contains(t, r.Text, "Saved page bitcoin, revision 1.")
	if len(r.Notices) != 1 || r.Notices[0].Target != "page:bitcoin" || r.Notices[0].Text != "Page saved: bitcoin" {
		t.Errorf("chip: %+v", r.Notices)
	}
	// A view can't take a page's ID.
	msg = ct.fail(t, "save_view", cfg(strings.Replace(chartYAML, "id: btc_price_chart", "id: bitcoin", 1)))
	contains(t, msg, "bitcoin")
}

func TestValidate(t *testing.T) {
	ct := newConfigTurn(t)
	r := ct.ok(t, "validate", cfg(chartYAML))
	contains(t, r.Text, "The view btc_price_chart is valid. Nothing was saved.")
	r = ct.ok(t, "validate", cfg(badChart))
	contains(t, r.Text, "The view daily_btc has errors. Nothing was saved.", "3 errors:")
	r = ct.ok(t, "validate", cfg(pageYAML))
	contains(t, r.Text, "The page bitcoin has errors.", "btc_price_chart")
	r = ct.ok(t, "validate", cfg("rows: ["))
	contains(t, r.Text, "has errors", "line")
	if _, err := ct.env.Project.DB.Config(context.Background(), store.KindView, "btc_price_chart"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("validate saved: %v", err)
	}
	// validate doesn't count toward the repair limit.
	for range RepairAttempts + 3 {
		ct.ok(t, "validate", cfg(badChart))
	}
	ct.fail(t, "save_view", cfg(badChart))
	if k, s := ct.answer(); k != chat.NoticeEarlyStop || !strings.Contains(s, "save_view daily_btc") {
		t.Errorf("early stop: %s %q", k, s)
	}
}

func TestReadConfig(t *testing.T) {
	ct := newConfigTurn(t)
	msg := ct.fail(t, "read_config", cid("btc_price_chart"))
	contains(t, msg, `No view or page "btc_price_chart": the project has none saved.`)
	ct.ok(t, "save_view", cfg(chartYAML))
	ct.ok(t, "save_page", cfg(pageYAML))
	r := ct.ok(t, "read_config", cid("btc_price_chart"))
	if r.Text != "view btc_price_chart (chart), revision 1:\n"+chartYAML {
		t.Errorf("read: %q", r.Text)
	}
	r = ct.ok(t, "read_config", cid("bitcoin"))
	if r.Text != "page bitcoin, revision 1:\n"+pageYAML {
		t.Errorf("read: %q", r.Text)
	}
	msg = ct.fail(t, "read_config", cid("btc"))
	contains(t, msg, `No view or page "btc". Saved: btc_price_chart, bitcoin.`)
}

func TestOpenPage(t *testing.T) {
	ct := newConfigTurn(t)
	ct.ok(t, "save_view", cfg(chartYAML))
	ct.ok(t, "save_page", cfg(pageYAML))
	r := ct.ok(t, "open_page", cid("bitcoin"))
	contains(t, r.Text, "open the page bitcoin")
	if len(r.Notices) != 1 || r.Notices[0].Kind != chat.NoticeOpenPage || r.Notices[0].Target != "page:bitcoin" || r.Notices[0].Details != "" {
		t.Errorf("chip: %+v", r.Notices)
	}
	r = ct.ok(t, "open_page", map[string]any{"id": "bitcoin", "filters": map[string]any{"from_date": "2026-01-01"}})
	if len(r.Notices) != 1 || r.Notices[0].Details != `{"from_date":"2026-01-01"}` {
		t.Errorf("chip: %+v", r.Notices)
	}
	msg := ct.fail(t, "open_page", map[string]any{"id": "bitcoin", "filters": map[string]any{"to_date": "x"}})
	contains(t, msg, "no filter to_date", "Its filters: from_date.")
	msg = ct.fail(t, "open_page", cid("btc_price_chart"))
	contains(t, msg, "btc_price_chart is a view, not a page")
	msg = ct.fail(t, "open_page", cid("nope"))
	contains(t, msg, `No view or page "nope"`)
}

func TestDeleteConfig(t *testing.T) {
	ctx := context.Background()
	ct := newConfigTurn(t)
	for _, y := range []string{chartYAML, editKeywordYAML, keywordListYAML} {
		ct.ok(t, "save_view", cfg(y))
	}
	ct.ok(t, "save_page", cfg(pageYAML))
	ct.ok(t, "save_page", cfg(`version: 1
id: keywords
title: Keywords
rows:
  - [ { view: keyword_list } ]
  - [ { button: "Bitcoin", open_page: bitcoin } ]
`))

	// Refused while something uses it, naming what.
	msg := ct.fail(t, "delete_config", cid("btc_price_chart"))
	contains(t, msg, "Not deleted: the view btc_price_chart is used by page bitcoin.")
	msg = ct.fail(t, "delete_config", cid("edit_keyword"))
	contains(t, msg, "Not deleted: the view edit_keyword is used by view keyword_list.")
	msg = ct.fail(t, "delete_config", cid("bitcoin"))
	contains(t, msg, "Not deleted: the page bitcoin is used by page keywords.")
	msg = ct.fail(t, "delete_config", cid("gone"))
	contains(t, msg, `No view or page "gone"`)

	// A page nothing uses is deleted, with its chip; then its views are free.
	r := ct.ok(t, "delete_config", cid("keywords"))
	contains(t, r.Text, "Deleted page keywords. Undo turn brings it back.")
	if len(r.Notices) != 1 || r.Notices[0].Text != "Page deleted: keywords" || r.Notices[0].Change == "" {
		t.Fatalf("chip: %+v", r.Notices)
	}
	// In a turn of its own, so Undo turn reverts the delete alone.
	ct.begin(2)
	r = ct.ok(t, "delete_config", cid("keyword_list"))
	contains(t, r.Text, "Deleted view keyword_list.")
	if _, err := ct.env.Project.DB.Config(ctx, store.KindView, "keyword_list"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("still there: %v", err)
	}
	// The change log has op delete, and Undo turn brings the view back.
	op, err := store.Do(ctx, ct.env.Project.DB.DB, limit.Interactive, func(tx *sql.Tx) (string, error) {
		var op string
		err := tx.QueryRow(`SELECT op FROM _jenab_changes WHERE id = ?`, r.Notices[0].Change).Scan(&op)
		return op, err
	})
	if err != nil || op != "delete" {
		t.Fatalf("change op %q, %v", op, err)
	}
	res, err := ct.env.Project.DB.Undo(ctx, []id.Source{ct.env.Source}, store.UndoOptions{})
	if err != nil || len(res.Conflicts) > 0 {
		t.Fatalf("undo: %+v, %v", res, err)
	}
	c, err := ct.env.Project.DB.Config(ctx, store.KindView, "keyword_list")
	if err != nil || c.YAML != keywordListYAML {
		t.Fatalf("after undo: %+v, %v", c, err)
	}
	if undone, err := ct.env.Project.DB.ChangeUndone(ctx, id.Change(r.Notices[0].Change)); err != nil || !undone {
		t.Errorf("ChangeUndone: %v, %v", undone, err)
	}
}

func TestRepairLimit(t *testing.T) {
	ct := newConfigTurn(t)
	for i := 1; i <= RepairAttempts; i++ {
		msg := ct.fail(t, "save_view", cfg(badChart))
		if strings.Contains(msg, "last try") {
			t.Fatalf("try %d is the last: %s", i, msg)
		}
	}
	msg := ct.fail(t, "save_view", cfg(badChart))
	contains(t, msg, "Not saved: 3 errors:", fmt.Sprintf("That was the last try: %d saves", RepairAttempts+1), "show the user the remaining errors")
	// A sixth is refused without being checked, even when it is fixed.
	msg = ct.fail(t, "save_view", cfg(strings.Replace(chartYAML, "btc_price_chart", "daily_btc", 1)))
	contains(t, msg, "is the limit; don't try again")
	if strings.Contains(msg, "errors:") {
		t.Errorf("checked: %s", msg)
	}
	// The config at the limit was told to stop: no early-stop notice.
	if k, s := ct.answer(); s != "" {
		t.Errorf("early stop at the limit: %s %q", k, s)
	}
	// Another config has its own count.
	ct.ok(t, "save_view", cfg(chartYAML))
	// save_page counts apart from save_view.
	ct.fail(t, "save_page", cfg(strings.Replace(pageYAML, "id: bitcoin", "id: daily_btc", 1)+"  - [ { view: nope } ]\n"))
	// The next turn starts again.
	ct.begin(2)
	ct.ok(t, "save_view", cfg(strings.Replace(chartYAML, "btc_price_chart", "daily_btc", 1)))
}

func TestRepairResetOnSave(t *testing.T) {
	ct := newConfigTurn(t)
	for range RepairAttempts {
		ct.fail(t, "save_view", cfg(badChart))
	}
	ct.ok(t, "save_view", cfg(strings.Replace(chartYAML, "btc_price_chart", "daily_btc", 1)))
	for range RepairAttempts {
		if msg := ct.fail(t, "save_view", cfg(badChart)); strings.Contains(msg, "last try") {
			t.Fatalf("the count didn't start again: %s", msg)
		}
	}
}

func TestEarlyStop(t *testing.T) {
	ct := newConfigTurn(t)
	ct.fail(t, "save_view", cfg(badChart))
	if len(ct.checks) != 1 {
		t.Fatalf("%d checks, want 1", len(ct.checks))
	}
	ct.fail(t, "save_view", cfg(badChart))
	if len(ct.checks) != 1 {
		t.Fatalf("a check a failure: %d", len(ct.checks))
	}
	k, s := ct.answer()
	if k != chat.NoticeEarlyStop || s != "save_view daily_btc failed validation (3 errors) and was not saved. Fix and save it, or tell the user why not." {
		t.Errorf("notice: %s %q", k, s)
	}
	// Once a turn.
	if _, s := ct.answer(); s != "" {
		t.Errorf("twice: %q", s)
	}

	// A failed save fixed later in the turn has no notice.
	ct.begin(2)
	ct.fail(t, "save_view", cfg(badChart))
	ct.ok(t, "save_view", cfg(strings.Replace(chartYAML, "btc_price_chart", "daily_btc", 1)))
	if _, s := ct.answer(); s != "" {
		t.Errorf("notice after the fix: %q", s)
	}

	// One error, and a config that doesn't parse, named by its id line.
	ct.begin(3)
	ct.fail(t, "save_page", cfg("id: broken\nrows: [\n"))
	_, s = ct.answer()
	contains(t, s, "save_page broken failed validation (1 error)")

	// Without a turn (a subagent), no count and no notice.
	ct.env.AtTurnEnd, ct.env.BeforeAnswer = nil, nil
	for range RepairAttempts + 2 {
		if msg := ct.fail(t, "save_view", cfg(badChart)); strings.Contains(msg, "limit") || strings.Contains(msg, "last try") {
			t.Fatalf("limit without a turn: %s", msg)
		}
	}
}

func TestConfigCard(t *testing.T) {
	ctx := context.Background()
	ct := newConfigTurn(t)
	db := ct.env.Project.DB
	before, err := db.ConfigStamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, y := range []string{chartYAML, editKeywordYAML, keywordListYAML} {
		ct.ok(t, "save_view", cfg(y))
	}
	ct.ok(t, "save_page", cfg(pageYAML))
	after, err := db.ConfigStamp(ctx)
	if err != nil || after <= before {
		t.Fatalf("stamp %d → %d, %v", before, after, err)
	}
	card, err := view.Card(ctx, db, testNow)
	if err != nil {
		t.Fatal(err)
	}
	contains(t, card,
		"- btc_prices (2 rows):", "; views btc_price_chart\n",
		"- watch_keywords (0 rows):", "; views keyword_list; forms edit_keyword\n",
		"Views:\n- btc_price_chart (chart): \"Bitcoin price chart\"\n- edit_keyword (form): \"Edit keyword\"\n- keyword_list (table): \"News keywords\"",
		"Pages:\n- bitcoin: \"Bitcoin\"; views btc_price_chart")
	// A delete changes the stamp and the card.
	ct.ok(t, "delete_config", cid("bitcoin"))
	if s, _ := db.ConfigStamp(ctx); s <= after {
		t.Errorf("stamp after a delete: %d", s)
	}
	card, _ = view.Card(ctx, db, testNow)
	if strings.Contains(card, "Pages:") {
		t.Errorf("deleted page on the card:\n%s", card)
	}
}
