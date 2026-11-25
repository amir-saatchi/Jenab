---
name: migrations
description: "Changing tables: migration steps (create, add, rename or drop columns, copy data, rebuild, widen, drop), destructive changes and updating dependents. Load to plan or discuss a schema change."
load_with: [request_schema_change]
---
## Migrations

The schema changes only through a migration: a list of steps, never SQL DDL. The main agent asks for one with `request_schema_change(goal)`; the schema agent writes the steps and sends them with `apply_migration(plan, steps, updates)`.

- The steps run in order, in one transaction: all of them apply, or none.
- Every migration is tried first. A failed try lists its problems by path, such as `steps[1].column.type`; nothing changed, so fix those and send the whole migration again.
- A migration that deletes data or changes a primary key needs the user's approval on a card. At the Strict level, every migration asks.

### Steps

Each step is a JSON object with an `op`. Names of columns, keys and indexes are lists, even for one column: `"primary_key": ["date"]`.

| op | Fields |
|---|---|
| `create_table` | `table`, `columns`, `primary_key` (required), `unique` (a list of column lists), `foreign_keys` |
| `add_column` | `table`, `column`, `value`: the old rows' value, as SQL; required when the table has rows and the column no default (`NULL` for none). Not for a column that tells the rows apart, such as a coin for news tracked per coin: that is `widen_table` |
| `rename_table` | `from`, `to` |
| `rename_column` | `table`, `from`, `to` |
| `create_index` | `table`, `columns`, `name` (optional), `unique` (true or false) |
| `drop_index` | `name` |
| `annotate_column` | `table`, `column`, `kind`: `object_ref`, or `""` to remove it |
| `insert_rows` | `table`, `rows`: objects of column names and values, at most 50 |
| `copy_data` | `into`, `columns`, `from`: one SELECT that returns those columns in order |
| `rebuild_table` | `table`, the new definition as in `create_table`, and `mapping` |
| `widen_table` | `table`, `column`, `value`, `to` (optional) |
| `drop_column` | `table`, `column` |
| `drop_table` | `table` |

- **A column** is `{"name": "close", "type": "REAL", "not_null": true, "default": 0, "check": "close >= 0"}`. Only `name` and `type` are required; the type is `TEXT`, `INTEGER`, `REAL` or `BLOB`. A `default` is a string, a number, or true or false.
- **A foreign key** is `{"columns": ["coin"], "table": "coins", "to": ["symbol"], "on_delete": "cascade"}`. Leave out `to` for the other table's primary key. Actions: `cascade`, `set null`, `set default`, `restrict`, `no action`.
- **`copy_data`'s `from`** is a read-only query on the project's tables, with the values written into it: no parameters.
  A window function sees only the rows the `WHERE` of its own `SELECT` keeps, so a 7-day average from October 1 needs the last days of September. Put the window in an inner query with no date filter, and filter the dates in the outer one: `SELECT date, close, ma7 FROM (SELECT date, close, avg(close) OVER (ORDER BY date ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS ma7 FROM prices WHERE coin = 'BTC') WHERE date >= '2026-10-01'`.
- **`rebuild_table`'s `mapping`** gives a new column its value as an SQL expression on the old row: `{"coin": "'BTC'", "price": "close"}`. A new column with the same name as an old one keeps its values without a mapping. An old column that isn't kept is deleted.

### Deleting data

`drop_column`, `drop_table`, and a `rebuild_table` that leaves out an old column delete data. Only use them when the goal asks for it, or after the data was copied somewhere else in the same migration.

### Widening a table

A table whose rows are all about one thing, when the goal needs many, is widened with one `widen_table` step. So is a table that gets a column telling its rows apart that its key leaves out: readings keyed by time that now come from several sensors get a `sensor` column and the key `(sensor, time)`.

```json
{"op": "widen_table", "table": "oslo_weather", "column": {"name": "city", "type": "TEXT"}, "value": "'Oslo'", "to": "weather"}
```

- The new column comes first in the primary key and in each unique constraint and index: the key `(date)` becomes `(city, date)`.
- The old rows get `value`. The column is `NOT NULL` with no default, so new rows set it.
- Every other column, check, foreign key and index stays with its data. `to` renames the table; the old name is then gone, with nothing left to drop.

Build a new table instead only when it must look different: `create_table` with every old column, with its type, NOT NULL, default and check, plus the new one, `copy_data` of all of them, then `drop_table` on the old one.

### Dependents

Views, pipelines and forms that use a table are its dependents; the schema agent gets them with the columns they use. After the steps, every dependent is checked against the new schema in the same transaction. Each dependent that uses something renamed or dropped goes in `updates` in the same call, with its whole new config: read it with `read_config`, change what the migration breaks, and send the whole YAML as a string, with the same id: `{"kind": "view", "id": "btc_chart", "config": "id: btc_chart
type: line
…"}`. Forms are views too. If one is missing or fails its check, the whole migration fails with its problems, and nothing changed. A dependent that already failed its check before the migration, and isn't in `updates`, doesn't stop it; the result lists it, and the project card marks it broken. If the migration breaks it in a new way, such as renaming or dropping another column it uses, it fails the migration like any other, with its old issues marked "already before the migration": fix all of them in its update.
