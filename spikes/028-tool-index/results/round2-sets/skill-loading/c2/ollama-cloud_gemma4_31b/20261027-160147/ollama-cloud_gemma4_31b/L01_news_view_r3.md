# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call describe_table {"name":"btc_news"}

> result describe_table:
> table btc_news
> - url TEXT (primary key 1)
> - date TEXT
> - title TEXT
> - reason TEXT
> - _run_id TEXT
> - _fetched_at TEXT
> rows: 4

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_view {"views":["version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\ndefault_sort: { field: date, direction: desc }\n"]}

> result save_view (error):
> there is no tool named save_view

> call call_tool→save_view {"views":["version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\ndefault_sort: { field: date, direction: desc }\n"]}

> result save_view (error):
> wrong arguments: save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url FROM btc_news\n  WHERE date \u003e= :from\nfilters:\n  - { param: from, control: date, label: \"From\", default: \"today-7d\", required: true }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }\ndefault_sort: { field: date, direction: desc }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the `news_last_week` table view, which shows news from the last 7 days, sorted by date (newest first).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2448 | 0 | 0 | 186 | tool_use | 1.6 s |  |
| 2 | 2696 | 0 | 0 | 55 | tool_use | 0.8 s |  |
| 3 | 5373 | 0 | 0 | 680 | tool_use | 3.7 s |  |
| 4 | 6112 | 0 | 0 | 205 | tool_use | 1.2 s |  |
| 5 | 6355 | 0 | 0 | 62 | tool_use | 0.9 s |  |
| 6 | 6493 | 0 | 0 | 184 | tool_use | 1.3 s |  |
| 7 | 6699 | 0 | 0 | 41 | end | 1.1 s |  |
