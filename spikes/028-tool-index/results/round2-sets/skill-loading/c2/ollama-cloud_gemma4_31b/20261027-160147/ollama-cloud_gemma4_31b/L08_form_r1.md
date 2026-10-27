# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

> call describe_tool {"name":"save_form"}

*Skill loaded: config-guide*

> result load_skill:
> 

> result describe_tool:
> save_form
> Validate and save a form view config (full YAML, type: form). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete form config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_form {"yaml":"version: 1\nid: add_btc_news\ntitle: Add Bitcoin News\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: false }\nsubmit: { action: insert, table: btc_news }\n"}

> result save_form:
> saved form view (revision 1).

**Agent:** I've added the form `add_btc_news` for manually adding news items to the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2437 | 0 | 0 | 313 | tool_use | 2.9 s |  |
| 2 | 5448 | 0 | 0 | 379 | tool_use | 3.1 s |  |
| 3 | 5850 | 0 | 0 | 37 | end | 1.0 s |  |
