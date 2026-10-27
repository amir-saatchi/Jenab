# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"sql-queries"}

> call describe_tool {"name":"save_view"}

*Skill loaded: sql-queries*

> result load_skill:
> 

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→get_config {"name":"btc_news_table"}

> result get_config (error):
> wrong arguments: get_config takes no argument name. get_config needs id. Its arguments: id (required). describe_tool gives their schema.

> call call_tool→get_config {"id":"btc_news_table"}

> result get_config:
> version: 1
> id: btc_news_table
> title: Bitcoin news
> type: table
> query: SELECT date, title, reason, url FROM btc_news
> default_sort: { field: date, direction: desc }
> columns:
>   - { field: date, label: "Date", format: date }
>   - { field: title, label: "Title" }
>   - { field: reason, label: "Why it matters" }
>   - { field: url, label: "Link", format: url }
> 

> call call_tool→save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\" }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've created the `news_last_week` table view, which shows the date, title, and link of news from the last 7 days, sorted by the newest first.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2140 | 0 | 0 | 349 | tool_use | 2.5 s |  |
| 2 | 2661 | 0 | 0 | 96 | tool_use | 1.3 s |  |
| 3 | 2804 | 0 | 0 | 53 | tool_use | 1.0 s |  |
| 4 | 2992 | 0 | 0 | 494 | tool_use | 2.3 s |  |
| 5 | 3508 | 0 | 0 | 48 | end | 1.0 s |  |
