# P2-03 — Undo turn
**Type:** Feature
**Status:** Done
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
Done on 2026-10-18 (commit b52b1fa), after four review rounds.

- **Built:** `internal/store/undo.go` (plan, conflicts, revert), `undo_roles.go` (chat roles), `internal/agent/undo.go` (the turn, the notice), and *Undo turn* / *Redo turn* in the turn footer with a conflict dialog.
- **Numbers:**
  - A 5,000-row turn on a 100,000-row table undoes in 0.47–0.55 s, and in 0.81–0.89 s under an already undone turn of the same size.
  - The largest undo (20,000 rows) takes 1.4–1.9 s, redo 1.6–2.5 s, on a machine about 40% busy.
- **Changes from the ticket:**
  - Every reverted change gets its own undo entry, even with no rows left to revert. The parity of a change's undo chain says whether it is undone, so redo is undo of the undo.
  - A later change counts as a conflict unless an undo really put that item back. Skipped items, roles too, flip their chain with a no-op entry.
  - Migrations conflict by commit order: a migration writes one marker row per table through `writeMigration` (contract in P2-04).
  - One undo reverts at most 20,000 row entries (`MaxUndoEntries`); a bigger turn is refused before anything is written.
  - The notice for the model is plain text; table names get bidi isolates only in the UI.
- **Still open:**
  - Redoing two turns oldest first (undo B, undo A, redo A, redo B) gives a false conflict on B's rows. Nothing is lost, but B can't be redone without a skip.
  - A migration after the turn's rows blocks undo of the whole table (decision in P2-04).
  - Cleanup drops changes in ID order, so a migration marker can go before a turn that it should still block. It needs a migration right at the cutoff.
  - No-op role records show in `RoleChanges()`; a future role history should hide `before = after`.
  - Revert run (Phase 4) needs a chunked, resumable form, since the cap refuses more than 20,000 rows.
