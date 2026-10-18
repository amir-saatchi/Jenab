# P2-02 — Change log and typed writes
**Type:** Feature
**Status:** Open
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
