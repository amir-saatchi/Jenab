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

> call call_tool→save_form {"yaml":"id: add_news_item\ntype: form\ntitle: Add News Item\ntable: btc_news\nfields:\n  - name: url\n    label: URL\n    type: text\n    required: true\n  - name: date\n    label: Date\n    type: date\n    required: true\n  - name: title\n    label: Title\n    type: text\n    required: true\n  - name: reason\n    label: Reason\n    type: text\n    required: false\n"}

> result save_form:
> saved form view (revision 1).

**Agent:** I've added the "Add News Item" form to manually add entries to the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2129 | 0 | 0 | 349 | tool_use | 3.8 s |  |
| 2 | 2557 | 0 | 0 | 442 | tool_use | 1.9 s |  |
| 3 | 3023 | 0 | 0 | 34 | end | 3.7 s |  |
