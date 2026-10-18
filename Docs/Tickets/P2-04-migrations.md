# P2-04 — Migrations
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P2-01, P2-02
**Requirements:** R-30, R-31, R-32, N-22, N-31

## Goal
Go turns structured migration steps into DDL and runs them in one transaction, or not at all (SPEC 7.5, 8.2).

## Scope
- **Steps:** every `op` in the 8.2 table, from `create_table` to `drop_table`. Each step is validated with an error path the model can use, e.g. `steps[2].column.type: unknown type "STRING" (use TEXT, INTEGER, REAL or BLOB)`.
- **SQL chosen by Go:** `ALTER TABLE` where SQLite allows it, otherwise SQLite's documented table rebuild, with foreign keys off before the transaction and `foreign_key_check` before commit.
- **One transaction:** the steps, any `copy_data` (its query passes the SQL guard), the change-log entry (`migrate`, source `migration:<id>`), and the schema version bump.
- **Schema guard before commit:** the `_jenab_*` entries are unchanged, there are no triggers, SQL views or virtual tables, every agent table has an explicit primary key, and `foreign_key_check` is clean.
- **Try:** the same transaction, always rolled back, returning the full report. A long try runs on a `VACUUM INTO` copy.
- **Long migrations:** estimated from row counts (over 500,000 rows rewritten or scanned), with the time estimate for the approval card and the free-space check (2× on the project drive, 0.5× on the temp drive).
- **Destructive steps:** a snapshot with the safe-copy rule first; the last 5 are kept. Restoring one shows how many writes will be lost.
- **`_jenab_columns`:** `annotate_column` (e.g. `object_ref`). Renaming a table or column must update it, because P2-02 skips stale annotations without a word.
- **`copy_data` runs on the writer, not in a query worker (P2-01):** SQLite can't interrupt a prepare or a long function call, and has no per-query memory limit. So the `from` query needs a bound of its own: run it in a worker and insert the rows through the writer, or explain why the in-process risk is acceptable.
- **Limits from P2-02:** typed writes refuse tables with more than 500 columns, tables with triggers (temp ones too), and BLOB values in keys, so the schema guard keeps tables inside these limits. Migrations write their own change entry with `writeChange` (op `migrate`) and, in the same transaction, one marker row in `_jenab_change_rows` per table they alter: `table_name` = the table, `row_key` = `''`, `kind` = `migrate` (`rowMigrated`), `before` = NULL. Undo (P2-03) compares the markers' rowid with the turn's own rows, so a later migration of a table counts as a conflict. A migration with no marker for a table is invisible to undo. P2-03 adds a store helper that writes the change and its markers; use it. **To decide here:** today any marker after the turn's rows blocks undo of that whole table, so a common flow (insert rows, add a column, fill it) can't be undone without skipping the table. A non-destructive step like `add_column` may not need to block.
- **Dependents:** the hook for views, pipelines and forms, validated inside the transaction. There are none until Phases 3 and 4, so Phase 2 tests it with a fake dependent.

## Done when
- Each step has a test on real SQLite, and SPEC 9.5's "track Ethereum" migration runs as one test: seven steps, nothing lost.
- A failing step or guard check leaves the schema and data byte-for-byte as before.
- A crash test kills a migration and a snapshot mid-way: the old schema or the new one, and only complete snapshots with a final name.
- A long migration on 1 million rows keeps reads working, with p99 under 250 ms (SPIKE-010).

## Result
