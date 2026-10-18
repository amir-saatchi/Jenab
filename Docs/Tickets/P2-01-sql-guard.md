# P2-01 — SQL guard
**Type:** Feature
**Status:** Done
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
Done on 2026-10-18 (commit dcea668), after four review rounds. SPEC 2.2 now describes what was built.

- **Built:**
  - `internal/sqlguard`: the lexer, the budget, the EXPLAIN check, and the function allow-list and deny list.
  - `internal/sqlworker`: the worker process, its pool and the protocol.
  - `ProjectDB.AgentQuery` in `internal/store`.
- **Bypasses found in review, now refused:**
  - a `#` parameter (Tcl form `#a(...)`) hiding a `;`, so the EXPLAIN step ran the hidden statements;
  - a BOM at the start of a name;
  - `json_each` used as an alias to switch the virtual-table check off;
  - `rtreecheck()` reading internal tables.
  Layer 1 is now an allow-list of characters. Layer 2 prepares the query once in a form that only compiles as one statement, and checks `VOpen` by the eponymous table's address.
- **Why there is a worker:** SQLite can't interrupt a prepare or a single long function call, and this build has no per-query memory limit. Review found nested subqueries, JSON aggregates, `instr`, `LIKE` and big arguments costing seconds and gigabytes in-process. Each worker is capped at 256 MiB by a Job Object and killed at the deadline. All those attacks stop within the deadline, and the app's own memory doesn't grow.
- **Numbers:**
  - Layer 1 takes 2–4 µs and layer 2 about 100 µs.
  - The worker round trip adds 0.1–0.25 ms. A worker starts in 35–70 ms, or about 1.7 s on the first start after a new build, so one starts in the background when a project opens.
  - Five realistic analytic queries on 100k rows take 0.3–0.8 s and peak at 52–70 MiB.
- **Tests:** every attack is a permanent case. A byte sweep runs 4,112 queries, and `FuzzAgentQuery` (seeded with SPIKE-007's 63 cases) checks that a query that passes changes nothing.
- **Changes from the ticket:**
  - Values are capped at 1 MiB for agent queries (decided by the user), parameters at `?999`, and results at 8 MiB.
  - The 300-value budget was tried and dropped, because it refused normal reports.
  - Paths in `file:` URIs are now escaped (`#` and `%` broke them).
- **Still open:**
  - Linux and macOS build but are untested. Their memory limit is a 20 ms watchdog, not a hard cap.
  - `copy_data` runs on the writer without a worker (see P2-04).
  - `db.query` needs its own result limit (see P2-05).
