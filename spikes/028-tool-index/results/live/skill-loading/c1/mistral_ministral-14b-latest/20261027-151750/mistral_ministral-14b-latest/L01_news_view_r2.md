# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

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

> call save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News last week\ndescription: Bitcoin news from the last 7 days\nquery: SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days') ORDER BY date DESC\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\" }\n  - { field: url, label: \"Link\", format: url }\ndefault_sort: { field: date, direction: desc }"}

> result save_view:
> saved view (revision 1).

**Agent:** The table view **news_last_week** has been saved. It shows Bitcoin news from the last 7 days, sorted by date (newest first), with columns for **Date**, **Title**, and **Link**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 0 | 0 | 0 | 0 |  | 0.6 s | mistral: rate_limited (429): {"object":"error","message":"Rate limit exceeded","type":"rate_limited","param":null,"code"… |
| 2 | 3459 | 128 | 0 | 13 | tool_use | 1.0 s |  |
| 3 | 5973 | 128 | 0 | 15 | tool_use | 1.0 s |  |
| 4 | 6103 | 128 | 0 | 130 | tool_use | 1.7 s |  |
| 5 | 6246 | 128 | 0 | 45 | end | 1.1 s |  |
