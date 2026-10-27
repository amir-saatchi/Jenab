# L03_add_column

Add an optional column volume (a number) to btc_prices for the daily trading volume.

Good behaviour: Load migrations before the first write; no skills outside migrations, database-design.

## Turn 1

**User:** Add an optional column volume (a number) to btc_prices for the daily trading volume.

> call load_skill {"name":"migrations"}

> call call_tool→describe_tool {"name":"get_config"}

*Skill loaded: migrations*

> result load_skill:
> 

> result describe_tool (error):
> there is no tool named "describe_tool". The tools in the index: ask_user, bucket_delete, bucket_list, bucket_put, bucket_read, get_config, list_chats, read_ref, save_form, save_pipeline, save_view, search_ref, web_search

> call describe_tool {"name":"get_config"}

> result describe_tool:
> get_config
> Read the saved YAML of a view or pipeline by id.
> 
> Arguments (JSON Schema):
> {"properties":{"id":{"type":"string"}},"required":["id"],"type":"object"}

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

**Agent:** I've added the optional `volume` column (number) to the `btc_prices` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2438 | 0 | 0 | 419 | tool_use | 2.4 s |  |
| 2 | 3530 | 0 | 0 | 225 | tool_use | 1.5 s |  |
| 3 | 3812 | 0 | 0 | 123 | tool_use | 0.9 s |  |
| 4 | 4634 | 0 | 0 | 480 | tool_use | 2.3 s |  |
| 5 | 5139 | 0 | 0 | 37 | end | 0.8 s |  |
