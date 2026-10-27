# L02_schedule

Change the daily_btc pipeline so it runs at 07:30 UTC instead of 08:00.

Good behaviour: Load pipelines before the first write; no skills outside pipelines.

## Turn 1

**User:** Change the daily_btc pipeline so it runs at 07:30 UTC instead of 08:00.

> call describe_tool {"name":"get_config"}

> call describe_tool {"name":"save_pipeline"}

> result describe_tool:
> get_config
> Read the saved YAML of a view or pipeline by id.
> 
> Arguments (JSON Schema):
> {"properties":{"id":{"type":"string"}},"required":["id"],"type":"object"}

> result describe_tool:
> save_pipeline
> Validate and save a pipeline config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete pipeline config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→get_config {"id":"daily_btc"}

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

> call call_tool→save_pipeline {"yaml":"version: 1\nid: daily_btc\ndescription: Daily BTC price and the 5 news items most likely to affect it\ntrigger:\n  schedule: \"30 7 * * *\"\n  timezone: UTC\n  catch_up: once\n  manual: true\nsteps:\n  - id: price\n    use: http.get\n    with:\n      url: https://api.example.com/btc/daily\n\n  - id: save_price\n    use: db.insert\n    with:\n      table: btc_prices\n      on_conflict: upsert\n      row:\n        date: ${{ today() }}\n        price: ${{ steps.price.json.close }}\n\n  - id: keywords\n    use: db.query\n    with:\n      sql: SELECT keyword FROM watch_keywords\n\n  - id: …

> result save_pipeline:
> saved pipeline (revision 2).

**Agent:** The `daily_btc` pipeline has been updated to run at 07:30 UTC.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2136 | 0 | 0 | 236 | tool_use | 1.9 s |  |
| 2 | 2501 | 0 | 0 | 40 | tool_use | 0.9 s |  |
| 3 | 2973 | 0 | 0 | 915 | tool_use | 3.5 s |  |
| 4 | 0 | 0 | 0 | 0 |  | 75.4 s | ollama-cloud: transport: Post "https://ollama.com/api/chat": unexpected EOF |
| 5 | 3910 | 0 | 0 | 31 | end | 1.0 s |  |
