---
name: sql-queries
description: "SQL for the query tool, db.query and view queries: the SQLite dialect, what the read-only SQL guard rejects, and named parameters. Load to write or debug SQL."
---
## 5. SQL rules

SQLite dialect. Only a single SELECT (or WITH ... SELECT); no writes, no PRAGMA, no ATTACH, no `_jenab_*` or `sqlite_*` tables. Named parameters only (`:name`), never string building. Use `describe_table` or `query` when unsure about columns or data.
