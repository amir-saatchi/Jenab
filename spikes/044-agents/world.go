package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The day the runs play on: the seed's last price is the day before.
var today = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

// seedSQL is SPEC 9's Bitcoin project: its three tables, with 30 days of
// prices, a week of news and five keywords. The tables are real, so
// query, describe_table and insert are the app's own tools.
func seedSQL() string {
	var b strings.Builder
	b.WriteString(`CREATE TABLE btc_prices(date TEXT PRIMARY KEY, price REAL, _run_id TEXT, _fetched_at TEXT);
CREATE TABLE btc_news(url TEXT PRIMARY KEY, date TEXT, title TEXT, reason TEXT, _run_id TEXT, _fetched_at TEXT);
CREATE TABLE watch_keywords(id INTEGER PRIMARY KEY, keyword TEXT NOT NULL UNIQUE);
`)
	for i := range 30 {
		d := today.AddDate(0, 0, -30+i)
		p := 68000 + 2500*math.Sin(float64(i)/4) + 40*float64(i)
		fmt.Fprintf(&b, "INSERT INTO btc_prices VALUES('%s', %.2f, 'run_%02d', '%sT08:00:05Z');\n", d.Format(time.DateOnly), p, i+1, d.Format(time.DateOnly))
	}
	news := [][2]string{
		{"Spot Bitcoin ETFs record their largest weekly inflow since March", "Large ETF inflows add steady buying pressure."},
		{"SEC delays its decision on three new crypto ETF applications", "Delays push back new demand and can weigh on sentiment."},
		{"Fed signals a pause in rate cuts", "Higher rates for longer make risk assets less attractive."},
		{"Miners sell more BTC as fees fall after the halving", "Miner selling adds supply to the market."},
		{"Large exchange halts withdrawals for four hours during a price swing", "Outages hurt trust and can deepen sell-offs."},
		{"EU's MiCA rules take full effect for stablecoin issuers", "Stablecoin rules change liquidity on European exchanges."},
		{"Bitcoin hash rate reaches a new record", "A higher hash rate signals miner confidence."},
		{"A major European bank adds Bitcoin custody for clients", "Bank custody opens the door to new institutional money."},
	}
	for i, n := range news {
		d := today.AddDate(0, 0, -8+i)
		fmt.Fprintf(&b, "INSERT INTO btc_news VALUES('https://news.example.com/crypto/%d', '%s', '%s', '%s', 'run_%02d', '%sT08:00:09Z');\n",
			1000+i, d.Format(time.DateOnly), strings.ReplaceAll(n[0], "'", "''"), strings.ReplaceAll(n[1], "'", "''"), 22+i, d.Format(time.DateOnly))
	}
	for i, k := range []string{"bitcoin", "ETF", "SEC", "Fed", "regulation"} {
		fmt.Fprintf(&b, "INSERT INTO watch_keywords VALUES(%d, '%s');\n", i+1, k)
	}
	return b.String()
}

func seed(ctx context.Context, p *project.Project) error {
	_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (sql.Result, error) {
		return tx.ExecContext(ctx, seedSQL())
	})
	return err
}

