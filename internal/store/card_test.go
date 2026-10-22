package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

var cardTime = time.Date(2026, 10, 21, 14, 30, 0, 0, time.FixedZone("", 2*3600))

func migrateSteps(t *testing.T, p *ProjectDB, steps ...string) {
	t.Helper()
	raw := make([]json.RawMessage, len(steps))
	for i, s := range steps {
		raw[i] = json.RawMessage(s)
	}
	if _, err := p.Migrate(context.Background(), id.SourceUser, Migration{Steps: raw}, MigrateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func card(t *testing.T, p *ProjectDB) string {
	t.Helper()
	c, err := p.Card(context.Background(), cardTime)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(c) / 4; n >= CardTokens {
		t.Errorf("the card is about %d tokens", n)
	}
	return c
}

func TestCardEmpty(t *testing.T) {
	p, _ := openTestProject(t)
	want := "Project card, as of 2026-10-21 14:30 (UTC+02:00). Row counts and date ranges are from then; query for numbers now.\n" +
		"Tables: none yet.\nBucket: empty."
	if got := card(t, p); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// The Bitcoin project's tables (SPEC 9.1), an object reference and the
// bucket.
func TestCardBitcoin(t *testing.T) {
	p, _ := openTestProject(t)
	if err := p.SetMeta(context.Background(), map[string]string{"name": "Bitcoin"}); err != nil {
		t.Fatal(err)
	}
	migrateSteps(t, p,
		`{"op": "create_table", "table": "btc_prices", "columns": [{"name": "date", "type": "TEXT"}, {"name": "price", "type": "REAL"}, {"name": "_run_id", "type": "TEXT"}, {"name": "_fetched_at", "type": "TEXT"}], "primary_key": ["date"]}`,
		`{"op": "create_table", "table": "btc_news", "columns": [{"name": "url", "type": "TEXT"}, {"name": "date", "type": "TEXT"}, {"name": "title", "type": "TEXT"}, {"name": "reason", "type": "TEXT"}, {"name": "chart", "type": "TEXT"}, {"name": "_run_id", "type": "TEXT"}, {"name": "_fetched_at", "type": "TEXT"}], "primary_key": ["url"]}`,
		`{"op": "annotate_column", "table": "btc_news", "column": "chart", "kind": "object_ref"}`,
		`{"op": "create_table", "table": "watch_keywords", "columns": [{"name": "id", "type": "INTEGER"}, {"name": "keyword", "type": "TEXT", "not_null": true}], "primary_key": ["id"], "unique": [["keyword"]]}`,
		`{"op": "insert_rows", "table": "btc_prices", "rows": [
			{"date": "2026-09-01", "price": 57000, "_run_id": "r1", "_fetched_at": "2026-09-01T08:00:00Z"},
			{"date": "2026-09-02", "price": 57500, "_run_id": "r2", "_fetched_at": "2026-09-02T08:00:00Z"},
			{"date": "2026-09-30", "price": 61000, "_run_id": "r3", "_fetched_at": "2026-09-30T08:00:00Z"}]}`,
		`{"op": "insert_rows", "table": "btc_news", "rows": [{"url": "https://example.test/a", "date": "2026-09-02", "title": "A", "reason": "ETF", "_run_id": "r2", "_fetched_at": "2026-09-02T08:00:00Z"}]}`,
		`{"op": "insert_rows", "table": "watch_keywords", "rows": [{"id": 1, "keyword": "ETF"}, {"id": 2, "keyword": "halving"}]}`,
	)
	for _, k := range []string{"images/btc/2026-09-01.png", "images/btc/2026-09-02.png", "reports/weekly.pdf", "notes.txt"} {
		if _, err := p.PutObject(context.Background(), limit.Interactive, id.SourceUser, k, strings.NewReader(k), PutOptions{Source: "upload"}); err != nil {
			t.Fatal(err)
		}
	}
	want := `Project card of "Bitcoin", as of 2026-10-21 14:30 (UTC+02:00). Row counts and date ranges are from then; query for numbers now.
Tables:
- btc_news (1 row): url TEXT NOT NULL, date TEXT, title TEXT, reason TEXT, chart TEXT object_ref, _run_id TEXT, _fetched_at TEXT; key (url); date 2026-09-02; _fetched_at 2026-09-02T08:00:00Z
- btc_prices (3 rows): date TEXT NOT NULL, price REAL, _run_id TEXT, _fetched_at TEXT; key (date); date 2026-09-01 to 2026-09-30; _fetched_at 2026-09-01T08:00:00Z to 2026-09-30T08:00:00Z
- watch_keywords (2 rows): id INTEGER, keyword TEXT NOT NULL; key (id); unique (keyword)
Bucket: images/ 2 objects, reports/ 1 object, 1 object at the top.`
	if got := card(t, p); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// A project whose card would pass the size limit lists table names only.
func TestCardOverCap(t *testing.T) {
	p, _ := openTestProject(t)
	var steps []string
	cols := make([]string, 20)
	for i := range cols {
		cols[i] = fmt.Sprintf(`{"name": "measure_%02d", "type": "REAL"}`, i)
	}
	for i := range 30 {
		steps = append(steps, fmt.Sprintf(`{"op": "create_table", "table": "sensor_%02d", "columns": [{"name": "id", "type": "INTEGER"}, %s], "primary_key": ["id"]}`,
			i, strings.Join(cols, ", ")))
	}
	migrateSteps(t, p, steps...)
	got := card(t, p)
	if strings.Contains(got, "measure_") || !strings.Contains(got, "Tables (30; ") || !strings.Contains(got, "describe_table") ||
		!strings.Contains(got, ": sensor_00, sensor_01, ") || !strings.HasSuffix(got, "sensor_29.\nBucket: empty.") {
		t.Errorf("got\n%s", got)
	}
}

// Even the names may not fit: the rest are counted.
func TestCardNamesCapped(t *testing.T) {
	names := make([]string, 2000)
	for i := range names {
		names[i] = fmt.Sprintf("table_with_a_long_name_%04d", i)
	}
	got := cardNames(names, CardTokens*4)
	if len(got) > CardTokens*4 || !strings.Contains(got, "table_with_a_long_name_0000, ") || !strings.HasSuffix(got, " more.") {
		t.Errorf("%d bytes: …%s", len(got), got[max(0, len(got)-80):])
	}
}

// With no room for even one name, the short form says so.
func TestCardNamesNoRoom(t *testing.T) {
	got := cardNames([]string{"prices", "news"}, 0)
	if !strings.HasSuffix(got, "rows): no room to list them.") || strings.Contains(got, ", and") {
		t.Errorf("%q", got)
	}
}

// Long bucket prefixes are counted, not named, past the bucket line's
// budget, so even the short form stays under the limit.
func TestCardLongPrefixes(t *testing.T) {
	p, _ := openTestProject(t)
	for i := range 20 {
		k := fmt.Sprintf("%s%02d/x.txt", strings.Repeat("p", 490), i)
		if _, err := p.PutObject(context.Background(), limit.Interactive, id.SourceUser, k, strings.NewReader(k), PutOptions{Source: "upload"}); err != nil {
			t.Fatal(err)
		}
	}
	var steps []string
	cols := `, {"name": "a_rather_long_column_name", "type": "REAL"}`
	for i := range 150 {
		steps = append(steps, fmt.Sprintf(`{"op": "create_table", "table": "table_with_a_long_name_%03d", "columns": [{"name": "id", "type": "INTEGER"}%s], "primary_key": ["id"]}`, i, cols))
	}
	migrateSteps(t, p, steps...)
	got := card(t, p) // checks the limit
	bucket := got[strings.LastIndex(got, "\nBucket: ")+1:]
	if len(bucket) > cardBucketBytes || !strings.Contains(bucket, "and 18 more prefixes.") ||
		!strings.Contains(got, "\nTables (150; ") || !strings.Contains(got, "table_with_a_long_name_000, ") {
		t.Errorf("%d bytes of bucket line:\n%s", len(bucket), got)
	}
}

// A date column whose first row is NULL is still a date column.
func TestCardDateFirstRowNull(t *testing.T) {
	p, _ := openTestProject(t)
	migrateSteps(t, p,
		`{"op": "create_table", "table": "news", "columns": [{"name": "url", "type": "TEXT"}, {"name": "date", "type": "TEXT"}, {"name": "title", "type": "TEXT"}], "primary_key": ["url"]}`,
		`{"op": "insert_rows", "table": "news", "rows": [{"url": "a", "title": "No date"}, {"url": "b", "date": "2026-09-03", "title": "B"}, {"url": "c", "date": "2026-09-05", "title": "C"}]}`,
	)
	if got := card(t, p); !strings.Contains(got, "- news (3 rows): url TEXT NOT NULL, date TEXT, title TEXT; key (url); date 2026-09-03 to 2026-09-05\n") {
		t.Errorf("got\n%s", got)
	}
}

// A table with more date columns than one result can hold is read in
// parts. Migrations make tables of at most maxColumns columns, but a
// table made outside the app can have up to 2,000.
func TestCardWideTable(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	cols := make([]string, 1200)
	for i := range cols {
		cols[i] = fmt.Sprintf("c%04d TEXT", i)
	}
	mustExec(t, p.DB, "CREATE TABLE wide ("+strings.Join(cols, ", ")+")")
	mustExec(t, p.DB, "INSERT INTO wide (c0000, c1199) VALUES ('2026-09-01', '2026-09-30')")
	ranges, err := readTx(ctx, p.DB, func(tx *sql.Tx) ([]dateRange, error) {
		tb, err := describe(ctx, tx, "wide", true)
		if err != nil {
			return nil, err
		}
		return dateRanges(ctx, tx, &tb)
	})
	want := []dateRange{{"c0000", "2026-09-01", "2026-09-01"}, {"c1199", "2026-09-30", "2026-09-30"}}
	if err != nil || !slices.Equal(ranges, want) {
		t.Errorf("%v, %v", ranges, err)
	}
	if _, err := p.Card(ctx, cardTime); err != nil {
		t.Error(err)
	}
}

// A Persian name keeps its ZWNJ; only quotes and line breaks are escaped.
func TestCardName(t *testing.T) {
	p, _ := openTestProject(t)
	name := "می‌خواهم \"بیت‌کوین\"\nدوم"
	if err := p.SetMeta(context.Background(), map[string]string{"name": name}); err != nil {
		t.Fatal(err)
	}
	want := `Project card of "می` + "‌" + `خواهم \"بیت` + "‌" + `کوین\"\nدوم", as of `
	if got := card(t, p); !strings.HasPrefix(got, want) {
		t.Errorf("got\n%q\nwant prefix\n%q", got, want)
	}
}

// A long name is cut at MaxTitle characters, not bytes.
func TestCardLongName(t *testing.T) {
	p, _ := openTestProject(t)
	if err := p.SetMeta(context.Background(), map[string]string{"name": strings.Repeat("بیت", 200)}); err != nil {
		t.Fatal(err)
	}
	want := `Project card of "` + strings.Repeat("بیت", MaxTitle/3) + "ب…" + `", as of `
	if got := card(t, p); !strings.HasPrefix(got, want) {
		t.Errorf("got\n%q\nwant prefix\n%q", got, want)
	}
}

func TestCountText(t *testing.T) {
	for n, want := range map[int]string{0: "0 rows", 1: "1 row", 999: "999 rows", 1000: "1,000 rows", 1234567: "1,234,567 rows"} {
		if got := countText(n, "row", "rows"); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
}
