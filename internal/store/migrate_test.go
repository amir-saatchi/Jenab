package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

func mig(s string) id.Source { return id.SourceOf("migration", s) }

// steps reads a JSON list of steps.
func steps(t testing.TB, js string) Migration {
	t.Helper()
	var m Migration
	if err := json.Unmarshal([]byte(js), &m.Steps); err != nil {
		t.Fatalf("steps: %v", err)
	}
	return m
}

func migrateOK(t testing.TB, p *ProjectDB, js string, o MigrateOptions) MigrationReport {
	t.Helper()
	r, err := p.Migrate(context.Background(), mig(id.New()), steps(t, js), o)
	if err != nil {
		t.Fatalf("migrate: %v\nreport %+v", err, r)
	}
	return r
}

// migrateFails runs a migration that must fail with a problem whose path
// and message contain path and msg.
func migrateFails(t testing.TB, p *ProjectDB, js, path, msg string) MigrationReport {
	t.Helper()
	r, err := p.Migrate(context.Background(), mig(id.New()), steps(t, js), MigrateOptions{Approved: true})
	var me *MigrationError
	if !errors.As(err, &me) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("migrate: %v, want a MigrationError", err)
	}
	for _, pr := range me.Problems {
		if strings.Contains(pr.Path, path) && strings.Contains(pr.Msg, msg) {
			return r
		}
	}
	t.Fatalf("problems %v, want %s: …%s…", me.Problems, path, msg)
	return r
}

func schemaSQL(t testing.TB, p *ProjectDB, name string) string {
	t.Helper()
	return one[string](t, p.DB, `SELECT coalesce(max(sql), '') FROM sqlite_schema WHERE name = ?`, name)
}

func TestMigrateCreateTable(t *testing.T) {
	p, _ := openTestProject(t)
	r := migrateOK(t, p, `[{"op": "create_table", "table": "coins", "columns": [
		{"name": "symbol", "type": "text"}, {"name": "name", "type": "TEXT", "not_null": true, "default": ""},
		{"name": "rank", "type": "INTEGER", "check": "rank > 0"}], "primary_key": ["symbol"], "unique": [["name"]]},
		{"op": "create_table", "table": "holdings", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "amount", "type": "REAL"}],
		"primary_key": ["coin"], "foreign_keys": [{"columns": ["coin"], "table": "coins", "on_delete": "cascade"}]}]`, MigrateOptions{})
	if r.SchemaVersion != 1 || r.Change == "" || len(r.Destructive) > 0 || r.Snapshot != "" {
		t.Errorf("report %+v", r)
	}
	s := schemaSQL(t, p, "coins")
	for _, want := range []string{`"symbol" TEXT NOT NULL`, `DEFAULT ''`, "CHECK (rank > 0\n)", `PRIMARY KEY ("symbol")`, `UNIQUE ("name")`} {
		if !strings.Contains(s, want) {
			t.Errorf("coins %q lacks %q", s, want)
		}
	}
	if s := schemaSQL(t, p, "holdings"); !strings.Contains(s, `REFERENCES "coins" ON DELETE CASCADE`) {
		t.Errorf("holdings %q", s)
	}
	if v, _ := p.SchemaVersion(context.Background()); v != 1 {
		t.Errorf("schema version %d", v)
	}
	op := one[string](t, p.DB, `SELECT op || ' ' || target || ' ' || (after->>'schema_version') FROM _jenab_changes`)
	if op != "migrate schema 1" {
		t.Errorf("change %q", op)
	}
	migrateFails(t, p, `[{"op": "create_table", "table": "COINS", "columns": [{"name": "a", "type": "TEXT"}], "primary_key": ["a"]}]`,
		"steps[0].table", "already exists")
}

// Every problem of a request is reported at once, at its path.
func TestMigrateProblems(t *testing.T) {
	p, _ := openTestProject(t)
	_, err := p.Migrate(context.Background(), mig("1"), steps(t, `[
		{"op": "create_table", "table": "_jenab_x", "columns": [{"name": "a", "type": "STRING"}, {"name": "b c", "type": "TEXT"}]},
		{"op": "create_tables"},
		{"op": "add_column", "table": "t", "column": {"name": "x", "type": "TEXT", "colour": 1}},
		{"op": "copy_data", "into": "t", "columns": ["a"], "from": "DELETE FROM t"},
		{"op": "insert_rows", "table": "t", "rows": [1]}]`), MigrateOptions{})
	var me *MigrationError
	if !errors.As(err, &me) {
		t.Fatal(err)
	}
	got := fmt.Sprint(me.Problems)
	for _, want := range []string{
		`steps[0].table: "_jenab_x" is reserved`,
		`steps[0].columns[0].type: unknown type "STRING" (use TEXT, INTEGER, REAL or BLOB)`,
		`steps[0].columns[1].name: "b c" is not a plain name`,
		`steps[0].primary_key: is required`,
		`steps[1].op: unknown op "create_tables"`,
		`steps[2].column.colour: unknown field`,
		`steps[3].from: the query was refused`,
		`steps[4].rows[0]: must be an object`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
	// Steps are checked against the schema before the writer.
	migrateFails(t, p, `[{"op": "drop_table", "table": "nope"}]`, "steps[0].table", `there is no table "nope" (the project has no tables yet)`)
}

func TestMigrateAddColumn(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, msg("1"), "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	migrateOK(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "volume", "type": "REAL", "default": 0}}]`, MigrateOptions{})
	if one[float64](t, p.DB, `SELECT volume FROM prices`) != 0 {
		t.Error("the default wasn't set")
	}
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "x", "type": "TEXT", "not_null": true}}]`,
		"steps[0].value", "widen_table")
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "Volume", "type": "TEXT"}}]`,
		"steps[0].column.name", "already has")
	// A check the rows break.
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "y", "type": "INTEGER", "check": "y IS NOT NULL"}, "value": "NULL"}]`,
		"steps[0]", "CHECK constraint failed")
	// The old rows get their value explicitly (8.2).
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "src", "type": "TEXT"}}]`,
		"steps[0].value", "widen_table")
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "src", "type": "TEXT"}, "value": "nope("}]`,
		"steps[0].value", "")
	migrateOK(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "src", "type": "TEXT"}, "value": "'feed-' || coin"}]`, MigrateOptions{})
	if got := one[string](t, p.DB, `SELECT src FROM prices`); got != "feed-BTC" {
		t.Errorf("src %q", got)
	}
	if one[int](t, p.DB, `SELECT count(*) FROM pragma_table_info('prices') WHERE name = 'src' AND dflt_value IS NULL`) != 1 {
		t.Error("src has a default")
	}
	// An empty table needs no value.
	mustExec(t, p.DB, `CREATE TABLE e (k TEXT PRIMARY KEY)`)
	migrateOK(t, p, `[{"op": "add_column", "table": "e", "column": {"name": "v", "type": "TEXT"}}]`, MigrateOptions{})
	// NOT NULL without a default: a rebuild with the column last, the
	// indexes kept.
	mustExec(t, p.DB, `CREATE INDEX by_price ON prices (price)`)
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "x", "type": "TEXT", "not_null": true}, "value": "NULL"}]`,
		"steps[0].value", "failed: prices.x: value must give every old row a value")
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "x", "type": "TEXT", "not_null": true, "check": "x <> 'BTC'"}, "value": "coin"}]`,
		"steps[0]", "CHECK constraint failed")
	migrateOK(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "x", "type": "TEXT", "not_null": true}, "value": "lower(coin)"}]`, MigrateOptions{})
	if got := one[string](t, p.DB, `SELECT group_concat(name || ':' || "notnull" || ':' || pk) FROM pragma_table_info('prices')`); !strings.HasSuffix(got, ",x:1:0") {
		t.Errorf("columns %s", got)
	}
	if got := dump(t, p, "prices"); !strings.Contains(got, "btc]") {
		t.Errorf("rows %s", got)
	}
	if one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE name = 'by_price'`) != 1 {
		t.Error("the index is gone")
	}
	mustExec(t, p.DB, `INSERT INTO e (k) VALUES ('a')`)
	migrateOK(t, p, `[{"op": "add_column", "table": "e", "column": {"name": "w", "type": "TEXT", "not_null": true}, "value": "k || '!'"}]`, MigrateOptions{})
	if got := dump(t, p, "e"); got != "e: [a <nil> a!]" {
		t.Errorf("rows %s", got)
	}
}

