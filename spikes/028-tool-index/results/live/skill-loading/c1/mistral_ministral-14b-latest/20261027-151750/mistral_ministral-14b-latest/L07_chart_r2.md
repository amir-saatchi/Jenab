# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call get_config {"id":"btc_news_table"}

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

**Agent:** To create a bar chart showing the number of saved news items per day, I will:

1. Create a query that groups news items by date and counts them.
2. Save a new chart view with this query.

Here is the query and view config:

```yaml
version: 1
id: btc_news_per_day
title: News per day
type: chart
chart_type: bar
query: |
  SELECT
    date,
    COUNT(*) AS count
  FROM btc_news
  GROUP BY date
  ORDER BY date
x: { field: date, label: "Date", format: date }
y: [{ field: count, label: "Number of news items" }]
```

I will now save this view.

> call save_view {"yaml":"version: 1\nid: btc_news_per_day\ntitle: News per day\ntype: chart\nchart_type: bar\nquery: |\n  SELECT\n    date,\n    COUNT(*) AS count\n  FROM btc_news\n  GROUP BY date\n  ORDER BY date\nx: { field: date, label: \"Date\", format: date }\ny: [{ field: count, label: \"Number of news items\" }]"}

> result save_view:
> saved view (revision 1).

**Agent:** The bar chart view **"News per day"** has been saved successfully.

You can now add it to a page or use it directly. Would you like to add it to an existing page or create a new one?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 117 | 3456 | 0 | 13 | tool_use | 0.9 s |  |
| 2 | 71 | 6016 | 0 | 15 | tool_use | 0.8 s |  |
| 3 | 6089 | 128 | 0 | 262 | tool_use | 3.2 s |  |
| 4 | 3035 | 3456 | 0 | 44 | end | 1.3 s |  |
