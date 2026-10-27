# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call call_tool→save_view {"name":"save_view","view_config":"version: 1\nid: add_btc_news\ntitle: Add News Item\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: today }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: false }\nsubmit: { action: insert, table: btc_news }\n"}

> result save_view (error):
> wrong arguments: save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: add_btc_news\ntitle: Add News Item\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: today }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: false }\nsubmit: { action: insert, table: btc_news }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the form `add_btc_news` for manually inserting news items with the URL, date, title, and reason.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2437 | 0 | 0 | 126 | tool_use | 1.9 s |  |
| 2 | 5185 | 0 | 0 | 357 | tool_use | 1.7 s |  |
| 3 | 5580 | 0 | 0 | 65 | tool_use | 0.8 s |  |
| 4 | 5721 | 0 | 0 | 163 | tool_use | 1.1 s |  |
| 5 | 5906 | 0 | 0 | 38 | end | 0.8 s |  |
