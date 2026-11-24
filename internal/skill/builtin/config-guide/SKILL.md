---
name: config-guide
description: "Views and pages: table, chart, stat and form views, filters, formats, row actions, page rows and blocks, changing and removing them. Load before save_view, save_page or delete_config."
load_with: [save_view, save_page, delete_config]
---
## Views and pages: save_view and save_page

A view is one block (`table`, `chart`, `stat` or `form`), saved with `save_view(config)`. A page arranges saved views and static blocks in rows, saved with `save_page(config)`; pages are what the user opens. Configs are YAML only: no HTML, CSS, colours or code. Save every view before the page that uses it. Both tools validate first and return all errors together, each with its line; then nothing was saved, so fix every error and save the complete config again. `validate(config)` runs the same checks without saving. Use `describe_table` or `query` when unsure about columns or data.

The project card lists the saved views (ID, type, title), the pages with the views they show, and for each table the views and forms that use it.

### Views

Common: `version: 1`, `id`, `title`, `type` (table/chart/stat/form), `description`, `query` (table/chart/stat: one read-only SQLite `SELECT` or `WITH ... SELECT`, plain SQL, no `${{ }}`), `params` (fixed parameter values), `filters`. Named parameters only (`:name`), never string building. Bind every `:name` in the query with a filter or with `params`, not both.
Filters bind named query parameters: `{ param, control: text|number|date|select|checkbox, label, default, required, options }`. `required: true` stops the user from clearing it. Date defaults: ISO date, `today`, `today-7d`, `today-3m`, `today-1y`, `start_of_month`, `start_of_year`. A cleared optional filter binds NULL: write `(:city IS NULL OR city = :city)`. `select` needs `options`: a list `[A, B]` or `{ query: "SELECT ..." }` returning one column.

Table: `columns` [{field, label, format, format_options, align}], `default_sort` {field, direction: asc|desc} (always required, also for small tables), `page_size` (default 50), `rows_from` {table, key} (needed for row_actions; the query must select the key), `row_actions`. No ORDER BY, LIMIT or OFFSET in a table query; the runtime sorts and pages.
Formats: text, number, currency, percent, date, datetime, url, boolean, badge, image, file. Options go in `format_options`, e.g. `{ currency: USD, decimals: 2 }`. `badge` shows a value as a coloured label: `format_options: { tones: { open: blue, done: green, failed: red } }`; other values are neutral. `image` and `file` need a column holding bucket keys (an object-reference column) and show a thumbnail or a download link.
Row actions, one button per row:
- `{ label, open_form: <form id> }` opens an update form for that row; the form must be saved first.
- `{ label, delete: true, confirm: true }` deletes the row.
- `{ label, set: { <column>: <value> } }` updates the row without a form. Only columns of `rows_from.table`, not the key, `_run_id` or `_fetched_at`. Values are literals, `now()` or `today()`, and must fit the column types.
- `bulk: true` (on `set` and `delete`) adds a checkbox column; the action then also works on the selected rows of the loaded page, at most 500.
Row-action writes are recorded in the change log and can be undone.

Chart: `chart_type` line|bar|scatter|pie, `x` {field, label, format}, `y` [{field, label}], `series_by` (a field that splits rows into series; then y has exactly one entry). Pie: `x` is the label field, `y` has exactly one entry (the value, never negative), no `series_by`; at most 12 slices, the rest are summed into "Other". Line and scatter are sorted by `x`. Only the first 10,000 rows (and 1,000 bars) are drawn, so aggregate in the query (GROUP BY day, month, city) for long ranges.

Stat: one number. `value` {field, format, format_options}, `change` {field, format} (shown green or red, e.g. the change against yesterday), `caption` (short text under the number). The query must return exactly one row; ORDER BY and LIMIT are allowed here.

Fields in columns, default_sort, x, y, series_by, value and change must be columns of the query result (aliases count).

```yaml
version: 1
id: weather_chart
title: Temperature
type: chart
chart_type: line
query: |
  SELECT date, city, temp FROM weather
  WHERE date >= :from AND (:city IS NULL OR city = :city)
filters:
  - { param: from, control: date, label: "From", default: "today-30d", required: true }
  - { param: city, control: select, label: "City", options: { query: "SELECT name FROM cities" } }
x: { field: date, label: "Date", format: date }
y:
  - { field: temp, label: "°C" }
series_by: city
```

```yaml
version: 1
id: weather_today
title: Today
type: stat
query: |
  SELECT temp, temp - LAG(temp) OVER (ORDER BY date) AS change
  FROM weather WHERE city = :city ORDER BY date DESC LIMIT 1
params: { city: Berlin }
value: { field: temp, format: number, format_options: { decimals: 1 } }
change: { field: change, format: number }
caption: "°C in Berlin"
```

