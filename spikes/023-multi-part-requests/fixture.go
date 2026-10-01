package main

// The fixture project "Crypto watch": an in-memory SQLite database (modernc.org/sqlite, no cgo)
// with synthetic BTC prices and news, the stored pipeline configs, and the coded fakes for
// run_pipeline and call_api. All data is synthetic.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

var dbSeq atomic.Int64

// fixed closes so the answers are known: 30-day max 66,120.00 on 09-12, min 58,940.10 on 09-03,
// 7-day max 65,230.80 on 09-26, yesterday (09-28) 64,512.30.
var fixedClose = map[string]float64{
	"2026-09-03": 58940.10, "2026-09-12": 66120.00,
	"2026-09-21": 62310.40, "2026-09-22": 62980.10, "2026-09-23": 63455.00, "2026-09-24": 64950.20,
	"2026-09-25": 64100.00, "2026-09-26": 65230.80, "2026-09-27": 63880.50, "2026-09-28": 64512.30,
}

var newsRows = [][4]string{
	// published_at, title, source, slug
	{"2026-09-28T17:10:00Z", "Spot bitcoin ETFs log a fourth day of inflows", "coindesk.example", "etf-inflows"},
	{"2026-09-28T09:30:00Z", "Bitcoin hashrate sets a new record", "theblock.example", "hashrate-record"},
	{"2026-09-27T15:45:00Z", "Fed minutes weigh on risk assets; BTC slips 2%", "reuters.example", "fed-minutes"},
	{"2026-09-26T12:00:00Z", "BTC tops 65,000 USD for the first time in two weeks", "coindesk.example", "btc-65k"},
	{"2026-09-25T08:20:00Z", "Miner reserves fall to a three-year low", "cryptoslate.example", "miner-reserves"},
	{"2026-09-24T19:05:00Z", "Options expiry: 4.1B USD in BTC contracts", "decrypt.example", "options-expiry"},
	{"2026-09-22T10:40:00Z", "Large exchange outflows continue", "theblock.example", "exchange-outflows"},
	{"2026-09-19T14:15:00Z", "Lightning capacity passes 6,000 BTC", "bitcoinmag.example", "lightning-capacity"},
	{"2026-09-15T07:55:00Z", "Stablecoin supply reaches a record", "coindesk.example", "stablecoin-supply"},
	{"2026-09-12T21:30:00Z", "BTC jumps 4% after US inflation comes in low", "reuters.example", "cpi-jump"},
	{"2026-09-08T11:00:00Z", "Mt. Gox repayment deadline moved again", "decrypt.example", "mtgox-deadline"},
	{"2026-09-03T16:25:00Z", "BTC falls below 59,000 USD on ETF outflows", "cryptoslate.example", "btc-59k"},
}

