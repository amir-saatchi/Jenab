# SPIKE-007 — SQL guard without an authorizer
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can we keep agent-written SQL safe without relying on SQLite's authorizer, so that any cgo-free driver works (SPEC 2.2, 7.5)?

Guards for agent SQL (`query`, `db.query`, view queries, the SELECT in `copy_data`):
1. Runs on a `query_only` connection
2. `ATTACH` blocked with `SQLITE_LIMIT_ATTACHED = 0`, set when the connection opens
3. Exactly one statement: reject if the prepared statement leaves a non-empty tail
4. EXPLAIN check: no `_jenab_*` table, no direct `sqlite_schema`, no virtual table (`VOpen`), no `OpenWrite`
5. Timeout through `context`

Schema guard for migrations, before commit:
6. `_jenab_*` rows in `sqlite_schema` are unchanged
7. No triggers or SQL views exist
8. Every agent table has an explicit primary key

## Done when
- A list of at least 25 bypass attempts is blocked by guards 1–5 on both modernc and ncruces. It includes `pragma_*` functions, views, CTEs, quoting, comments hiding a second statement, `ATTACH` through a CTE, `load_extension`, `sqlite_dbpage` and recursive CTEs.
- Migrations that try to drop, alter or add a trigger to a `_jenab_*` table, or create a view or trigger, are rolled back by guards 6–8.
- Normal queries still pass, and the guards add less than 1 ms per query.

## Result
Run on 2026-09-27, Windows 11, Go 1.26.0, SQLite 3.53.4, with modernc v1.59.0 and ncruces v0.35.6. Code and full output: [spikes/007-sql-guard](../../spikes/007-sql-guard/) (`results-modernc.md`, `results-ncruces.md`).

The guards were built as three layers, run in this order:

1. **Text check** (in Go, before SQLite sees the query): a small lexer that follows SQLite's rules for strings, quoted names and comments. It rejects:
   - more than one statement
   - anything not starting with `SELECT`, `WITH` or `VALUES`
   - names starting with `_jenab_`, `sqlite_` or `pragma_`
   - virtual table modules other than `json_each` and `json_tree`
   - `load_extension` and similar functions
   - NUL bytes
2. **EXPLAIN check**: the compiled program must not:
   - read an internal table, `sqlite_schema`, or the temp or attached schemas
   - open a virtual table, unless it is `json_each` or `json_tree`
   - start a write transaction or use a write opcode
3. **Connection**: `query_only`, `SQLITE_LIMIT_ATTACHED = 0`, `SQLITE_LIMIT_LENGTH = 16 MiB`, and a 2 s timeout.

| Check | modernc | ncruces |
|---|---|---|
| 63 agent query cases (11 allowed, 52 attacks) | **63/63 correct** | **63/63 correct** |
| Where the attacks were stopped | Text 45, EXPLAIN 2, connection 5 | Same |
| 19 migration cases (6 valid, 13 attacks) | **19/19 correct**, every rollback clean | **19/19 correct** |
| Cost: text check / EXPLAIN check / running the query | 2.3 µs / 68 µs / 799 µs | 2.3 µs / 76 µs / 1,053 µs |
| Schema guard with 104 tables | ~1 ms per migration | ~1 ms |
| A string with several statements | **Runs all of them**, even under `EXPLAIN` | Rejected: "multiple statements" |
| Modules compiled in | dbstat, fts5, fts5vocab, geopoly, rtree, sqlite_dbpage | fts5, fts5vocab, generate_series |
| `load_extension` | "not authorized" | Not compiled in |

**Findings**
- **Some PRAGMAs act while the statement is being prepared.** Just preparing `PRAGMA query_only = 0`, or running `EXPLAIN PRAGMA query_only = 0`, turns read-only off on both drivers. So the text check must run before any prepare, including `EXPLAIN` and column lookups during validation.
- **No single layer is enough on its own:**
  - The EXPLAIN check misses `PRAGMA` statements and NUL tricks.
  - The connection layer misses all reads of internal tables.
  - The text check misses `WITH … DELETE`; the EXPLAIN check and `query_only` stop it.
  - Only the connection layer stops runaway queries, through the timeout and the length limit.
- **modernc refuses to `EXPLAIN` a query with unbound named parameters.** The check binds NULL to every parameter.
- **Temporary tables inside a query are fine.** Recursive CTEs and sorting use `Insert` on a temporary table, so row opcodes are not a sign of writing. Writing a real table always shows `OpenWrite` or a write `Transaction`.
- **The attack list also includes** the `IN tablename` shortcut, `INDEXED BY`, quoted `pragma_*` names, `CREATE TEMP VIEW`, and a temp trigger on the writer connection. The schema guard checks `sqlite_temp_schema` too.

## Decision
**Decided (2026-09-27): adopt the three layers as SPEC 2.2 describes them.** Also:
- reset `PRAGMA query_only = 1` each time a reader is taken from the pool, so one wrongly prepared statement cannot leave a writable reader behind
- set `SQLITE_LIMIT_ATTACHED = 0` on the writer as well

The authorizer is not needed. **Both drivers work**, so SPIKE-001 can choose on other grounds. ncruces has stricter defaults: it rejects multi-statement strings and compiles in fewer modules. With modernc, the text check is the only thing that stops a second statement, backed by the `query_only` reset.
