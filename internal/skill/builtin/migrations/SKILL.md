---
name: migrations
description: "Changing tables: migration steps (create, add, rename or drop columns, copy data, rebuild, drop), destructive changes and updating dependents. Load to plan or discuss a schema change."
load_with: [request_schema_change]
---
## Migrations

The schema changes only through a migration: a list of steps, never SQL DDL. The main agent asks for one with `request_schema_change(goal)`; the schema agent writes the steps and sends them with `apply_migration(plan, steps, updates)`.

- The steps run in order, in one transaction: all of them apply, or none.
- Every migration is tried first. A failed try lists its problems by path, such as `steps[1].column.type`; nothing changed, so fix those and send the whole migration again.
- A migration that deletes data needs the user's approval on a card, and a snapshot of the project is taken first. One that changes a table's primary key needs approval too, but no snapshot: if two rows get the same key, it fails and nothing changes. At the Strict level, every migration asks.

### Steps

Each step is a JSON object with an `op`. Names of columns, keys and indexes are lists, even for one column: `"primary_key": ["date"]`.

| op | Fields |
|---|---|
| `create_table` | `table`, `columns`, `primary_key` (required), `unique` (a list of column lists), `foreign_keys` |
| `add_column` | `table`, `column` |
| `rename_table` | `from`, `to` |
| `rename_column` | `table`, `from`, `to` |
| `create_index` | `table`, `columns`, `name` (optional), `unique` (true or false) |
| `drop_index` | `name` |
| `annotate_column` | `table`, `column`, `kind`: `object_ref`, or `""` to remove it |
| `insert_rows` | `table`, `rows`: objects of column names and values, at most 50 |
| `copy_data` | `into`, `columns`, `from`: one SELECT that returns those columns in order |
| `rebuild_table` | `table`, the new definition as in `create_table`, and `mapping` |
| `drop_column` | `table`, `column` |
| `drop_table` | `table` |

- **A column** is `{"name": "close", "type": "REAL", "not_null": true, "default": 0, "check": "close >= 0"}`. Only `name` and `type` are required; the type is `TEXT`, `INTEGER`, `REAL` or `BLOB`. A `default` is a string, a number, or true or false.
- **A foreign key** is `{"columns": ["coin"], "table": "coins", "to": ["symbol"], "on_delete": "cascade"}`. Leave out `to` for the other table's primary key. Actions: `cascade`, `set null`, `set default`, `restrict`, `no action`.
- **`copy_data`'s `from`** is a read-only query on the project's tables, with the values written into it: no parameters.
- **`rebuild_table`'s `mapping`** gives a new column its value as an SQL expression on the old row: `{"coin": "'BTC'", "price": "close"}`. A new column with the same name as an old one keeps its values without a mapping. An old column that isn't kept is deleted.

### Deleting data

`drop_column`, `drop_table`, and a `rebuild_table` that leaves out an old column delete data. Only use them when the goal asks for it, or after the data was copied somewhere else in the same migration.

To move data to a new table: `create_table`, then `copy_data`, then `drop_table` on the old one, in one migration.

```json
{"plan": "Keep the BTC prices in a general prices table with a coin column, so other coins fit.",
 "steps": [
  {"op": "create_table", "table": "prices", "columns": [{"name": "coin", "type": "TEXT", "not_null": true}, {"name": "date", "type": "TEXT", "not_null": true}, {"name": "close", "type": "REAL"}], "primary_key": ["coin", "date"]},
  {"op": "copy_data", "into": "prices", "columns": ["coin", "date", "close"], "from": "SELECT 'BTC', date, close FROM btc_prices"},
  {"op": "drop_table", "table": "btc_prices"}]}
```

### Dependents

Views, pipelines and forms that use a table are its dependents; the schema agent gets them with the columns they use. After the steps, every dependent is checked against the new schema in the same transaction. Each dependent that uses something renamed or dropped goes in `updates` in the same call, with its full new config: `{"kind": "view", "id": "btc_chart", "config": …}`. If one is missing, the whole migration fails.
