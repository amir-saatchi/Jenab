# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_form {"yaml":"version: 1\nid: add_btc_news\ntitle: Add News Item\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: true }\nsubmit: { action: insert, table: btc_news }\n"}

> result save_form:
> saved form view (revision 1).

**Agent:** The form `add_btc_news` has been created for adding news items to the `btc_news` table.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3073 | 0 | 0 | 138 | tool_use | 1.4 s |  |
| 2 | 5833 | 0 | 0 | 525 | tool_use | 4.9 s |  |
| 3 | 6381 | 0 | 0 | 38 | end | 0.8 s |  |
