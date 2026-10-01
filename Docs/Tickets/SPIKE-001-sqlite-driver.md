# SPIKE-001 — SQLite driver capabilities
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which cgo-free SQLite driver supports what SPEC v0.3 needs: `modernc.org/sqlite` or `github.com/ncruces/go-sqlite3`?

Checks, for each driver:
1. FTS5 is available (history search, SPEC 2.3)
2. Agent queries can be blocked from `_jenab_*` tables: authorizer callback, or `tables_used()` as a fallback (SPEC 2.2)
3. `VACUUM INTO` works on a `query_only` connection (SPEC 7.5)
4. DDL rolls back inside a transaction (SPEC 7.5)
5. Table rebuild with `foreign_keys = OFF` and `foreign_key_check` (SPEC 7.5)
6. In WAL mode, a reader is not blocked by an open write transaction (SPEC 7.2)
7. Rough write speed: 100,000 rows in 500-row transactions

## Done when
Each check has a pass/fail result for each driver, and a driver is chosen.

## Result
Run on 2026-09-27, Windows 11, Go 1.26.0. Both drivers bundle SQLite 3.53.4. Tested versions: `modernc.org/sqlite` v1.59.0 and `github.com/ncruces/go-sqlite3` v0.35.6. Code: [spikes/001-sqlite-driver](../../spikes/001-sqlite-driver/).

| Check | modernc | ncruces |
|---|---|---|
| FTS5 (create, MATCH, bm25, snippet) | Pass, built in | Pass, via `ext/fts5.Register` per connection (not in the default build) |
| Authorizer callback | **Fail**: not in the Go API (only pre-update, commit and rollback hooks) | **Pass**: `Conn.SetAuthorizer`; blocked all 12 cases* |
| `tables_used()` | Fail: not compiled in | Fail: not compiled in |
| Table detection from `EXPLAIN` bytecode | Pass, 8/8 cases** | Pass, 8/8 cases** |
| Result column names without executing | Pass (`ColumnInfo`) | Pass (`Stmt.ColumnName`) |
| `VACUUM INTO` on a `query_only` connection | **Fail**: "attempt to write a readonly database" | **Fail**: same |
| `VACUUM INTO` on a separate plain connection | Pass: 6 MB in 74 ms; writer kept committing, max latency 6 ms | Pass: 63 ms; max latency 6 ms |
| DDL rollback in a transaction | Pass | Pass |
| Table rebuild with foreign keys off | Pass: 0 violations, child rows kept | Pass |
| WAL reader during an open write transaction | Pass: not blocked, sees committed data only | Pass |
| UPSERT, RETURNING, JSON `->>` | Pass | Pass |
| 100k rows in 500-row transactions, 4 concurrent readers | 267–440 ms, 0 read errors | 273–402 ms, 0 read errors |
| Memory per extra connection | ~2.4–3.0 MB | ~2.3–2.4 MB |

\* The 12 cases were direct access, quoted and schema-qualified names, access through a view, a CTE and a subquery, `PRAGMA`, `pragma_table_info()`, `ATTACH`, and direct `sqlite_schema` reads; plus normal queries still allowed. The first run missed `pragma_table_info()`. The fix is to deny reads of `pragma_*` too, which is fine because the agent has `describe_table`.

\** The `EXPLAIN` method maps `OpenRead`/`OpenWrite` root pages to tables through `sqlite_schema`, and treats `VOpen` (virtual tables) as blocked. It worked for direct access, views, CTEs, subqueries, index-only lookups and `pragma_*` functions. Caveat: SQLite does not promise that the `EXPLAIN` output format stays stable.

**Findings that change the spec**
- `query_only` connections reject `VACUUM INTO`. Snapshots and dry-run copies need a separate plain connection outside the reader pool. This doesn't slow the writer. SPEC 7.5 is already corrected.
- Neither driver has `tables_used()`.
- Speed and memory are about the same for both drivers, so performance doesn't decide the choice.

## Decision
**Reopened on 2026-09-27.** SPEC v0.4 no longer relies on an authorizer. Agent SQL goes through a text check, the `EXPLAIN` check and a guarded connection, and agent DDL is replaced by structured migration steps. [SPIKE-007](SPIKE-007-sql-guard.md) confirmed this on both drivers: 63/63 query cases and 19/19 migration cases.

**Decided (2026-09-27): `modernc.org/sqlite`.**
- It is the most widely used cgo-free driver, with `database/sql`, built-in FTS5 and `sqlite.Limit`.
- The trade-off: its defaults are looser than ncruces'. It runs every statement in a multi-statement string, and it compiles in `dbstat` and `sqlite_dbpage`. The guards cover both.
- ncruces stays the fallback. Its authorizer and stricter defaults would be an extra layer.

The earlier proposal is kept below for reference.

**Earlier proposal: `github.com/ncruces/go-sqlite3`**
- It is the only one with a real authorizer, SQLite's documented mechanism for untrusted SQL. That gives prepare-time blocking of `_jenab_*` tables, `ATTACH` and `pragma_*`.
- FTS5 works through `ext/fts5.Register` in the connection-open callback.
- Keep the `EXPLAIN` check as a second layer in validation. It works on both drivers, so switching drivers later stays possible.
- Agent queries run on a reader pool whose connections get the authorizer. Internal code uses its own pools without it.
- Risk to track: a single maintainer, which is equally true for modernc. Version pinned; the spike tests become regression tests.
