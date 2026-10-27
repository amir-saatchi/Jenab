# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"sql-queries"}

*Skill loaded: sql-queries*

> result load_skill:
> 

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

> call call_tool→save_view {"config":"version: 1\nid: news_last_week\ntitle: News last week\ntype: table\nquery: SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\" }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view (error):
> wrong arguments: save_view takes no argument config. save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call call_tool→save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News last week\ntype: table\nquery: SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\" }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the `news_last_week` table view, showing news from the last 7 days (date, title, and link), sorted by date descending.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2140 | 0 | 0 | 319 | tool_use | 2.8 s |  |
| 2 | 2557 | 0 | 0 | 179 | tool_use | 2.1 s |  |
| 3 | 2783 | 0 | 0 | 44 | tool_use | 1.3 s |  |
| 4 | 2962 | 0 | 0 | 434 | tool_use | 4.1 s |  |
| 5 | 3442 | 0 | 0 | 153 | tool_use | 3.4 s |  |
| 6 | 3617 | 0 | 0 | 46 | end | 1.3 s |  |
