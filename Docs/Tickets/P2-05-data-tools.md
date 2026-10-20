# P2-05 — Data tools: query, describe_table, insert
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P2-01, P2-02
**Requirements:** R-33

## Goal
The agent reads the project's tables with `query` and adds small entries with `insert` (SPEC 8.1).

## Scope
- **`query(sql, params)`:** through `AgentQuery`. It returns up to 200 rows or 4,000 tokens, plus the total count. A bigger result is stored as a ref with a preview (3.7).
- **Limits from P2-01 and P2-02:**
  - `AgentQuery` returns at most 8 MiB of rows (`Rows.Full`), with values of at most 1 MiB. `db.query` in pipelines streams up to 100,000 rows (SPEC 6.5), so it needs its own result limit, not the 8 MiB one.
  - Over 500 rows, a typed `Insert` can fail in a later chunk and keep the earlier ones: a partial unique index, a CHECK constraint or a STRICT type error. The tool must report this as a partial write, using `WriteResult`.
- **`describe_table(name)`:** columns, types, keys, annotations and the row count.
- **`insert(table, rows)`:** at most 50 rows a call, through the typed `Insert`, with the source `message:<id>`. Each row is checked against the column types and `NOT NULL` before the write, and every error comes back at once, with its row and column.
- **Areas:** the three tools have `Areas: tool.Tables`, so block 1 names the project's tables and no longer says "no tables yet" (TASK-014).
- **The `sql-queries` skill:** the SQLite dialect and the guard's rules (8.9), with `load_with` left empty as decided in SPIKE-025.
- **Scenarios:** a set where the agent answers questions from a fixture table, including a refused query (an internal table) that it fixes.

## Done when
- Tool tests for each limit (200 rows, 4,000 tokens, 50 rows), each guard refusal and each insert error.
- The new scenario set passes on the reference models (gemini-3.5-flash-lite and gemma4:31b) with 3 reps.

## Result
Done on 2026-10-20 (commit 1626ef9), after three review rounds.

- **Built:**
  - `query`, `describe_table` and `insert` in `internal/tool/data.go`, with `store.DescribeTable` and `store.Tables` in `internal/store/tables.go`.
  - The built-in `sql-queries` skill.
  - In the scenario runner, a `data:` SQL field per set, and the `scenarios/data-tools` set.
- **Tests:** a test for each limit (200 rows, 4,000 tokens, the 10,000-row ref cap, 50 rows), each guard refusal, each insert error and the partial write. A test runs every example in the skill through `query`.
- **Scenarios (3 reps):**
  - data-tools: 12/12 on gemini-3.5-flash-lite and 12/12 on gemma4:31b.
  - missing-features, rescored with the final checks: gemini 11/18, gemma 15/18.
- **Changes from the ticket:**
  - Values come back as stored: the worker's outer SELECT drops the declared column type, so DATE and DATETIME text isn't turned into times. This costs one extra probe statement per query.
  - A result over `tool_preview_tokens` that still fits 4,000 tokens and 200 rows is stored and shown whole with its ref, so it can be stubbed later.
  - `params` are checked against the query's own parameters: a missing value, an unused key or two keys for one parameter is refused. Parameter names must start with a letter, so `:_c` and `$1` are refused. `?01` is read as `?1`.
  - A refusal carries `Schema`, so the hint that lists the tables doesn't depend on the refusal's wording.
  - A set's own skill replaces a built-in skill of the same name; the scenario sets keep their short `sql-queries` draft.
  - The missing-features set gets the real data tools on an empty database. Its checks now fail only offers to create tables, pipelines or views, and accept ’ as well as '.
- **Open points:**
  - SPEC 3.7 caps a preview at `tool_preview_tokens` (1,500), but 8.1 lets `query` return up to 4,000 tokens whole.
  - `query` is marked ReadsDB, not Untrusted.
  - `db.query`'s own result limit is left for the pipelines ticket.
- **Known limits:**
  - The probe costs as much as the query when a CTE used twice in FROM is materialized, so such a query reaches the 10 s limit at half its old length.
  - missing-features F2 is 0/3 on both models: they tell the user to create the table themselves, and no such path exists yet. P2-04's create-table removes the cause; rerun the set and update F2 then.
  - The F1 check misses a list such as "I can query tables and build views".
- **Follow-ups:** `insert` doesn't return the keys SQLite picks for INTEGER PRIMARY KEY rows, and `describe_table` doesn't show CHECK constraints.
