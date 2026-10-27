# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** The table view `news_last_week` has been created. It shows the date, title, and link for news from the last 7 days, sorted with the newest first.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3084 | 0 | 0 | 341 | tool_use | 2.1 s |  |
| 2 | 6047 | 0 | 0 | 507 | tool_use | 2.5 s |  |
| 3 | 6576 | 0 | 0 | 46 | end | 0.8 s |  |