func openFixtureDB() (*sql.DB, error) {
	name := fmt.Sprintf("file:fx%d?mode=memory&cache=shared", dbSeq.Add(1))
	db, err := sql.Open("sqlite", name)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	stmts := []string{
		`CREATE TABLE prices(coin TEXT NOT NULL, date TEXT NOT NULL, open REAL, high REAL, low REAL, close REAL, currency TEXT NOT NULL DEFAULT 'USD', PRIMARY KEY(coin, date))`,
		`CREATE TABLE news(url TEXT PRIMARY KEY, coin TEXT, title TEXT, source TEXT, published_at TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			return nil, err
		}
	}
	d0 := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	prev := 61000.0
	for i := 0; i < 60; i++ {
		d := d0.AddDate(0, 0, i).Format("2006-01-02")
		c, ok := fixedClose[d]
		if !ok {
			c = 62000 + 2600*math.Sin(float64(i)*0.7) + 400*math.Cos(float64(i)*1.9)
			c = math.Round(c*100) / 100
		}
		hi := math.Round(math.Max(prev, c)*1.008*100) / 100
		lo := math.Round(math.Min(prev, c)*0.992*100) / 100
		if _, err := db.Exec(`INSERT INTO prices VALUES('BTC',?,?,?,?,?,'USD')`, d, prev, hi, lo, c); err != nil {
			db.Close()
			return nil, err
		}
		prev = c
	}
	for _, n := range newsRows {
		url := "https://" + n[2] + "/2026/" + n[0][5:7] + "/" + n[0][8:10] + "/" + n[3]
		if _, err := db.Exec(`INSERT INTO news VALUES(?,?,?,?,?)`, url, "BTC", n[1], n[2], n[0]); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(`PRAGMA query_only = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

var reSelect = regexp.MustCompile(`(?is)^\s*(select|with)\b`)

func runQuery(db *sql.DB, q string) string {
	if !reSelect.MatchString(q) {
		return "error: only one read-only SELECT is allowed"
	}
	if strings.Contains(strings.TrimRight(strings.TrimSpace(q), ";"), ";") {
		return "error: only one statement is allowed"
	}
	rows, err := db.Query(q)
	if err != nil {
		return "error: " + err.Error()
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var b strings.Builder
	b.WriteString(strings.Join(cols, " | ") + "\n")
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "error: " + err.Error()
		}
		n++
		if n > 200 {
			continue
		}
		var cells []string
		for _, v := range vals {
			switch x := v.(type) {
			case nil:
				cells = append(cells, "NULL")
			case []byte:
				cells = append(cells, string(x))
			case float64:
				cells = append(cells, fmt.Sprintf("%.2f", x))
			default:
				cells = append(cells, fmt.Sprint(x))
			}
		}
		b.WriteString(strings.Join(cells, " | ") + "\n")
	}
	if err := rows.Err(); err != nil {
		return "error: " + err.Error()
	}
	fmt.Fprintf(&b, "(%d rows)", n)
	return b.String()
}

func describeTable(db *sql.DB, name string) string {
	switch name {
	case "prices", "news":
	default:
		return fmt.Sprintf("error: table %q not found (tables: prices, news)", name)
	}
	rows, err := db.Query(`SELECT name, type, pk FROM pragma_table_info(?)`, name)
	if err != nil {
		return "error: " + err.Error()
	}
	defer rows.Close()
	var b strings.Builder
	fmt.Fprintf(&b, "table %s\n", name)
	for rows.Next() {
		var n, t string
		var pk int
		rows.Scan(&n, &t, &pk)
		k := ""
		if pk > 0 {
			k = fmt.Sprintf(" (primary key %d)", pk)
		}
		fmt.Fprintf(&b, "- %s %s%s\n", n, t, k)
	}
	var cnt int
	db.QueryRow(`SELECT count(*) FROM ` + name).Scan(&cnt)
	fmt.Fprintf(&b, "rows: %d", cnt)
	return b.String()
}

var configs = map[string]string{
	"daily_prices": `version: 1
id: daily_prices
title: Daily price and news
inputs:
  coin: { type: string, enum: [BTC, ETH], default: BTC }
  currency: { type: string, enum: [USD, EUR], default: USD }
trigger: { schedule: "0 8 * * *", timezone: Europe/Berlin }
steps:
  - id: price
    use: http.get
    with: { url: "https://api.coingecko.example/simple/price", query: { ids: "{{ inputs.coin }}", vs_currencies: "{{ inputs.currency }}" } }
    on_error: { retry: 3, then: fail }
  - id: save_price
    use: db.upsert
    with: { table: prices, rows: "{{ steps.price.rows }}" }
  - id: news
    use: feed.read
    with: { url: "https://news.example.com/{{ inputs.coin }}.rss", since: 1d }
    on_error: { retry: 2, then: skip }
  - id: save_news
    use: db.upsert
    with: { table: news, rows: "{{ steps.news.items }}" }`,
	"backfill_prices": `version: 1
id: backfill_prices
title: Backfill daily prices
inputs:
  coin: { type: string, enum: [BTC, ETH] }
  days: { type: integer, min: 1, max: 365 }
  currency: { type: string, enum: [USD, EUR], default: USD }
steps:
  - id: history
    use: http.get
    with: { url: "https://api.coingecko.example/coins/{{ inputs.coin }}/ohlc", query: { days: "{{ inputs.days }}", vs_currency: "{{ inputs.currency }}" } }
    on_error: { retry: 3, then: fail }
  - id: save
    use: db.upsert
    with: { table: prices, rows: "{{ steps.history.rows }}" }`,
	"fear_greed": `version: 1
id: fear_greed
title: Fear and greed index
trigger: { schedule: "0 9 * * *", timezone: Europe/Berlin }
steps:
  - id: index
    use: http.get
    with: { url: "https://api.alternative.example/fng/" }
  - id: summary
    use: llm.summarize
    with: { text: "{{ steps.index.body }}", max_tokens: 400 }
  - id: save
    use: db.insert
    with: { table: news, rows: [{ url: "fng-{{ run.date }}", coin: BTC, title: "{{ steps.summary.text }}", source: alternative.example }] }`,
}

var prices = map[string]string{ // coin/currency -> today's price
	"BTC/USD": "64,210.50", "BTC/EUR": "55,190.20", "ETH/USD": "2,587.40", "ETH/EUR": "2,224.10",
}

var dayChange = map[string]string{"BTC": "+1.8%", "ETH": "+2.3%"}

var newsToday = map[string]string{
	"BTC": `"Spot bitcoin ETFs add 410M USD in a day"; "SEC delays decision on staking ETFs"; "Miner reserves keep falling"`,
	"ETH": `"ETH gas fees at a two-year low"; "Staking queue shortens after upgrade"; "ETH ETF inflows turn positive"`,
}

func argStr(a map[string]any, k string) string {
	if v, ok := a[k]; ok && v != nil {
		switch x := v.(type) {
		case string:
			return x
		case float64:
			return fmt.Sprint(x)
		default:
			b, _ := json.Marshal(x)
			return string(b)
		}
	}
	return ""
}

func argMap(a map[string]any, k string) map[string]any {
	if m, ok := a[k].(map[string]any); ok {
		return m
	}
	// some models send inputs as a JSON string
	if s, ok := a[k].(string); ok {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			return m
		}
	}
	return map[string]any{}
}

