# P3-04 — Migrations keep views and forms working
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 3)
**Needs:** P3-01, P3-02, P2-06
**Requirements:** R-31

## Goal
A migration and the updates to the views and forms that depend on it commit in one transaction, or not at all, and a view that no longer validates says so (R-31, SPEC 5.7, 7.5, 8.2).

## Scope
- **Dependents:** from the stored configs, find the views, forms and `rows_from` that use each table and column (the same data the card lists, P3-03).
- **The schema agent (8.2):** its input lists the dependents with the columns they use; it reads full configs with `read_config` and returns updated configs with the migration. The migration and those config saves commit together, each config save validated against the new schema first. Save them with the store's `SaveConfigTx` in the migration's transaction; it doesn't publish, so send `project:configs` for them after the commit (P3-01).
- **Re-validation:** after a migration, open views re-validate on their next load. A view that fails shows an error state on its block (P3-09) and is marked in the project card.
- **Undo:** config saves made by a migration have the source `migration:<id>` and are not reverted by *Undo turn*, as migrations aren't (2.6).
- **Scenario:** in `scenarios/views-and-pages` (or a new set), rename a column used by a table view and a form; the view and form keep working.

## Done when
- Tests: a migration with dependent updates commits all or nothing; a dependent that fails validation stops the migration with its errors; a broken view is marked in the card.
- The scenario passes on the reference models with 3 reps, or the failures are recorded.

## Result
Done on 2026-11-25. Report: [P3-04-report](reports/P3-04-report.md).

- **One transaction:** a migration validates the views and forms that use the tables it changes, against the new schema, on its own connection. It saves the updated configs the schema agent sends with the source `migration:<id>`, so *Undo turn* keeps them, and commits them with the schema change or rolls everything back.
- **What blocks:** an updated view, or one that passed before, that fails afterwards; and a view that was already broken but gets a new issue, such as a column it used being dropped. The problems go back to the schema agent.
- **Broken views:** views already broken with no new issue don't block. The migration reports them, the project card marks them with their first issue, and their blocks get an error with the issues (`Block.Issues`, for P3-09).
- **Schema agent:** it gets each dependent with its type and the columns it uses, and reads configs with `read_config`.
- **Speed:** only the views that use a changed table are checked. With 203 views, an `add_column` took about 17 ms reaching 2 views and about 65 ms reaching 201.
- **Scenario:** `scenarios/schema-dependents` (rename a column used by a table view and a form) passed 3/3 on gemini-3.5-flash-lite and 3/3 on mistral-large-4.
- **Review:** three Medium and five Low points are fixed. A new break in an already broken view now blocks the migration, the check is limited to the views reached, and the broken-view cache follows bucket changes.
