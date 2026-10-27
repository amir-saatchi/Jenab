# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"database-design"}

*Skill loaded: database-design*

> result load_skill:
> 

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

**Agent:** The `btc_news` table already has the columns needed for a manual news item: `url`, `date`, `title`, and `reason`. I will create a form view for adding news items manually.

First, I'll define the form configuration in YAML.

> call call_tool→save_form {"config":"type: form\ntitle: \"Add news item\"\ncolumns:\n  - name: url\n    label: URL\n    type: text\n    required: true\n  - name: date\n    label: Date\n    type: date\n    required: true\n  - name: title\n    label: Title\n    type: text\n    required: true\n  - name: reason\n    label: Reason\n    type: text\n    required: false"}

> result save_form (error):
> wrong arguments: save_form takes no argument config. save_form needs yaml. Its arguments: yaml (required). describe_tool gives their schema.

> call describe_tool {"name":"save_form"}

> result describe_tool:
> save_form
> Validate and save a form view config (full YAML, type: form). Returns all errors with paths, or success.
> 
> Arguments (JSON Schema):
> {"properties":{"yaml":{"description":"the complete form config as YAML","type":"string"}},"required":["yaml"],"type":"object"}

> call call_tool→save_form {"yaml":"type: form\ntitle: \"Add news item\"\ncolumns:\n  - name: url\n    label: URL\n    type: text\n    required: true\n  - name: date\n    label: Date\n    type: date\n    required: true\n  - name: title\n    label: Title\n    type: text\n    required: true\n  - name: reason\n    label: Reason\n    type: text\n    required: false"}

> result save_form:
> saved form view (revision 1).

**Agent:** The form **"Add news item"** has been saved. It allows manual entry of news items with fields for **URL**, **Date**, **Title**, and **Reason**.

Would you like to add a pipeline or automation to insert these entries into the `btc_news` table?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 23 | 2176 | 0 | 13 | tool_use | 1.0 s |  |
| 2 | 125 | 2176 | 0 | 14 | tool_use | 0.8 s |  |
| 3 | 186 | 2176 | 0 | 176 | tool_use | 2.5 s |  |
| 4 | 393 | 2176 | 0 | 13 | tool_use | 0.9 s |  |
| 5 | 472 | 2176 | 0 | 120 | tool_use | 2.3 s |  |
| 6 | 604 | 2176 | 0 | 58 | end | 2.6 s |  |
