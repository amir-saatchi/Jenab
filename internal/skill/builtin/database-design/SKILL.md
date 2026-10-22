---
name: database-design
description: "Designing tables: primary keys, column types, names, object_ref columns, pipeline columns, and widening a table instead of adding parallel ones. Load to design or discuss tables."
needs: [request_schema_change]
---
## Designing tables

The project's tables are in SQLite. Only the schema agent changes them, through `request_schema_change(goal)`. Give it what the user wants, with the names they gave and the data to keep; it designs the rest by these rules.

### Rules

- **Design for the goal as stated.** Add what the goal needs now, not columns for guesses about later.
- **Every table has an explicit primary key.** Pick the columns that name a row: `date` for one price a day, `(coin, date)` for one price a day per coin, `url` for news items. An `id INTEGER` key is for rows with no natural key, such as notes.
- **Names** are lower_snake_case: `daily_prices`, `closed_at`. Table names are plural nouns. Don't start a name with `_jenab_` or `sqlite_`.
- **Types** are `TEXT`, `INTEGER`, `REAL` or `BLOB`:
  - Dates and times are `TEXT` in ISO 8601: `2026-03-01` or `2026-03-01T14:30:00Z`.
  - Money and prices are `REAL`; counts and flags (0 or 1) are `INTEGER`.
  - A key column is never `BLOB`.
- **`not_null`** for columns every row has.
- **`check`** for simple rules on one row: `price >= 0`, `side IN ('buy', 'sell')`.
- **Bucket files:** a column that holds bucket keys (files the project stores) is annotated `object_ref`.
- **Pipeline data:** tables that pipelines fill also get `_run_id TEXT` and `_fetched_at TEXT`; the runtime fills them. Copied rows keep both.

### Widening a table

When the goal widens, such as a second coin or a second city, widen the table with a key column, `prices(coin, date, …)`, through `widen_table` (`migrations`). Not parallel tables like `btc_prices` and `eth_prices`, and not a plain added column.

- The general table keeps every old column, such as `_run_id` and `_fetched_at`, also when the goal names only some.
- The key must fit the wider goal: when news is tracked per coin, the key is `(coin, url)`, not `url`.
- Existing rows get their new values explicitly. A column `default` is never used to label old rows, because it would also label new rows that lack a value.
- Views, pipelines and forms that use the old table are updated in the same migration.

### Deleting

Dropping a table or a column deletes data, and changing a primary key fails if two rows end up with the same key, so the user always approves both on a card. Keep data unless the user asked to delete it.
