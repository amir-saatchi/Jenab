# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"config-guide"}

> call load_skill {"name":"sql-queries"}

> call query {"sql":"SELECT date FROM btc_news LIMIT 1"}

*Skill loaded: config-guide*

> result load_skill:
> 

*Skill loaded: sql-queries*

> result load_skill:
> 

> result query:
> date
> 2026-10-05
> (1 rows)

> call save_view {"config":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view (error):
> there is no tool named save_view

> call call_tool→save_view {"config":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view (error):
> wrong arguments: save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call call_tool→save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the `news_last_week` table view. It shows the date, title, and link from the `btc_news` table, filtered for the last 7 days and sorted by date in descending order.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2448 | 0 | 0 | 306 | tool_use | 1.9 s |  |
| 2 | 5499 | 0 | 0 | 794 | tool_use | 4.0 s |  |
| 3 | 6352 | 0 | 0 | 200 | tool_use | 1.4 s |  |
| 4 | 6590 | 0 | 0 | 188 | tool_use | 1.1 s |  |
| 5 | 6800 | 0 | 0 | 58 | end | 0.8 s |  |
