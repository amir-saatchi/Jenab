package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
)

// TestMain lets the test binary be its own query worker.
func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(m.Run())
}

// dataFixture is a project with agent tables: prices with a two-column
// key, a CHECK and an object_ref column; coins with a rowid key, a
// UNIQUE column, a date, a boolean and a generated column; holdings with
// a foreign key; big with 250 rows; a table without a primary key; and
// a virtual table, which only layer 2 of the guard refuses.
var dataFixture = []string{
	`CREATE TABLE prices (date TEXT NOT NULL, coin TEXT NOT NULL, price REAL NOT NULL CHECK (price > 0), volume INTEGER,
		note TEXT DEFAULT '', chart TEXT, PRIMARY KEY (date, coin))`,
	`CREATE TABLE coins (id INTEGER PRIMARY KEY, symbol TEXT NOT NULL UNIQUE, name TEXT, listed DATE, active BOOLEAN NOT NULL DEFAULT 1,
		label TEXT GENERATED ALWAYS AS (symbol || ' ' || name))`,
	`CREATE TABLE holdings (id INTEGER PRIMARY KEY, coin_id INTEGER NOT NULL REFERENCES coins(id) ON DELETE CASCADE, amount REAL)`,
	`CREATE INDEX holdings_coin ON holdings (coin_id)`,
	`CREATE TABLE big (id INTEGER PRIMARY KEY, body TEXT)`,
	`CREATE TABLE loose (a TEXT, b TEXT)`,
	`CREATE VIRTUAL TABLE notes USING fts5(body)`,
	`INSERT INTO _jenab_columns VALUES ('prices', 'chart', 'object_ref')`,
	`INSERT INTO prices VALUES ('2026-10-16', 'BTC', 60900, 120, 'نیم‌روز', NULL), ('2026-10-16', 'ETH', 2410.5, 300, 'a|b
c', NULL),
		('2026-10-17', 'BTC', 61240.25, NULL, '', NULL)`,
	`INSERT INTO coins (symbol, name, listed) VALUES ('BTC', 'Bitcoin', '2009-01-03'), ('ETH', 'Ethereum', '2015-07-30')`,
	`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 250)
	 INSERT INTO big (id, body) SELECT i, printf('%0200d', i) FROM n`,
}

// dataEnv is a test env on dataFixture.
func dataEnv(t *testing.T) *Env {
	t.Helper()
	env := testEnv(t)
	env.PreviewTokens = 1500 // tool_preview_tokens' default
	execAll(t, env, dataFixture...)
	return env
}

