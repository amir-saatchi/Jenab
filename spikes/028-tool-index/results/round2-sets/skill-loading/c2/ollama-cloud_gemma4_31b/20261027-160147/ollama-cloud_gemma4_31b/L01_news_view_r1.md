# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_view {"views":["version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url AS link FROM btc_news\n  WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: link, label: \"Link\", format: url }\n"]}

> result save_view (error):
> there is no tool named save_view

> call call_tool→save_view {"views":["version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url AS link FROM btc_news\n  WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: link, label: \"Link\", format: url }\n"]}

> result save_view (error):
> wrong arguments: save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\nquery: |\n  SELECT date, title, url AS link FROM btc_news\n  WHERE date \u003e= date('now', '-7 days')\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: link, label: \"Link\", format: url }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the `news_last_week` table view. It shows the date, title, and link for news from the last 7 days, sorted by newest first.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2448 | 0 | 0 | 320 | tool_use | 2.8 s |  |
| 2 | 5390 | 0 | 0 | 409 | tool_use | 2.7 s |  |
| 3 | 5858 | 0 | 0 | 183 | tool_use | 1.6 s |  |
| 4 | 6079 | 0 | 0 | 132 | tool_use | 1.1 s |  |
| 5 | 6287 | 0 | 0 | 160 | tool_use | 1.2 s |  |
| 6 | 6469 | 0 | 0 | 55 | end | 0.9 s |  |
