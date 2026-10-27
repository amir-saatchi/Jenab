# P2-10 — Session notes
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P1-06, P2-09
**Requirements:** R-20, R-25

## Goal
Each chat keeps notes on its current work, as block 5 (SPEC 3.1, 3.4).

## Scope
- **The SPEC's shape:** the request list moved to SPIKE-036 (later), and SPIKE-029 (subjects) and SPIKE-034 (topics) are deferred, so nothing else goes next to the notes in block 5 for now.
- **`update_session_notes(content, expected_revision)`:** the revision rule of memory. A cap of 2,500 tokens, of which the `summary` section (at most 1,000 tokens) is written only by *Refresh memory*.
- **Storage:** `session_notes` in `chats.db` (created in P1-06), deleted with the chat and emptied by *Clear*.
- **Context:** block 5, rebuilt at a cut. Edits between cuts are already in the turn as the tool call, so they need no notice.
- **Where things go:** "Would a new chat in this project need it? Then project memory. Only this conversation? Then session notes" (3.4), in the tool descriptions.
- **UI:** the notes, read-only, from the chat header.

## Done when
- Tests: the revision conflict, the caps, the summary section refused to the agent, *Clear*, the block after a cut.
- A scenario: a plan in a long chat survives a cut, and the model continues it without searching the history.

## Result
Done on 2026-10-27 (commit 58ea518).

- **Store:** a `summary` column in `session_notes` (a new format step). One revision for both sections. `SaveNotes` refuses a stale revision with the current content, and caps at 2,500 tokens counting the summary. `SaveSummary`, for *Refresh memory*, has its own 1,000 cap. *Clear* and deleting the chat remove the notes.
- **Tool:** `update_session_notes`, with the Memory effect, so subagents don't get it. Its description and `update_memory`'s have the 3.4 "where things go" line. Copy-back drops the block's first line, the section headers, a reworded or Persian summary header, and a leading copy of the summary; the model's own lines, headings and indentation are kept.
- **Context:** block 5 at each cut, after the card and Mother's chat list and before the earlier turns, so the cache point stays last. Mother's chat list falls back to the summary's first line.
- **UI:** a *Notes* button in the chat header opens a read-only sheet with the summary and notes, right-to-left where needed. Not checked in the running app.
- **Scenario `session-notes`** (a six-part plan, then "next", with a cut at turn 5):
  - gemma4:31b 2/3, gemini-3.5-flash-lite 0/3.
  - The plan survived the cut in every run that saved notes, and no run searched the history once it had notes. The failures are the model dropping exact detail when writing its notes (the question's wording in part 6). One Gemini run never saved notes and used `read_messages` instead. Recorded, not tuned.
- **Review:** two rounds. Fixed: copying the block back, header matching, the model's own short lines and headings, an untested cap branch, an extra chat reload per save, and the scenario's time match.
- **Found, not fixed here:** SPIKE-032's number check counts numbers in tool descriptions as sources, with its coarse match, so a made-up 1,040 counts as sourced through `update_memory`'s "1,000". It only affects scenario runs. P2-10's description has no cap number for this reason.
- **Left for P2-11:** see its ticket.
