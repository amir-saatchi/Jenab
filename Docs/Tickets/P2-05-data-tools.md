# P2-05 — Data tools: query, describe_table, insert
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P2-01, P2-02
**Requirements:** R-33

## Goal
The agent reads the project's tables with `query` and adds small entries with `insert` (SPEC 8.1).

## Scope
- **`query(sql, params)`:** through `AgentQuery`. It returns up to 200 rows or 4,000 tokens, plus the total count. A bigger result is stored as a ref with a preview (3.7).
- **`describe_table(name)`:** columns, types, keys, annotations and the row count.
- **`insert(table, rows)`:** at most 50 rows a call, through the typed `Insert`, with the source `message:<id>`. Each row is checked against the column types and `NOT NULL` before the write, and every error comes back at once, with its row and column.
- **Areas:** the three tools have `Areas: tool.Tables`, so block 1 names the project's tables and no longer says "no tables yet" (TASK-014).
- **The `sql-queries` skill:** the SQLite dialect and the guard's rules (8.9), with `load_with` left empty as decided in SPIKE-025.
- **Scenarios:** a set where the agent answers questions from a fixture table, including a refused query (an internal table) that it fixes.

## Done when
- Tool tests for each limit (200 rows, 4,000 tokens, 50 rows), each guard refusal and each insert error.
- The new scenario set passes on the reference models (gemini-3.5-flash-lite and gemma4:31b) with 3 reps.

## Result
