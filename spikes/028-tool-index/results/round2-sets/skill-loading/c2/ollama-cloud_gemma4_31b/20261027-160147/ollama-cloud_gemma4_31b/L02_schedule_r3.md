# L02_schedule

Change the daily_btc pipeline so it runs at 07:30 UTC instead of 08:00.

Good behaviour: Load pipelines before the first write; no skills outside pipelines.

## Turn 1

**User:** Change the daily_btc pipeline so it runs at 07:30 UTC instead of 08:00.

> call load_skill {"name":"pipelines"}

> call call_tool→get_config {"name":"daily_btc"}

*Skill loaded: pipelines*

> result load_skill:
> 

> result get_config (error):
> wrong arguments: get_config needs id. Its arguments: id (required). describe_tool gives their schema.

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

> call call_tool→save_pipeline {"config":"version: 1\nid: daily_btc\ndescription: Daily BTC price and the 5 news items most likely to affect it\ntrigger:\n  schedule: \"30 7 * * *\"\n  timezone: UTC\n  catch_up: once\n  manual: true\nsteps:\n  - id: price\n    use: http.get\n    with:\n      url: https://api.example.com/btc/daily\n\n  - id: save_price\n    use: db.insert\n    with:\n      table: btc_prices\n      on_conflict: upsert\n      row:\n        date: ${{ today() }}\n        price: ${{ steps.price.json.close }}\n\n  - id: keywords\n    use: db.query\n    with:\n      sql: SELECT keyword FROM watch_keywords\n\n  - id…

> result save_pipeline (error):
> wrong arguments: save_pipeline needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call call_tool→save_pipeline {"yaml":"version: 1\nid: daily_btc\ndescription: Daily BTC price and the 5 news items most likely to affect it\ntrigger:\n  schedule: \"30 7 * * *\"\n  timezone: UTC\n  catch_up: once\n  manual: true\nsteps:\n  - id: price\n    use: http.get\n    with:\n      url: https://api.example.com/btc/daily\n\n  - id: save_price\n    use: db.insert\n    with:\n      table: btc_prices\n      on_conflict: upsert\n      row:\n        date: ${{ today() }}\n        price: ${{ steps.price.json.close }}\n\n  - id: keywords\n    use: db.query\n    with:\n      sql: SELECT keyword FROM watch_keywords\n\n  - id: …

> result save_pipeline:
> saved pipeline (revision 2).

**Agent:** The `daily_btc` pipeline has been updated to run at 07:30 UTC.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2444 | 0 | 0 | 182 | tool_use | 1.4 s |  |
| 2 | 4245 | 0 | 0 | 63 | tool_use | 0.8 s |  |
| 3 | 4739 | 0 | 0 | 502 | tool_use | 2.2 s |  |
| 4 | 5279 | 0 | 0 | 462 | tool_use | 2.4 s |  |
| 5 | 5763 | 0 | 0 | 41 | end | 0.8 s |  |