func TestMigrateRenames(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('prices', 'note', 'object_ref')`)
	migrateOK(t, p, `[{"op": "rename_column", "table": "prices", "from": "NOTE", "to": "chart"},
		{"op": "rename_table", "from": "prices", "to": "quotes"}]`, MigrateOptions{})
	if got := dump(t, p, "_jenab_columns"); got != "_jenab_columns: [quotes chart object_ref]" {
		t.Errorf("annotations %s", got)
	}
	if !strings.Contains(schemaSQL(t, p, "quotes"), `"chart"`) {
		t.Error(schemaSQL(t, p, "quotes"))
	}
	migrateFails(t, p, `[{"op": "rename_column", "table": "quotes", "from": "nope", "to": "x"}]`, "steps[0].from", `no column "nope"`)
	// A child's foreign key follows its parent's new name.
	mustExec(t, p.DB, `CREATE TABLE alerts (coin TEXT, date TEXT, PRIMARY KEY (coin, date), FOREIGN KEY (coin, date) REFERENCES quotes)`)
	mustExec(t, p.DB, `INSERT INTO quotes (coin, date) VALUES ('BTC', 'd1')`)
	mustExec(t, p.DB, `INSERT INTO alerts VALUES ('BTC', 'd1')`)
	migrateOK(t, p, `[{"op": "rename_table", "from": "quotes", "to": "prices"}]`, MigrateOptions{})
	if s := schemaSQL(t, p, "alerts"); !strings.Contains(s, `REFERENCES "prices"`) {
		t.Errorf("alerts %q", s)
	}
}

func TestMigrateIndexes(t *testing.T) {
	p := pricesProject(t)
	r := migrateOK(t, p, `[{"op": "create_index", "table": "prices", "columns": ["date"]},
		{"op": "create_index", "table": "prices", "columns": ["price"], "name": "by_price", "unique": true}]`, MigrateOptions{})
	if r.Steps[0].Notes[0] != "the index is named prices_date" {
		t.Errorf("notes %v", r.Steps[0].Notes)
	}
	if one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name IN ('prices_date', 'by_price')`) != 2 {
		t.Error("indexes missing")
	}
	migrateOK(t, p, `[{"op": "drop_index", "name": "BY_PRICE"}]`, MigrateOptions{})
	migrateFails(t, p, `[{"op": "drop_index", "name": "sqlite_autoindex_prices_1"}]`, "steps[0].name", "key or UNIQUE")
	migrateFails(t, p, `[{"op": "drop_index", "name": "_jenab_change_rows_key"}]`, "steps[0].name", "no index")
}

func TestMigrateAnnotate(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	o, err := p.PutObject(ctx, limit.Interactive, msg("1"), "charts/a.png", strings.NewReader("png"), PutOptions{Source: "tool"})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, ?), ('BTC', 'd2', 2, NULL)`, o.Key)
	migrateOK(t, p, `[{"op": "annotate_column", "table": "prices", "column": "note", "kind": "object_ref"}]`, MigrateOptions{})
	if got := dump(t, p, "_jenab_columns"); got != "_jenab_columns: [prices note object_ref]" {
		t.Errorf("annotations %s", got)
	}
	migrateOK(t, p, `[{"op": "annotate_column", "table": "prices", "column": "note", "kind": ""}]`, MigrateOptions{})
	mustExec(t, p.DB, `UPDATE prices SET note = 'nope' WHERE date = 'd2'`)
	migrateFails(t, p, `[{"op": "annotate_column", "table": "prices", "column": "note", "kind": "object_ref"}]`,
		"steps[0].column", `not keys of stored files, such as "nope"`)
}

func TestMigrateInsertRows(t *testing.T) {
	p, _ := openTestProject(t)
	migrateOK(t, p, `[{"op": "create_table", "table": "coins", "columns": [{"name": "symbol", "type": "TEXT"}, {"name": "rank", "type": "INTEGER"}],
		"primary_key": ["symbol"]}, {"op": "insert_rows", "table": "coins", "rows": [{"symbol": "BTC", "rank": 1}, {"symbol": "ETH", "rank": "2"}]}]`,
		MigrateOptions{})
	if got := dump(t, p, "coins"); got != "coins: [BTC 1] [ETH 2]" {
		t.Errorf("rows %s", got)
	}
	migrateFails(t, p, `[{"op": "insert_rows", "table": "coins", "rows": [{"symbol": "SOL", "rank": "x"}]}]`, "steps[0].rows[0].rank", "")
	migrateFails(t, p, `[{"op": "insert_rows", "table": "coins", "rows": [{"symbol": "BTC"}]}]`, "steps[0].rows[0]", "UNIQUE")
	rows := strings.Repeat(`{"symbol": "X"},`, 51)
	migrateFails(t, p, `[{"op": "insert_rows", "table": "coins", "rows": [`+strings.TrimSuffix(rows, ",")+`]}]`, "steps[0].rows", "at most 50")
}

func TestMigrateCopyData(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, NULL), ('BTC', 'd2', 2, NULL), ('ETH', 'd1', 3, NULL)`)
	r := migrateOK(t, p, `[{"op": "create_table", "table": "btc", "columns": [{"name": "date", "type": "TEXT"}, {"name": "price", "type": "REAL"}],
		"primary_key": ["date"]},
		{"op": "copy_data", "into": "btc", "columns": ["date", "price"], "from": "SELECT date, price FROM prices WHERE coin = 'BTC' -- only BTC"}]`,
		MigrateOptions{})
	if r.Steps[1].Rows != 2 || dump(t, p, "btc") != "btc: [d1 1] [d2 2]" {
		t.Errorf("rows %d, %s", r.Steps[1].Rows, dump(t, p, "btc"))
	}
	migrateFails(t, p, `[{"op": "copy_data", "into": "btc", "columns": ["date", "price"], "from": "SELECT * FROM _jenab_changes"}]`,
		"steps[0].from", "refused")
	migrateFails(t, p, `[{"op": "copy_data", "into": "btc", "columns": ["date", "price"], "from": "SELECT date FROM prices"}]`,
		"steps[0].from", "one column for each")
	migrateFails(t, p, `[{"op": "copy_data", "into": "btc", "columns": ["date", "price"], "from": "SELECT date, price FROM prices"}]`,
		"steps[0]", "UNIQUE")
	migrateFails(t, p, `[{"op": "copy_data", "into": "btc", "columns": ["date"], "from": "SELECT :d"}]`, "steps[0].from", "parameters")
	// Values are capped as on a reader (16 MiB), not as an agent query's
	// 1 MiB, which SQLite would apply to the whole record.
	mustExec(t, p.DB, `CREATE TABLE notes (id INTEGER PRIMARY KEY, a TEXT, b TEXT)`)
	migrateOK(t, p, `[{"op": "copy_data", "into": "notes", "columns": ["id", "a", "b"], "from": "SELECT 1, hex(zeroblob(300000)), hex(zeroblob(300000))"}]`,
		MigrateOptions{})
	if one[int](t, p.DB, `SELECT length(a) + length(b) FROM notes`) != 1_200_000 {
		t.Error("the values were cut")
	}
	migrateFails(t, p, `[{"op": "copy_data", "into": "notes", "columns": ["a"], "from": "SELECT zeroblob(17000000)"}]`, "steps[0]", "too big")
}

