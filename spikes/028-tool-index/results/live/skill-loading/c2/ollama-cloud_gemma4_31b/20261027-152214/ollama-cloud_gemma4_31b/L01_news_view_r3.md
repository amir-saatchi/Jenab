# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

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

> call load_skill {"name":"sql-queries"}

*Skill loaded: sql-queries*

> result load_skill:
> 

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

> call call_tool {"arguments":{"arguments":{"config":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\" }\n  - { field: url, label: \"Link\", format: url }\n"},"name":"save_view"},"name":"call_tool"}

> result call_tool (error):
> there is no tool named "call_tool". The tools in the index: ask_user, bucket_delete, bucket_list, bucket_put, bucket_read, get_config, list_chats, read_messages, read_ref, save_form, save_pipeline, save_view, search_history, search_ref, web_search

> call call_tool {"arguments":{"arguments":{"config":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\" }\n  - { field: url, label: \"Link\", format: url }\n"},"name":"save_view"},"name":"call_tool"}

> result call_tool (error):
> there is no tool named "call_tool". The tools in the index: ask_user, bucket_delete, bucket_list, bucket_put, bucket_read, get_config, list_chats, read_messages, read_ref, save_form, save_pipeline, save_view, search_history, search_ref, web_search

*[the model sent an empty answer]*

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2140 | 0 | 0 | 183 | tool_use | 1.6 s |  |
| 2 | 2385 | 0 | 0 | 62 | tool_use | 1.1 s |  |
| 3 | 2544 | 0 | 0 | 519 | tool_use | 3.4 s |  |
| 4 | 3198 | 0 | 0 | 377 | tool_use | 2.1 s |  |
| 5 | 3665 | 0 | 0 | 368 | tool_use | 2.2 s |  |
| 6 | 4123 | 0 | 0 | 461 | end | 2.6 s |  |