func execAll(t *testing.T, env *Env, stmts ...string) {
	t.Helper()
	_, err := store.Do(context.Background(), env.Project.DB.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return struct{}{}, fmt.Errorf("%s: %w", s, err)
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// output runs a call as a turn does: through Output, which stores a long
// result with its preview.
func output(t *testing.T, env *Env, name, args string) chat.ToolResult {
	t.Helper()
	r, err := call(t, env, name, args)
	out, err := Output(context.Background(), Call{ID: "c1", Args: json.RawMessage(args), Env: env}, name, 1, r, err)
	if err != nil {
		t.Fatalf("%s(%s): %v", name, args, err)
	}
	return out
}

func stored(t *testing.T, env *Env, key string) string {
	t.Helper()
	_, rc, err := env.Project.DB.OpenObject(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lacks(t *testing.T, got string, not ...string) {
	t.Helper()
	for _, n := range not {
		if strings.Contains(got, n) {
			t.Errorf("unexpected %q in:\n%s", n, got)
		}
	}
}

func TestQuerySmall(t *testing.T) {
	env := dataEnv(t)
	out := output(t, env, "query", `{"sql": "SELECT coin, price, volume, note FROM prices WHERE date = :d ORDER BY coin", "params": {"d": "2026-10-16"}}`)
	if out.IsError || out.Ref != "" {
		t.Fatalf("%+v", out)
	}
	// The values: a Persian note keeps its ZWNJ; a | and a line break are
	// escaped, so each row stays on its line.
	want := "coin | price | volume | note\nBTC | 60900 | 120 | نیم‌روز\nETH | 2410.5 | 300 | a\\|b\\nc\n(2 rows)"
	if out.Text != want {
		t.Errorf("got\n%s\nwant\n%s", out.Text, want)
	}
	// Positional parameters by number, a NULL, a blob, a date column and
	// a row count of 1.
	got := mustText(t, env, "query", `{"sql": "SELECT volume, ?1 AS n, x'0102' AS b FROM prices WHERE coin = ?1 AND date = ?", "params": {"1": "BTC", "2": "2026-10-17"}}`)
	if got != "volume | n | b\nNULL | BTC | [blob, 2 bytes]\n(1 row)" {
		t.Errorf("got\n%s", got)
	}
	contains(t, mustText(t, env, "query", `{"sql": "SELECT listed FROM coins ORDER BY id"}`), "2009-01-03\n2015-07-30\n(2 rows)")
	contains(t, mustText(t, env, "query", `{"sql": "SELECT 0.1 + 0.2 AS x, 1e20 AS y, @t AS t", "params": {"@t": true}}`), "0.30000000000000004 | 1e+20 | 1")
	contains(t, mustText(t, env, "query", `{"sql": "SELECT * FROM prices WHERE 0"}`), "date | coin | price | volume | note | chart\n(0 rows)")
}

// TestQueryRowLimit: a result of more than 200 rows is stored whole, and
// the context gets its first 200 rows (SPEC 8.1, 3.7).
func TestQueryRowLimit(t *testing.T) {
	env := dataEnv(t)
	out := output(t, env, "query", `{"sql": "SELECT id FROM big ORDER BY id"}`)
	if out.IsError || !strings.HasPrefix(out.Ref, "cache/tool/") {
		t.Fatalf("%+v", out)
	}
	contains(t, out.Text, "[query result — 250 rows — ", "showing the first 200 rows — ref: "+out.Ref+"]", "\n200\n",
		"Narrow the query with WHERE, GROUP BY or LIMIT, or use read_ref(ref, offset)")
	lacks(t, out.Text, "\n201\n", "the ref holds")
	full := stored(t, env, out.Ref)
	if !strings.HasPrefix(full, "id\n1\n2\n") || !strings.HasSuffix(full, "\n250\n(250 rows)") {
		t.Errorf("stored: %q…%q", full[:20], full[len(full)-20:])
	}
	// The footer's offset is where the preview stops in the stored text.
	next := strings.Index(full, "\n201\n") + 1
	contains(t, out.Text, fmt.Sprintf("the next offset is %d]", next))
	contains(t, mustText(t, env, "read_ref", fmt.Sprintf(`{"ref": %q, "offset": %d}`, out.Ref, next)), "\n201\n202\n")
	// At a cut the preview becomes a stub with the row count.
	if s := Stub(out); s != "[query result — 250 rows — ref: "+out.Ref+"]" {
		t.Errorf("stub %q", s)
	}
	// 200 rows exactly fit.
	out = output(t, env, "query", `{"sql": "SELECT id FROM big WHERE id <= 200"}`)
	if out.Ref != "" || !strings.HasSuffix(out.Text, "\n200\n(200 rows)") {
		t.Errorf("200 rows: %q", out.Text[len(out.Text)-30:])
	}
}

// TestQueryTokenLimit: rows over 4,000 tokens are stored, and the preview
// has the whole rows that fit.
func TestQueryTokenLimit(t *testing.T) {
	env := dataEnv(t)
	out := output(t, env, "query", `{"sql": "SELECT body FROM big WHERE id <= 100 ORDER BY id"}`)
	if out.Ref == "" {
		t.Fatalf("not stored: %s", out.Text)
	}
	// Each row is 201 bytes, and the column line 5: 79 rows fit in 16,000.
	contains(t, out.Text, "[query result — 100 rows — ", "showing the first 79 rows", fmt.Sprintf("%0200d\n[", 79))
	lacks(t, out.Text, fmt.Sprintf("%0200d", 80))
	body, _, _ := strings.Cut(out.Text[strings.Index(out.Text, "]\n")+2:], "\n[")
	if n := tokens(body); n > QueryTokens {
		t.Errorf("the preview's rows are %d tokens", n)
	}
	// Under the limit but over tool_preview_tokens, the rows stay whole,
	// and are also stored under a header with the ref, so a cut makes
	// them a stub instead of keeping them in every later request.
	out = output(t, env, "query", `{"sql": "SELECT body FROM big WHERE id <= 70"}`)
	if !strings.HasPrefix(out.Ref, "cache/tool/") || tokens(out.Text) <= env.PreviewTokens {
		t.Fatalf("70 rows: ref %q, %d tokens", out.Ref, tokens(out.Text))
	}
	head := "[query result — 70 rows — " + count(tokens(stored(t, env, out.Ref))) + " tokens — ref: " + out.Ref + "]\nbody\n"
	if !strings.HasPrefix(out.Text, head) || !strings.HasSuffix(out.Text, fmt.Sprintf("\n%0200d", 70)) {
		t.Errorf("70 rows: %.120q … %q", out.Text, out.Text[len(out.Text)-40:])
	}
	if full := stored(t, env, out.Ref); !strings.HasSuffix(full, fmt.Sprintf("%0200d\n(70 rows)", 70)) || strings.Contains(full, "[query result") {
		t.Errorf("stored ends %q", full[len(full)-30:])
	}
	if s := Stub(out); s != "[query result — 70 rows — ref: "+out.Ref+"]" {
		t.Errorf("stub %q", s)
	}
	// Within tool_preview_tokens, no ref.
	out = output(t, env, "query", `{"sql": "SELECT body FROM big WHERE id <= 20"}`)
	if out.Ref != "" || !strings.HasSuffix(out.Text, "(20 rows)") {
		t.Errorf("20 rows: ref %q", out.Ref)
	}
	// One row over the limit: the preview has its start.
	out = output(t, env, "query", `{"sql": "SELECT 'x' AS a, printf('%.20000c', 'y') AS b"}`)
	contains(t, out.Text, "[query result — 1 row — ", "showing the start of the first row", "a | b\nx | yyy")
	if n := tokens(out.Text); n > QueryTokens+100 {
		t.Errorf("preview of %d tokens", n)
	}
}

// TestQueryStoredCap: the ref holds at most 10,000 rows, and a count over
// 100,000 says "more than".
func TestQueryStoredCap(t *testing.T) {
	env := dataEnv(t)
	out := output(t, env, "query", `{"sql": "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 12000) SELECT i FROM n"}`)
	contains(t, out.Text, "[query result — 12,000 rows — ", "[the ref holds the first 10,000 rows. Narrow")
	if full := stored(t, env, out.Ref); !strings.HasSuffix(full, "\n10000\n(the first 10,000 of 12,000 rows)") {
		t.Errorf("stored ends %q", full[len(full)-50:])
	}
	out = output(t, env, "query", `{"sql": "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 150000) SELECT i FROM n"}`)
	contains(t, out.Text, "[query result — more than 100,000 rows — ")
}

// TestQueryRefusals: each layer's refusal comes back as an error the
// model can act on, and internal names get the list of tables.
func TestQueryRefusals(t *testing.T) {
	env := dataEnv(t)
	hint := "the project's tables are: big, coins, holdings, loose, prices. Use describe_table"
	for _, c := range []struct{ sql, want string }{
		{"SELECT * FROM _jenab_columns", "line 1:15: `_jenab_columns` is internal. " + "Jenab's own tables"},
		{"SELECT name FROM sqlite_schema", "`sqlite_schema` is reserved"},
		{"SELECT * FROM pragma_table_info('prices')", "queries can't use PRAGMA functions"},
		{"SELECT 1; DELETE FROM prices", "only one statement is allowed"},
		{"DELETE FROM prices", "a query must start with SELECT, WITH or VALUES, not `DELETE`"},
		{"PRAGMA query_only = 0", "a query must start with SELECT, WITH or VALUES"},
		{"SELECT load_extension('x')", "`load_extension` is not allowed"},
		{"SELECT * FROM dbstat", "json_each and json_tree are the only virtual tables"},
		{"SELECT 1 # 2", "is not allowed here"},
		{"SELECT * FROM notes", "the query was refused: "}, // layer 2: a virtual table
		{"SELECT 1 FROM (" + strings.Repeat("SELECT 1 UNION ALL ", 501) + "SELECT 1)", "more than 500 SELECT"},
		{"SELECT '" + strings.Repeat("x", 70000) + "'", "at most 65536 are allowed"},
	} {
		msg := toolError(t, env, "query", fmt.Sprintf(`{"sql": %q}`, c.sql))
		contains(t, msg, "the query was refused: ", c.want)
		if strings.Contains(c.want, "internal") || strings.Contains(c.want, "reserved") || strings.Contains(c.want, "PRAGMA") {
			contains(t, msg, hint)
		}
	}
	// SQLite's own errors, without the driver's code.
	msg := toolError(t, env, "query", `{"sql": "SELECT prise FROM prices"}`)
	if msg != "the query failed: no such column: prise" {
		t.Errorf("got %q", msg)
	}
	contains(t, toolError(t, env, "query", `{"sql": "SELECT * FROM price"}`), "no such table: price; the project's tables are: big, coins")
	contains(t, toolError(t, env, "query", `{}`), "wrong arguments")
}

// TestQueryParams: params must match the query's own parameters, so no
// value binds to the wrong place or leaves one NULL.
func TestQueryParams(t *testing.T) {
	env := dataEnv(t)
	for _, c := range []struct{ sql, params, want string }{
		// Numbers, names with or without their prefix, and both kinds.
		{"SELECT ?1 AS a, ? AS b, ?1 AS c", `{"1": 5, "2": "x"}`, "a | b | c\n5 | x | 5\n(1 row)"},
		{"SELECT ? AS a, ? AS b", `{"?2": "y", "?1": "x"}`, "a | b\nx | y\n(1 row)"},
		{"SELECT :coin AS a, @coin AS b, $n AS c", `{":coin": "BTC", "$n": 2}`, "a | b | c\nBTC | BTC | 2\n(1 row)"},
		{"SELECT ?1 AS a, :coin AS b, ?3 AS c", `{"1": 1, "coin": "BTC", "3": 3}`, "a | b | c\n1 | BTC | 3\n(1 row)"},
		{"SELECT :بها AS a", `{"بها": 1}`, "a\n1\n(1 row)"},
		// ?01 is ?1, and a bare ? after :c is ?2.
		{"SELECT ?01 AS a, ?002 AS b, ?1 AS c", `{"1": 5, "2": 6}`, "a | b | c\n5 | 6 | 5\n(1 row)"},
		{"SELECT :c AS a, ? AS b", `{"c": 1, "2": 2}`, "a | b\n1 | 2\n(1 row)"},
		{"SELECT 1 AS a", `{}`, "a\n1\n(1 row)"},
	} {
		got := mustText(t, env, "query", fmt.Sprintf(`{"sql": %q, "params": %s}`, c.sql, c.params))
		if got != c.want {
			t.Errorf("%s %s:\n got %q\nwant %q", c.sql, c.params, got, c.want)
		}
	}
	for _, c := range []struct{ sql, params, want string }{
		// A value that would bind to another parameter, or none.
		{"SELECT ?1, ?2", `{"1": 5, "coin": "BTC"}`, "params has no value for ?2"},
		{"SELECT ?1, ?2", `{"2": 5}`, "params has no value for ?1"},
		{"SELECT ? AS x, ? AS y", `{"1": 1}`, "params has no value for ?2; give each of the query's parameters a value"},
		{"SELECT :coin", `{}`, "params has no value for :coin"},
		{"SELECT :coin", `{"coin": 1, "date": 2}`, "params.date: the query has no parameter date; its parameters are :coin"},
		{"SELECT 1", `{"1": 1}`, "params.1: the query has no parameter 1; it has none"},
		{"SELECT ?1", `{"1": 1, "?3": 3}`, "params.?3: the query has no parameter ?3; its parameters are ?1"},
		// Two keys for one parameter.
		{"SELECT :coin", `{"coin": 1, ":coin": 2}`, `params: ":coin" and "coin" are the same parameter; give it once`},
		{"SELECT ?1", `{"1": 1, "01": 2}`, `params: "01" and "1" are the same parameter`},
		{"SELECT ?1", `{"0": 1}`, `params: "0" is not a parameter's name or number`},
		// Values it can't bind.
		{"SELECT :a", `{"a": [1, 2]}`, "params.a: a list or an object can't be a parameter"},
		{"SELECT :a", `{"a": 1e999}`, "params.a: 1e999 is not a number"},
		// A name the driver could never bind.
		{"SELECT :_c", `{"_c": 1}`, "parameter names start with a letter, like :name"},
	} {
		contains(t, toolError(t, env, "query", fmt.Sprintf(`{"sql": %q, "params": %s}`, c.sql, c.params)), c.want)
	}
}

// TestQueryDates: a DATE or DATETIME column shows its text as stored, so a
// value copied from a result finds its row again.
func TestQueryDates(t *testing.T) {
	env := dataEnv(t)
	execAll(t, env, `CREATE TABLE events (id INTEGER PRIMARY KEY, at DATETIME, day DATE, ts TIMESTAMP)`,
		`INSERT INTO events (at, day, ts) VALUES ('2026-10-01 14:30:00', '2026-09-27 00:00:00', '2026-10-01T14:30:00.5+03:30'), ('2026-10-02', 'soon', 1760000000)`)
	got := mustText(t, env, "query", `{"sql": "SELECT at, day, ts FROM events ORDER BY id"}`)
	want := "at | day | ts\n2026-10-01 14:30:00 | 2026-09-27 00:00:00 | 2026-10-01T14:30:00.5+03:30\n2026-10-02 | soon | 1760000000\n(2 rows)"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	for _, q := range []string{
		`{"sql": "SELECT id FROM events WHERE at = :at", "params": {"at": "2026-10-01 14:30:00"}}`,
		`{"sql": "SELECT id FROM events WHERE day = '2026-09-27 00:00:00' AND ts = '2026-10-01T14:30:00.5+03:30'"}`,
		// Through a CTE and with the column twice.
		`{"sql": "WITH e AS (SELECT at, at FROM events) SELECT * FROM e WHERE at = '2026-10-01 14:30:00'"}`,
	} {
		contains(t, mustText(t, env, "query", q), "\n(1 row)")
	}
	contains(t, mustText(t, env, "query", `{"sql": "WITH e AS (SELECT at, at FROM events) SELECT * FROM e WHERE at = '2026-10-01 14:30:00'"}`),
		"at | at:1\n2026-10-01 14:30:00 | 2026-10-01 14:30:00\n")
}

// TestQueryErrorKinds: a query over its time or memory is an error the
// model sees; a broken worker is a system failure.
func TestQueryErrorKinds(t *testing.T) {
	env := dataEnv(t)
	ctx := context.Background()
	var te *Error
	if err := queryError(ctx, env, &sqlworker.MemoryError{Limit: 256 << 20}); !errors.As(err, &te) ||
		!strings.Contains(te.Msg, "the query used more than 256 MiB and was stopped; make the query smaller") {
		t.Errorf("memory: %v", err)
	}
	slow := fmt.Errorf("the query ran longer than 10s and was stopped: %w", context.DeadlineExceeded)
	if err := queryError(ctx, env, slow); !errors.As(err, &te) || !strings.Contains(te.Msg, "longer than 10s and was stopped; make the query cheaper") {
		t.Errorf("deadline: %v", err)
	}
	if err := queryError(ctx, env, store.ErrClosed); !errors.As(err, &te) {
		t.Errorf("closed: %v", err)
	}
	broken := errors.New("sqlworker: the worker stopped: exit status 2")
	if err := queryError(ctx, env, broken); errors.As(err, &te) {
		t.Errorf("a broken worker is the model's error: %v", err)
	}
}

func TestDescribeTable(t *testing.T) {
	env := dataEnv(t)
	got := mustText(t, env, "describe_table", `{"name": "Prices"}`)
	want := `Table prices: 3 rows
Columns:
- date TEXT NOT NULL — primary key
- coin TEXT NOT NULL — primary key
- price REAL NOT NULL
- volume INTEGER
- note TEXT DEFAULT ''
- chart TEXT — object_ref: holds bucket keys
Primary key: (date, coin)`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	contains(t, mustText(t, env, "describe_table", `{"name": "coins"}`), "Table coins: 2 rows",
		"- id INTEGER — primary key, picked by SQLite when left out", "- listed DATE", "- active BOOLEAN NOT NULL DEFAULT 1",
		"- label TEXT — generated: SQLite computes it", "Unique index sqlite_autoindex_coins_1: (symbol)")
	contains(t, mustText(t, env, "describe_table", `{"name": "holdings"}`), "Table holdings: 0 rows",
		"Index holdings_coin: (coin_id)", "Foreign key: (coin_id) references coins (id), on delete cascade")
	contains(t, mustText(t, env, "describe_table", `{"name": "loose"}`), "Primary key: none, so insert can't write to it")

	contains(t, toolError(t, env, "describe_table", `{"name": "price"}`), `there is no table "price"; the project's tables are: big, coins, holdings, loose, prices.`)
	contains(t, toolError(t, env, "describe_table", `{"name": "_jenab_columns"}`), "_jenab_columns is internal to Jenab")
	contains(t, toolError(t, env, "describe_table", `{"name": "sqlite_schema"}`), "sqlite_schema is internal to Jenab")
	contains(t, toolError(t, env, "describe_table", `{"name": "notes"}`), `there is no table "notes"`)
	contains(t, toolError(t, testEnv(t), "describe_table", `{"name": "x"}`), "the project has no tables yet.")
}

func TestInsert(t *testing.T) {
	env := dataEnv(t)
	ctx := context.Background()
	got := mustText(t, env, "insert", `{"table": "prices", "rows": [
		{"date": "2026-10-18", "coin": "BTC", "price": 61500, "volume": "42", "note": "یادداشت"},
		{"Date": "2026-10-18", "coin": "ETH", "price": "2420.75"}]}`)
	if got != "Inserted 2 rows into prices." {
		t.Errorf("got %q", got)
	}
	contains(t, mustText(t, env, "query", `{"sql": "SELECT coin, price, typeof(volume), volume, note FROM prices WHERE date = '2026-10-18' ORDER BY coin"}`),
		"BTC | 61500 | integer | 42 | یادداشت\nETH | 2420.75 | null | NULL | \n(2 rows)")
	// The change log has the write, from this message, so Undo turn
	// reverts it.
	src, err := store.Query(ctx, env.Project.DB.DB, `SELECT source || ' ' || target || ' ' || op FROM _jenab_changes`, nil,
		func(r *sql.Rows) (string, error) {
			var s string
			return s, r.Scan(&s)
		})
	if err != nil || len(src) != 1 || src[0] != string(env.Source)+" table:prices insert" {
		t.Errorf("changes %v, %v", src, err)
	}
	// A rowid key is picked; a boolean becomes 1 or 0; a date is text.
	contains(t, mustText(t, env, "insert", `{"table": "coins", "rows": [{"symbol": "SOL", "listed": "2020-03-16", "active": false}]}`), "Inserted 1 row into coins.")
	contains(t, mustText(t, env, "query", `{"sql": "SELECT id, active, label FROM coins WHERE symbol = 'SOL'"}`), "3 | 0 | NULL")
}

// TestInsertLimit: at most 50 rows a call.
func TestInsertLimit(t *testing.T) {
	env := dataEnv(t)
	rows := func(n, from int) string {
		var rs []string
		for i := range n {
			rs = append(rs, fmt.Sprintf(`{"id": %d, "body": "r"}`, from+i))
		}
		return `{"table": "big", "rows": [` + strings.Join(rs, ",") + `]}`
	}
	contains(t, mustText(t, env, "insert", rows(50, 1000)), "Inserted 50 rows into big.")
	msg := toolError(t, env, "insert", rows(51, 2000))
	contains(t, msg, "insert takes at most 50 rows a call, not 51; nothing was written")
	contains(t, mustText(t, env, "query", `{"sql": "SELECT count(*) FROM big"}`), "300\n(1 row)")
	contains(t, toolError(t, env, "insert", `{"table": "big", "rows": []}`), "wrong arguments")
}

// TestInsertCheck: every error of the type check comes back at once, with
// its row and column, and nothing is written.
func TestInsertCheck(t *testing.T) {
	env := dataEnv(t)
	msg := toolError(t, env, "insert", `{"table": "prices", "rows": [
		{"date": "2026-10-18", "coin": "BTC", "price": "abc", "volume": 2.5},
		{"date": "2026-10-18", "price": 5, "volume": "12x", "chart": {"a": 1}},
		{"date": "2026-10-18", "coin": "ETH", "price": null, "notes": "x"},
		{},
		{"date": "2026-10-18", "coin": "SOL", "price": 1, "note": true}]}`)
	want := `nothing was written; 9 errors:
- rows[0].price: the column takes a number, not "abc"
- rows[0].volume: the column takes a whole number, not 2.5
- rows[1].chart: a list or an object can't be stored in a column; give JSON as a string
- rows[1].volume: the column takes a whole number, not "12x"
- rows[1].coin: the column is part of the primary key, so it needs a value
- rows[2].notes: prices has no column "notes"
- rows[2].price: the column is NOT NULL, so it can't be null
- rows[3]: the row has no columns
- rows[4].note: the column is TEXT, so it takes text, not true`
	if msg != want {
		t.Errorf("got\n%s\nwant\n%s", msg, want)
	}
	msg = toolError(t, env, "insert", `{"table": "coins", "rows": [
		{"symbol": "X", "listed": "yesterday", "active": "yes", "label": "x"},
		{"name": "No symbol", "listed": "2026-10-18 09:30:00"},
		{"symbol": "Y", "listed": 20261018, "active": 1}]}`)
	contains(t, msg, "nothing was written; 5 errors:",
		`- rows[0].active: the column is BOOLEAN, so it takes true, false, 1 or 0, not "yes"`,
		"- rows[0].label: the column is generated: SQLite computes its value",
		`- rows[0].listed: the column is DATE, so it takes a date or time as text, like 2026-10-18 or 2026-10-18 14:30:00, not "yesterday"`,
		"- rows[1].symbol: the column is NOT NULL and has no default, so it needs a value",
		"- rows[2].listed: the column is DATE, so it takes a date or time as text")
	contains(t, mustText(t, env, "query", `{"sql": "SELECT count(*) FROM prices"}`), "3\n(1 row)")
	contains(t, mustText(t, env, "query", `{"sql": "SELECT count(*) FROM coins"}`), "2\n(1 row)")

	// Many bad rows list the first 50 errors.
	var rs []string
	for range 50 {
		rs = append(rs, `{"id": "a", "body": "b", "x": 1}`)
	}
	msg = toolError(t, env, "insert", `{"table": "big", "rows": [`+strings.Join(rs, ",")+`]}`)
	contains(t, msg, "nothing was written; 100 errors:", "\n- and 50 more")
}

// TestInsertWriteErrors: what only the write checks fails the call, with
// the row, and nothing is written.
func TestInsertWriteErrors(t *testing.T) {
	env := dataEnv(t)
	for _, c := range []struct{ args, want string }{
		{`{"table": "prices", "rows": [{"date": "2026-10-19", "coin": "BTC", "price": 1}, {"date": "2026-10-16", "coin": "BTC", "price": 1}]}`,
			"nothing was written: rows[1]: a row with that key already exists"},
		{`{"table": "prices", "rows": [{"date": "2026-10-19", "coin": "BTC", "price": -1}]}`,
			"nothing was written: rows[0]: CHECK constraint failed: price > 0"},
		{`{"table": "coins", "rows": [{"symbol": "DOGE"}, {"symbol": "BTC"}]}`,
			"nothing was written: rows[1]: a unique value is taken: the values of coins.symbol must be unique"},
		{`{"table": "holdings", "rows": [{"coin_id": 99, "amount": 1}]}`,
			"nothing was written: rows[0]: FOREIGN KEY constraint failed"},
		{`{"table": "prices", "rows": [{"date": "2026-10-19", "coin": "BTC", "price": 1, "chart": "charts/none.png"}]}`,
			"nothing was written: table prices: no object at charts/none.png"},
		{`{"table": "_jenab_meta", "rows": [{"key": "a", "value": "b"}]}`, "_jenab_meta is internal to Jenab"},
		{`{"table": "nope", "rows": [{"a": 1}]}`, `there is no table "nope"; the project's tables are: big, coins`},
		{`{"table": "notes", "rows": [{"body": "x"}]}`, `there is no table "notes"`},
		{`{"table": "loose", "rows": [{"a": "x"}]}`, "loose has no primary key, so insert can't write to it"},
	} {
		contains(t, toolError(t, env, "insert", c.args), c.want)
	}
	contains(t, mustText(t, env, "query", `{"sql": "SELECT (SELECT count(*) FROM prices) || ' ' || (SELECT count(*) FROM coins) || ' ' || (SELECT count(*) FROM holdings) AS n"}`),
		"3 2 0\n(1 row)")
}

// TestInsertPartialWrite: a write of more than 500 rows that fails in a
// later chunk keeps the chunks before it (P2-02). insert takes 50 rows,
// so it can't meet this itself; writeError reports it for any caller of
// Insert.
func TestInsertPartialWrite(t *testing.T) {
	env := dataEnv(t)
	rows := make([]store.Row, 600)
	for i := range rows {
		rows[i] = store.Row{"date": fmt.Sprintf("2027-%04d", i), "coin": "BTC", "price": 1.0}
	}
	rows[550]["price"] = -1.0 // the CHECK fails in the second chunk
	res, err := env.Project.DB.Insert(context.Background(), limit.Interactive, env.Source, "prices", rows, store.ConflictFail)
	if err == nil {
		t.Fatal("no error")
	}
	var te *Error
	if err := writeError("prices", len(rows), res, err); !errors.As(err, &te) ||
		te.Msg != "only part of the rows were written: 500 rows of 600 went into prices before rows[550]: CHECK constraint failed: price > 0. "+
			"The written rows stay, and Undo turn reverts them; the other rows were not written." {
		t.Errorf("got %v", err)
	}
	contains(t, mustText(t, env, "query", `{"sql": "SELECT count(*) FROM prices WHERE date LIKE '2027-%'"}`), "500\n(1 row)")

	// A system error with nothing written stays a system error.
	if err := writeError("prices", 1, store.WriteResult{}, errors.New("disk I/O error")); errors.As(err, &te) {
		t.Errorf("system error as the model's: %v", err)
	}
}

// TestSQLSkill runs the sql-queries skill's examples and the forms it
// names through query, so the skill only teaches what the guard lets
// through.
func TestSQLSkill(t *testing.T) {
	env := dataEnv(t)
	b, err := os.ReadFile("../skill/builtin/sql-queries/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	var qs []string
	// A checkout with core.autocrlf has CRLF line ends.
	for _, block := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "```sql\n")[1:] {
		block, _, _ = strings.Cut(block, "```")
		for _, q := range strings.Split(block, ";") {
			if q = strings.TrimSpace(q); q != "" {
				qs = append(qs, q)
			}
		}
	}
	if len(qs) != 3 {
		t.Fatalf("%d examples", len(qs))
	}
	qs = append(qs,
		"SELECT date('now', '-7 days'), date('2026-10-18', 'start of month'), datetime('now', 'localtime'), julianday('2026-10-18') - julianday('2026-10-01')",
		"SELECT price - lag(price) OVER (ORDER BY date), sum(price) OVER (ORDER BY date), avg(price) OVER (ORDER BY date ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) FROM prices",
		`SELECT '{"a": 1}' ->> '$.a', json_extract('{"a": 2}', '$.a'), iif(1, 'y', 'n'), coalesce(NULL, 0), nullif(1, 1), round(2.345, 2), 5 / 2.0, CAST(5 AS REAL)`,
		"SELECT value FROM json_each('[1, 2]')",
		"SELECT * FROM prices WHERE note GLOB '*|*' OR coin LIKE 'b%' LIMIT 1 OFFSET 0",
	)
	for _, q := range qs {
		ps := map[string]any{}
		if strings.Contains(q, ":coin") {
			ps["coin"] = "BTC"
		}
		args, _ := json.Marshal(map[string]any{"sql": q, "params": ps})
		if r, err := call(t, env, "query", string(args)); err != nil {
			t.Errorf("%s: %v", q, err)
		} else if strings.HasSuffix(r.Text, "(0 rows)") && !strings.Contains(q, "30 days") {
			t.Errorf("%s: no rows", q)
		}
	}
}
