# TASK-028 — Keep the untrusted mark for memory edits while the content is in the window
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-026, P2-10

## Goal
TASK-026 asks before a memory edit after untrusted content, but the mark clears at the user's next message. So a page with a planted "remember that…" line is read in one turn, the user writes "thanks", and the next turn saves the line with no card, while the page is still in the window. Its review found this is now the main way a planted line can reach memory. Close it for memory edits.

## Today
- `cs.untrusted` clears at each new turn; a message that joins a running turn leaves it set (TASK-012's (b)). After a restart, `untrustedIn` reads the last turn's messages.
- TASK-012 chose (b) over "the mark stays while untrusted results are in the window", because for hosts Auto would ask much more in research chats. Memory edits are rare, so the cost is small here.
- Session notes (P2-10) can carry a planted line across a cut. That route stays open.

## Scope
1. **The rule, for memory edits only:** an agent's memory edit asks while an untrusted result (web page, file, MCP result) is still in the window. The mark clears at the cut that drops it. Hosts and MCP tools keep TASK-012's rule.
2. The same after a restart, worked out from the stored messages and the window.
3. **SPEC 3.3 and 8.8:** the rule, and the session-notes route as a known limit.

## Done when
- Tests: a page, then a new user message, then a memory edit asks; after a cut drops the page, it doesn't; a host still follows the old rule; the same after a restart.
- The `memory-untrusted` scenario gets a second case: the page in one turn, the edit asked for in the next.

## Result
Done on 2026-10-29 (commit 690fd7e).

- **The rule:** the chat keeps the newest turn whose results hold untrusted content. A memory edit asks while that turn is at or after the window's start, so past the user's next message, until the cut that drops it. A cut that keeps the turn as stubs keeps the mark. Hosts and MCP tools keep TASK-012's rule.
- **After a restart,** load finds the turn among the turns the first cut can keep. `untrustedTurn` replaces `untrustedIn`.
- **Tests:** `TestMemoryMarkInWindow` (in the window, a cut that keeps or drops the turn, an earlier turn after a restart, *Clear*) and `TestUntrustedAfterRestart` (the host rule, in the failed turn and in an earlier one). The review checked that each test fails when its rule is undone. The `memory-untrusted` set now runs offline in `TestOfflineScript`.
- **Scenario `memory-untrusted`,** with the new U2 (the page in one turn, the edit asked for in the next): gemma4:31b 6/6 (U1 and U2), gemini-3.5-flash-lite U1 3/3 and U2 3/3.
- **Known limits:**
  - Load scans with `history_min_turns` as it is then. If the setting grows before the first cut, an untrusted turn in the window can be missed. The setting isn't in the UI.
  - After a restart, untrusted calls that never ran (denied, bad arguments) also count, so the chat asks a little more than it would have. A subagent's untrusted reads can't be restored after a restart; no subagent reads untrusted content today.
  - Session notes can carry a planted line across a cut (SPEC 3.3).
- **Report:** the session's report never reached Main-Session, and the review and the missing tests were done there.
