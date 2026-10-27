# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"config-guide"}

> call load_skill {"name":"sql-queries"}

*Skill loaded: config-guide*

> result load_skill:
> 

*Skill loaded: sql-queries*

> result load_skill:
> 

> call save_view {"yaml":"version: 1\nid: btc_news_daily_count\ntitle: News Items per Day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT date, count(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Items\" }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've created the bar chart `btc_news_daily_count` which shows the number of saved news items per day.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3068 | 0 | 0 | 177 | tool_use | 4.2 s |  |
| 2 | 5962 | 0 | 0 | 253 | tool_use | 2.0 s |  |
| 3 | 6237 | 0 | 0 | 56 | end | 1.2 s |  |
