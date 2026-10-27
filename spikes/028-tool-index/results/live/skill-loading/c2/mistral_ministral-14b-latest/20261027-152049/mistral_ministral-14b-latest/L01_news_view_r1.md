# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"sql-queries"}

*Skill loaded: sql-queries*

> result load_skill:
> 

> call describe_table {"name":"btc_news"}

> result describe_table:
> table btc_news
> - url TEXT (primary key 1)
> - date TEXT
> - title TEXT
> - reason TEXT
> - _run_id TEXT
> - _fetched_at TEXT
> rows: 4

> call query {"sql":"SELECT date FROM btc_news ORDER BY date DESC LIMIT 1"}

> result query:
> date
> 2026-10-05
> (1 rows)

> call save_view {"type":"string"}

> result save_view (error):
> there is no tool named save_view

> call call_tool→save_view {"config":"type: table\nid: news_last_week\ntitle: \"Bitcoin news last week\"\nquery: \"SELECT date, title, url as link FROM btc_news WHERE date \u003e= date('now', '-7 days') ORDER BY date DESC\""}

> result save_view (error):
> wrong arguments: save_view takes no argument config. save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"type: table\nid: news_last_week\ntitle: \"Bitcoin news last week\"\nquery: \"SELECT date, title, url as link FROM btc_news WHERE date \u003e= date('now', '-7 days') ORDER BY date DESC\""}

> result save_view:
> saved view (revision 1).

**Agent:** The view **news_last_week** has been created and shows the Bitcoin news from the last 7 days, ordered by date (newest first). It includes the columns: **date, title, and link**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2080 | 128 | 0 | 14 | tool_use | 1.0 s |  |
| 2 | 2178 | 128 | 0 | 14 | tool_use | 0.6 s |  |
| 3 | 2239 | 128 | 0 | 23 | tool_use | 0.6 s |  |
| 4 | 0 | 0 | 0 | 0 |  | 0.3 s | mistral: rate_limited (429): {"object":"error","message":"Rate limit exceeded","type":"rate_limited","param":null,"code"… |
| 5 | 2282 | 128 | 0 | 11 | tool_use | 0.6 s |  |
| 6 | 2303 | 128 | 0 | 75 | tool_use | 1.2 s |  |
| 7 | 234 | 2304 | 0 | 13 | tool_use | 0.5 s |  |
| 8 | 2487 | 128 | 0 | 75 | tool_use | 1.1 s |  |
| 9 | 2573 | 128 | 0 | 44 | end | 1.3 s |  |
