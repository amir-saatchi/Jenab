# TASK-022 — Undo the skipped items later
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P2-03, P2-09

## Goal
When *Undo turn* meets a conflict, the user can skip the conflicting items (SPEC 2.6). Today a skipped item can never be undone afterwards, even once the conflict is gone. It should stay undoable: the user can undo it later, for example after undoing the later change that stood in the way.

## Today
- A skipped item flips its undo chain with a no-op entry (P2-03 Result), so the chain reads as undone although nothing was put back. A later *Undo turn* finds the turn already undone, and redo flips the item back with another no-op.
- This holds for rows, files, objects and project memory sections. User memory already differs: P2-09 made a skip leave the edit in effect, so a later undo tries again. The new model should make both work the same way.
- P2-03 also left open: redoing two turns oldest first gives a false conflict (undo B, undo A, redo A, redo B).

## Scope
1. **Confirm first** with failing tests: a turn undone with a skipped row, then the later change undone, then *Undo turn* again. The same for a file, an object, a project memory section and a user memory section.
2. **Decide the model** and write it into SPEC 2.5 and 2.6 before building. For example, a skipped item records a `skipped` entry that doesn't count in the chain's parity, and a turn with skipped items reads as *partly undone*. *Undo turn* on a partly undone turn undoes only the items still in effect, and asks about the ones that still conflict.
3. **Chat:** the turn's undo chip says "partly undone: 2 items skipped [undo the rest]". Redo of a partly undone turn redoes only what was undone.
4. **Keep:** the 20,000-entry cap, the 90-day window, and P2-09's user memory log in `registry.db`.
5. Fix the false conflict on redo in the same model if it falls out naturally; otherwise leave it open and say why.

## Done when
- Tests for each kind (row, file, object, project memory, user memory): skip, clear the conflict, undo the rest, redo, and undo again.
- The existing undo and memory tests still pass.
- SPEC 2.5 and 2.6 describe skipped items and partly undone turns.

## Result
Done on 2026-10-26 (commit a4c10bb), after three review rounds.

- **Built:**
  - **Per-item undo state:** an undo writes an `s` entry for each item it leaves, so a skipped item stays undoable. The redo false conflict (undo B, undo A, redo A, redo B) is fixed too: a later chain that cancels out is no conflict. The same for rows, files, objects, project and user memory, and roles.
  - **Partly undone turns:** the side comes from the newest undo or redo across the three databases. The footer says "partly undone: N items not undone" or "partly redone: N items not redone", with *Undo the rest* or *Redo the rest*, and the notice says "the rest of turn N". If every item is skipped, nothing is written.
  - **Undo window:** a chain whose first change is past the cleanup watermark is history as a whole, since the cleanup drops its `s` entries.
  - **Old projects:** `project.db` format step 5 adds the `s` entries older undos left out.
  - Errors say undone or redone as it was (`agent.PartError`).
- **Tests:** skip, clear the conflict, undo the rest, redo and undo again for each kind; redo oldest first; the cleanup between two partial undos; old undos at format 4; the side across databases. A random test in review (600 runs of 60 steps) never overwrote a change in effect. A 5,000-row turn undoes in 0.1–0.2 s.
- **Known limits:**
  - The side is chosen by wall-clock time. If the clock moves back, only the footer labels can be wrong; each button runs its own side's full plan.
  - Step 5 reads an item as skipped where an old undo really changed nothing for it, so such a turn reads partly undone until one more click.
  - A turn older than 90 days that was undone recently can't be redone.
  - Not tried in the running app; the footer is covered by unit tests only.
