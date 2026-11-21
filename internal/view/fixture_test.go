package view

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The fixture is SPEC 9's Bitcoin project: its tables, the six views of
// 9.3 saved, and a table of files for the object_ref checks. The tests
// share it; each saves its own configs under IDs of its own.

var fixture *store.ProjectDB

var fixtureTables = []string{
	`CREATE TABLE btc_prices(date TEXT PRIMARY KEY, price REAL NOT NULL, _run_id TEXT, _fetched_at TEXT) WITHOUT ROWID`,
	`CREATE TABLE btc_news(url TEXT PRIMARY KEY, date TEXT NOT NULL, title TEXT NOT NULL, reason TEXT, _run_id TEXT, _fetched_at TEXT) WITHOUT ROWID`,
	`CREATE TABLE watch_keywords(id INTEGER PRIMARY KEY, keyword TEXT NOT NULL UNIQUE, added_at TEXT NOT NULL DEFAULT (datetime('now')))`,
	`CREATE TABLE docs(id INTEGER PRIMARY KEY, title TEXT NOT NULL, file TEXT, cover TEXT, note TEXT,
		status TEXT, read INTEGER NOT NULL DEFAULT 0, score REAL, doubled INTEGER GENERATED ALWAYS AS (id * 2) VIRTUAL)`,
	`CREATE TABLE pairs(a TEXT NOT NULL, b TEXT NOT NULL, note TEXT, PRIMARY KEY (a, b)) WITHOUT ROWID`,
	`INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`,
	`INSERT INTO btc_prices VALUES ('2026-11-19', 91000, NULL, NULL), ('2026-11-20', 92500, NULL, NULL)`,
	`INSERT INTO watch_keywords(keyword) VALUES ('ETF'), ('halving')`,
}

// The six views of SPEC 9.3. btc_price_table's run_pipeline action is
// left out: there are no pipelines before Phase 4.
var specViews = map[string]string{
	"btc_price_table": `version: 1
id: btc_price_table
title: Bitcoin price
type: table
query: |
  SELECT date, price FROM btc_prices
  WHERE date >= :from_date
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d", required: true }
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: price, label: "Price", format: currency, format_options: { currency: USD } }
`,
	"btc_price_chart": `version: 1
id: btc_price_chart
title: Bitcoin price chart
type: chart
chart_type: line
query: SELECT date, price FROM btc_prices
x: { field: date, label: "Date", format: date }
y:
  - { field: price, label: "Price (USD)" }
`,
	"btc_news_table": `version: 1
id: btc_news_table
title: Bitcoin news
type: table
query: SELECT date, title, reason, url FROM btc_news
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: title, label: "Title", format: text }
  - { field: reason, label: "Why it matters", format: text }
  - { field: url, label: "Link", format: url }
`,
	"btc_today": `version: 1
id: btc_today
title: Today
type: stat
query: |
  SELECT price, price / LAG(price) OVER (ORDER BY date) - 1 AS change
  FROM btc_prices ORDER BY date DESC LIMIT 1
value: { field: price, format: currency, format_options: { currency: USD } }
change: { field: change, format: percent }
`,
	"keyword_list": `version: 1
id: keyword_list
title: News keywords
type: table
query: SELECT id, keyword FROM watch_keywords
default_sort: { field: keyword, direction: asc }
columns:
  - { field: keyword, label: "Keyword", format: text }
rows_from: { table: watch_keywords, key: id }
row_actions:
  - { label: "Delete", delete: true, confirm: true }
`,
	"add_keyword": `version: 1
id: add_keyword
title: Add news keyword
type: form
fields:
  - { field: keyword, label: "Keyword", control: text, required: true }
submit:
  action: insert
  table: watch_keywords
`,
}

// specPage is SPEC 5.9's page without its run_pipeline action.
const specPage = `version: 1
id: bitcoin
title: Bitcoin
description: Daily price and the news most likely to move it
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d", required: true }
rows:
  - [ { view: btc_price_chart, span: 8 }, { view: btc_today, span: 4 } ]
  - [ { view: btc_price_table, span: 5 }, { view: btc_news_table, span: 7 } ]
  - [ { card: "Keywords", rows: [ [ { view: keyword_list }, { view: add_keyword } ] ] } ]
`

// More saved configs the tests point to.
var extraViews = map[string]string{
	"edit_keyword": `version: 1
id: edit_keyword
title: Edit keyword
type: form
load: "SELECT keyword FROM watch_keywords WHERE id = :key"
fields:
  - { field: keyword, control: text, required: true }
submit: { action: update, table: watch_keywords, key: id }
`,
}

func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(run0(m))
}

func run0(m *testing.M) int {
	dir, err := os.MkdirTemp("", "jenab-view-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	ctx := context.Background()
	p, err := store.OpenProject(ctx, dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer p.Close(ctx)
	_, err = store.Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for _, s := range fixtureTables {
			if _, err := tx.Exec(s); err != nil {
				return struct{}{}, fmt.Errorf("%s: %w", s, err)
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := p.PutObject(ctx, limit.Interactive, id.SourceUser, "images/logo.png", strings.NewReader("png"), store.PutOptions{Source: "upload"}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fixture = p
	for _, vs := range []map[string]string{specViews, extraViews} {
		for cid, y := range vs {
			pv, err := ParseView(Default, []byte(y))
			if err != nil {
				fmt.Fprintln(os.Stderr, cid, err)
				return 1
			}
			if err := saveConfig(store.KindView, cid, pv.Type, y); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
	}
	if err := saveConfig(store.KindPage, "bitcoin", "page", specPage); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func saveConfig(kind store.ConfigKind, cid, typ, y string) error {
	_, err := fixture.SaveConfig(context.Background(), store.ConfigSave{Kind: kind, ID: cid, Type: typ, Title: cid, YAML: y, Source: id.SourceUser})
	return err
}

// validator checks against the fixture.
func validator() *Validator { return &Validator{Env: ProjectEnv{DB: fixture}} }

func checkView(t *testing.T, src string) Result {
	t.Helper()
	_, res, err := validator().View(context.Background(), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func checkPage(t *testing.T, src string) Result {
	t.Helper()
	_, res, err := validator().Page(context.Background(), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// wantOK fails unless res has no errors and no warnings.
func wantOK(t *testing.T, res Result) {
	t.Helper()
	if len(res.Issues) > 0 {
		t.Errorf("want no issues, got:\n%s", res)
	}
}

// want fails unless res has an issue at path whose message has each of
// the parts; warning says which kind.
func want(t *testing.T, res Result, warning bool, path string, parts ...string) Issue {
	t.Helper()
	for _, i := range res.Issues {
		if i.Path != path || i.Warning != warning {
			continue
		}
		ok := true
		for _, p := range parts {
			if !strings.Contains(i.Msg, p) {
				ok = false
			}
		}
		if ok {
			return i
		}
	}
	kind := "an error"
	if warning {
		kind = "a warning"
	}
	t.Errorf("want %s at %s with %q, got:\n%s", kind, path, parts, res)
	return Issue{}
}

func wantErr(t *testing.T, res Result, path string, parts ...string) Issue {
	t.Helper()
	return want(t, res, false, path, parts...)
}

func wantWarn(t *testing.T, res Result, path string, parts ...string) Issue {
	t.Helper()
	return want(t, res, true, path, parts...)
}

// wantN fails unless res has n errors.
func wantN(t *testing.T, res Result, n int) {
	t.Helper()
	if got := len(res.Errors()); got != n {
		t.Errorf("want %d errors, got %d:\n%s", n, got, res)
	}
}
