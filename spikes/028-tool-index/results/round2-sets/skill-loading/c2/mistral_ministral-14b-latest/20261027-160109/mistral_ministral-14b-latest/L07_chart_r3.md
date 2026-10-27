# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call query {"sql":"SELECT date, COUNT(*) as count FROM btc_news GROUP BY date"}

> result query:
> date | count
> 2026-10-02 | 1
> 2026-10-03 | 1
> 2026-10-04 | 1
> 2026-10-05 | 1
> (4 rows)

**Agent:** Here is the YAML configuration for a bar chart showing the number of saved news items per day:

```yaml
version: 1
id: btc_news_per_day
title: "Bitcoin News Items per Day"
type: chart
chart_type: bar
query: |
  SELECT date, COUNT(*) as count
  FROM btc_news
  GROUP BY date
x:
  field: date
  label: "Date"
  format: date
y:
  - field: count
    label: "Number of News Items"
```

I will now save this configuration.

> call save_view {"version":"1","id":"btc_news_per_day","title":"Bitcoin News Items per Day","type":"chart","chart_type":"bar","query":"SELECT date, COUNT(*) as count FROM btc_news GROUP BY date","x":{"field":"date","label":"Date","format":"date"},"y":[{"field":"count","label":"Number of News Items"}]}

> result save_view (error):
> there is no tool named save_view

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: btc_news_per_day\ntitle: \"Bitcoin News Items per Day\"\ntype: chart\nchart_type: bar\nquery: SELECT date, COUNT(*) as count FROM btc_news GROUP BY date\nx:\n  field: date\n  label: \"Date\"\n  format: date\ny:\n  - field: count\n    label: \"Number of News Items\""}

> result save_view:
> saved view (revision 1).

**Agent:** The bar chart **"Bitcoin News Items per Day"** has been saved successfully. You can now add it to a page or view it directly if it appears in the list of views.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 39 | 2560 | 0 | 13 | tool_use | 1.1 s |  |
| 2 | 4985 | 128 | 0 | 24 | tool_use | 1.4 s |  |
| 3 | 5204 | 0 | 0 | 230 | tool_use | 4.9 s |  |
| 4 | 356 | 5120 | 0 | 13 | tool_use | 1.4 s |  |
| 5 | 433 | 5120 | 0 | 113 | tool_use | 3.3 s |  |
| 6 | 3117 | 2560 | 0 | 38 | end | 1.7 s |  |
