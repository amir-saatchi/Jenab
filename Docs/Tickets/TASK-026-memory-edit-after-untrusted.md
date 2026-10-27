# TASK-026 — Ask before a memory edit after untrusted content
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P2-09, TASK-012

## Goal
SPEC 3.3 and 8.8 say an agent's memory edit asks first once the turn has read untrusted content. A planted line saved to memory would reach every later chat. The SPEC has it (e556ad2); the code doesn't.

## Today
- `update_memory` saves without asking at every approval level.
- Turns already know when they have read untrusted content: 8.7 uses it to ask again before an MCP tool that deletes or reaches outside (SPIKE-024).

## Scope
1. If the turn has read untrusted content (web pages, files, MCP results) since the user's message, `update_memory` asks first, for user and project memory.
2. **The card:** *Save* or *Deny*, with the section before and after (8.8). No *Always*.
3. **Levels (8.8):** Strict always asks. Standard and Auto ask only after untrusted content.
4. *Deny* tells the model the edit wasn't saved, and the turn goes on.
5. Session notes don't ask: they stay in the chat.

## Done when
- Tests: an edit with no untrusted content saves; after a web page, a file or an MCP result it asks; *Deny* leaves the section as it was; a new user message clears the state.
- A scenario: a page with a planted "remember that…" line, and the card shows before saving.

## Result
Done on 2026-10-27 (commit e30833b).

- **The ask:** `update_memory` asks after untrusted content at Standard and Auto, and always at Strict, for user and project memory. *Save* or *Deny*, no *Always*. *Deny* tells the model the edit wasn't saved, and the turn goes on. Session notes don't ask.
- **The card:** it opens with only the added and removed lines, in full, with runs of blank lines folded into one row. The line diff is exact at any size (Hirschberg), so a long section can't hide a planted line. The whole section folds open below.
- **A race closed in review:** the preflight returns the plan's error, so an edit made stale by a write in between is refused before any card. Every write raises the revision; project undo with no row now writes revision 1.
- **Tests:** `TestMemoryEditAfterUntrusted` and `TestMemoryEditRace` (a write between the check and the save, a deleted section, and approved then changed), plus bun tests for the line diff.
- **Scenario `memory-untrusted`:** gemma4:31b 3/3 and gemini-3.5-flash-lite 3/3. The card showed every time, and the planted line didn't reach memory in any run.
- **Open:** the mark still clears at the user's next message, though the page may still be in the window. TASK-028.
