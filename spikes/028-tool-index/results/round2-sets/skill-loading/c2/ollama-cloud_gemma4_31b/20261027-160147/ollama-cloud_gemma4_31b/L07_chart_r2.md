# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_view {"views":["version: 1\nid: btc_news_daily_count\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT date, COUNT(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"]}

> result save_view (error):
> there is no tool named save_view

> call call_tool→save_view {"views":["version: 1\nid: btc_news_daily_count\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT date, COUNT(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"]}

> result save_view (error):
> wrong arguments: save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: btc_news_daily_count\ntitle: News items per day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT date, COUNT(*) as count FROM btc_news GROUP BY date ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Count\" }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've created a bar chart view called "News items per day" (`btc_news_daily_count`) that counts the number of news items for each date in the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2432 | 0 | 0 | 182 | tool_use | 1.5 s |  |
| 2 | 5236 | 0 | 0 | 370 | tool_use | 1.9 s |  |
| 3 | 5665 | 0 | 0 | 151 | tool_use | 1.0 s |  |
| 4 | 5854 | 0 | 0 | 58 | tool_use | 1.1 s |  |
| 5 | 5988 | 0 | 0 | 129 | tool_use | 1.3 s |  |
| 6 | 6139 | 0 | 0 | 58 | end | 1.2 s |  |
