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
2. Save this as a new chart view.

Here is the configuration for the chart view:

> call save_view {"yaml":"version: 1\nid: btc_news_per_day\ntitle: Bitcoin News per Day\ndescription: Number of news items saved per day\nquery: |\n  SELECT date, COUNT(*) as count\n  FROM btc_news\n  GROUP BY date\n  ORDER BY date\n  \ntype: chart\nchart_type: bar\nx: { field: date, label: \"Date\", format: date }\ny:\n  - { field: count, label: \"Number of News Items\" }"}

> result save_view:
> saved view (revision 1).

**Agent:** The bar chart **"Bitcoin News per Day"** has been created and shows the number of saved news items per day. You can add it to a page or view it directly.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 117 | 3456 | 0 | 13 | tool_use | 1.4 s |  |
| 2 | 2631 | 3456 | 0 | 15 | tool_use | 0.9 s |  |
| 3 | 2761 | 3456 | 0 | 170 | tool_use | 1.9 s |  |
| 4 | 255 | 6144 | 0 | 37 | end | 1.6 s |  |
