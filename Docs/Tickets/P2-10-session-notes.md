# P2-10 — Session notes
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P1-06, P2-09, SPIKE-028
**Requirements:** R-20, R-25

## Goal
Each chat keeps notes on its current work, as block 5 (SPEC 3.1, 3.4).

## Scope
- **Decide first:** SPIKE-028 (the request list) may put objects next to the notes in block 5, and its decision sets this ticket's final shape. SPIKE-029 (subjects) and SPIKE-034 (topics) are deferred. Until then the scope below is the SPEC's.
- **`update_session_notes(content, expected_revision)`:** the revision rule of memory. A cap of 2,500 tokens, of which the `summary` section (at most 1,000 tokens) is written only by *Refresh memory*.
- **Storage:** `session_notes` in `chats.db` (created in P1-06), deleted with the chat and emptied by *Clear*.
- **Context:** block 5, rebuilt at a cut. Edits between cuts are already in the turn as the tool call, so they need no notice.
- **Where things go:** "Would a new chat in this project need it? Then project memory. Only this conversation? Then session notes" (3.4), in the tool descriptions.
- **UI:** the notes, read-only, from the chat header.

## Done when
- Tests: the revision conflict, the caps, the summary section refused to the agent, *Clear*, the block after a cut.
- A scenario: a plan in a long chat survives a cut, and the model continues it without searching the history.

## Result
