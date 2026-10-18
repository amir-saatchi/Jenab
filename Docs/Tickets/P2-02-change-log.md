# P2-02 — Change log and typed writes
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P1-03, P1-05
**Requirements:** R-34, N-08, N-30

## Goal
Every write to `project.db` is a typed request with one change-log entry in the same transaction (SPEC 2.5, 7.3, 7.4).

## Scope
- **Tables:** `_jenab_changes` and `_jenab_change_rows` as in 2.5, with indexes on `(table_name, row_key, change_id)` and `(change_id)`.
- **Typed writes:** `Insert` (with a conflict rule: fail, skip or upsert), `Update` and `Delete` on agent tables. Go builds the SQL from the schema, with names checked and quoted. Large writes go in chunks of 500 rows, each with its own transaction.
- **Row entries:** the primary key as a JSON array, the kind (`i`, `u`, `d`) and the before-image. Tables without an explicit primary key are refused (2.2).
- **The bucket joins the log:** `PutObject`, `DeleteObject` and `MoveObject` write a change entry with the object's previous version (P1-05 left this for Phase 2). Object-reference columns get their key check and are updated on `move`, through `_jenab_columns`.
- **Sources:** each entry has an `id.Source` (`message:<id>`, `user`, `migration:<id>`, …). The message is written before its change (2.3), as P1-10 already does.
- **Cleanup:** a daily job, working forward from a stored watermark, drops row entries older than 90 days (configurable) and keeps the counts per kind in `after`.

## Done when
- A 5,000-row upsert with the change log takes under 1 s on the development machine (N-08).
- A crash test: a write killed mid-chunk leaves whole chunks only, each with its entry, and never an entry without its message.
- Cleanup on a log with 1 million entries keeps the conflict lookup under 10 ms (SPIKE-009).

## Result
Done on 2026-10-18 (commit 7f7a47b), after three review rounds.

- **Built:** `Insert`, `Update` and `Delete` in `internal/store/rows.go`; the log, the chunks and the cleanup in `changes.go`; foreign-key actions in `cascade.go`. Bucket ops log their revisions, and `MoveObject` updates `object_ref` columns.
- **Numbers:**
  - A 5,000-row upsert with half the rows updated takes 150–300 ms on a quiet machine, and up to 900 ms under heavy load.
  - The conflict lookup at 1M entries takes 6.6–7.2 ms on a quiet machine (2.7–8.9 ms after cleanup), with an index search on both sides. Under load it goes over 10 ms, so both timing checks run only with `JENAB_PERF_TEST=1`.
- **Crash test:** 10 kills, with requests cut between chunks every time. Only whole chunks remain, every row entry has its change, and every change has its message.
- **Changes from the ticket and SPEC 2.5:**
  - A `move` op, and an index on `source` for undo.
  - Blobs in before-images are stored as `{"$blob":"<hex>"}`.
  - `Update` and `Delete` take key lists only. The upsert target is always the primary key.
  - Cleanup moves its watermark onto a change as soon as it starts cleaning it, so undo never sees a half-cleaned change.
- **Found in review, now handled:**
  - Foreign-key actions (cascade, set null, set default, on update) are logged in the same change, deepest first, with at most 10,000 row changes per write.
  - Schema `ON CONFLICT REPLACE` or `IGNORE` is overridden with `OR ABORT`.
  - Inserts over 500 rows are checked against every unique index first, with each index's collation and affinity.
  - Invalid UTF-8 and BLOB keys are refused, as are tables with triggers or more than 500 columns.
- **Still open:**
  - A partial unique index, a CHECK, a STRICT type error, or another request writing between chunks can fail a later chunk. Earlier chunks then stay, and `WriteResult` counts them (P2-05 reports this).
  - The retention (`Deps.ChangeKeep`) is not in config.yaml yet.
  - Notes for undo are in P2-03, and notes for migrations in P2-04.
