package main

// The correct state after each task. T2 and T3 are the SPEC 9.2/9.3 configs verbatim (plus a
// news table, which 9.3 does not have); T1 and T4 follow SPEC 9.1 and 9.5. Each task starts
// from the reference state of the previous task; the seeds stand in for data collected so far.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const refT1 = `{"steps":[
 {"op":"create_table","table":"btc_prices","columns":[{"name":"date","type":"TEXT"},{"name":"price","type":"REAL"},{"name":"_run_id","type":"TEXT"},{"name":"_fetched_at","type":"TEXT"}],"primary_key":"date"},
 {"op":"create_table","table":"btc_news","columns":[{"name":"url","type":"TEXT"},{"name":"date","type":"TEXT"},{"name":"title","type":"TEXT"},{"name":"reason","type":"TEXT"},{"name":"_run_id","type":"TEXT"},{"name":"_fetched_at","type":"TEXT"}],"primary_key":"url"},
 {"op":"create_table","table":"watch_keywords","columns":[{"name":"id","type":"INTEGER"},{"name":"keyword","type":"TEXT","not_null":true}],"primary_key":"id","unique":["keyword"]}
]}`

// SPEC 9.2, verbatim.
const refPipeline = `version: 1
id: daily_btc
description: Daily BTC price and the 5 news items most likely to affect it
trigger:
  schedule: "0 8 * * *"
  timezone: UTC
  catch_up: once
  manual: true
steps:
  - id: price
    use: http.get
    with:
      url: https://api.example.com/btc/daily

  - id: save_price
    use: db.insert
    with:
      table: btc_prices
      on_conflict: upsert
      row:
        date: ${{ today() }}
        price: ${{ steps.price.json.close }}

  - id: keywords
    use: db.query
    with:
      sql: SELECT keyword FROM watch_keywords

  - id: news
    use: web.search
    with:
      query: "bitcoin ${{ join(map(steps.keywords.rows, .keyword), ' ') }} news"
      limit: 20
      freshness: day

  - id: top5
    use: llm.select
    with:
      items: ${{ steps.news.results }}
      count: 5
      instruction: Pick the items most likely to affect the Bitcoin price today.
      add: { reason: string }

  - id: news_rows
    use: transform.map
    with:
      items: ${{ steps.top5.items }}
      fields:
        url: ${{ item.url }}
        title: ${{ item.title }}
        reason: ${{ item.reason }}
        date: ${{ today() }}

  - id: save_news
    use: db.insert_many
    with:
      table: btc_news
      on_conflict: ignore
      rows: ${{ steps.news_rows.items }}
`

// SPEC 9.3, verbatim, in save order (the form before nothing depends on it; the price table's
// action needs the pipeline, which T2 saved).
var refViewsT3 = []string{`version: 1
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
actions:
  - { label: "Refresh now", run_pipeline: daily_btc }
`, `version: 1
id: btc_price_chart
title: Bitcoin price chart
type: chart
chart_type: line
query: SELECT date, price FROM btc_prices
x: { field: date, label: "Date", format: date }
y:
  - { field: price, label: "Price (USD)" }
`, `version: 1
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
`, `version: 1
id: add_keyword
title: Add news keyword
type: form
fields:
  - { field: keyword, label: "Keyword", control: text, required: true }
submit:
  action: insert
  table: watch_keywords
`,
	// not in SPEC 9.3: the task asks for a news table too
	`version: 1
id: btc_news_table
title: Bitcoin news
type: table
query: SELECT date, title, reason, url FROM btc_news
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: title, label: "Title" }
  - { field: reason, label: "Why it matters" }
  - { field: url, label: "Link", format: url }
`}

// SPEC 9.5 as migration steps plus the dependent updates.
const refT4Steps = `[
 {"op":"create_table","table":"coins","columns":[{"name":"symbol","type":"TEXT"}],"primary_key":"symbol"},
 {"op":"insert_rows","table":"coins","rows":[{"symbol":"BTC"},{"symbol":"ETH"}]},
 {"op":"create_table","table":"prices","columns":[{"name":"coin","type":"TEXT"},{"name":"date","type":"TEXT"},{"name":"price","type":"REAL"},{"name":"_run_id","type":"TEXT"},{"name":"_fetched_at","type":"TEXT"}],"primary_key":["coin","date"]},
 {"op":"copy_data","into":"prices","columns":["coin","date","price","_run_id","_fetched_at"],"from":"SELECT 'BTC', date, price, _run_id, _fetched_at FROM btc_prices"},
 {"op":"rename_table","from":"btc_news","to":"news"},
 {"op":"add_column","table":"news","column":{"name":"coin","type":"TEXT","not_null":true,"default":"BTC"}},
 {"op":"drop_table","table":"btc_prices"}
]`

const refPipelineT4 = `version: 1
id: daily_btc
description: Daily price of every tracked coin and the 5 news items most likely to affect them
trigger:
  schedule: "0 8 * * *"
  timezone: UTC
  catch_up: once
  manual: true
