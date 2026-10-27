# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_view {"views":["version: 1\nid: add_btc_news\ntitle: Add Bitcoin News\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: false }\nsubmit: { action: insert, table: btc_news }\n"]}

> result save_view (error):
> there is no tool named save_view

> call call_tool→save_view {"views":["version: 1\nid: add_btc_news\ntitle: Add Bitcoin News\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: false }\nsubmit: { action: insert, table: btc_news }\n"]}

> result save_view (error):
> wrong arguments: save_view needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_view"}

> result describe_tool:
> save_view
> Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete view config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_view {"yaml":"version: 1\nid: add_btc_news\ntitle: Add Bitcoin News\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: false }\nsubmit: { action: insert, table: btc_news }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** I've added the form view `add_btc_news` for manually adding items to the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2437 | 0 | 0 | 221 | tool_use | 1.5 s |  |
| 2 | 5280 | 0 | 0 | 276 | tool_use | 2.8 s |  |
| 3 | 5615 | 0 | 0 | 190 | tool_use | 3.2 s |  |
| 4 | 5843 | 0 | 0 | 54 | tool_use | 1.7 s |  |
| 5 | 5973 | 0 | 0 | 167 | tool_use | 1.6 s |  |
| 6 | 6162 | 0 | 0 | 35 | end | 1.0 s |  |
