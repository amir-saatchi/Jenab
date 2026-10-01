package main

// The four tasks (SPEC 9) and their hand-written assertions. Assertions check the final state
// of a task: schema facts, and behaviour through the mock dry run (no network), opened views
// and a simulated form submit.

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type Task struct {
	ID     string
	Prompt string
	Needs  []string // artifacts that must be saved: "migration" or config ids
	Check  func(p *Project) []Check
}

type Check struct {
	Name string
	OK   bool
	Why  string `json:",omitempty"`
}

var tasks = []Task{
	{ID: "T1", Needs: []string{"migration"}, Check: checkT1, Prompt: `New project: track the daily Bitcoin price and the news most likely to move it. Create the tables:
- btc_prices: one row per day: date (YYYY-MM-DD) and the price in USD. Filled daily by a pipeline.
- btc_news: news items: url (the key), date, title, and reason (why it matters). Filled daily by a pipeline.
- watch_keywords: keywords the user types to steer the news search: id (integer key) and keyword (required, unique).
Only the tables for now; the pipeline and the views come later.`},
	{ID: "T2", Needs: []string{"daily_btc"}, Check: checkT2, Prompt: `Create the pipeline daily_btc. Every day at 08:00 UTC (and also when run by hand) it:
1. fetches today's BTC price from https://api.example.com/btc/daily (a placeholder; the JSON response looks like {"date": "2026-09-28", "close": 65000.5, "currency": "USD"}) and saves it in btc_prices for today's date;
2. searches the web for Bitcoin news from the last day, including the watch keywords in the search;
3. lets the LLM pick the 5 items most likely to affect the price, with a short reason for each;
4. saves them in btc_news.
Running it twice on the same day must not fail or create duplicates.`},
	{ID: "T3", Needs: []string{"btc_price_table", "btc_price_chart", "btc_news_table", "keyword_list", "add_keyword"}, Check: checkT3, Prompt: `Create these views:
1. btc_price_table: a table of the BTC price, newest first, with a "From" date filter that defaults to 90 days ago, and a "Refresh now" button that runs daily_btc.
2. btc_price_chart: a line chart of the price over time.
3. btc_news_table: a table of the saved news (date, title, reason, link), newest first.
4. keyword_list: a table of the watch keywords where each row can be deleted.
5. add_keyword: a form to add a watch keyword.`},
	{ID: "T4", Needs: []string{"migration"}, Check: checkT4, Prompt: `Also track Ethereum. Move the prices into one general table prices(coin, date, price), keeping every BTC row collected so far (coin 'BTC'), give each news item a coin, and drop btc_prices once nothing needs it. Keep the pipeline id daily_btc, but make it fetch the price of every tracked coin (ETH comes from https://api.example.com/eth/daily, same JSON as BTC) and search news for all tracked coins, with the LLM also saying which coin each picked item is about. The price table and chart must show both coins (the chart one line per coin), and the news table must show the coin. Update the pipeline and the views in the same change as the migration.`},
}

func taskByID(id string) *Task {
	for i := range tasks {
		if tasks[i].ID == id {
			return &tasks[i]
		}
	}
	return nil
}

// ---- helpers ----

func ck(name string, ok bool, why string, a ...any) Check {
	c := Check{Name: name, OK: ok}
	if !ok {
		c.Why = fmt.Sprintf(why, a...)
	}
	return c
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	for i := range x {
		x[i] = strings.ToLower(x[i])
	}
	for i := range y {
		y[i] = strings.ToLower(y[i])
	}
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, ",") == strings.Join(y, ",")
}

func hasCols(t *Table, cols ...string) (bool, string) {
	var miss []string
	for _, c := range cols {
		if t.col(c) == nil {
			miss = append(miss, c)
		}
	}
	return len(miss) == 0, strings.Join(miss, ", ")
}

func numeric(t string) bool {
	t = strings.ToUpper(t)
	return t == "REAL" || t == "NUMERIC" || strings.Contains(t, "DOUBLE") || strings.Contains(t, "FLOAT") || t == "DECIMAL"
}

func pipeTable(t *Table) (bool, string) {
	return hasCols(t, "_run_id", "_fetched_at")
}

