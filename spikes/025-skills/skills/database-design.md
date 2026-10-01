---
name: database-design
description: "Designing tables: primary keys, the _run_id and _fetched_at columns, names, and widening a table (e.g. a second coin) instead of parallel tables. Load to design or discuss tables."
---
Rules: every table has an explicit primary key. Tables filled by pipelines also get `_run_id TEXT` and `_fetched_at TEXT` (the runtime fills them). Names are lower_snake_case. When a goal widens (a second coin, a second city), migrate to one general table with a key column (`prices(coin, date, …)`) instead of parallel tables.
