# P2-03 — Undo turn
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P2-02
**Requirements:** R-16, R-35, N-08

## Goal
*Undo turn* reverts everything one turn changed, and only that (SPEC 2.6).

## Scope
- **`ProjectDB.Undo`:** reverts every change whose source is a message of the turn, newest first. Inserted rows are deleted, updated and deleted rows are restored, and configs, memory and objects return to their previous revision.
- **Conflicts:** a later change to the same row, config, memory section or object stops the undo and lists the conflicts. The user skips them or cancels.
- **Undo as a change:** with the source `undo:<change_id>`, so it can be redone.
- **Window:** changes past the cleanup limit are history only and can't be undone.
- **Not undone:** migrations (2.6). A turn with a migration says so and undoes the rest.
- **Role changes:** undo for a chat's role, from `role_changes` (P1-06 left it for Phase 2).
- **UI:** *Undo turn* in the turn footer, with what it will revert ("12 rows in btc_prices, 1 memory section"). The conflict list is shown in a dialog. The chat gets a notice that the turn was undone, so the model knows.
- **Revert run** uses the same code with `run:<id>`; its button comes with the pipeline page in Phase 4.
- **What P2-02 leaves for undo:**
  - **Order:** follow the rowid of `_jenab_change_rows`, never `change_id`, because chunks of different requests interleave. One row can appear twice in a change, once per chunk. A request cut by a crash or an error leaves a partial entry, which is undone as it stands.
  - **Window:** refuse changes at or below `changes_cleaned_to`, not a fixed 90 days. That includes a change that is only partly cleaned.
  - **Foreign keys:**
    - Cascaded rows are logged in the same change, deepest first. Run each undo with `PRAGMA defer_foreign_keys = ON`, because rows on a reference cycle have no safe order.
    - Undo writes go through the typed write path, so that `reach` and `log` record what cascades touch. That includes rows added after the undone change that now point at the value being restored. Rows reached outside the undone set count as conflicts.
  - **Before-images:** decode `{"$blob":"<hex>"}`. Compare keys in SQLite's `json_array` form. Skip the `object_ref` check when restoring a row.

## Done when
- Undoing a 5,000-row turn takes under 1 s (N-08).
- Tests: undo and redo; a conflict from another chat; a turn with an insert, an update, a delete, a memory edit and a bucket put; a turn past the window; a turn with a migration.

## Result
