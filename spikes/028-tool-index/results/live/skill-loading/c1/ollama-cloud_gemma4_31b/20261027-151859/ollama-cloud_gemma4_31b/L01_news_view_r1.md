# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"config-guide"}

> call load_skill {"name":"sql-queries"}

*Skill loaded: config-guide*

> result load_skill:
> 

*Skill loaded: sql-queries*

> result load_skill:
> 

> call save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News last week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\ndefault_sort: { field: date, direction: desc }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the `news_last_week` table view, showing news from the last 7 days (sorted newest first) with columns for date, title, and link.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3084 | 0 | 0 | 464 | tool_use | 4.4 s |  |
| 2 | 6265 | 0 | 0 | 1667 | tool_use | 6.7 s |  |
| 3 | 7954 | 0 | 0 | 55 | end | 1.1 s |  |