func TestMigrateRebuild(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `CREATE INDEX prices_note ON prices (note)`)
	mustExec(t, p.DB, `CREATE INDEX prices_price ON prices (price)`)
	mustExec(t, p.DB, `CREATE TABLE alerts (coin TEXT, date TEXT, PRIMARY KEY (coin, date), FOREIGN KEY (coin, date) REFERENCES prices)`)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, 'a'), ('BTC', 'd2', 2, NULL)`)
	mustExec(t, p.DB, `INSERT INTO alerts VALUES ('BTC', 'd1')`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('prices', 'note', 'object_ref')`)
	// Without approval, a step that drops data is refused before it runs.
	js := `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "cents", "type": "INTEGER", "not_null": true}], "primary_key": ["coin", "date"], "mapping": {"cents": "CAST(price * 100 AS INTEGER)"}}]`
	r, err := p.Migrate(context.Background(), mig("1"), steps(t, js), MigrateOptions{})
	if !errors.Is(err, ErrNeedsApproval) || len(r.Destructive) != 1 || !r.Steps[0].Destructive {
		t.Fatalf("%v, report %+v", err, r)
	}
	r = migrateOK(t, p, js, MigrateOptions{Approved: true})
	if got := dump(t, p, "prices"); got != "prices: [BTC d1 100] [BTC d2 200]" {
		t.Errorf("rows %s", got)
	}
	if r.Snapshot == "" || r.Steps[0].Rows != 2 {
		t.Errorf("report %+v", r)
	}
	notes := strings.Join(r.Steps[0].Notes, "; ")
	if !strings.Contains(notes, "the column note is dropped") || !strings.Contains(notes, "the index prices_note is dropped") {
		t.Errorf("notes %s", notes)
	}
	if one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE name = 'prices_price'`) != 0 {
		t.Error("prices_price lost its column, but is still there")
	}
	if one[int](t, p.DB, `SELECT count(*) FROM _jenab_columns`) != 0 {
		t.Error("the dropped column's annotation is still there")
	}
	// The alerts' foreign key still points to prices, and holds.
	mustExec(t, p.DB, `PRAGMA foreign_key_check`)
	// A mapping that breaks NOT NULL, and one that breaks the foreign key.
	migrateFails(t, p, `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "x", "type": "TEXT", "not_null": true}], "primary_key": ["coin", "date"]}]`, "steps[0].mapping", "NOT NULL")
	migrateFails(t, p, `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"}],
		"primary_key": ["coin", "date"], "mapping": {"date": "date || 'x'"}}]`, "schema", "rows of alerts refer to rows of prices")
	// A default never labels the old rows (SPEC 8.2).
	migrateFails(t, p, `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "src", "type": "TEXT", "default": "api"}], "primary_key": ["coin", "date"], "mapping": {"src": "'api'"}}]`, "steps[0].columns[2].default", "leave out the default")
}

func TestMigrateWiden(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE btc_news (url TEXT NOT NULL PRIMARY KEY, title TEXT NOT NULL, slug TEXT UNIQUE, n INTEGER DEFAULT 0, _run_id TEXT, file TEXT)`)
	mustExec(t, p.DB, `CREATE INDEX btc_news_title ON btc_news (title)`)
	mustExec(t, p.DB, `INSERT INTO btc_news VALUES ('u1', 'A', 's1', 1, 'r1', NULL), ('u2', 'B', NULL, 2, 'r2', NULL)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('btc_news', 'file', 'object_ref')`)
	js := `[{"op": "widen_table", "table": "btc_news", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'", "to": "news"}]`
	try := migrateOK(t, p, js, MigrateOptions{Try: true})
	if !reflect.DeepEqual(try.KeysChanged, []string{"btc_news"}) || len(try.Lost) > 0 || try.LostRows["btc_news"] != 2 {
		t.Fatalf("try %+v", try)
	}
	if _, err := p.Migrate(context.Background(), mig("1"), steps(t, js), MigrateOptions{}); !errors.Is(err, ErrNeedsApproval) {
		t.Fatalf("without approval: %v", err)
	}
	migrateOK(t, p, js, MigrateOptions{Planned: &try})
	if got := dump(t, p, "news"); !strings.Contains(got, "[BTC u1 A s1 1 r1 <nil>]") || !strings.Contains(got, "[BTC u2 B <nil> 2 r2 <nil>]") {
		t.Errorf("rows %s", got)
	}
	for q, want := range map[string]string{
		`SELECT group_concat(name) FROM (SELECT name FROM pragma_table_info('news') WHERE pk > 0 ORDER BY pk)`:           "coin,url",
		`SELECT group_concat(i.name) FROM pragma_index_list('news') l, pragma_index_info(l.name) i WHERE l.origin = 'u'`: "coin,slug",
		`SELECT "notnull" || ifnull(dflt_value, '') FROM pragma_table_info('news') WHERE name = 'coin'`:                  "1",
		`SELECT ifnull(dflt_value, '') FROM pragma_table_info('news') WHERE name = 'n'`:                                  "0",
		`SELECT tbl_name FROM sqlite_schema WHERE name = 'btc_news_title'`:                                               "news",
		`SELECT table_name FROM _jenab_columns`:                                                                          "news",
	} {
		if got := one[string](t, p.DB, q); got != want {
			t.Errorf("%s: %q, want %q", q, got, want)
		}
	}
	if one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE name = 'btc_news'`) != 0 {
		t.Error("btc_news is still there")
	}
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "coin", "type": "TEXT"}, "value": "'ETH'"}]`,
		"steps[0].column.name", "already has a column")
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "TEXT", "default": "en"}, "value": "'en'"}]`,
		"steps[0].column.default", "no default")
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "TEXT"}, "value": "nope("}]`,
		"steps[0].value", "")
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "TEXT"}, "value": "NULL"}]`,
		"steps[0].value", "every old row a value")
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "BLOB"}, "value": "x''"}]`,
		"steps[0].column.type", "BLOB")
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "TEXT"}, "value": "'en'", "to": "btc_news_title"}]`,
		"steps[0].to", "is taken (index)")
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "TEXT", "check": "nope(lang)"}, "value": "'en'"}]`,
		"steps[0].column.check", "")
	// Column CHECKs stay; a table CHECK can't be carried over.
	mustExec(t, p.DB, `CREATE TABLE prices (date TEXT PRIMARY KEY, price REAL CHECK (price > 0) CHECK (price <> ')'), note TEXT)`)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('d1', 5, NULL)`)
	migrateFails(t, p, `[{"op": "widen_table", "table": "news", "column": {"name": "lang", "type": "TEXT"}, "value": "'en'", "to": "prices"}]`,
		"steps[0].to", "already exists")
	migrateOK(t, p, `[{"op": "widen_table", "table": "prices", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'"}]`, MigrateOptions{Approved: true})
	if _, err := Do(context.Background(), p.DB, limit.Interactive, func(tx *sql.Tx) (sql.Result, error) {
		return tx.Exec(`INSERT INTO prices VALUES ('ETH', 'd1', -1, NULL)`)
	}); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("a negative price: %v", err)
	}
	mustExec(t, p.DB, `CREATE TABLE quotes (date TEXT PRIMARY KEY, bid REAL, ask REAL, CHECK (bid <= ask))`)
	migrateFails(t, p, `[{"op": "widen_table", "table": "quotes", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'"}]`,
		"steps[0].table", "table CHECK")
	// A key changed by a rebuild and then widened is not only widened.
	mustExec(t, p.DB, `CREATE TABLE rk (a TEXT NOT NULL, b TEXT NOT NULL, PRIMARY KEY (a))`)
	mustExec(t, p.DB, `CREATE TABLE wk (d TEXT PRIMARY KEY)`)
	r, err := p.Migrate(context.Background(), mig("rk"), steps(t, `[{"op": "rebuild_table", "table": "rk",
		"columns": [{"name": "a", "type": "TEXT", "not_null": true}, {"name": "b", "type": "TEXT", "not_null": true}], "primary_key": ["a", "b"]},
		{"op": "widen_table", "table": "rk", "column": {"name": "c", "type": "TEXT"}, "value": "'x'"},
		{"op": "widen_table", "table": "wk", "column": {"name": "c", "type": "TEXT"}, "value": "'x'"}]`), MigrateOptions{Try: true})
	if err != nil || !slices.Equal(r.KeysChanged, []string{"rk", "rk", "wk"}) || !slices.Equal(r.KeysWidened, []string{"wk"}) {
		t.Errorf("%v, keys %v, widened %v", err, r.KeysChanged, r.KeysWidened)
	}
	// A carried-over check goes through layer 1 again.
	mustExec(t, p.DB, `CREATE TABLE odd (date TEXT PRIMARY KEY, v REAL CHECK (v > 0 OR sqlite_version() <> ''))`)
	migrateFails(t, p, `[{"op": "widen_table", "table": "odd", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'"}]`,
		"steps[0].table", `refuses the CHECK of "v"`)
}

// widen_table puts the new column in front of unique indexes and foreign
// keys to the table itself, and refuses what it can't widen.
func TestMigrateWidenRefs(t *testing.T) {
	p, _ := openTestProject(t)
	widen := func(table string) string {
		return `[{"op": "widen_table", "table": "` + table + `", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'"}]`
	}
	// A unique index from create_index, with a collation.
	mustExec(t, p.DB, `CREATE TABLE prices (date TEXT PRIMARY KEY, price REAL)`)
	mustExec(t, p.DB, `CREATE UNIQUE INDEX by_date ON prices (date COLLATE NOCASE DESC)`)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('d1', 1)`)
	migrateOK(t, p, widen("prices"), MigrateOptions{Approved: true})
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('ETH', 'd1', 2)`)
	if got := one[string](t, p.DB, `SELECT sql FROM sqlite_schema WHERE name = 'by_date'`); got != `CREATE UNIQUE INDEX "by_date" ON "prices" ("coin", "date" COLLATE "NOCASE" DESC)` {
		t.Errorf("by_date %s", got)
	}
	mustExec(t, p.DB, `CREATE TABLE p2 (date TEXT PRIMARY KEY, price REAL)`)
	mustExec(t, p.DB, `CREATE UNIQUE INDEX p2_big ON p2 (price) WHERE price > 10`)
	migrateFails(t, p, widen("p2"), "steps[0].table", "unique index p2_big")
	// A foreign key from another table, and one to the table itself.
	mustExec(t, p.DB, `CREATE TABLE news (url TEXT PRIMARY KEY, parent TEXT REFERENCES news)`)
	mustExec(t, p.DB, `CREATE TABLE notes (id INTEGER PRIMARY KEY, url TEXT REFERENCES news (url))`)
	mustExec(t, p.DB, `INSERT INTO news VALUES ('u1', NULL), ('u2', 'u1')`)
	mustExec(t, p.DB, `INSERT INTO notes VALUES (1, 'u2')`)
	migrateFails(t, p, widen("news"), "steps[0].table", "rebuild_table notes before this step, with a coin column and the foreign key (coin, url)")
	migrateOK(t, p, `[{"op": "rebuild_table", "table": "notes", "columns": [{"name": "id", "type": "INTEGER"}, {"name": "coin", "type": "TEXT"}, {"name": "url", "type": "TEXT"}],
		"primary_key": ["id"], "foreign_keys": [{"columns": ["coin", "url"], "table": "news", "to": ["coin", "url"]}], "mapping": {"coin": "'BTC'"}},
		{"op": "widen_table", "table": "news", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'"}]`, MigrateOptions{Approved: true})
	if got := one[string](t, p.DB, `SELECT group_concat("from" || '>' || ifnull("to", '')) FROM pragma_foreign_key_list('news')`); got != "coin>,parent>" {
		t.Errorf("news refers to itself by %s", got)
	}
	mustExec(t, p.DB, `PRAGMA foreign_key_check`)
	// A rowid key.
	mustExec(t, p.DB, `CREATE TABLE memos (id INTEGER PRIMARY KEY, text TEXT)`)
	migrateFails(t, p, widen("memos"), "steps[0].table", "numbers new rows by itself")
}

