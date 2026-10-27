# L01_news_view

Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call query {"sql":"SELECT date, title, url FROM btc_news ORDER BY date DESC LIMIT 7"}

> result query:
> date | title | url
> 2026-10-05 | Seed story 1 | https://news.example.com/seed/1
> 2026-10-04 | Seed story 2 | https://news.example.com/seed/2
> 2026-10-03 | Seed story 3 | https://news.example.com/seed/3
> 2026-10-02 | Seed story 4 | https://news.example.com/seed/4
> (4 rows)

**Agent:** The `btc_news` table only has 4 rows, so the query returns all available news. To show the last 7 days of news, we need to filter for the last 7 days and ensure the query returns the newest first.

Here is the config for the `news_last_week` table view:

```yaml
version: 1
id: news_last_week
title: News Last Week
type: table
description: Bitcoin news from the last 7 days
query: |
  SELECT date, title, url
  FROM btc_news
  WHERE date >= date('now', '-7 days')
  ORDER BY date DESC
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: title, label: "Title", format: text }
  - { field: url, label: "Link", format: url }
```

I will now save this view.

> call save_view {"version":1,"id":"news_last_week","title":"News Last Week","type":"table","description":"Bitcoin news from the last 7 days","query":"SELECT date, title, url FROM btc_news WHERE date \u003e= date('now', '-7 days') ORDER BY date DESC","default_sort":{"field":"date","direction":"desc"},"columns":[{"field":"date","label":"Date","format":"date"},{"field":"title","label":"Title","format":"text"},{"field":"url","label":"Link","format":"url"}]}

> result save_view (error):
> there is no tool named save_view

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: news_last_week\ntitle: News Last Week\ntype: table\ndescription: Bitcoin news from the last 7 days\nquery: |\n  SELECT date, title, url\n  FROM btc_news\n  WHERE date \u003e= date('now', '-7 days')\n  ORDER BY date DESC\ndefault_sort: { field: date, direction: desc }\ncolumns:\n  - { field: date, label: \"Date\", format: date }\n  - { field: title, label: \"Title\", format: text }\n  - { field: url, label: \"Link\", format: url }"}

> result save_view:
> saved view (revision 1).

**Agent:** The table view **news_last_week** has been successfully saved. It displays Bitcoin news from the last 7 days, sorted by date (newest first).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2485 | 128 | 0 | 13 | tool_use | 1.4 s |  |
| 2 | 4999 | 128 | 0 | 27 | tool_use | 1.3 s |  |
| 3 | 5143 | 128 | 0 | 359 | tool_use | 7.6 s |  |
| 4 | 3112 | 2560 | 0 | 13 | tool_use | 1.0 s |  |
| 5 | 5621 | 128 | 0 | 162 | tool_use | 4.8 s |  |
| 6 | 5794 | 128 | 0 | 33 | end | 11.0 s |  |