func count(q Q, sqlText string, args ...any) int {
	var n int
	if err := q.QueryRowContext(bg, sqlText, args...).Scan(&n); err != nil {
		return -1
	}
	return n
}

func fnum(v any) float64 {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case float64:
		return t
	}
	return math.NaN()
}

// ---- T1 ----

func checkT1(p *Project) []Check {
	s, _ := loadSchema(p.R)
	var out []Check
	if t := s["btc_prices"]; t == nil {
		out = append(out, ck("btc_prices: date key, numeric price, _run_id/_fetched_at", false, "table missing (tables: %s)", strings.Join(tableNames(s), ", ")))
	} else {
		ok, miss := hasCols(t, "date")
		pc := priceCol(t)
		if pc == nil {
			ok, miss = false, strings.TrimPrefix(miss+", price", ", ")
		}
		pok, pmiss := pipeTable(t)
		why := []string{}
		if !ok {
			why = append(why, "missing "+miss)
		}
		if !pok {
			why = append(why, "missing "+pmiss)
		}
		if !sameSet(t.PK, []string{"date"}) {
			why = append(why, "primary key is ("+strings.Join(t.PK, ", ")+")")
		}
		if c := pc; c != nil && !numeric(c.Type) {
			why = append(why, "price is "+c.Type)
		}
		out = append(out, ck("btc_prices: date key, numeric price, _run_id/_fetched_at", len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	if t := s["btc_news"]; t == nil {
		out = append(out, ck("btc_news: url key, date/title/reason, _run_id/_fetched_at", false, "table missing"))
	} else {
		ok, miss := hasCols(t, "url", "date", "title", "reason")
		pok, pmiss := pipeTable(t)
		why := []string{}
		if !ok {
			why = append(why, "missing "+miss)
		}
		if !pok {
			why = append(why, "missing "+pmiss)
		}
		if !sameSet(t.PK, []string{"url"}) {
			why = append(why, "primary key is ("+strings.Join(t.PK, ", ")+")")
		}
		out = append(out, ck("btc_news: url key, date/title/reason, _run_id/_fetched_at", len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	t := s["watch_keywords"]
	if t == nil {
		out = append(out, ck("watch_keywords: integer id key, keyword NOT NULL UNIQUE", false, "table missing"))
		out = append(out, ck("watch_keywords: rejects duplicate and empty keyword, assigns id", false, "table missing"))
		return out
	}
	why := []string{}
	if !sameSet(t.PK, []string{"id"}) {
		why = append(why, "primary key is ("+strings.Join(t.PK, ", ")+")")
	} else if c := t.col("id"); !strings.EqualFold(c.Type, "INTEGER") {
		why = append(why, "id is "+c.Type)
	}
	if c := t.col("keyword"); c == nil {
		why = append(why, "no keyword column")
	} else {
		if !c.NotNull {
			why = append(why, "keyword is nullable")
		}
		if !t.isKey([]string{"keyword"}) {
			why = append(why, "keyword is not unique")
		}
	}
	out = append(out, ck("watch_keywords: integer id key, keyword NOT NULL UNIQUE", len(why) == 0, "%s", strings.Join(why, "; ")))
	// behaviour, on a copy
	db, c, path, err := copyProject(p)
	if err != nil {
		out = append(out, ck("watch_keywords: rejects duplicate and empty keyword, assigns id", false, "copy: %v", err))
		return out
	}
	r := &mockRun{db: db, conn: c, path: path}
	defer r.close()
	why = nil
	if _, err := c.ExecContext(bg, "INSERT INTO watch_keywords(keyword) VALUES ('ETF')"); err != nil {
		why = append(why, "first insert failed: "+cleanSQLiteErr(err))
	} else {
		if _, err := c.ExecContext(bg, "INSERT INTO watch_keywords(keyword) VALUES ('ETF')"); err == nil {
			why = append(why, "duplicate accepted")
		}
		if n := count(c, "SELECT count(*) FROM watch_keywords WHERE id IS NULL"); n != 0 {
			why = append(why, "id not assigned")
		}
	}
	if _, err := c.ExecContext(bg, "INSERT INTO watch_keywords(keyword) VALUES (NULL)"); err == nil {
		why = append(why, "NULL keyword accepted")
	}
	out = append(out, ck("watch_keywords: rejects duplicate and empty keyword, assigns id", len(why) == 0, "%s", strings.Join(why, "; ")))
	return out
}

// ---- T2 ----

func cronDaily8(s string) bool {
	f := strings.Fields(s)
	if len(f) != 5 {
		return false
	}
	return (f[0] == "0" || f[0] == "00") && (f[1] == "8" || f[1] == "08") && f[2] == "*" && f[3] == "*" && (f[4] == "*" || f[4] == "0-6" || f[4] == "0-7" || f[4] == "1-7")
}

func stepsUsing(c *Config, use string) []map[string]any {
	var out []map[string]any
	for _, s := range asList(c.Val["steps"]) {
		if m := asMap(s); asStr(m["use"]) == use {
			out = append(out, m)
		}
	}
	return out
}

func checkTrigger(c *Config) Check {
	tr := asMap(c.Val["trigger"])
	tz := asStr(tr["timezone"])
	return ck("trigger: 08:00 daily, UTC, manual", cronDaily8(asStr(tr["schedule"])) && (tz == "UTC" || tz == "Etc/UTC") && tr["manual"] == true,
		"schedule %q timezone %q manual %v", asStr(tr["schedule"]), tz, tr["manual"])
}

func checkT2(p *Project) []Check {
	c := p.Configs["daily_btc"]
	if c == nil || c.Kind != "pipeline" {
		return []Check{ck("pipeline daily_btc saved", false, "not saved (saved: %s)", strings.Join(p.configIDs(), ", "))}
	}
	out := []Check{ck("pipeline daily_btc saved", true, ""), checkTrigger(c)}
	sel := stepsUsing(c, "llm.select")
	okSel := false
	for _, m := range sel {
		w := asMap(m["with"])
		if fnum(normVal(w["count"])) == 5 && asMap(w["add"])["reason"] != nil {
			okSel = true
		}
	}
	out = append(out, ck("llm.select picks 5 and adds reason", okSel, "%d llm.select steps, none with count 5 and add.reason", len(sel)))
	fresh := false
	for _, m := range stepsUsing(c, "web.search") {
		if asStr(asMap(m["with"])["freshness"]) == "day" {
			fresh = true
		}
	}
	out = append(out, ck("web.search limited to the last day", fresh, "no web.search with freshness: day"))

	r := runPipelineMock(p, "daily_btc", 2)
	defer r.close()
	out = append(out, ck("mock run succeeds twice (rerun safe)", r.Err == "", "%s", r.Err))
	if r.conn == nil {
		return out
	}
	okURL := len(r.Fetched) > 0
	for _, f := range r.Fetched {
		if !strings.HasPrefix(f, "https://api.example.com/btc/daily") {
			okURL = false
		}
	}
	out = append(out, ck("fetches only the given price URL", okURL, "fetched %v", r.Fetched))
	today := time.Now().UTC().Format("2006-01-02")
	var price float64
	n := count(r.conn, "SELECT count(*) FROM btc_prices WHERE date = ?", today)
	r.conn.QueryRowContext(bg, "SELECT \"" + priceName(r.conn) + "\" FROM btc_prices WHERE date = ?", today).Scan(&price)
	old := count(r.conn, "SELECT count(*) FROM btc_prices WHERE date < ?", today)
	out = append(out, ck("btc_prices: one row today = 65000.5, seeds kept", n == 1 && price == 65000.5 && old == 10,
		"today rows %d price %v, older rows %d", n, price, old))
	runID := count(r.conn, "SELECT count(*) FROM btc_prices WHERE date = ? AND _run_id LIKE 'run_mock_%'", today)
	out = append(out, ck("btc_prices row has _run_id (runtime fill)", runID == 1, "row without _run_id"))
	nn := count(r.conn, "SELECT count(*) FROM btc_news WHERE url NOT LIKE 'https://news.example.com/seed/%'")
	withReason := count(r.conn, "SELECT count(*) FROM btc_news WHERE length(trim(coalesce(reason, ''))) > 0 AND url LIKE 'https://news.example.com/%' AND url NOT LIKE 'https://news.example.com/seed/%' AND length(coalesce(title,'')) > 0 AND length(coalesce(date,'')) >= 10")
	out = append(out, ck("btc_news: 5 new rows with url, title, date, reason (no duplicates after rerun)", nn == 5 && withReason == 5, "%d new rows, %d complete", nn, withReason))
	q := strings.ToLower(strings.Join(r.Queries, " | "))
	okQ := len(r.Queries) > 0 && (strings.Contains(q, "bitcoin") || strings.Contains(q, "btc"))
	var missKW []string
	for _, k := range seedKeywords {
		if !strings.Contains(q, strings.ToLower(k)) {
			missKW = append(missKW, k)
		}
	}
	out = append(out, ck("search mentions bitcoin and every watch keyword", okQ && len(missKW) == 0, "queries %q, missing %v", r.Queries, missKW))
	return out
}

// ---- T3 ----

func viewOf(p *Project, id, typ string) (*Config, string) {
	c := p.Configs[id]
	if c == nil || c.Kind != "view" {
		return nil, fmt.Sprintf("view %s not saved", id)
	}
	if c.Type != typ {
		return nil, fmt.Sprintf("view %s is a %s, not a %s", id, c.Type, typ)
	}
	return c, ""
}

func firstKey(rows []map[string]any, keys ...string) string {
	if len(rows) == 0 {
		return ""
	}
	for _, k := range keys {
		if _, ok := rows[0][k]; ok {
			return k
		}
	}
	return ""
}

func checkT3(p *Project) []Check {
	var out []Check
	today := time.Now().UTC()
	// 1. price table
	name := "btc_price_table: 10 seeded prices newest first, From filter ~90 days, Refresh runs daily_btc"
	if c, why := viewOf(p, "btc_price_table", "table"); c == nil {
		out = append(out, ck(name, false, "%s", why))
	} else {
		var why []string
		rows, err := openView(p.R, c.Val, nil)
		ds := asMap(c.Val["default_sort"])
		if err != nil {
			why = append(why, "open: "+cleanSQLiteErr(err))
		} else if len(rows) != 10 {
			why = append(why, fmt.Sprintf("%d rows", len(rows)))
		} else if pk := firstKey(rows, "price", "close", "price_usd"); pk == "" || fnum(rows[0][pk]) != 60100 {
			why = append(why, fmt.Sprintf("first row %v is not the newest price", rows[0]))
		}
		if asStr(ds["direction"]) != "desc" {
			why = append(why, "not newest first")
		}
		okF := false
		for _, f := range asList(c.Val["filters"]) {
			fm := asMap(f)
			if asStr(fm["control"]) == "date" {
				d := resolveDateDefault(asStr(fm["default"]), today)
				if t, err := time.Parse("2006-01-02", d); err == nil {
					days := today.Sub(t).Hours() / 24
					okF = days >= 88 && days <= 92
				}
			}
		}
		if !okF {
			why = append(why, "no date filter defaulting to ~90 days ago")
		}
		okA := false
		for _, a := range asList(c.Val["actions"]) {
			if asStr(asMap(a)["run_pipeline"]) == "daily_btc" {
				okA = true
			}
		}
		if !okA {
			why = append(why, "no action running daily_btc")
		}
		out = append(out, ck(name, len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	// 2. chart
	name = "btc_price_chart: line chart, date on x, price on y, 10 points"
	if c, why := viewOf(p, "btc_price_chart", "chart"); c == nil {
		out = append(out, ck(name, false, "%s", why))
	} else {
		var why []string
		rows, err := openView(p.R, c.Val, nil)
		x := asStr(asMap(c.Val["x"])["field"])
		if err != nil {
			why = append(why, "open: "+cleanSQLiteErr(err))
		} else if len(rows) != 10 {
			why = append(why, fmt.Sprintf("%d rows", len(rows)))
		} else if _, err := time.Parse("2006-01-02", asStr(rows[0][x])); err != nil {
			why = append(why, fmt.Sprintf("x %s is not a date (%v)", x, rows[0][x]))
		} else {
			okY := false
			for _, y := range asList(c.Val["y"]) {
				if v := fnum(rows[0][asStr(asMap(y)["field"])]); v >= 60000 && v <= 61100 {
					okY = true
				}
			}
			if !okY {
				why = append(why, "no y series with the price")
			}
		}
		if asStr(c.Val["chart_type"]) != "line" {
			why = append(why, "chart_type "+asStr(c.Val["chart_type"]))
		}
		out = append(out, ck(name, len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	// 3. news table
	name = "btc_news_table: 4 seeded news newest first with date, title, reason, link"
	if c, why := viewOf(p, "btc_news_table", "table"); c == nil {
		out = append(out, ck(name, false, "%s", why))
	} else {
		var why []string
		rows, err := openView(p.R, c.Val, nil)
		if err != nil {
			why = append(why, "open: "+cleanSQLiteErr(err))
		} else if len(rows) != 4 {
			why = append(why, fmt.Sprintf("%d rows", len(rows)))
		}
		if asStr(asMap(c.Val["default_sort"])["direction"]) != "desc" {
			why = append(why, "not newest first")
		}
		have := map[string]bool{}
		for _, col := range asList(c.Val["columns"]) {
			have[asStr(asMap(col)["field"])] = true
		}
		for _, f := range []string{"date", "title", "reason", "url"} {
			if !have[f] {
				why = append(why, "no column "+f)
			}
		}
		out = append(out, ck(name, len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	// 4. keyword list
	name = "keyword_list: 3 keywords, rows deletable"
	if c, why := viewOf(p, "keyword_list", "table"); c == nil {
		out = append(out, ck(name, false, "%s", why))
	} else {
		var why []string
		rows, err := openView(p.R, c.Val, nil)
		if err != nil {
			why = append(why, "open: "+cleanSQLiteErr(err))
		} else if len(rows) != 3 {
			why = append(why, fmt.Sprintf("%d rows", len(rows)))
		}
		if asStr(asMap(c.Val["rows_from"])["table"]) != "watch_keywords" {
			why = append(why, "rows_from is not watch_keywords")
		}
		del := false
		for _, a := range asList(c.Val["row_actions"]) {
			if asMap(a)["delete"] == true {
				del = true
			}
		}
		if !del {
			why = append(why, "no delete row action")
		}
		out = append(out, ck(name, len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	// 5. form
	name = "add_keyword: insert form, keyword required, submit adds a row"
	if c, why := viewOf(p, "add_keyword", "form"); c == nil {
		out = append(out, ck(name, false, "%s", why))
	} else {
		var why []string
		sub := asMap(c.Val["submit"])
		if asStr(sub["action"]) != "insert" || asStr(sub["table"]) != "watch_keywords" {
			why = append(why, fmt.Sprintf("submit %v", sub))
		}
		vals := map[string]any{}
		for _, f := range asList(c.Val["fields"]) {
			fm := asMap(f)
			if asStr(fm["field"]) == "keyword" {
				vals["keyword"] = "mining"
				if fm["required"] != true {
					why = append(why, "keyword not required")
				}
			}
		}
		if len(vals) == 0 {
			why = append(why, "no keyword field")
		} else if len(why) == 0 {
			db, conn, path, err := copyProject(p)
			if err != nil {
				why = append(why, "copy: "+err.Error())
			} else {
				r := &mockRun{db: db, conn: conn, path: path}
				if err := submitForm(conn, c.Val, vals); err != nil {
					why = append(why, "submit: "+cleanSQLiteErr(err))
				} else if count(conn, "SELECT count(*) FROM watch_keywords WHERE keyword = 'mining'") != 1 {
					why = append(why, "row not added")
				}
				r.close()
			}
		}
		out = append(out, ck(name, len(why) == 0, "%s", strings.Join(why, "; ")))
	}
	return out
}

// ---- T4 ----

// newsTable finds the table that holds the seeded news (renamed or not).
func newsTable(q Q, s map[string]*Table) string {
	for _, n := range tableNames(s) {
		if s[n].col("url") != nil && count(q, `SELECT count(*) FROM `+qi(n)+` WHERE url LIKE 'https://news.example.com/seed/%'`) == 4 {
			return n
		}
	}
	return ""
}

// viewReading finds the view by id, or else a view of that type whose query reads the table.
func viewReading(p *Project, id, typ, table string) *Config {
	if c := p.Configs[id]; c != nil && c.Kind == "view" && c.Type == typ {
		return c
	}
	for _, i := range p.configIDs() {
		c := p.Configs[i]
		if c.Kind == "view" && c.Type == typ && strings.Contains(strings.ToLower(asStr(c.Val["query"])), table) {
			return c
		}
	}
	return nil
}

// coinsShown opens a view and returns the coins seen, trying coin filter values if needed.
func coinsShown(q Q, v map[string]any) (map[string]bool, string, error) {
	seen := map[string]bool{}
	var coinCol string
	collect := func(over map[string]any) error {
		rows, err := openView(q, v, over)
		if err != nil {
			return err
		}
		for _, r := range rows {
			for k, x := range r {
				lk := strings.ToLower(k)
				if !strings.Contains(lk, "coin") && !strings.Contains(lk, "symbol") {
					continue
				}
				if s, ok := x.(string); ok && (s == "BTC" || s == "ETH") {
					seen[s] = true
					coinCol = k
				}
			}
		}
		return nil
	}
	if err := collect(nil); err != nil {
		return seen, "", err
	}
	if len(seen) < 2 {
		for _, f := range asList(v["filters"]) {
			p := asStr(asMap(f)["param"])
			if strings.Contains(p, "coin") || strings.Contains(p, "symbol") {
				collect(map[string]any{p: "BTC"})
				collect(map[string]any{p: "ETH"})
			}
		}
	}
	return seen, coinCol, nil
}

func checkT4(p *Project) []Check {
	s, _ := loadSchema(p.R)
	var out []Check
	t := s["prices"]
	if t == nil {
		out = append(out, ck("prices(coin, date, price): key (coin, date), _run_id/_fetched_at", false, "no prices table (tables: %s)", strings.Join(tableNames(s), ", ")))
		out = append(out, ck("all 10 BTC rows copied with coin BTC", false, "no prices table"))
	} else {
		var why []string
		if ok, miss := hasCols(t, "coin", "date", "price", "_run_id", "_fetched_at"); !ok {
			why = append(why, "missing "+miss)
		}
		if !sameSet(t.PK, []string{"coin", "date"}) && !t.isKey([]string{"coin", "date"}) {
			why = append(why, "(coin, date) is not a key; primary key ("+strings.Join(t.PK, ", ")+")")
		}
		out = append(out, ck("prices(coin, date, price): key (coin, date), _run_id/_fetched_at", len(why) == 0, "%s", strings.Join(why, "; ")))
		n := count(p.R, `SELECT count(*) FROM prices WHERE coin = 'BTC' AND price BETWEEN 60000 AND 61001 AND _run_id LIKE 'run_seed_%'`)
		out = append(out, ck("all 10 BTC rows copied with coin BTC (price and _run_id kept)", n == 10, "%d of 10 seeded rows found", n))
	}
	out = append(out, ck("btc_prices dropped", s["btc_prices"] == nil, "btc_prices still exists"))
	nt := newsTable(p.R, s)
	if nt == "" {
		out = append(out, ck("news table has coin; old news are BTC", false, "seeded news rows not found"))
	} else {
		n := -1
		if s[nt].col("coin") != nil {
			n = count(p.R, `SELECT count(*) FROM `+qi(nt)+` WHERE coin = 'BTC' AND url LIKE 'https://news.example.com/seed/%'`)
		}
		out = append(out, ck("news table has coin; old news are BTC", n == 4, "table %s: %d of 4 seeded rows with coin BTC", nt, n))
	}
	c := p.Configs["daily_btc"]
	if c == nil {
		out = append(out, ck("pipeline daily_btc still saved", false, "missing"))
		return out
	}
	out = append(out, checkTrigger(c))
	r := runPipelineMock(p, "daily_btc", 2)
	defer r.close()
	out = append(out, ck("mock run succeeds twice (rerun safe)", r.Err == "", "%s", r.Err))
	if r.conn == nil {
		return out
	}
	fb, fe := false, false
	for _, f := range r.Fetched {
		fb = fb || strings.HasPrefix(f, "https://api.example.com/btc/daily")
		fe = fe || strings.HasPrefix(f, "https://api.example.com/eth/daily")
	}
	out = append(out, ck("fetches the BTC and the ETH URL", fb && fe, "fetched %v", r.Fetched))
	today := time.Now().UTC().Format("2006-01-02")
	if s["prices"] != nil {
		var b, e float64
		nb := count(r.conn, "SELECT count(*) FROM prices WHERE coin = 'BTC' AND date = ?", today)
		ne := count(r.conn, "SELECT count(*) FROM prices WHERE coin = 'ETH' AND date = ?", today)
		r.conn.QueryRowContext(bg, "SELECT price FROM prices WHERE coin = 'BTC' AND date = ?", today).Scan(&b)
		r.conn.QueryRowContext(bg, "SELECT price FROM prices WHERE coin = 'ETH' AND date = ?", today).Scan(&e)
		out = append(out, ck("prices today: BTC 65000.5 and ETH 3200.25, one row each", nb == 1 && ne == 1 && b == 65000.5 && e == 3200.25,
			"BTC rows %d (%v), ETH rows %d (%v)", nb, b, ne, e))
	} else {
		out = append(out, ck("prices today: BTC 65000.5 and ETH 3200.25, one row each", false, "no prices table"))
	}
	if nt != "" && s[nt].col("coin") != nil {
		n := count(r.conn, `SELECT count(*) FROM `+qi(nt)+` WHERE url NOT LIKE 'https://news.example.com/seed/%'`)
		good := count(r.conn, `SELECT count(*) FROM `+qi(nt)+` WHERE url NOT LIKE 'https://news.example.com/seed/%' AND coin IN ('BTC', 'ETH') AND length(trim(coalesce(reason, ''))) > 0`)
		out = append(out, ck("news: 5 new items, each with coin BTC/ETH and a reason", n == 5 && good == 5, "%d new rows, %d with coin and reason", n, good))
	} else {
		out = append(out, ck("news: 5 new items, each with coin BTC/ETH and a reason", false, "no news table with coin"))
	}
	q := strings.ToLower(strings.Join(r.Queries, " | "))
	out = append(out, ck("news search covers both coins", (strings.Contains(q, "bitcoin") || strings.Contains(q, "btc")) && (strings.Contains(q, "ethereum") || strings.Contains(q, "eth")),
		"queries %q", r.Queries))
	// views, opened on the copy after the run so ETH rows exist
	if v := viewReading(p, "btc_price_table", "table", "prices"); v == nil {
		out = append(out, ck("price table shows both coins", false, "no table view"))
	} else {
		seen, _, err := coinsShown(r.conn, v.Val)
		out = append(out, ck("price table shows both coins", err == nil && seen["BTC"] && seen["ETH"], "coins %v err %v", seen, err))
	}
	if v := viewReading(p, "btc_price_chart", "chart", "prices"); v == nil {
		out = append(out, ck("price chart: one line per coin (series_by)", false, "no chart view"))
	} else {
		seen, _, err := coinsShown(r.conn, v.Val)
		sb := asStr(v.Val["series_by"])
		out = append(out, ck("price chart: one line per coin (series_by)", err == nil && seen["BTC"] && seen["ETH"] && sb != "",
			"series_by %q coins %v err %v", sb, seen, err))
	}
	if v := viewReading(p, "btc_news_table", "table", "news"); v == nil {
		out = append(out, ck("news table view shows the coin", false, "no news table view"))
	} else {
		rows, err := openView(r.conn, v.Val, nil)
		hasCoin := false
		for _, col := range asList(v.Val["columns"]) {
			if strings.Contains(asStr(asMap(col)["field"]), "coin") {
				hasCoin = true
			}
		}
		out = append(out, ck("news table view shows the coin", err == nil && len(rows) > 0 && hasCoin, "rows %d coin column %v err %v", len(rows), hasCoin, err))
	}
	return out
}

// priceCol accepts price, price_usd, close... (the T1 prompt says "the price in USD").
func priceCol(t *Table) *Column {
	for i, c := range t.Cols {
		n := strings.ToLower(c.Name)
		if strings.Contains(n, "price") || n == "close" || n == "usd" {
			return &t.Cols[i]
		}
	}
	return nil
}

// priceName is the model's price column in btc_prices (price, price_usd, close...), "price" if unknown.
func priceName(q Q) string {
	if s, err := loadSchema(q); err == nil {
		if t := s["btc_prices"]; t != nil {
			if c := priceCol(t); c != nil {
				return c.Name
			}
		}
	}
	return "price"
}