func TestMigrateDropColumn(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `CREATE INDEX prices_note ON prices (note)`)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, 'a')`)
	r := migrateOK(t, p, `[{"op": "drop_column", "table": "prices", "column": "note"}]`, MigrateOptions{Approved: true})
	if got := dump(t, p, "prices"); got != "prices: [BTC d1 1]" || r.Steps[0].Notes[0] != "the index prices_note is dropped with the column" {
		t.Errorf("%s, notes %v", got, r.Steps[0].Notes)
	}
	migrateFails(t, p, `[{"op": "drop_column", "table": "prices", "column": "coin"}]`, "steps[0].column", "primary key")
	// A column in a UNIQUE constraint: SQLite can't drop it, so Go rebuilds.
	mustExec(t, p.DB, `CREATE TABLE coins (symbol TEXT PRIMARY KEY NOT NULL, name TEXT UNIQUE, rank INTEGER DEFAULT 0)`)
	mustExec(t, p.DB, `INSERT INTO coins VALUES ('BTC', 'Bitcoin', 1)`)
	r = migrateOK(t, p, `[{"op": "drop_column", "table": "coins", "column": "name"}]`, MigrateOptions{Approved: true})
	if got := dump(t, p, "coins"); got != "coins: [BTC 1]" || !strings.Contains(strings.Join(r.Steps[0].Notes, ";"), "rebuilt") {
		t.Errorf("%s, notes %v", got, r.Steps[0].Notes)
	}
	if s := schemaSQL(t, p, "coins"); !strings.Contains(s, `"rank" INTEGER DEFAULT 0`) {
		t.Errorf("coins %q", s)
	}
	// Column CHECKs are carried over; a table CHECK can't be.
	mustExec(t, p.DB, `CREATE TABLE c2 (a TEXT PRIMARY KEY, b TEXT UNIQUE CHECK (b <> ''), "c ""x""" TEXT CHECK (length("c ""x""") < 9))`)
	migrateOK(t, p, `[{"op": "drop_column", "table": "c2", "column": "b"}]`, MigrateOptions{Approved: true})
	if s := schemaSQL(t, p, "c2"); !strings.Contains(s, `CHECK (length("c ""x""") < 9`) {
		t.Errorf("c2 %q", s)
	}
	mustExec(t, p.DB, `CREATE TABLE c3 (a TEXT PRIMARY KEY, b TEXT UNIQUE, c TEXT, CONSTRAINT bc CHECK (b <> c))`)
	migrateFails(t, p, `[{"op": "drop_column", "table": "c3", "column": "b"}]`, "steps[0].column", "use rebuild_table")
}

func TestMigrateDropTable(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `CREATE TABLE alerts (coin TEXT, date TEXT, PRIMARY KEY (coin, date), FOREIGN KEY (coin, date) REFERENCES prices)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('prices', 'note', 'object_ref')`)
	// alerts would point to a missing table.
	migrateFails(t, p, `[{"op": "drop_table", "table": "prices"}]`, "schema", "foreign key to prices")
	migrateOK(t, p, `[{"op": "drop_table", "table": "alerts"}, {"op": "drop_table", "table": "prices"}]`, MigrateOptions{Approved: true})
	if one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE name IN ('prices', 'alerts')`) != 0 || one[int](t, p.DB, `SELECT count(*) FROM _jenab_columns`) != 0 {
		t.Error("tables or annotations left")
	}
}

// The schema guard catches what raw SQL could make, here run through a
// dependent's Update, which runs in the migration's transaction.
func TestMigrateGuard(t *testing.T) {
	p := pricesProject(t)
	for _, c := range []struct{ sql, want string }{
		{`CREATE TRIGGER tr AFTER INSERT ON prices BEGIN SELECT 1; END`, "tr is a trigger"},
		{`CREATE VIEW v AS SELECT 1`, "v is a view"},
		{`CREATE TABLE nokey (a TEXT)`, "nokey has no explicit primary key"},
		{`CREATE INDEX _jenab_x ON _jenab_changes (at)`, "internal tables"},
		{`CREATE TABLE pins (k TEXT PRIMARY KEY REFERENCES _jenab_objects (key) ON DELETE CASCADE)`, "foreign key to _jenab_objects, one of Jenab's own tables"},
		{`ALTER TABLE prices ADD COLUMN b BLOB`, ""},
	} {
		p.AddDependent(&rawDependent{sql: c.sql})
		r, err := p.Migrate(context.Background(), mig("1"), Migration{
			Steps:   steps(t, `[{"op": "add_column", "table": "prices", "column": {"name": "z", "type": "TEXT"}}]`).Steps,
			Updates: []DependentUpdate{{Kind: "raw", ID: "1"}}}, MigrateOptions{})
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: %v", c.sql, err)
			}
			continue
		}
		if err == nil || !strings.Contains(fmt.Sprint(r.Problems), c.want) {
			t.Errorf("%s: %v, problems %v", c.sql, err, r.Problems)
		}
	}
}

// rawDependent runs raw SQL in its Update, for tests of the guard.
type rawDependent struct{ sql string }

func (d *rawDependent) Kind() string { return "raw" }
func (d *rawDependent) Update(tx *sql.Tx, _ id.Source, _ DependentUpdate) error {
	_, err := tx.Exec(d.sql)
	return err
}
func (d *rawDependent) Check(*sql.Tx) ([]Problem, error) { return nil, nil }

// fakeViews is a dependent kind like Phase 3's views: a config is a query,
// kept in a table the test makes, and checks by preparing it.
type fakeViews struct{}

func (fakeViews) Kind() string { return "view" }
func (fakeViews) Update(tx *sql.Tx, _ id.Source, u DependentUpdate) error {
	var c struct{ Query string }
	if err := json.Unmarshal(u.Config, &c); err != nil {
		return invalid{err}
	}
	_, err := tx.Exec(`INSERT INTO _jenab_test_views (id, query) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET query = excluded.query`, u.ID, c.Query)
	return err
}
func (fakeViews) Check(tx *sql.Tx) ([]Problem, error) {
	vs, err := queryTx(tx, `SELECT id, query FROM _jenab_test_views ORDER BY id`, nil, func(r *sql.Rows) ([2]string, error) {
		var v [2]string
		return v, r.Scan(&v[0], &v[1])
	})
	if err != nil {
		return nil, err
	}
	var probs []Problem
	for _, v := range vs {
		if st, err := tx.Prepare(v[1]); err != nil {
			probs = append(probs, Problem{Path: "view:" + v[0], Msg: sqliteMsg(err)})
		} else {
			st.Close()
		}
	}
	return probs, nil
}

// List gives each view with the tables its query names, for Dependents.
func (fakeViews) List(_ context.Context, tx *sql.Tx) ([]DependentInfo, error) {
	return queryTx(tx, `SELECT id, query FROM _jenab_test_views ORDER BY id DESC`, nil, func(r *sql.Rows) (DependentInfo, error) {
		d := DependentInfo{Kind: "view"}
		var q string
		err := r.Scan(&d.ID, &q)
		for _, f := range strings.Fields(q) {
			if strings.HasPrefix(f, "btc_") {
				d.Uses = append(d.Uses, f)
			}
		}
		return d, err
	})
}

