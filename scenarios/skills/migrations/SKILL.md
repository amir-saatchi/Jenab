---
name: migrations
description: "Changing tables: apply_migration steps (create, add or drop columns, copy data, drop) and updating dependent views and pipelines. Load before apply_migration; new tables: also database-design."
load_with: [apply_migration]
---
## 1. Schema changes: apply_migration

Never write SQL DDL. Send structured steps; each step has `op`:

| op | fields |
|---|---|
| create_table | `table`, `columns` [{`name`, `type` TEXT/INTEGER/REAL/NUMERIC/BLOB, `not_null`, `default`, `check`}], `primary_key` (column or list, required), `unique` (list of column or column lists), `foreign_keys` [{`columns`, `references`: {`table`, `columns`}, `on_delete`}] |
| add_column | `table`, `column` {name, type, not_null, default, check}; NOT NULL needs a default |
| rename_table | `from`, `to` |
| rename_column | `table`, `from`, `to` |
| create_index | `table`, `columns`, `unique`, `name` (optional) |
| drop_index | `name` |
| annotate_column | `table`, `column`, `kind: object_ref` (columns holding bucket keys) |
| insert_rows | `table`, `rows` (list of objects, at most 50) |
| copy_data | `into`, `columns`, `from` (one SELECT returning those columns in order) |
| rebuild_table | `table`, new definition as in create_table, `mapping` {new_col: old_col or null} |
| drop_column | `table`, `column` |
| drop_table | `table` |

Steps run in order in one transaction; afterwards every saved view and pipeline is re-validated against the new schema. Every view and pipeline that uses a renamed or dropped table (see "Dependents" in the project card) must go in the SAME call, updated, as full YAML strings in `views` and `pipelines`; otherwise the whole migration is rejected. Read each one with get_config first.
To move data into a new table: create_table, then copy_data with a SELECT that returns the columns in order (`SELECT 'BTC', date, price FROM old`), then drop_table the old one. To add a NOT NULL column to a table with rows, give it a default.

```json
{"steps":[{"op":"create_table","table":"cities","columns":[{"name":"name","type":"TEXT"},{"name":"country","type":"TEXT","not_null":true}],"primary_key":"name"},
 {"op":"insert_rows","table":"cities","rows":[{"name":"Oslo","country":"NO"}]}]}
```
