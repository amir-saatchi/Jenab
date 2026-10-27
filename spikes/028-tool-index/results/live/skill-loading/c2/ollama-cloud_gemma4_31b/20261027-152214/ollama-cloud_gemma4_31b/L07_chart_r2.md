# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call query {"sql":"SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date DESC"}

> result query:
> date | count
> 2026-10-05 | 1
> 2026-10-04 | 1
> 2026-10-03 | 1
> 2026-10-02 | 1
> (4 rows)

> call call_tool→get_config {"name":"get_config"}

> result get_config (error):
> wrong arguments: get_config takes no argument name. get_config needs id. Its arguments: id (required). describe_tool gives their schema.

> call call_tool→get_config {"id":"btc_price_chart"}

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

> call call_tool→save_view {"config":"version: 1\nid: btc_news_count_chart\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"}

> result save_view (error):
> wrong arguments: save_view takes no argument config. save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call call_tool→save_view {"yaml":"version: 1\nid: btc_news_count_chart\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've created a bar chart called "News items per day" (`btc_news_count_chart`) that shows the number of news items saved for each date.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2124 | 0 | 0 | 208 | tool_use | 1.7 s |  |
| 2 | 2410 | 0 | 0 | 107 | tool_use | 1.1 s |  |
| 3 | 2564 | 0 | 0 | 55 | tool_use | 1.0 s |  |
| 4 | 2712 | 0 | 0 | 287 | tool_use | 19.0 s |  |
| 5 | 3045 | 0 | 0 | 128 | tool_use | 1.6 s |  |
| 6 | 3195 | 0 | 0 | 46 | end | 3.9 s |  |