// Dependents lists the configs of the kinds that can list them, by kind
// and ID; a migration's report says what its destructive steps drop.
func TestDependents(t *testing.T) {
	p := ethereumProject(t)
	p.AddDependent(&rawDependent{})
	ctx := context.Background()
	ds, err := p.Dependents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []DependentInfo{{Kind: "view", ID: "btc_today", Uses: []string{"btc_prices"}}, {Kind: "view", ID: "news", Uses: []string{"btc_news"}}}
	if !reflect.DeepEqual(ds, want) {
		t.Errorf("Dependents %+v, want %+v", ds, want)
	}
	rep, err := p.Migrate(ctx, id.SourceUser, Migration{Steps: []json.RawMessage{
		json.RawMessage(`{"op": "drop_column", "table": "btc_prices", "column": "_Run_ID"}`),
		json.RawMessage(`{"op": "create_table", "table": "tmp", "columns": [{"name": "a", "type": "TEXT"}], "primary_key": ["a"]}`),
		json.RawMessage(`{"op": "drop_table", "table": "tmp"}`)}}, MigrateOptions{Try: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Lost, []string{"btc_prices._run_id", "tmp"}) || !slices.Equal(rep.Destructive, []int{0, 2}) {
		t.Errorf("Lost %v, Destructive %v", rep.Lost, rep.Destructive)
	}

	empty, _ := openTestProject(t)
	if ds, err := empty.Dependents(ctx); err != nil || ds != nil {
		t.Errorf("no kinds: %v, %v", ds, err)
	}
}

func ethereumProject(t *testing.T) *ProjectDB {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE btc_prices (date TEXT PRIMARY KEY, price REAL, _run_id TEXT, _fetched_at TEXT)`)
	mustExec(t, p.DB, `CREATE TABLE btc_news (url TEXT PRIMARY KEY, title TEXT, reason TEXT)`)
	mustExec(t, p.DB, `CREATE TABLE _jenab_test_views (id TEXT PRIMARY KEY, query TEXT)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_test_views VALUES ('btc_today', 'SELECT price FROM btc_prices ORDER BY date DESC LIMIT 1'),
		('news', 'SELECT title, reason FROM btc_news')`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 1000)
		INSERT INTO btc_prices SELECT date('2024-01-01', '+' || i || ' days'), 40000 + i, 'run' || (i % 7), '2026-10-0' || (i % 9 + 1) FROM n`)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 300)
		INSERT INTO btc_news SELECT 'https://example.com/' || i, 'Title ' || i, 'reason ' || i FROM n`)
	p.AddDependent(fakeViews{})
	return p
}

// ethereumSteps are the seven steps of SPEC 9.5.
const ethereumSteps = `[
	{"op": "create_table", "table": "coins", "columns": [{"name": "symbol", "type": "TEXT"}], "primary_key": ["symbol"]},
	{"op": "insert_rows", "table": "coins", "rows": [{"symbol": "BTC"}, {"symbol": "ETH"}]},
	{"op": "create_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "price", "type": "REAL"}, {"name": "_run_id", "type": "TEXT"}, {"name": "_fetched_at", "type": "TEXT"}],
		"primary_key": ["coin", "date"]},
	{"op": "copy_data", "into": "prices", "columns": ["coin", "date", "price", "_run_id", "_fetched_at"],
		"from": "SELECT 'BTC', date, price, _run_id, _fetched_at FROM btc_prices"},
	{"op": "rebuild_table", "table": "btc_news", "columns": [{"name": "coin", "type": "TEXT", "not_null": true},
		{"name": "url", "type": "TEXT"}, {"name": "title", "type": "TEXT"}, {"name": "reason", "type": "TEXT"}],
		"primary_key": ["coin", "url"], "mapping": {"coin": "'BTC'"}},
	{"op": "rename_table", "from": "btc_news", "to": "news"},
	{"op": "drop_table", "table": "btc_prices"}]`

func ethereumUpdates() []DependentUpdate {
	return []DependentUpdate{
		{Kind: "view", ID: "btc_today", Config: json.RawMessage(`{"query": "SELECT price FROM prices WHERE coin = 'BTC' ORDER BY date DESC LIMIT 1"}`)},
		{Kind: "view", ID: "news", Config: json.RawMessage(`{"query": "SELECT coin, title, reason FROM news"}`)},
		{Kind: "view", ID: "prices", Config: json.RawMessage(`{"query": "SELECT coin, date, price FROM prices"}`)},
	}
}

// SPEC 9.5 as one migration: nothing already collected is lost.
func TestMigrateEthereum(t *testing.T) {
	p := ethereumProject(t)
	ctx := context.Background()
	before := dump(t, p, "btc_prices")
	news := dump(t, p, "btc_news")
	m := steps(t, ethereumSteps)
	// Without the view updates, the views that read btc_prices and
	// btc_news fail their check, and nothing changes.
	r, err := p.Migrate(ctx, mig("eth"), m, MigrateOptions{Approved: true})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(fmt.Sprint(r.Problems), "view:btc_today: no such table: btc_prices") {
		t.Fatalf("%v, problems %v", err, r.Problems)
	}
	if dump(t, p, "btc_prices") != before || one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE name IN ('coins', 'prices', 'news')`) != 0 {
		t.Fatal("a failed migration changed the project")
	}
	m.Updates = ethereumUpdates()
	r, err = p.Migrate(ctx, mig("eth"), m, MigrateOptions{})
	if !errors.Is(err, ErrNeedsApproval) || fmt.Sprint(r.Destructive) != "[6]" {
		t.Fatalf("%v, destructive %v", err, r.Destructive)
	}
	r, err = p.Migrate(ctx, mig("eth"), m, MigrateOptions{Approved: true})
	if err != nil {
		t.Fatalf("%v: %v", err, r.Problems)
	}
	if r.Snapshot == "" || len(r.Updated) != 3 || r.SchemaVersion != 1 || r.Estimate.Long {
		t.Errorf("report %+v", r)
	}
	if got := dump(t, p, "coins"); got != "coins: [BTC] [ETH]" {
		t.Error(got)
	}
	// prices holds every btc_prices row, with coin BTC and the same values.
	after := strings.ReplaceAll(dump(t, p, "prices"), "[BTC ", "[")
	if strings.TrimPrefix(after, "prices: ") != strings.TrimPrefix(before, "btc_prices: ") {
		t.Error("prices lost rows or values")
	}
	if got := strings.ReplaceAll(dump(t, p, "news"), "[BTC ", "["); strings.TrimPrefix(got, "news: ") != strings.TrimPrefix(news, "btc_news: ") {
		t.Error("news lost rows or values")
	}
	if one[int](t, p.DB, `SELECT count(*) FROM sqlite_schema WHERE name IN ('btc_prices', 'btc_news')`) != 0 {
		t.Error("the old tables are still there")
	}
	// One article can now be stored for both coins.
	if _, err := p.Insert(ctx, limit.Interactive, msg("1"), "news", []Row{{"coin": "ETH", "url": "https://example.com/1", "title": "t"}}, ConflictFail); err != nil {
		t.Error(err)
	}
	if got := one[string](t, p.DB, `SELECT query FROM _jenab_test_views WHERE id = 'news'`); got != "SELECT coin, title, reason FROM news" {
		t.Error(got)
	}
	// The snapshot has the old schema, and a restore would lose two changes.
	snaps, err := p.Snapshots(ctx)
	if err != nil || len(snaps) != 1 || snaps[0].Name != r.Snapshot || snaps[0].WritesSince != 2 {
		t.Fatalf("snapshots %+v, %v", snaps, err)
	}
}

// A failed migration leaves the file byte for byte as it was.
func TestMigrateRollbackBytes(t *testing.T) {
	p := ethereumProject(t)
	ctx := context.Background()
	fileHash := func() string {
		if _, err := doConn(ctx, p.DB, limit.Interactive, func(ctx context.Context, c *sql.Conn) (struct{}, error) {
			_, err := c.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
			return struct{}{}, err
		}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p.Path())
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(b))
	}
	before := fileHash()
	m := steps(t, ethereumSteps)
	m.Updates = ethereumUpdates()
	// The eighth step fails in SQLite, after the seven before it ran.
	m.Steps = append(m.Steps, json.RawMessage(`{"op": "add_column", "table": "prices", "column": {"name": "x", "type": "INTEGER", "check": "x IS NOT NULL"}, "value": "NULL"}`))
	r, err := p.Migrate(ctx, mig("eth"), m, MigrateOptions{Approved: true})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(fmt.Sprint(r.Problems), "steps[7]") || r.Steps[3].Rows != 1000 {
		t.Fatalf("%v, problems %v, steps %+v", err, r.Problems, r.Steps)
	}
	if after := fileHash(); after != before {
		t.Error("the file changed")
	}
	// A try runs every step and the checks, and changes nothing either.
	m.Steps = m.Steps[:7]
	r, err = p.Migrate(ctx, mig("eth"), m, MigrateOptions{Try: true})
	if err != nil || !r.Tried || r.Change != "" || r.SchemaVersion != 1 || r.Steps[3].Rows != 1000 {
		t.Fatalf("%v, report %+v", err, r)
	}
	if after := fileHash(); after != before {
		t.Error("a try changed the file")
	}
	// The steps run, and the schema guard fails: the dropped table is still
	// a parent.
	m.Steps = slices.Insert(slices.Clone(m.Steps), 6, json.RawMessage(`{"op": "create_table", "table": "alerts",
		"columns": [{"name": "d", "type": "TEXT"}], "primary_key": ["d"], "foreign_keys": [{"columns": ["d"], "table": "btc_prices"}]}`))
	r, err = p.Migrate(ctx, mig("eth"), m, MigrateOptions{Approved: true})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(fmt.Sprint(r.Problems), "schema: the table alerts has a foreign key to btc_prices, which doesn't exist") {
		t.Fatalf("%v, problems %v", err, r.Problems)
	}
	if after := fileHash(); after != before {
		t.Error("a migration that failed the guard changed the file")
	}
}

