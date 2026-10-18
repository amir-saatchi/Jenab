# P2-01 — SQL guard
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P1-03
**Requirements:** N-20

## Goal
Every query the agent writes passes the three guard layers before it reads anything (SPEC 2.2, CODE-OUTLINE 4 `sqlguard`).

## Scope
- **Layer 1, the text check:** the lexer from SPIKE-007, in Go, before SQLite sees the query. One statement; `SELECT`, `WITH` or `VALUES` only; no `_jenab_`, `sqlite_` or `pragma_` names; only the `json_each` and `json_tree` modules; no `load_extension`; no NUL bytes.
- **Layer 2, the EXPLAIN check:** NULL bound to every parameter. It rejects opening internal tables, `sqlite_schema`, temp or attached schemas, virtual tables other than layer 1's, and write opcodes. The table map is cached per `schema_version`.
- **Layer 3, the guarded reader:** `query_only`, `SQLITE_LIMIT_ATTACHED = 0` and `SQLITE_LIMIT_LENGTH = 16 MiB` are already set by P1-03. This layer adds a deadline per caller (10 s for `query`, 2 s for views later) and interrupts the query when it runs out.
- `ProjectDB.AgentQuery(ctx, q, args, max)` runs the three layers and returns rows, the total count and whether the result was cut.
- **Errors the model can fix:** the layer, the reason and the position, e.g. "line 1:15: `_jenab_changes` is internal". SQLite's own errors (no such column) pass through with their text.

## Done when
- SPIKE-007's 52 attacks and 11 normal queries run as permanent tests on `modernc.org/sqlite`: every attack is refused, every normal query runs.
- A query over its deadline is interrupted and leaves the reader usable.
- A benchmark keeps layer 1 under 10 µs and layer 2 under 200 µs per query.

## Result
