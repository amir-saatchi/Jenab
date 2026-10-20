# P2-04 — Migrations
**Type:** Feature
**Status:** Done
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
Done on 2026-10-20 (commit 3d25c8c), after three review rounds.

- **Built:**
  - `ProjectDB.Migrate` in `internal/store/migrate.go`: all 12 ops of 8.2 parsed into typed steps, unknown fields refused, and every problem returned at once with its path. One transaction with foreign keys off around it: the steps, the dependents' updates and checks, the schema guard, the `schema_version` bump and the change log.
  - A planner on a reader finds the destructive steps and estimates rows, time and bytes. Destructive steps without approval return `ErrNeedsApproval`. The plan is checked again inside the transaction, and a migration that now drops more is refused.
  - Try, with a long try on a `VACUUM INTO` copy. The free-space check for long migrations, in the new `internal/disk` package.
  - Snapshots in `snapshots/project-migration-<ULID>.db` (`snapshots.go`), the last 5 kept, and `RestoreSnapshot` on a closed project.
  - The dependents hook (`AddDependent`) and `annotate_column`. Renames, rebuilds and drops update `_jenab_columns`.
- **Tests:**
  - One real-SQLite test per op, and SPEC 9.5's seven steps with three view updates as one migration.
  - The file is byte-for-byte the same after a failing step, a failing guard and a try.
  - The crash test kills migrations and snapshots mid-way (12–14 s).
  - 1M rows: reads kept working, p99 0.5 ms.
- **Decided here:**
  - **Undo:** a table that a migration only touched with `add_column`, `create_index` or `drop_index` gets a `migrate_keep` marker, so it doesn't block undo of earlier writes (SPEC 2.6). Every other op still blocks.
  - **`copy_data` runs on the writer, in the app's process.** A worker can't see the migration's own uncommitted tables, and piping the rows out and back would cost a lot. Its bounds are:
    - SQL guard layers 1 and 2;
    - no parameters;
    - `SQLITE_LIMIT_LENGTH` at the reader's 16 MiB;
    - a deadline of 1 minute plus 20 µs per source row.

    What's left is one long SQL function call, which runs until the deadline stops it.
  - **The snapshot is taken while the migration holds the writer,** so no write lands between the snapshot and the migration. Writes wait while it is made (SPEC 7.5).
- **Changes from the ticket:**
  - Expressions in checks and mappings must stay inside their own bracket (`sqlguard.CheckExpr`), and layer 2 runs on them before the DDL.
  - Foreign keys into `_jenab_*` tables are refused.
  - Rebuilds keep `STRICT` and `WITHOUT ROWID`. A rebuild from the schema text refuses clauses Go can't carry over.
  - The foreign-key check covers only the touched tables and their children, or every agent table after a dependent update.
- **Open points:**
  - `RestoreSnapshot` isn't wired into `project.Manager` or the UI. Bucket bytes swept after the snapshot aren't handled on restore.
  - The progress notice during a long migration and the approval card come with P2-06 or later.
  - The time estimate uses SPEC 7.5's slow-end numbers and is 6–8× high on this machine. The snapshot's own time isn't measured.
  - `drop_column` rebuilds from the schema only when the table has no CHECK, COLLATE, GENERATED or AUTOINCREMENT; otherwise it asks for `rebuild_table`.
  - The free-space check isn't run again when the plan changes inside the transaction.