// The undo proposal: a migration that only adds columns or indexes to a
// table doesn't stop an undo of earlier writes to it; one that changes
// its rows or names does.
func TestMigrateUndo(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	turn := []id.Source{msg("01TURN")}
	if _, err := p.Insert(ctx, limit.Interactive, turn[0], "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Update(ctx, limit.Interactive, msg("01EDIT"), UpdateReq{Table: "prices", Set: Row{"price": 2.0},
		Keys: []Row{{"coin": "BTC", "date": "d1"}}}); err != nil {
		t.Fatal(err)
	}
	// A value for the old rows writes only the new column, so undo of the
	// turns before passes (2.6).
	migrateOK(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "volume", "type": "REAL"}, "value": "price * 10"},
		{"op": "create_index", "table": "prices", "columns": ["volume"]}]`, MigrateOptions{})
	if _, err := p.Update(ctx, limit.Interactive, msg("01VOL"), UpdateReq{Table: "prices", Set: Row{"volume": 5.0},
		Keys: []Row{{"coin": "BTC", "date": "d1"}}}); err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, []id.Source{msg("01VOL")}, UndoOptions{})
	undoOK(t, p, []id.Source{msg("01EDIT")}, UndoOptions{})
	if got := dump(t, p, "prices"); got != "prices: [BTC d1 1 <nil> 20]" {
		t.Errorf("after undo %s", got)
	}
	migrateOK(t, p, `[{"op": "rename_column", "table": "prices", "from": "note", "to": "memo"}]`, MigrateOptions{})
	r, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Reason != ConflictSchema {
		t.Fatalf("%v, conflicts %+v", err, r.Conflicts)
	}
}

// An add_column that writes the rows costs a copy or a rebuild of them.
func TestMigrateEstimateAddColumn(t *testing.T) {
	p := ethereumProject(t)
	for _, tc := range []struct {
		col, value string
		rate       time.Duration
	}{
		{`{"name": "x", "type": "TEXT"}`, "NULL", 0},
		{`{"name": "x", "type": "TEXT"}`, "'x'", rateCopy},
		{`{"name": "x", "type": "TEXT", "not_null": true}`, "'x'", rateRebuild},
	} {
		r, err := p.Migrate(context.Background(), mig("add"), steps(t, `[{"op": "add_column", "table": "btc_prices", "column": `+tc.col+`, "value": "`+tc.value+`"}]`),
			MigrateOptions{Try: true})
		switch {
		case err != nil:
			t.Errorf("%s: %v", tc.value, err)
		case tc.rate == 0 && (r.Estimate.Rows != 0 || r.Estimate.Bytes != 0):
			t.Errorf("%s: estimate %+v", tc.value, r.Estimate)
		case tc.rate != 0 && (r.Estimate.Rows != 1000 || r.Estimate.Time != 1000*tc.rate || r.Estimate.Bytes <= 0):
			t.Errorf("%s %s: estimate %+v", tc.col, tc.value, r.Estimate)
		}
	}
}

func TestMigrateEstimate(t *testing.T) {
	p := ethereumProject(t)
	ctx := context.Background()
	m := steps(t, ethereumSteps)
	m.Updates = ethereumUpdates()
	r, err := p.Migrate(ctx, mig("eth"), m, MigrateOptions{Try: true})
	if err != nil {
		t.Fatal(err)
	}
	// copy_data reads 1000 rows; rebuild_table rewrites 300.
	if r.Estimate.Rows != 1300 || r.Estimate.Time <= 0 || r.Estimate.Bytes <= 0 || r.Estimate.Long {
		t.Errorf("estimate %+v", r.Estimate)
	}
	// Free space: a destructive migration needs room for its snapshot.
	defer func(f func(string) (uint64, string, bool)) { freeSpace = f }(freeSpace)
	freeSpace = func(string) (uint64, string, bool) { return 10, "vol", true }
	if _, err := p.Migrate(ctx, mig("eth"), m, MigrateOptions{Approved: true}); !errors.Is(err, ErrNoSpace) {
		t.Errorf("err %v", err)
	}
	if err := checkSpace(t.TempDir(), Estimate{Bytes: 100}, true, 0); !errors.Is(err, ErrNoSpace) {
		t.Errorf("long: %v", err)
	}
	freeSpace = func(p string) (uint64, string, bool) { return 250, "vol", true }
	// 2×100 on the project's drive and 0.5×100 on the temp drive: the same one.
	if err := checkSpace(t.TempDir(), Estimate{Bytes: 100}, true, 0); err != nil {
		t.Errorf("250 free: %v", err)
	}
	if err := checkSpace(t.TempDir(), Estimate{Bytes: 101}, true, 0); !errors.Is(err, ErrNoSpace) {
		t.Errorf("251 needed: %v", err)
	}
}

// Snapshots: the last five are kept, and a restore puts one back and says
// how many changes it lost.
func TestMigrateSnapshots(t *testing.T) {
	p, dir := openTestProject(t)
	ctx := context.Background()
	for i := range 7 {
		migrateOK(t, p, fmt.Sprintf(`[{"op": "create_table", "table": "t%d", "columns": [{"name": "a", "type": "TEXT"}], "primary_key": ["a"]}]`, i),
			MigrateOptions{})
		if i > 0 {
			migrateOK(t, p, fmt.Sprintf(`[{"op": "drop_table", "table": "t%d"}]`, i-1), MigrateOptions{Approved: true})
		}
	}
	snaps, err := p.Snapshots(ctx)
	if err != nil || len(snaps) != keepSnapshots {
		t.Fatalf("%d snapshots, %v", len(snaps), err)
	}
	if snaps[0].WritesSince != 1 || snaps[4].WritesSince != 9 {
		t.Errorf("writes since %+v", snaps)
	}
	if _, err := RestoreSnapshot(ctx, dir, snaps[0].Name); err == nil {
		t.Error("restored over an open project")
	}
	p.Close(ctx)
	lost, err := RestoreSnapshot(ctx, dir, snaps[0].Name)
	if err != nil || lost != 1 {
		t.Fatalf("lost %d, %v", lost, err)
	}
	if _, err := RestoreSnapshot(ctx, dir, "../project.db"); !errors.Is(err, ErrNoSnapshot) {
		t.Error(err)
	}
	q, err := OpenProject(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(ctx)
	// Before the last drop: t5 and t6 are there.
	if got := one[string](t, q.DB, `SELECT group_concat(name) FROM (SELECT name FROM sqlite_schema WHERE name LIKE 't_' ORDER BY name)`); got != "t5,t6" {
		t.Errorf("tables %s", got)
	}
	if v, _ := q.SchemaVersion(ctx); v != 12 {
		t.Errorf("schema version %d", v)
	}
	if ents, _ := filepath.Glob(filepath.Join(dir, "*.partial*")); len(ents) > 0 {
		t.Errorf("partials %v", ents)
	}
}

// A long migration of 1M rows (SPEC 7.5): readers keep their p99 under
// 250 ms while it runs, a long try runs on a copy without holding the
// writer, and the estimate is in the right range. Skipped with -short.
func TestMigrateLong(t *testing.T) {
	if testing.Short() {
		t.Skip("1M rows")
	}
	const rows = 1_000_000
	p, dir := openTestProject(t)
	ctx := context.Background()
	start := time.Now()
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, coin TEXT, price REAL, note TEXT)")
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO t SELECT i, 'C' || (i % 100), i * 0.5, 'note ' || i FROM n`, rows)
	t.Logf("filled %d rows in %v", rows, time.Since(start).Round(time.Millisecond))
	js := `[{"op": "rebuild_table", "table": "t", "columns": [{"name": "id", "type": "INTEGER"}, {"name": "coin", "type": "TEXT", "not_null": true},
		{"name": "price", "type": "REAL"}, {"name": "cents", "type": "INTEGER"}, {"name": "note", "type": "TEXT"}],
		"primary_key": ["id"], "mapping": {"cents": "CAST(price * 100 AS INTEGER)"}},
		{"op": "create_index", "table": "t", "columns": ["coin", "price"]}]`

	// reads runs point and range reads until stop, and returns their times.
	reads := func(stop <-chan struct{}) <-chan []time.Duration {
		out := make(chan []time.Duration, 1)
		go func() {
			var ds []time.Duration
			for i := 0; ; i++ {
				select {
				case <-stop:
					out <- ds
					return
				default:
				}
				s := time.Now()
				_, err := Query(ctx, p.DB, "SELECT id, price FROM t WHERE id BETWEEN ? AND ? + 50", []any{i * 7919 % rows, i * 7919 % rows},
					func(r *sql.Rows) (int, error) {
						var a int
						var b float64
						return a, r.Scan(&a, &b)
					})
				if err != nil {
					t.Error(err)
				}
				ds = append(ds, time.Since(s))
				time.Sleep(2 * time.Millisecond)
			}
		}()
		return out
	}
	p99 := func(ds []time.Duration) time.Duration {
		slices.Sort(ds)
		return ds[len(ds)*99/100]
	}

	// The try: a write to another table goes through while it runs.
	mustExec(t, p.DB, "CREATE TABLE other (id INTEGER PRIMARY KEY)")
	tryDone := make(chan MigrationReport, 1)
	start = time.Now()
	go func() { tryDone <- migrateOK(t, p, js, MigrateOptions{Try: true}) }()
	time.Sleep(300 * time.Millisecond)
	ws := time.Now()
	mustExec(t, p.DB, "INSERT INTO other VALUES (1)")
	wrote := time.Since(ws)
	r := <-tryDone
	t.Logf("try on a copy: %v; a write during it took %v", time.Since(start).Round(time.Millisecond), wrote.Round(time.Millisecond))
	if !r.Estimate.Long || r.Change != "" || r.Steps[0].Rows != rows {
		t.Errorf("try report %+v", r)
	}
	if wrote > time.Second {
		t.Errorf("a write waited %v behind the try", wrote)
	}
	if ents, _ := os.ReadDir(filepath.Join(dir, "tmp")); len(ents) > 0 {
		t.Errorf("the try left %d files in tmp", len(ents))
	}

	stop := make(chan struct{})
	got := reads(stop)
	start = time.Now()
	r = migrateOK(t, p, js, MigrateOptions{})
	took := time.Since(start)
	close(stop)
	ds := <-got
	t.Logf("migration: %v for %d rows (estimate %v, %d rows, %s); %d reads during it, p99 %v, max %v",
		took.Round(time.Millisecond), rows, r.Estimate.Time.Round(time.Millisecond), r.Estimate.Rows, byteText(r.Estimate.Bytes),
		len(ds), p99(ds).Round(time.Microsecond), slices.Max(ds).Round(time.Microsecond))
	if len(ds) < 20 || p99(ds) > 250*time.Millisecond {
		t.Errorf("%d reads, p99 %v", len(ds), p99(ds))
	}
	if !r.Estimate.Long || r.Estimate.Rows != 2*rows {
		t.Errorf("estimate %+v", r.Estimate)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM t WHERE cents = CAST(price * 100 AS INTEGER)") != rows {
		t.Error("rows lost or wrong")
	}
	if fi, err := os.Stat(p.Path() + "-wal"); err == nil && fi.Size() > 1<<20 {
		t.Errorf("the WAL is %s after the checkpoint", byteText(fi.Size()))
	}
}