Form (no `query`): `fields` [{field, label, control: text|textarea|number|date|select|checkbox|file, required, default, options, prefix}], `submit` {action: insert|update, table, key (update)}, `load` (update only: a query that fills the fields and receives the row key as `:key`). `options` work as in filters. A `file` control uploads into the bucket under `prefix` (e.g. `uploads/`) and stores the key in an object-reference column. Insert forms: field names are table columns; every NOT NULL column without a default needs a field with `required: true`. Update forms are opened by an `open_form` row action or button. Values are checked against the column types before writing.

```yaml
version: 1
id: edit_task
title: Edit task
type: form
load: "SELECT title, status, due FROM tasks WHERE id = :key"
fields:
  - { field: title, label: "Task", control: text, required: true }
  - { field: status, label: "Status", control: select, options: [open, done, failed] }
  - { field: due, label: "Due", control: date }
submit: { action: update, table: tasks, key: id }
```

```yaml
version: 1
id: task_list
title: Tasks
type: table
query: |
  SELECT id, title, status, due FROM tasks
  WHERE (:status IS NULL OR status = :status)
filters:
  - { param: status, control: select, label: "Status", options: [open, done, failed] }
default_sort: { field: due, direction: asc }
columns:
  - { field: title, label: "Task", format: text }
  - { field: status, label: "Status", format: badge, format_options: { tones: { open: blue, done: green, failed: red } } }
  - { field: due, label: "Due", format: date }
rows_from: { table: tasks, key: id }
row_actions:
  - { label: "Edit", open_form: edit_task }
  - { label: "Mark done", set: { status: done }, bulk: true }
  - { label: "Delete", delete: true, confirm: true, bulk: true }
```

### Runtime rules

- Queries run on a read-only connection with a 2 s timeout per block. Keep them cheap; aggregate in SQL.
- Opening a view never calls an LLM or the network: a view only shows what is in the database.
- One statement only. No `_jenab_*`, `sqlite_*` or `pragma_*` names.
- Each block loads on its own, in parallel with the others on the page.
- After a schema change, views are validated again; one that no longer fits shows an error.
- Colours are only tones: `neutral`, `blue`, `green`, `amber`, `red`, `purple`. Anything else, such as hex colours, is rejected.

### Pages

Fields: `version: 1`, `id`, `title`, `description`, `filters`, `actions`, `rows`. The page header shows the title, description, filters and actions, so the title needs no heading block. Page `actions` are buttons in the header: `{ label, open_page: <page id> }` or `{ label, open_form: <form id> }`.
Page filters are written like view filters. Each one binds to the parameter of the same name in every block that has it and hides that block's own filter. Each page filter must match a parameter of at least one block.
`rows` is a list of rows; each row is a list of 1–4 blocks. `span` is 1–12 columns; blocks without span share equally what the others leave of the 12, and the spans of a row add up to at most 12. There is no other layout: no CSS, no positions. Narrow windows stack rows into one column.
Blocks:
- `{ view: <view id>, span }`: a saved view; it must exist. Data always comes through views.
- `{ heading: "Text", span }`: a section title.
- `{ text: "Markdown", span }`: static text, no raw HTML.
- `{ card: "Title", rows, span }`: a titled frame around rows of blocks, with the same row rules on its own 12-column grid. No card inside a card.
- `{ filter: <page filter param>, span }`: shows that page filter here instead of in the header, e.g. a search box above a table. Each page filter is shown once.
- `{ button: "Label", open_page: <id> }` (or `open_form: <id>`), with optional `tone`, `confirm`, `span`.
- `{ image: <bucket key>, alt, span }`.
Writing data stays with form views and row actions. Put views on a page even when the project needs only one. After saving, call `open_page(id, filters)` to show the page in this chat.

```yaml
version: 1
id: weather
title: Weather
description: Daily temperature by city
filters:
  - { param: city, control: select, label: "City", options: { query: "SELECT name FROM cities" } }
rows:
  - [ { view: weather_today, span: 4 }, { view: weather_chart } ]
  - [ { card: "Tasks", rows: [ [ { view: task_list } ] ] } ]
```

### Changing and removing

- To change a view or a page, `read_config(id)`, edit it, and save the whole config under the same id. Pages that show the view show the new one.
- Taking a view off a page is a change of the page: save the page without that block. It doesn't delete the view, which stays for other pages and can go back on.
- Delete with `delete_config(id)` only when the user asked to remove that view or page, and the project card shows nothing else using it. A view still on a page, or opened by a button or a row action, is refused, with what uses it named.
- To remove a page, delete the page, then each of its views that no other page shows (the card's Pages lines say which). Leave the views other pages use.
- Deleting views and pages leaves the tables and their data as they are. Drop a table only when the user asks for the data itself to go.
- Undo turn takes back the saves and deletes of its turn.
