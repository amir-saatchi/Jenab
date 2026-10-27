# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_form {"yaml":"version: 1\nid: add_news_item\ntitle: \"Add news item\"\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true, default: \"today\" }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea }\nsubmit:\n  action: insert\n  table: btc_news"}

> result save_form:
> saved form view (revision 1).

**Agent:** The form for adding a news item by hand is saved as **`add_news_item`**. You can now add it to a page or use it in a row action if needed.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 122 | 3456 | 0 | 13 | tool_use | 2.5 s |  |
| 2 | 6092 | 0 | 0 | 138 | tool_use | 3.5 s |  |
| 3 | 6115 | 128 | 0 | 39 | end | 1.3 s |  |
