# L08_form

Add a form for adding a news item by hand: url, date, title and reason.

Good behaviour: Load config-guide before the first write; no skills outside config-guide.

## Turn 1

**User:** Add a form for adding a news item by hand: url, date, title and reason.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call save_form {"yaml":"version: 1\nid: add_btc_news\ntitle: Add news item\ntype: form\nfields:\n  - { field: url, label: \"URL\", control: text, required: true }\n  - { field: date, label: \"Date\", control: date, required: true }\n  - { field: title, label: \"Title\", control: text, required: true }\n  - { field: reason, label: \"Reason\", control: textarea, required: true }\nsubmit:\n  action: insert\n  table: btc_news\n"}

> result save_form:
> saved form view (revision 1).

**Agent:** I've added the form `add_btc_news` for manually adding news items with the URL, date, title, and reason.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3073 | 0 | 0 | 124 | tool_use | 1.3 s |  |
| 2 | 5819 | 0 | 0 | 325 | tool_use | 3.8 s |  |
| 3 | 6167 | 0 | 0 | 39 | end | 0.9 s |  |
