# P2-11 — Refresh memory
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P2-09, P2-10
**Requirements:** R-26

## Goal
A button per chat makes memory and the session notes hold everything important from the chat, then cuts the history window (SPEC 3.5).

## Scope
- **Split:** messages after `memory_reviewed_up_to`, at turn boundaries, into chunks of up to 5 turns or 12,000 tokens, with tool outputs as stubs.
- **Map:** one background call per chunk, with the `fast` alias at background priority, within `max_parallel_calls`. Each returns items with a kind, text, scope and source messages. The output is JSON checked against a schema, the way SPIKE-030 gets it from any model.
- **Merge and compare:** one call merges the items in chat order and drops data values. It labels each item `present`, `missing`, `contradicts` or `outdated` against the current memory and notes, and rewrites the notes' `summary`.
- **Apply:** normal memory edits with the source `refresh:<chat_id>`, and one chip, `Memory refreshed: 3 added, 1 changed · view changes · undo`. *Undo* reverts the batch.
  - It asks first, with the diff in an approval card, at Strict, or when an edit changes a section the user wrote last (8.8).
- **Cut:** the chat keeps `history_min_turns` turns, and blocks 2–5 are rebuilt.
- **Progress:** `memory_reviewed_up_to` moves forward and chunk results go in `review_chunks`, so the next refresh reads only new messages. *Full refresh* discards them.
- **Cost:** the estimated tokens are shown before a long chat's refresh starts.
- **Stopping:** a refresh can be cancelled; nothing is applied until the merge finishes.

- **From P2-10** (`SaveSummary` exists, but nothing calls it yet):
  - When the notes are near the cap, a summary is refused with the notes' cap error, and nothing yet shortens the notes to make room.
  - The "over the cap but shorter than before" rule applies to `SaveNotes` only.
  - *Clear* deletes the row, so the revision goes back to 0. A refresh holding an older revision could match by chance.
  - A summary save bumps the chat's `seq`, but no event tells the UI yet.

## Done when
- Tests with `provider/fake`: the split boundaries, a second refresh reading only new turns, the approval at Strict and for a user's section, undo of the batch, cancel before apply.
- A scenario on the reference models: a 30-turn chat refreshed, then a new chat answers questions about it from memory alone, with the answers checked.

## Result
Done on 2026-10-29 (commit 5500900).

- **Built as SPEC 3.5 now says:** split, map with the turn before as context, merge, a re-read of memory before the card, apply, one chip with an all-or-nothing undo, the cut at the next turn, saved chunks, Full refresh, the estimate, and the busy chat.
- **Decisions with the user:**
  - JSON with the schema in the prompt for every model (SPIKE-030's text side). Each provider's JSON mode is TASK-029.
  - The chat is busy during a refresh.
  - One Save/Deny card. On *Deny* the summary still saves and progress moves on.
  - The card also asks when the reviewed turns or the context turn read untrusted content, at every level.
- **From P2-10:** the summary gets the "shorter than before" rule; with no room left, no summary is written. *Clear* is refused during a refresh. A summary save is counted by the chip written with it.
- **Review:** two rounds. Fixed:
  - a section the user deleted came back with no card;
  - undo was half-done across the two memories;
  - a failed finish said "nothing was changed";
  - a strict items schema could fail a whole refresh;
  - a stale card;
  - a turn could start during a refresh;
  - several Stop and cancel cases.
- **Scenario `refresh-memory`** (a synthetic 30-turn chat, then a new chat answers from memory alone, without searching the old chat):
  - gemma4:31b passed 3/3 in each of three batches.
  - gemini-3.5-flash-lite passed 3/3 in the first batch. After the prompts changed: 1 pass, 1 run lost to the PC going to sleep, and 1 lost to Gemini 503 "overloaded" errors. Not counted as refresh failures.
  - Without a refresh (the control, gemma), the chat searched the old chat and found two of three facts.
  - Recorded, not tuned: some assistant advice and one user question were saved as decisions or preferences, and the same fact landed in different sections.
- **Not checked in the running app** before the commit; the user is trying it.
- **Known limits:** undo commits project and user memory apart, so a crash between them leaves one half. A crash between the edits and the progress leaves edits with no chip; the next refresh reviews those turns again.
