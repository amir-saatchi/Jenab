# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call get_config {"id":"add_keyword"}

> result get_config (error):
> there is no tool named get_config

> call describe_tool {"name":"get_config"}

> result describe_tool:
> get_config
> Read the saved YAML of a view or pipeline by id.
> 
> Arguments (JSON Schema):
> {"properties":{"id":{"type":"string"}},"required":["id"],"type":"object"}

> call call_tool→get_config {"id":"add_keyword"}

> result get_config:
> version: 1
> id: add_keyword
> title: Add news keyword
> type: form
> fields:
>   - { field: keyword, label: "Keyword", control: text, required: true }
> submit:
>   action: insert
>   table: watch_keywords
> 

**Agent:** Here’s a form config for adding a **news item manually** (url, date, title, reason). I’ll save it as a new form view:

```yaml
version: 1
id: add_news_item
title: Add Bitcoin News Item
type: form
fields:
  - { field: url, label: "URL", control: text, required: true }
  - { field: date, label: "Date", control: date, required: true, default: "today" }
  - { field: title, label: "Title", control: text, required: true }
  - { field: reason, label: "Reason", control: textarea, required: true }
submit:
  action: insert
  table: btc_news
```

I’ll save this now.

> call save_view {"version":1,"id":"add_news_item","title":"Add Bitcoin News Item","type":"form","fields":[{"field":"url","label":"URL","control":"text","required":true},{"field":"date","label":"Date","control":"date","required":true,"default":"today"},{"field":"title","label":"Title","control":"text","required":true},{"field":"reason","label":"Reason","control":"textarea","required":true}],"submit":{"action":"insert","table":"btc_news"}}

> result save_view (error):
> there is no tool named save_view

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: add_news_item\ntitle: Add Bitcoin News Item\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: true }\nsubmit:\n  action: insert\ntable: btc_news"}

> result save_view:
> saved view (revision 1).

**Agent:** The form **"Add Bitcoin News Item"** has been saved successfully. You can now use it to manually add news items to the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 44 | 2560 | 0 | 13 | tool_use | 1.7 s |  |
| 2 | 126 | 4992 | 0 | 13 | tool_use | 1.0 s |  |
| 3 | 2614 | 2560 | 0 | 13 | tool_use | 1.2 s |  |
| 4 | 2671 | 2560 | 0 | 24 | tool_use | 5.2 s |  |
| 5 | 67 | 5248 | 0 | 324 | tool_use | 6.2 s |  |
| 6 | 3121 | 2560 | 0 | 13 | tool_use | 2.1 s |  |
| 7 | 3198 | 2560 | 0 | 154 | tool_use | 3.1 s |  |
| 8 | 3363 | 2560 | 0 | 34 | end | 1.3 s |  |
