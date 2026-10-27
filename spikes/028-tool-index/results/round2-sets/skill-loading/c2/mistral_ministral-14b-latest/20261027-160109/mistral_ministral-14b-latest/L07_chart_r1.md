# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call query {"sql":"SELECT date, COUNT(*) as count FROM btc_news GROUP BY date ORDER BY date"}

> result query:
> date | count
> 2026-10-02 | 1
> 2026-10-03 | 1
> 2026-10-04 | 1
> 2026-10-05 | 1
> (4 rows)

**Agent:** Here’s the YAML config for a bar chart showing the number of saved news items per day:

```yaml
version: 1
id: btc_news_per_day
title: "News Items per Day"
type: chart
chart_type: bar
query: |
  SELECT date, COUNT(*) as count
  FROM btc_news
  GROUP BY date
  ORDER BY date
x:
  field: date
  label: "Date"
  format: date
y:
  - field: count
    label: "Number of Items"
```

I’ll save this as a new view.

> call save_view {"version":"1","id":"btc_news_per_day","title":"News Items per Day","type":"chart","chart_type":"bar","query":"SELECT date, COUNT(*) as count FROM btc_news GROUP BY date ORDER BY date","x":{"field":"date","label":"Date","format":"date"},"y":[{"field":"count","label":"Number of Items"}]}

> result save_view (error):
> there is no tool named save_view

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: btc_news_per_day\ntitle: News Items per Day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT date, COUNT(*) as count\n  FROM btc_news\n  GROUP BY date\n  ORDER BY date\nx:\n  field: date\n  label: Date\n  format: date\ny:\n  - field: count\n    label: Number of Items"}

> result save_view:
> saved view (revision 1).

**Agent:** The bar chart view **"News Items per Day"** has been saved successfully.

You can now add it to a page or use it directly.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 39 | 2560 | 0 | 13 | tool_use | 1.4 s |  |
| 2 | 2553 | 2560 | 0 | 27 | tool_use | 1.8 s |  |
| 3 | 2647 | 2560 | 0 | 234 | tool_use | 4.8 s |  |
| 4 | 2923 | 2560 | 0 | 13 | tool_use | 1.0 s |  |
| 5 | 3000 | 2560 | 0 | 116 | tool_use | 3.8 s |  |
| 6 | 5687 | 0 | 0 | 29 | end | 1.6 s |  |
