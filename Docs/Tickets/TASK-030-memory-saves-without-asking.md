# TASK-030 — Memory saves without asking
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-026, TASK-028, P2-11

## Goal
The user tried *Refresh memory* on a chat that had read web pages. The Save/Deny card came up, and they couldn't tell what to answer. A card the user can't judge gets clicked through, so it adds friction and protects nothing. Claude, ChatGPT and Codex save memory without asking, with a visible notice and a way to undo. Do the same: at Standard and Auto, memory saves never ask. The notice says when the save came from a chat that read untrusted content, so the user can notice it without deciding anything.

## Today
- `rules["memory"]` in `internal/agent/approve.go` is `{ask, askIfUntrusted, askIfUntrusted}`. An agent's `update_memory` asks while a turn with untrusted results is in the window (TASK-026, TASK-028).
- `refresh.mustAsk` in `internal/agent/refresh.go` asks at Strict, when the reviewed turns or the context turn read untrusted content (at every level), and when an edit changes, deletes or brings back a section the user wrote last.
- The agent's edit shows a `memory_updated` chip; a refresh shows a `memory_refreshed` chip with *View changes* and *Undo*.

## Scope
1. **The rule:** at Standard and Auto, an agent's memory edit and a *Refresh memory* batch never ask. At Strict both still ask, as today.
   - Drop `askIfUntrusted` if nothing else uses it.
   - A refresh that changes a section the user wrote no longer asks either.
   - Keep the untrusted tracking (`cs.untrusted`, `cs.outside`, `untrustedTurn`): hosts still use it, and the chips need it.
2. **The chips:** the `memory_updated` and `memory_refreshed` chips get a short line when it applies:
   - "from a chat that read web pages, files or MCP results": the same test that asks today (the window mark for an agent's edit, `plan.untrusted` for a refresh);
   - "changed a section you wrote" for a refresh, the same test as `userWrote`.
   The line is stored with the notice, so it shows the same after a restart. Right-to-left text must still read correctly.
3. **Remove what's left over:** the untrusted line in the refresh card's risk text, and code paths only the removed asks used. The Strict card stays.
4. **Tests and scenarios:**
   - Update the tests that expect a card at Standard or Auto (`TestMemoryEditAfterUntrusted`, `TestMemoryMarkInWindow`, the refresh tests). They now check: no card, the edit is saved, and the chip carries the line. Strict still asks.
   - The `memory-untrusted` scenario and its offline run in `TestOfflineScript`: the check becomes "saved without a card, and the chip says it came from a chat that read web pages". Record whether the planted line reached memory; that's now expected to happen sometimes and isn't a failure.
   - Run `memory-untrusted` and `refresh-memory` once on each reference model (gemma4:31b, gemini-3.5-flash-lite). Keep the PC awake during the runs.

## Not in scope
- SPEC 3.3, 3.5 and 8.8: Main-Session updates them after the merge.
- Any new check of where an item came from.

## Done when
- At Standard and Auto, no memory card shows for an agent's edit or a refresh, in tests and in the scenarios. At Strict, the card still shows.
- The chips show the untrusted line and the "section you wrote" line when they apply, and not otherwise, also after a restart.
- `go test`, the frontend tests and the bindings check pass.

## Result
Done on 2026-10-29 (commit cd7804c).

- **The rule:** at Standard and Auto, an agent's memory edit and a *Refresh memory* batch save without asking. Strict asks as before. The untrusted tracking stays for hosts and the chips.
- **The chips:** `Notice.Note` stores a muted line under the chip, which the model doesn't read: "From a chat that read web pages, files or MCP results" and, for a refresh, "Changed a section you wrote". Only a save gets a line; a refresh that saved no edit, or whose user-section edit was skipped, gets none. Old chips look as before.
- **Tests:** the memory and refresh tests now check no card, the edit saved and the note, with Strict still asking. A Standard test for each removed refresh ask. The review checked that the tests fail when their rule is undone, and added one for a skipped user section.
- **Scenarios:** new asserts `no_approval` (cards the Auto level answered don't count) and `notice_shown`. `memory-untrusted` U1 and U2 and `refresh-memory` R1 passed on gemma4:31b and gemini-3.5-flash-lite, 19 Gemini requests. The planted line reached memory in no run.
- **Not checked in the running app.**
