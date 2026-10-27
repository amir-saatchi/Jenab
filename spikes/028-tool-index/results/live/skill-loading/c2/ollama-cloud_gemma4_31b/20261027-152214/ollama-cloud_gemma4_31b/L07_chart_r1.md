# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call describe_table {"name":"btc_news"}

> call call_tool→get_config {"id":"btc_price_chart"}

> result describe_table:
> table btc_news
> - url TEXT (primary key 1)
> - date TEXT
> - title TEXT
> - reason TEXT
> - _run_id TEXT
> - _fetched_at TEXT
> rows: 4

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

> call call_tool {"arguments":{"arguments":{"config":"version: 1\nid: btc_news_count_chart\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"},"name":"save_view"},"name":"call_tool"}

> result call_tool (error):
> there is no tool named "call_tool". The tools in the index: ask_user, bucket_delete, bucket_list, bucket_put, bucket_read, get_config, list_chats, read_messages, read_ref, save_form, save_pipeline, save_view, search_history, search_ref, web_search

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool {"arguments":{"arguments":{"yaml":"version: 1\nid: btc_news_count_chart\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"},"name":"save_view"},"name":"call_tool"}

> result call_tool (error):
> there is no tool named "call_tool". The tools in the index: ask_user, bucket_delete, bucket_list, bucket_put, bucket_read, get_config, list_chats, read_messages, read_ref, save_form, save_pipeline, save_view, search_history, search_ref, web_search

> call call_tool→save_view {"yaml":"version: 1\nid: btc_news_count_chart\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I have created a bar chart named "News items per day" that shows the number of saved news items per day.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2124 | 0 | 0 | 450 | tool_use | 3.2 s |  |
| 2 | 2727 | 0 | 0 | 262 | tool_use | 1.4 s |  |
| 3 | 3079 | 0 | 0 | 213 | tool_use | 1.4 s |  |
| 4 | 3368 | 0 | 0 | 135 | tool_use | 1.0 s |  |
| 5 | 3593 | 0 | 0 | 507 | tool_use | 2.4 s |  |
| 6 | 4122 | 0 | 0 | 34 | end | 7.8 s |  |