// An expression can't close the brackets Go puts it in.
func TestMigrateExprInjection(t *testing.T) {
	p := pricesProject(t)
	evil := `1), \"evil col\" ANY, b BLOB GENERATED ALWAYS AS (zeroblob(4)) VIRTUAL, CHECK (1`
	migrateFails(t, p, `[{"op": "create_table", "table": "x", "columns": [{"name": "a", "type": "TEXT", "check": "`+evil+`"}],
		"primary_key": ["a"]}]`, "steps[0].columns[0].check", "has no matching `(`")
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "a", "type": "TEXT", "check": "a > 0, 1"}}]`,
		"steps[0].column.check", "one expression")
	migrateFails(t, p, `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"}],
		"primary_key": ["coin", "date"], "mapping": {"date": "date), (SELECT key FROM _jenab_objects"}}]`, "steps[0].mapping.date", "refused")
	// Layer 2 runs on a check too, after the DDL: SQLite refuses a
	// subquery in a CHECK, and the migration rolls back.
	migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "z", "type": "TEXT",
		"check": "z IN (SELECT value FROM json_each('[1]'))"}}]`, "steps[0]", "")
	if one[int](t, p.DB, `SELECT count(*) FROM pragma_table_xinfo('prices')`) != 4 {
		t.Error("prices changed")
	}
}

// A foreign key can't point to Jenab's own tables.
func TestMigrateForeignKeyInternal(t *testing.T) {
	p, _ := openTestProject(t)
	migrateFails(t, p, `[{"op": "create_table", "table": "pins", "columns": [{"name": "k", "type": "TEXT"}], "primary_key": ["k"],
		"foreign_keys": [{"columns": ["k"], "table": "_jenab_objects", "to": ["key"], "on_delete": "cascade"}]}]`,
		"steps[0].foreign_keys[0].table", "Jenab's own tables")
}

// A migration planned before another one changed the schema is planned
// again; if more of its steps drop data now, it is refused, and its
// snapshot is removed.
func TestMigrateReplan(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, 'n')`)
	// Planned now, this rebuild drops note only.
	js := `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "price", "type": "REAL"}], "primary_key": ["coin", "date"]}]`
	release, added := make(chan struct{}), make(chan struct{})
	go Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec(`ALTER TABLE prices ADD COLUMN extra TEXT`)
		close(added)
		<-release
		return struct{}{}, err
	})
	<-added
	done := make(chan error, 1)
	var r MigrationReport
	go func() {
		var err error
		r, err = p.Migrate(ctx, mig("1"), steps(t, js), MigrateOptions{Approved: true})
		done <- err
	}()
	time.Sleep(300 * time.Millisecond) // the migration plans on a reader, then waits for the writer
	close(release)
	err := <-done
	if !errors.Is(err, ErrNeedsApproval) || !strings.Contains(err.Error(), "the schema changed") || r.Snapshot != "" {
		t.Fatalf("%v, report %+v", err, r)
	}
	if got := one[string](t, p.DB, `SELECT group_concat(name) FROM pragma_table_info('prices')`); got != "coin,date,price,note,extra" {
		t.Errorf("columns %s", got)
	}
	if snaps, _ := p.Snapshots(ctx); len(snaps) != 0 {
		t.Errorf("snapshots %+v", snaps)
	}
	// Planned again now, it goes through.
	migrateOK(t, p, js, MigrateOptions{Approved: true})
}

// A migration with the try the user approved drops nothing more than
// the try did: a column added while the card waited is not dropped with
// the rest (P2-06).
func TestMigratePlanned(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, 'n'), ('BTC', 'd2', 2, NULL)`)
	js := `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "price", "type": "REAL"}], "primary_key": ["coin", "date"]}]`
	try := migrateOK(t, p, js, MigrateOptions{Try: true})
	if !reflect.DeepEqual(try.Lost, []string{"prices.note"}) || try.LostRows["prices"] != 2 || len(try.KeysChanged) > 0 {
		t.Fatalf("try %+v", try)
	}
	mustExec(t, p.DB, `ALTER TABLE prices ADD COLUMN extra TEXT`)
	r, err := p.Migrate(ctx, mig("1"), steps(t, js), MigrateOptions{Planned: &try})
	if !errors.Is(err, ErrNeedsApproval) || !strings.Contains(err.Error(), "prices.extra") || r.Snapshot != "" {
		t.Fatalf("%v, report %+v", err, r)
	}
	if got := one[string](t, p.DB, `SELECT group_concat(name) FROM pragma_table_info('prices')`); got != "coin,date,price,note,extra" {
		t.Errorf("columns %s", got)
	}
	// Tried and approved again, it goes through.
	try = migrateOK(t, p, js, MigrateOptions{Try: true})
	if r := migrateOK(t, p, js, MigrateOptions{Planned: &try}); r.Snapshot == "" {
		t.Errorf("report %+v", r)
	}
	if got := dump(t, p, "prices"); got != "prices: [BTC d1 1] [BTC d2 2]" {
		t.Errorf("rows %s", got)
	}
}

// A rebuild that changes the primary key keeps every column, but needs
// approval anyway (SPEC 8.2), and the report names the table.
func TestMigrateKeysChanged(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, 'n'), ('ETH', 'd1', 2, NULL)`)
	js := `[{"op": "rename_column", "table": "prices", "from": "coin", "to": "symbol"},
		{"op": "rebuild_table", "table": "prices", "columns": [{"name": "symbol", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "price", "type": "REAL"}, {"name": "note", "type": "TEXT"}], "primary_key": ["symbol"]}]`
	try := migrateOK(t, p, js, MigrateOptions{Try: true})
	if !reflect.DeepEqual(try.KeysChanged, []string{"prices"}) || len(try.Lost) > 0 || len(try.Destructive) > 0 || try.LostRows["prices"] != 2 {
		t.Fatalf("try %+v", try)
	}
	if _, err := p.Migrate(ctx, mig("1"), steps(t, js), MigrateOptions{}); !errors.Is(err, ErrNeedsApproval) {
		t.Fatalf("without approval: %v", err)
	}
	if _, err := p.Migrate(ctx, mig("1"), steps(t, js), MigrateOptions{Planned: &MigrationReport{}}); !errors.Is(err, ErrNeedsApproval) ||
		!strings.Contains(err.Error(), "the primary key of prices") {
		t.Fatalf("with a try that changed no key: %v", err)
	}
	migrateOK(t, p, js, MigrateOptions{Planned: &try})
	// A renamed key column is the same key.
	js = `[{"op": "rename_column", "table": "prices", "from": "symbol", "to": "coin"},
		{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "price", "type": "REAL"}, {"name": "note", "type": "TEXT"}], "primary_key": ["COIN"]}]`
	if r := migrateOK(t, p, js, MigrateOptions{}); len(r.KeysChanged) > 0 {
		t.Errorf("report %+v", r)
	}
}

// The rows of a table renamed before a step drops from it are its own,
// under the new name.
func TestMigrateLostRows(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, 'n'), ('BTC', 'd2', 2, NULL)`)
	r := migrateOK(t, p, `[{"op": "rename_table", "from": "prices", "to": "quotes"}, {"op": "drop_column", "table": "quotes", "column": "note"}]`,
		MigrateOptions{Try: true})
	if !reflect.DeepEqual(r.Lost, []string{"quotes.note"}) || !reflect.DeepEqual(r.LostRows, map[string]int64{"quotes": 2}) {
		t.Errorf("report %+v", r)
	}
}

