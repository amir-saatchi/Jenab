# L03_add_column

Add an optional column volume (a number) to btc_prices for the daily trading volume.

Good behaviour: Load migrations before the first write; no skills outside migrations, database-design.

## Turn 1

**User:** Add an optional column volume (a number) to btc_prices for the daily trading volume.

> call load_skill {"name":"migrations"}

*Skill loaded: migrations*

> result load_skill:
> 

> call call_tool→get_config {"name":"btc_price_chart"}

> call call_tool→get_config {"name":"btc_price_table"}

> call call_tool→get_config {"name":"daily_btc"}

> result get_config (error):
> wrong arguments: get_config takes no argument name. get_config needs id. Its arguments: id (required). describe_tool gives their schema.

> result get_config (error):
> wrong arguments: get_config takes no argument name. get_config needs id. Its arguments: id (required). describe_tool gives their schema.

> result get_config (error):
> wrong arguments: get_config takes no argument name. get_config needs id. Its arguments: id (required). describe_tool gives their schema.

> call call_tool→get_config {"id":"btc_price_chart"}

> call call_tool→get_config {"id":"btc_price_table"}

> call call_tool→get_config {"id":"daily_btc"}

> result get_config:
> version: 1
> id: btc_price_chart
> title: Bitcoin price chart
> type: chart
> chart_type: line
> query: SELECT date, price FROM btc_prices
> x: { field: date, label: "Date", format: date }
> y:
>   - { field: price, label: "Price (USD)" }
> 

> result get_config:
> version: 1
> id: btc_price_table
> title: Bitcoin price
> type: table
> query: |
>   SELECT date, price FROM btc_prices
>   WHERE date >= :from_date
> filters:
>   - { param: from_date, control: date, label: "From", default: "today-90d", required: true }
> default_sort: { field: date, direction: desc }
> columns:
>   - { field: date, label: "Date", format: date }
>   - { field: price, label: "Price", format: currency, format_options: { currency: USD } }
> actions:
>   - { label: "Refresh now", run_pipeline: daily_btc }
> 

> result get_config:
> version: 1
> id: daily_btc
> description: Daily BTC price and the 5 news items most likely to affect it
> trigger:
>   schedule: "0 8 * * *"
>   timezone: UTC
>   catch_up: once
>   manual: true
> steps:
>   - id: price
>     use: http.get
>     with:
>       url: https://api.example.com/btc/daily
> 
>   - id: save_price
>     use: db.insert
>     with:
>       table: btc_prices
>       on_conflict: upsert
>       row:
>         date: ${{ today() }}
>         price: ${{ steps.price.json.close }}
> 
>   - id: keywords
>     use: db.query
>     with:
>       sql: SELECT keyword FROM watch_keywords
> 
>   - id: news
>     use: web.search
>     with:
>      …

> call apply_migration {"steps":[{"column":{"name":"volume","type":"REAL"},"op":"add_column","table":"btc_prices"}]}

> result apply_migration:
> applied: migration committed, schema version 1.

**Agent:** Added the optional `volume` column (type `REAL`) to the `btc_prices` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2130 | 0 | 0 | 303 | tool_use | 2.0 s |  |
| 2 | 3026 | 0 | 0 | 208 | tool_use | 1.3 s |  |
| 3 | 3368 | 0 | 0 | 221 | tool_use | 2.1 s |  |
| 4 | 4288 | 0 | 0 | 162 | tool_use | 1.2 s |  |
| 5 | 4475 | 0 | 0 | 36 | end | 0.7 s |  |
