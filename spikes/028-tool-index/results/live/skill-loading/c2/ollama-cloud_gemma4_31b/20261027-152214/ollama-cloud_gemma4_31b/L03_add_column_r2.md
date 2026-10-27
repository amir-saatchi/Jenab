# L03_add_column

Add an optional column volume (a number) to btc_prices for the daily trading volume.

Good behaviour: Load migrations before the first write; no skills outside migrations, database-design.

## Turn 1

**User:** Add an optional column volume (a number) to btc_prices for the daily trading volume.

> call load_skill {"name":"migrations"}

*Skill loaded: migrations*

> result load_skill:
> 

> call call_tool→get_config {"id":"btc_price_table"}

> call call_tool→get_config {"id":"btc_price_chart"}

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
> id: btc_price_chart
> title: Bitcoin price chart
> type: chart
> chart_type: line
> query: SELECT date, price FROM btc_prices
> x: { field: date, label: "Date", format: date }
> y:
>   - { field: price, label: "Price (USD)" }
> 

> call apply_migration {"steps":[{"column":{"name":"volume","type":"REAL"},"op":"add_column","table":"btc_prices"}]}

> result apply_migration:
> applied: migration committed, schema version 1.

**Agent:** Added the optional column `volume` (REAL) to the `btc_prices` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2130 | 0 | 0 | 385 | tool_use | 3.0 s |  |
| 2 | 3108 | 0 | 0 | 263 | tool_use | 1.7 s |  |
| 3 | 3641 | 0 | 0 | 78 | tool_use | 0.9 s |  |
| 4 | 3744 | 0 | 0 | 28 | end | 0.7 s |  |
