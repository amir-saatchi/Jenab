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

Here is the query and view configuration:

> call save_view {"yaml":"version: 1\nid: btc_news_per_day\ntitle: Bitcoin News per Day\ndescription: Number of Bitcoin news items saved each day\nquery: |\n  SELECT \n    date,\n    COUNT(*) as count\n  FROM btc_news\n  GROUP BY date\n  ORDER BY date\nchart_type: bar\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Number of News Items\" }"}

> result save_view:
> saved view (revision 1).

**Agent:** The bar chart **"Bitcoin News per Day"** has been created and saved. You can now add it to a page or use it directly.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 117 | 3456 | 0 | 13 | tool_use | 0.7 s |  |
| 2 | 2631 | 3456 | 0 | 15 | tool_use | 0.8 s |  |
| 3 | 2761 | 3456 | 0 | 169 | tool_use | 2.3 s |  |
| 4 | 2942 | 3456 | 0 | 30 | end | 0.9 s |  |