func normCoin(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "ETHEREUM":
		return "ETH"
	case "BITCOIN", "":
		return "BTC"
	}
	return s
}

func normCur(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return "USD"
	}
	return s
}

// pipelineRun checks the inputs and returns the finish text and the run length in seconds.
func pipelineRun(id string, in map[string]any) (finish string, delay float64, err string) {
	switch id {
	case "daily_prices":
		coin, cur := normCoin(argStr(in, "coin")), normCur(argStr(in, "currency"))
		p, ok := prices[coin+"/"+cur]
		if !ok {
			return "", 0, fmt.Sprintf("inputs: coin must be BTC or ETH and currency USD or EUR (got %s, %s)", coin, cur)
		}
		return fmt.Sprintf("success. prices +1 row: %s 2026-09-29 close %s %s (%s vs yesterday). news +3 rows: %s.", coin, p, cur, dayChange[coin], newsToday[coin]), 30, ""
	case "backfill_prices":
		coin, cur := normCoin(argStr(in, "coin")), normCur(argStr(in, "currency"))
		days := 0
		fmt.Sscanf(argStr(in, "days"), "%d", &days)
		if _, ok := prices[coin+"/"+cur]; !ok || days < 1 || days > 365 {
			return "", 0, fmt.Sprintf("inputs: coin BTC or ETH, days 1-365, currency USD or EUR (got %s, %d, %s)", coin, days, cur)
		}
		from := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
		return fmt.Sprintf("success. prices +%d rows: %s daily OHLC in %s from %s to 2026-09-28; 0 errors.", days, coin, cur, from), math.Max(20, float64(days)*4/3), ""
	case "fear_greed":
		return "success. news +1 row: fear and greed index 38 (fear), down from 44 yesterday.", 20, ""
	}
	return "", 0, fmt.Sprintf("pipeline %q not found (pipelines: daily_prices, backfill_prices, fear_greed)", id)
}

var reIDs = regexp.MustCompile(`(?i)ids=([a-z,]+)`)
var reVs = regexp.MustCompile(`(?i)vs_currenc(?:y|ies)=([a-z,]+)`)

// callAPI fakes the coingecko connection.
func callAPI(args map[string]any, raw string) string {
	conn := argStr(args, "connection")
	if conn != "" && conn != "coingecko" {
		return fmt.Sprintf("error: connection %q not found (connections: coingecko)", conn)
	}
	path := argStr(args, "path")
	q := argStr(args, "query")
	all := path + "&" + q + "&" + raw
	if !strings.Contains(path, "simple/price") {
		return "error: only GET /simple/price is available in this project"
	}
	ids := "bitcoin"
	if m := reIDs.FindStringSubmatch(all); m != nil {
		ids = strings.ToLower(m[1])
	} else if qm := argMap(args, "query"); qm["ids"] != nil {
		ids = strings.ToLower(argStr(qm, "ids"))
	}
	vs := "usd"
	if m := reVs.FindStringSubmatch(all); m != nil {
		vs = strings.ToLower(m[1])
	} else if qm := argMap(args, "query"); qm["vs_currencies"] != nil {
		vs = strings.ToLower(argStr(qm, "vs_currencies"))
	}
	out := map[string]map[string]float64{}
	num := map[string]float64{"BTC/USD": 64210.5, "BTC/EUR": 55190.2, "ETH/USD": 2587.4, "ETH/EUR": 2224.1}
	chg := map[string]float64{"BTC": 1.8, "ETH": 2.3}
	for _, id := range strings.Split(ids, ",") {
		coin := map[string]string{"bitcoin": "BTC", "ethereum": "ETH"}[strings.TrimSpace(id)]
		if coin == "" {
			continue
		}
		m := map[string]float64{}
		for _, c := range strings.Split(vs, ",") {
			c = strings.TrimSpace(c)
			if v, ok := num[coin+"/"+strings.ToUpper(c)]; ok {
				m[c] = v
				m[c+"_24h_change"] = chg[coin]
			}
		}
		out[strings.TrimSpace(id)] = m
	}
	b, _ := json.Marshal(out)
	return "200 OK " + string(b)
}
