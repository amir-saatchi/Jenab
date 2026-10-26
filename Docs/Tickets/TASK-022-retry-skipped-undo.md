# TASK-022 — Undo the skipped items later
**Type:** Task
**Status:** Open
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