steps:
  - id: coins
    use: db.query
    with:
      sql: SELECT symbol FROM coins

  - id: price
    use: http.get
    for_each: ${{ steps.coins.rows }}
    with:
      url: https://api.example.com/${{ lower(item.symbol) }}/daily

  - id: price_rows
    use: transform.map
    with:
      items: ${{ steps.price.each }}
      fields:
        coin: ${{ item.item.symbol }}
        date: ${{ today() }}
        price: ${{ item.json.close }}

  - id: save_prices
    use: db.insert_many
    with:
      table: prices
      on_conflict: upsert
      rows: ${{ steps.price_rows.items }}

  - id: keywords
    use: db.query
    with:
      sql: SELECT keyword FROM watch_keywords

  - id: news
    use: web.search
    with:
      query: "bitcoin ethereum ${{ join(map(steps.keywords.rows, .keyword), ' ') }} news"
      limit: 20
      freshness: day

  - id: top5
    use: llm.select
    with:
      items: ${{ steps.news.results }}
      count: 5
      instruction: Pick the items most likely to affect the price of the tracked coins today. Set coin to BTC or ETH.
      add: { reason: string, coin: string }

  - id: news_rows
    use: transform.map
    with:
      items: ${{ steps.top5.items }}
      fields:
        url: ${{ item.url }}
        title: ${{ item.title }}
        reason: ${{ item.reason }}
        coin: ${{ item.coin }}
        date: ${{ today() }}

  - id: save_news
    use: db.insert_many
    with:
      table: news
      on_conflict: ignore
      rows: ${{ steps.news_rows.items }}
`

var refViewsT4 = []string{`version: 1
id: btc_price_table
title: Coin prices
type: table
query: |
  SELECT coin, date, price FROM prices
  WHERE date >= :from_date AND (:coin IS NULL OR coin = :coin)
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d", required: true }
  - { param: coin, control: select, label: "Coin", options: { query: "SELECT symbol FROM coins" } }
default_sort: { field: date, direction: desc }
columns:
  - { field: coin, label: "Coin" }
  - { field: date, label: "Date", format: date }
  - { field: price, label: "Price", format: currency, format_options: { currency: USD } }
actions:
  - { label: "Refresh now", run_pipeline: daily_btc }
`, `version: 1
id: btc_price_chart
title: Coin price chart
type: chart
chart_type: line
query: |
  SELECT coin, date, price FROM prices
  WHERE (:coin IS NULL OR coin = :coin)
filters:
  - { param: coin, control: select, label: "Coin", options: { query: "SELECT symbol FROM coins" } }
x: { field: date, label: "Date", format: date }
y:
  - { field: price, label: "Price (USD)" }
series_by: coin
`, `version: 1
id: btc_news_table
title: Coin news
type: table
query: |
  SELECT date, coin, title, reason, url FROM news
  WHERE (:coin IS NULL OR coin = :coin)
filters:
  - { param: coin, control: select, label: "Coin", options: { query: "SELECT symbol FROM coins" } }
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: coin, label: "Coin" }
  - { field: title, label: "Title" }
  - { field: reason, label: "Why it matters" }
  - { field: url, label: "Link", format: url }
`}

var seedKeywords = []string{"ETF", "halving", "regulation"}

// seed stands in for ten days of collected data. It writes directly (not through a tool) and
// only into the reference tables (using the model's price column name, e.g. price_usd);
// a model's differently named tables are left alone.
func seed(p *Project) []string {
	var notes []string
	now := time.Now().UTC()
	for i := 10; i >= 1; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := p.W.ExecContext(bg, `INSERT INTO btc_prices(date, "`+priceName(p.W)+`", _run_id, _fetched_at) VALUES (?, ?, ?, ?)`,
			d, 60000.0+float64(i)*100, fmt.Sprintf("run_seed_%d", i), d+"T08:00:05Z"); err != nil {
			notes = append(notes, "btc_prices: "+cleanSQLiteErr(err))
			break
		}
	}
	for i := 1; i <= 4; i++ {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := p.W.ExecContext(bg, `INSERT INTO btc_news(url, date, title, reason, _run_id, _fetched_at) VALUES (?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("https://news.example.com/seed/%d", i), d, fmt.Sprintf("Seed story %d", i), "seed reason",
			fmt.Sprintf("run_seed_%d", i), d+"T08:00:09Z"); err != nil {
			notes = append(notes, "btc_news: "+cleanSQLiteErr(err))
			break
		}
	}
	for _, k := range seedKeywords {
		if _, err := p.W.ExecContext(bg, `INSERT INTO watch_keywords(keyword) VALUES (?)`, k); err != nil {
			notes = append(notes, "watch_keywords: "+cleanSQLiteErr(err))
			break
		}
	}
	return notes
}

// buildState makes a fresh project in the reference state before task t (1..4).
// It fails loudly: the reference configs must pass our own validation.
func buildState(dir string, t int) (*Project, error) {
	p, err := newProject(dir)
	if err != nil {
		return nil, err
	}
	fail := func(what, res string) (*Project, error) {
		p.Close()
		return nil, fmt.Errorf("reference %s did not validate:\n%s", what, res)
	}
	if t >= 2 {
		if res, wc := p.callTool("apply_migration", refT1); wc == nil || !wc.OK {
			return fail("T1 migration", res)
		}
		if n := seed(p); len(n) > 0 {
			return fail("seed", strings.Join(n, "\n"))
		}
	}
	if t >= 3 {
		if res, wc := p.callTool("save_pipeline", yamlJSON(refPipeline)); !wc.OK {
			return fail("T2 pipeline", res)
		}
	}
	if t >= 4 {
		for _, v := range refViewsT3 {
			tool := "save_view"
			if strings.Contains(v, "type: form") {
				tool = "save_form"
			}
			if res, wc := p.callTool(tool, yamlJSON(v)); !wc.OK {
				return fail("T3 view", res)
			}
		}
	}
	return p, nil
}

func yamlJSON(y string) string {
	b, _ := json.Marshal(map[string]string{"yaml": y})
	return string(b)
}

// refT4Args is the whole 9.5 change as one apply_migration call.
func refT4Args() string {
	var steps any
	json.Unmarshal([]byte(refT4Steps), &steps)
	b, _ := json.Marshal(map[string]any{"steps": steps, "pipelines": []string{refPipelineT4}, "views": refViewsT4})
	return string(b)
}
