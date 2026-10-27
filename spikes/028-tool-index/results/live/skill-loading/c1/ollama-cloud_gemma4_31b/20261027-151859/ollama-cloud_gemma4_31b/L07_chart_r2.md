# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"sql-queries"}

> call load_skill {"name":"config-guide"}

*Skill loaded: sql-queries*

> result load_skill:
> 

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_view {"yaml":"version: 1\nid: btc_news_daily_count\ntitle: News Items per Day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT date, COUNT(*) as count\n  FROM btc_news\n  GROUP BY date\n  ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Items\" }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've created a bar chart view `btc_news_daily_count` that shows the number of news items per day.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3068 | 0 | 0 | 175 | tool_use | 2.7 s |  |
| 2 | 5960 | 0 | 0 | 301 | tool_use | 1.5 s |  |
| 3 | 6283 | 0 | 0 | 41 | end | 0.8 s |  |