// The seeded configs: SPEC 9.2's pipeline, a weekly summary with no error
// handling (the reviewer's task), SPEC 9.3's views and the bitcoin page.
const (
	dailyBTC = `version: 1
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
    use: feed.read
    with:
      urls:
        - https://news.example.com/crypto/rss
        - https://markets.example.org/feed.xml
      match: ${{ map(steps.keywords.rows, .keyword) }}
      since: 1d
      limit: 40
  - id: top5
    use: llm.select
    with:
      items: ${{ steps.news.items }}
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
	weeklySummary = `version: 1
id: weekly_summary
description: Every Monday, a summary of the week's prices and news with the euro rate, saved as a file
trigger:
  schedule: "0 9 * * 1"
  timezone: UTC
steps:
  - id: prices
    use: db.query
    with:
      sql: SELECT date, price FROM btc_prices ORDER BY date DESC LIMIT 7
  - id: news
    use: db.query
    with:
      sql: SELECT date, title, reason FROM btc_news ORDER BY date DESC LIMIT 50
  - id: fx
    use: http.get
    with:
      url: https://api.example.com/fx/usd-eur
  - id: pages
    use: http.get
    for_each: ${{ steps.news.rows }}
    with:
      url: ${{ item.url }}
  - id: summary
    use: llm.extract
    with:
      items: ${{ steps.pages.each }}
      instruction: Summarise the week for a Bitcoin holder, with prices in USD and EUR.
      fields: { summary: string }
  - id: save
    use: bucket.put
    with:
      key: summaries/${{ today() }}.md
      content: ${{ steps.summary.items[0].summary }}
`
)

var seedViews = map[string]string{
	"btc_price_table": "version: 1\nid: btc_price_table\ntitle: Bitcoin price\ntype: table\nquery: SELECT date, price FROM btc_prices WHERE date >= :from_date\n",
	"btc_price_chart": "version: 1\nid: btc_price_chart\ntitle: Bitcoin price chart\ntype: chart\nchart_type: line\nquery: SELECT date, price FROM btc_prices\n",
	"btc_news_table":  "version: 1\nid: btc_news_table\ntitle: Bitcoin news\ntype: table\nquery: SELECT date, title, reason, url FROM btc_news\n",
	"btc_today":       "version: 1\nid: btc_today\ntitle: Today\ntype: stat\nquery: SELECT price FROM btc_prices ORDER BY date DESC LIMIT 1\n",
	"keyword_list":    "version: 1\nid: keyword_list\ntitle: News keywords\ntype: table\nquery: SELECT id, keyword FROM watch_keywords\n",
	"add_keyword":     "version: 1\nid: add_keyword\ntitle: Add news keyword\ntype: form\nsubmit: { action: insert, table: watch_keywords }\n",
}

const seedPage = "version: 1\nid: bitcoin\ntitle: Bitcoin\nrows:\n  - [btc_today, btc_price_chart]\n  - [btc_price_table, btc_news_table]\n  - [keyword_list, add_keyword]\n"

// steps are SPEC 6.5's step names.
var steps = []string{"http.get", "api.get", "http.download", "web.search", "feed.read", "html.extract", "json.extract", "transform.map", "transform.filter",
	"db.query", "db.insert", "db.insert_many", "db.update", "llm.select", "llm.extract", "bucket.put", "bucket.list", "app.notify", "script.starlark"}

// world is a run's configs and what the fake tools did. The tables are
// in the project's database.
type world struct {
	mu        sync.Mutex
	pipelines map[string]string
	views     map[string]string
	pages     map[string]string
	saved     map[string]string // pipelines saved in the run, by id
	runs      int
	conns     []string
}

func newWorld() *world {
	w := &world{pipelines: map[string]string{"daily_btc": dailyBTC, "weekly_summary": weeklySummary}, views: map[string]string{},
		pages: map[string]string{"bitcoin": seedPage}, saved: map[string]string{}}
	for k, v := range seedViews {
		w.views[k] = v
	}
	return w
}

// card is the project card: the app's card of the tables, then the
// pipelines, views and pages, which the app doesn't have yet.
func (w *world) card(ctx context.Context, p *project.Project) (string, error) {
	c, err := p.DB.Card(ctx, today)
	if err != nil {
		return "", err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var b strings.Builder
	b.WriteString(strings.TrimSpace(c) + "\n\nPipelines:")
	for _, id := range keys(w.pipelines) {
		var cfg struct {
			Description string
			Trigger     struct{ Schedule string }
		}
		_ = yaml.Unmarshal([]byte(w.pipelines[id]), &cfg)
		fmt.Fprintf(&b, "\n- %s: %s (schedule %q)", id, cfg.Description, cfg.Trigger.Schedule)
	}
	b.WriteString("\n\nViews: " + strings.Join(keys(w.views), ", "))
	b.WriteString("\n\nPages: " + strings.Join(keys(w.pages), ", "))
	return b.String(), nil
}

// cfgDoc is a parsed view, page or pipeline.
type cfgDoc struct {
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	Query   string `yaml:"query"`
	Trigger *struct {
		Schedule string `yaml:"schedule"`
	} `yaml:"trigger"`
	Steps []struct {
		ID   string         `yaml:"id"`
		Use  string         `yaml:"use"`
		With map[string]any `yaml:"with"`
	} `yaml:"steps"`
}

// check validates a config of kind ("view", "page" or "pipeline") and
// returns every error together, as save_* does (8.1).
func (w *world) check(ctx context.Context, p *project.Project, kind, text string) (cfgDoc, []string) {
	var c cfgDoc
	if err := yaml.Unmarshal([]byte(text), &c); err != nil {
		return c, []string{"the config is not valid YAML: " + err.Error()}
	}
	var errs []string
	if c.ID == "" {
		errs = append(errs, "id is missing")
	}
	switch kind {
	case "pipeline":
		if c.Trigger == nil {
			errs = append(errs, "trigger is missing")
		}
		if len(c.Steps) == 0 {
			errs = append(errs, "steps is empty")
		}
		tables, _ := p.DB.Tables(ctx)
		for i, s := range c.Steps {
			if s.ID == "" {
				errs = append(errs, fmt.Sprintf("step %d: id is missing", i+1))
			}
			if !slices.Contains(steps, s.Use) {
				errs = append(errs, fmt.Sprintf("step %s: unknown step %q", s.ID, s.Use))
			}
			if s.Use == "db.insert" || s.Use == "db.insert_many" || s.Use == "db.update" {
				t, _ := s.With["table"].(string)
				if !slices.Contains(tables, t) {
					errs = append(errs, fmt.Sprintf("step %s: no table %q; ask for it with request_schema_change", s.ID, t))
				}
			}
		}
	case "view":
		if c.Type == "" {
			errs = append(errs, "type is missing")
		}
		if c.Query == "" && c.Type != "form" {
			errs = append(errs, "query is missing")
		}
	}
	return c, errs
}

// insertTables are the tables a pipeline's steps write to.
func insertTables(text string) []string {
	var c cfgDoc
	_ = yaml.Unmarshal([]byte(text), &c)
	var out []string
	for _, s := range c.Steps {
		if strings.HasPrefix(s.Use, "db.insert") {
			if t, ok := s.With["table"].(string); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

func (w *world) store(kind string) map[string]string {
	switch kind {
	case "view":
		return w.views
	case "page":
		return w.pages
	}
	return w.pipelines
}

func errf(format string, a ...any) error { return &tool.Error{Msg: fmt.Sprintf(format, a...)} }

func schema(props string, required ...string) json.RawMessage {
	if required == nil {
		required = []string{}
	}
	r, _ := json.Marshal(required)
	return json.RawMessage(`{"type":"object","properties":{` + props + `},"required":` + string(r) + `}`)
}

type cfgArgs struct {
	Config string `json:"config"`
}

// tools are the fakes for the tools the app doesn't have yet: views and
// pipelines, the linked folder, the web and connections, the schema
// agent and task_status.
func (w *world) tools() []tool.Tool {
	cfgSchema := schema(`"config":{"type":"string","description":"The whole config as YAML."}`, "config")
	save := func(kind string) tool.Tool {
		area := tool.Views
		if kind == "pipeline" {
			area = tool.Pipelines
		}
		return tool.Func(tool.Spec{Name: "save_" + kind, Description: "Validate and save a " + kind + " config; returns all errors together.", Schema: cfgSchema,
			Effects: tool.WritesDB, Areas: area},
			func(ctx context.Context, env *tool.Env, a cfgArgs) (tool.Result, error) {
				c, errs := w.check(ctx, env.Project, kind, a.Config)
				if len(errs) > 0 {
					return tool.Result{}, errf("not saved:\n- %s", strings.Join(errs, "\n- "))
				}
				w.mu.Lock()
				defer w.mu.Unlock()
				w.store(kind)[c.ID] = a.Config
				if kind == "pipeline" {
					w.saved[c.ID] = a.Config
				}
				return tool.Result{Text: fmt.Sprintf("Saved %s %s.", kind, c.ID)}, nil
			})
	}
	type idArgs struct {
		ID string `json:"id"`
	}
	type runArgs struct {
		RunID string `json:"run_id"`
	}
	type dryArgs struct {
		Pipeline string `json:"pipeline"`
	}
	return []tool.Tool{
		save("view"), save("page"), save("pipeline"),
		tool.Func(tool.Spec{Name: "read_config", Description: "The stored view, form, page or pipeline as YAML, for editing.", Schema: schema(`"id":{"type":"string"}`, "id"),
			Effects: tool.ReadsDB, Areas: tool.Pipelines | tool.Views},
			func(_ context.Context, _ *tool.Env, a idArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				for _, k := range []string{"pipeline", "view", "page"} {
					if y, ok := w.store(k)[a.ID]; ok {
						return tool.Result{Text: y}, nil
					}
				}
				return tool.Result{}, errf("no view, page or pipeline %q", a.ID)
			}),
		tool.Func(tool.Spec{Name: "open_page", Description: "Open a page in this chat, e.g. after building or changing it.",
			Schema: schema(`"id":{"type":"string"},"filters":{"type":"object"}`, "id"), Areas: tool.Views},
			func(_ context.Context, _ *tool.Env, a idArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				if _, ok := w.pages[a.ID]; !ok {
					return tool.Result{}, errf("no page %q; pages: %s", a.ID, strings.Join(keys(w.pages), ", "))
				}
				return tool.Result{Text: "Opened page " + a.ID + " next to the chat."}, nil
			}),
		tool.Func(tool.Spec{Name: "validate", Description: "Validate a view, page or pipeline config without saving it.",
			Schema: schema(`"config":{"type":"string","description":"The whole config as YAML."},"kind":{"type":"string","enum":["view","page","pipeline"]}`, "config", "kind")},
			func(ctx context.Context, env *tool.Env, a struct {
				Config string `json:"config"`
				Kind   string `json:"kind"`
			}) (tool.Result, error) {
				if _, errs := w.check(ctx, env.Project, a.Kind, a.Config); len(errs) > 0 {
					return tool.Result{Text: "Errors:\n- " + strings.Join(errs, "\n- ")}, nil
				}
				return tool.Result{Text: "Valid."}, nil
			}),
		tool.Func(tool.Spec{Name: "dry_run", Description: "Dry-run a stored pipeline (6.8): each step runs on sample data; nothing is written.",
			Schema: schema(`"pipeline":{"type":"string","description":"The pipeline's id."},"inputs":{"type":"object"}`, "pipeline"), Effects: tool.Network, Areas: tool.Pipelines},
			func(ctx context.Context, env *tool.Env, a dryArgs) (tool.Result, error) {
				w.mu.Lock()
				y, ok := w.pipelines[a.Pipeline]
				w.mu.Unlock()
				if !ok {
					return tool.Result{}, errf("no pipeline %q", a.Pipeline)
				}
				c, errs := w.check(ctx, env.Project, "pipeline", y)
				if len(errs) > 0 {
					return tool.Result{Text: "Not run:\n- " + strings.Join(errs, "\n- ")}, nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Dry run of %s: %d steps, all succeeded on sample data.", a.Pipeline, len(c.Steps))
				for _, s := range c.Steps {
					fmt.Fprintf(&b, "\n- %s (%s): ok, 0.%ds", s.ID, s.Use, 2+len(s.ID)%7)
				}
				return tool.Result{Text: b.String()}, nil
			}),
		tool.Func(tool.Spec{Name: "run_pipeline", Description: "Start a pipeline run; returns the run ID. A finish notice follows.",
			Schema: schema(`"id":{"type":"string"},"inputs":{"type":"object"}`, "id"), Effects: tool.WritesDB | tool.Network, Areas: tool.Pipelines},
			func(_ context.Context, _ *tool.Env, a idArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				if _, ok := w.pipelines[a.ID]; !ok {
					return tool.Result{}, errf("no pipeline %q", a.ID)
				}
				w.runs++
				return tool.Result{Text: fmt.Sprintf("Started run r_%d of %s.", w.runs, a.ID)}, nil
			}),
		tool.Func(tool.Spec{Name: "run_status", Description: "A run's status and step summaries; a finish notice follows, so there is no need to poll.",
			Schema: schema(`"run_id":{"type":"string"}`, "run_id"), Effects: tool.ReadsDB, Areas: tool.Pipelines},
			func(_ context.Context, _ *tool.Env, a runArgs) (tool.Result, error) {
				return tool.Result{Text: "Run " + a.RunID + ": success, all steps ok."}, nil
			}),
		noFolder("list_files", "Files and folders under a path in the linked folder, with sizes.", `"path":{"type":"string"},"pattern":{"type":"string"}`),
		noFolder("read_file", "Lines of a text file in the linked folder.", `"path":{"type":"string"},"from":{"type":"integer"},"to":{"type":"integer"}`),
		noFolder("search_code", "Regex search in the linked folder; returns file, line and a snippet per match.", `"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string"}`),
		tool.Func(tool.Spec{Name: "web_search", Description: "Search the web; returns titles, links and snippets.",
			Schema:  schema(`"query":{"type":"string"},"limit":{"type":"integer"},"freshness":{"type":"string"},"source":{"type":"string","enum":["web","news","wikipedia","hn"]}`, "query"),
			Effects: tool.Network | tool.Untrusted},
			func(_ context.Context, _ *tool.Env, a struct {
				Query string `json:"query"`
			}) (tool.Result, error) {
				return tool.Result{Text: searchResults(a.Query)}, nil
			}),
		tool.Func(tool.Spec{Name: "fetch_page", Description: "Fetch a page as readable text.", Schema: schema(`"url":{"type":"string"}`, "url"), Effects: tool.Network | tool.Untrusted},
			func(_ context.Context, _ *tool.Env, a struct {
				URL string `json:"url"`
			}) (tool.Result, error) {
				return tool.Result{Text: page(a.URL)}, nil
			}),
		tool.Func(tool.Spec{Name: "read_feed", Description: "Read an RSS or Atom feed; returns dated items.",
			Schema: schema(`"url":{"type":"string"},"match":{"type":"array","items":{"type":"string"}}`, "url"), Effects: tool.Network | tool.Untrusted},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
				return tool.Result{Text: "3 items:\n- 2026-11-11 Ether rises 4% as staking inflows grow (https://news.example.com/crypto/1101)\n" +
					"- 2026-11-11 Bitcoin holds above $70,000 (https://news.example.com/crypto/1102)\n- 2026-11-10 New ETF filings for Solana (https://news.example.com/crypto/1103)"}, nil
			}),
		tool.Func(tool.Spec{Name: "save_link", Description: "Save a link to the Resources panel.",
			Schema: schema(`"url":{"type":"string"},"title":{"type":"string"},"note":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}`, "url"), Effects: tool.WritesDB},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
				return tool.Result{Text: "Saved the link."}, nil
			}),
		tool.Func(tool.Spec{Name: "connect_api", Description: "Propose an API connection; the user adds the key, if any, and confirms in a chat form.",
			Schema:  schema(`"id":{"type":"string"},"title":{"type":"string"},"base_url":{"type":"string"},"auth":{"type":"string"},"docs":{"type":"string"},"rate_limit":{"type":"string"}`, "id", "base_url"),
			Effects: tool.WritesDB},
			func(_ context.Context, _ *tool.Env, a struct {
				ID string `json:"id"`
			}) (tool.Result, error) {
				w.mu.Lock()
				w.conns = append(w.conns, a.ID)
				w.mu.Unlock()
				return tool.Result{Text: "The user confirmed the connection " + a.ID + "."}, nil
			}),
		tool.Func(tool.Spec{Name: "call_api", Description: "One GET through a saved connection; returns the response.",
			Schema: schema(`"connection":{"type":"string"},"path":{"type":"string"},"query":{"type":"object"}`, "connection", "path"), Effects: tool.Network | tool.Untrusted},
			func(_ context.Context, _ *tool.Env, a struct {
				Connection string `json:"connection"`
				Path       string `json:"path"`
			}) (tool.Result, error) {
				w.mu.Lock()
				ok := slices.Contains(w.conns, a.Connection)
				w.mu.Unlock()
				if !ok {
					return tool.Result{}, errf("no connection %q; propose one with connect_api", a.Connection)
				}
				return tool.Result{Text: `{"ethereum":{"usd":3121.55},"bitcoin":{"usd":70412.10}}`}, nil
			}),
		tool.Func(tool.Spec{Name: "request_schema_change", Description: "Hand a schema request to the schema agent, which plans and applies the migration (8.2).",
			Schema: schema(`"goal":{"type":"string","description":"What the schema should hold, in plain words."}`, "goal"), Effects: tool.Schema | tool.WritesDB},
			func(ctx context.Context, env *tool.Env, a struct {
				Goal string `json:"goal"`
			}) (tool.Result, error) {
				return schemaChange(ctx, env.Project, a.Goal)
			}),
		tool.Func(tool.Spec{Name: "task_status", Description: "Status of a background task; a finish notice follows, so there is no need to poll.",
			Schema: schema(`"id":{"type":"string"}`, "id")},
			func(_ context.Context, _ *tool.Env, a idArgs) (tool.Result, error) {
				return tool.Result{Text: "Task " + a.ID + ": running. A finish notice follows."}, nil
			}),
	}
}

func noFolder(name, desc, props string) tool.Tool {
	return tool.Func(tool.Spec{Name: name, Description: desc, Schema: schema(props), Effects: tool.Workspace},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
			return tool.Result{}, errf("no folder is linked to this project; the project's own files are in the bucket")
		})
}

// schemaChange is the fake schema agent: a goal about Ethereum gets a
// real eth_prices table; any other goal changes nothing.
func schemaChange(ctx context.Context, p *project.Project, goal string) (tool.Result, error) {
	low := strings.ToLower(goal)
	if !strings.Contains(low, "eth") {
		return tool.Result{Text: "The schema agent made no change: the tables already hold what this goal needs."}, nil
	}
	tables, err := p.DB.Tables(ctx)
	if err != nil {
		return tool.Result{}, err
	}
	if slices.Contains(tables, "eth_prices") {
		return tool.Result{Text: "No change: eth_prices(date TEXT PRIMARY KEY, price REAL, _run_id TEXT, _fetched_at TEXT) already exists."}, nil
	}
	_, err = store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (sql.Result, error) {
		return tx.ExecContext(ctx, "CREATE TABLE eth_prices(date TEXT PRIMARY KEY, price REAL, _run_id TEXT, _fetched_at TEXT)")
	})
	if err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Text: "Applied: created eth_prices(date TEXT PRIMARY KEY, price REAL, _run_id TEXT, _fetched_at TEXT)."}, nil
}

// apis are the price APIs the web fakes know.
var apis = []struct{ Name, URL, Text string }{
	{"CoinGecko", "https://docs.coingecko.com/reference/introduction", "CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). " +
		"Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd."},
	{"CoinCap", "https://docs.coincap.io/", "CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. " +
		"History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum."},
	{"Binance", "https://developers.binance.com/docs/binance-spot-api-docs/rest-api", "Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. " +
		"Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD."},
}

func searchResults(q string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Results for %q:", q)
	for _, a := range apis {
		fmt.Fprintf(&b, "\n- %s API documentation (%s): %s", a.Name, a.URL, clip(a.Text, 120))
	}
	b.WriteString("\n- Best free crypto price APIs in 2026 (https://blog.example.com/crypto-apis): a comparison of CoinGecko, CoinCap, Binance and others.")
	return b.String()
}

func page(url string) string {
	low := strings.ToLower(url)
	for _, a := range apis {
		if strings.Contains(low, strings.ToLower(a.Name)) {
			return a.Text
		}
	}
	if strings.Contains(low, "blog.example.com") {
		var b strings.Builder
		b.WriteString("Best free crypto price APIs in 2026.")
		for _, a := range apis {
			b.WriteString("\n\n" + a.Name + ": " + a.Text)
		}
		return b.String()
	}
	return "The page has no readable text."
}