// A copy's deadline comes from the planner's count of the rows it reads,
// not from the table it writes to.
func TestMigrateCopyDeadline(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 300)
		INSERT INTO prices SELECT 'C', i, i, NULL FROM n`)
	defer func(f func(int64) time.Duration) { copyDeadline = f }(copyDeadline)
	var asked int64
	copyDeadline = func(rows int64) time.Duration {
		asked = rows
		return -1 // passed already; a short one can lose the race with the copy
	}
	migrateFails(t, p, `[{"op": "create_table", "table": "c", "columns": [{"name": "d", "type": "TEXT"}], "primary_key": ["d"]},
		{"op": "copy_data", "into": "c", "columns": ["d"], "from": "SELECT date FROM prices"}]`, "steps[1].from", "ran longer than")
	if asked != 300 {
		t.Errorf("the deadline was sized for %d rows", asked)
	}
}

// Mapping keys match the new columns as SQLite compares names, and a
// column moved by a mapping keeps its annotation.
func TestMigrateMappingNames(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	o, err := p.PutObject(ctx, limit.Interactive, msg("1"), "charts/a.png", strings.NewReader("png"), PutOptions{Source: "tool"})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, ?)`, o.Key)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('prices', 'note', 'object_ref')`)
	migrateOK(t, p, `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"},
		{"name": "price", "type": "REAL"}, {"name": "tag", "type": "TEXT"}, {"name": "chart", "type": "TEXT"}], "primary_key": ["coin", "date"],
		"mapping": {"Tag": "'x'", "PRICE": "price * 100", "chart": "NOTE"}}]`, MigrateOptions{Approved: true})
	if got := dump(t, p, "prices"); got != "prices: [BTC d1 100 x "+o.Key+"]" {
		t.Errorf("rows %s", got)
	}
	if got := dump(t, p, "_jenab_columns"); got != "_jenab_columns: [prices chart object_ref]" {
		t.Errorf("annotations %s", got)
	}
	migrateFails(t, p, `[{"op": "rebuild_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT"}, {"name": "date", "type": "TEXT"}],
		"primary_key": ["coin", "date"], "mapping": {"date": "date", "DATE": "date"}}]`, "steps[0].mapping", "has a mapping already")
}

// Foreign keys name their parent in any case.
func TestMigrateForeignKeyCase(t *testing.T) {
	p, _ := openTestProject(t)
	migrateOK(t, p, `[{"op": "create_table", "table": "coins", "columns": [{"name": "symbol", "type": "TEXT"}], "primary_key": ["symbol"]},
		{"op": "create_table", "table": "holdings", "columns": [{"name": "coin", "type": "TEXT"}], "primary_key": ["coin"],
		"foreign_keys": [{"columns": ["coin"], "table": "Coins"}]},
		{"op": "insert_rows", "table": "coins", "rows": [{"symbol": "BTC"}]},
		{"op": "insert_rows", "table": "holdings", "rows": [{"coin": "BTC"}]}]`, MigrateOptions{})
	migrateFails(t, p, `[{"op": "insert_rows", "table": "holdings", "rows": [{"coin": "ETH"}]}]`, "", "")
}

// Rebuilds keep a table's options, and refuse what they can't carry over.
func TestMigrateRebuildOptions(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE s (k TEXT PRIMARY KEY NOT NULL, checked_at TEXT, v INTEGER UNIQUE) STRICT, WITHOUT ROWID`)
	mustExec(t, p.DB, `INSERT INTO s VALUES ('a', 'now', 1)`)
	mustExec(t, p.DB, `CREATE INDEX s_rebuild ON s (checked_at)`) // the rebuild's own name for the new table
	r := migrateOK(t, p, `[{"op": "rebuild_table", "table": "s", "columns": [{"name": "k", "type": "TEXT"}, {"name": "checked_at", "type": "TEXT"},
		{"name": "v", "type": "INTEGER"}, {"name": "w", "type": "REAL"}], "primary_key": ["k"]}]`, MigrateOptions{})
	if s := schemaSQL(t, p, "s"); !strings.HasSuffix(s, "WITHOUT ROWID, STRICT") || !strings.Contains(strings.Join(r.Steps[0].Notes, ";"), "STRICT") {
		t.Errorf("s %q, notes %v", s, r.Steps[0].Notes)
	}
	// drop_column rebuilds from the schema for a UNIQUE column; checked_at
	// is a name, not a CHECK.
	mustExec(t, p.DB, `CREATE TABLE u (k TEXT PRIMARY KEY NOT NULL, checked_at TEXT, v INTEGER UNIQUE) STRICT`)
	migrateOK(t, p, `[{"op": "drop_column", "table": "u", "column": "v"}]`, MigrateOptions{Approved: true})
	if s := schemaSQL(t, p, "u"); !strings.Contains(s, "STRICT") || strings.Contains(s, `"v"`) {
		t.Errorf("u %q", s)
	}
	mustExec(t, p.DB, `CREATE TABLE d (k TEXT PRIMARY KEY NOT NULL, p TEXT UNIQUE REFERENCES u DEFERRABLE INITIALLY DEFERRED)`)
	migrateFails(t, p, `[{"op": "drop_column", "table": "d", "column": "p"}]`, "steps[0].column", "DEFERRABLE")
}

// A rename that only changes the case goes through another name.
func TestMigrateRenameCase(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, NULL)`)
	// The name the rename passes through is taken by an index.
	mustExec(t, p.DB, `CREATE INDEX prices_rename ON prices (price)`)
	migrateOK(t, p, `[{"op": "rename_table", "from": "prices", "to": "Prices"}, {"op": "rename_column", "table": "Prices", "from": "price", "to": "Price"}]`,
		MigrateOptions{})
	if got := one[string](t, p.DB, `SELECT name FROM sqlite_schema WHERE type = 'table' AND name = 'Prices'`); got != "Prices" {
		t.Error(got)
	}
	if got := one[string](t, p.DB, `SELECT group_concat(name) FROM pragma_table_info('Prices')`); got != "coin,date,Price,note" {
		t.Error(got)
	}
	migrateFails(t, p, `[{"op": "rename_table", "from": "Prices", "to": "Prices"}]`, "steps[0].to", "already")
}

// A mapping that overwrites a column whose name differs from the new one
// only in case drops its data, and needs approval.
func TestMigrateMappingCase(t *testing.T) {
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE q (k TEXT PRIMARY KEY NOT NULL, Price REAL)`)
	mustExec(t, p.DB, `INSERT INTO q VALUES ('a', 1.5)`)
	js := `[{"op": "rebuild_table", "table": "q", "columns": [{"name": "k", "type": "TEXT"}, {"name": "price", "type": "REAL"}],
		"primary_key": ["k"], "mapping": {"price": "0"}}]`
	r, err := p.Migrate(context.Background(), mig("1"), steps(t, js), MigrateOptions{})
	if !errors.Is(err, ErrNeedsApproval) || !slices.Equal(r.Destructive, []int{0}) {
		t.Fatalf("%v, destructive %v", err, r.Destructive)
	}
	if got := dump(t, p, "q"); got != "q: [a 1.5]" {
		t.Errorf("rows %s", got)
	}
	if r := migrateOK(t, p, js, MigrateOptions{Approved: true}); r.Snapshot == "" {
		t.Error("no snapshot")
	}
	if got := dump(t, p, "q"); got != "q: [a 0]" {
		t.Errorf("rows %s", got)
	}
}

// Layer 2 sees an added column's check before SQLite runs it on the rows.
func TestMigrateAddColumnCheckFirst(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 1, NULL)`)
	mustExec(t, p.DB, `CREATE VIEW keys AS SELECT key FROM _jenab_objects`)
	r := migrateFails(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "z", "type": "TEXT",
		"check": "z IN (SELECT key FROM keys)"}, "value": "NULL"}]`, "steps[0].column.check", "internal")
	if len(r.Steps[0].SQL) != 0 {
		t.Errorf("ran %v", r.Steps[0].SQL)
	}
	mustExec(t, p.DB, `DROP VIEW keys`) // the schema guard refuses views
	// The check sees the table's name and rowid, as it does in SQLite.
	migrateOK(t, p, `[{"op": "add_column", "table": "prices", "column": {"name": "vol", "type": "REAL", "check": "prices.vol >= 0"}, "value": "NULL"},
		{"op": "add_column", "table": "prices", "column": {"name": "n", "type": "INTEGER", "check": "rowid > 0 AND n > 0"}, "value": "NULL"}]`, MigrateOptions{})
	mustExec(t, p.DB, `CREATE TABLE w (k TEXT PRIMARY KEY NOT NULL) WITHOUT ROWID`)
	mustExec(t, p.DB, `INSERT INTO w VALUES ('a')`)
	migrateOK(t, p, `[{"op": "add_column", "table": "w", "column": {"name": "vol", "type": "REAL", "check": "w.vol >= 0"}, "value": "NULL"}]`, MigrateOptions{})
	migrateFails(t, p, `[{"op": "add_column", "table": "w", "column": {"name": "n", "type": "INTEGER", "check": "rowid > 0"}, "value": "NULL"}]`,
		"steps[0].column.check", "rowid")
}

// After a dependent's Update, which may write any table, the schema guard
// checks the foreign keys of every table.
func TestMigrateGuardDependentWrites(t *testing.T) {
	p := pricesProject(t)
	mustExec(t, p.DB, `CREATE TABLE coins (symbol TEXT PRIMARY KEY NOT NULL)`)
	mustExec(t, p.DB, `CREATE TABLE holdings (coin TEXT PRIMARY KEY NOT NULL REFERENCES coins)`)
	p.AddDependent(&rawDependent{sql: `INSERT INTO holdings VALUES ('ETH')`})
	r, err := p.Migrate(context.Background(), mig("1"), Migration{
		Steps:   steps(t, `[{"op": "add_column", "table": "prices", "column": {"name": "z", "type": "TEXT"}}]`).Steps,
		Updates: []DependentUpdate{{Kind: "raw", ID: "1"}}}, MigrateOptions{})
	if err == nil || !strings.Contains(fmt.Sprint(r.Problems), "1 rows of holdings refer to rows of coins") {
		t.Fatalf("%v, problems %v", err, r.Problems)
	}
	if got := dump(t, p, "holdings"); got != "holdings: " {
		t.Errorf("rows %s", got)
	}
}

// The snapshot is taken on the writer, so a write queued while the
// migration was planned lands before it or after the migration.
func TestMigrateSnapshotOnWriter(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	release, held := make(chan struct{}), make(chan struct{})
	go Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		close(held)
		<-release
		_, err := tx.Exec(`INSERT INTO prices VALUES ('BTC', 'd1', 1, 'late')`)
		return struct{}{}, err
	})
	<-held
	done := make(chan error, 1)
	var r MigrationReport
	go func() {
		var err error
		r, err = p.Migrate(ctx, mig("1"), steps(t, `[{"op": "drop_column", "table": "prices", "column": "note"}]`), MigrateOptions{Approved: true})
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(p.snapshotDir(), r.Snapshot)
	db, err := sql.Open("sqlite", sqlworker.FileURI(snap)+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var note string
	if err := db.QueryRow(`SELECT note FROM prices`).Scan(&note); err != nil || note != "late" {
		t.Errorf("the snapshot has %q, %v", note, err)
	}
}
