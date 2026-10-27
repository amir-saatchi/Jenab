# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call describe_tool {"name":"save_form"}

> result describe_tool:
> save_form
> Validate and save a form view config (full YAML, type: form). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete form config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→get_config {"name":"add_keyword"}

> result get_config (error):
> wrong arguments: get_config takes no argument name. get_config needs id. Its arguments: id (required). describe_tool gives their schema.

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

> call call_tool→save_form {"yaml":"version: 1\nid: add_btc_news\ntitle: Add news item\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: text, required: false }\nsubmit:\n  action: insert\n  table: btc_news\n"}

> result save_form:
> saved form view (revision 1).

**Agent:** I've added the form "Add news item" (`add_btc_news`) to manually insert records into the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2129 | 0 | 0 | 200 | tool_use | 2.8 s |  |
| 2 | 2408 | 0 | 0 | 176 | tool_use | 1.2 s |  |
| 3 | 2631 | 0 | 0 | 42 | tool_use | 0.7 s |  |
| 4 | 2748 | 0 | 0 | 275 | tool_use | 1.6 s |  |
| 5 | 3046 | 0 | 0 | 41 | end | 0.9 s |  |
