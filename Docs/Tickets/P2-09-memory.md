# P2-09 — User and project memory
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P2-02, P2-03
**Requirements:** R-20, R-24

## Goal
User memory and project memory, in sections with caps and revisions, as blocks 2 and 3 (SPEC 3.1, 3.3).

## Scope
- **Storage:** `_jenab_memory` in `project.db`, and `user_memory` in `registry.db` (created in P1-03). Default sections `goals`, `preferences`, `decisions` and `conventions`; the agent may add more.
- **`update_memory(scope, section, content, expected_revision)`:** a stale revision is refused with the current content, so the agent can merge. Empty content deletes the section. Caps: 1,000 tokens for user memory, 2,000 for project memory.
- **Change log and undo:** every edit is a change with the target `memory:<scope>/<section>`. User memory lives in `registry.db`, so its before-revision is kept there and the entry in the project's log points to it. Decide this in the ticket.
- **Content rule:** memory holds intent, never values that live in tables (3.3). It is in the tool's description, the same for every model.
- **Context:** blocks 2 and 3, rebuilt at a cut. An edit from another chat between cuts reaches this chat as a notice, `[memory updated by another chat: preferences → prices in EUR]`, and joins its block at the next cut. For Anthropic, the cache point moves after block 5 (3.1).
- **Chat:** the chip `Memory updated: preferences [undo] [edit]`.
- **UI:** project memory in project settings and user memory in *Settings*, as editable sections. A user's edit is recorded with the source `user`.
- Subagents, pipelines and the schema agent can't write memory.

## Done when
- Tests: a revision conflict and the merge, the caps, delete by empty content, undo of an edit, the notice to a second chat and the block after its cut.
- A scenario: the user states a preference in one chat, and a new chat follows it without being told.

## Result
Done on 2026-10-25 (commit bbb6844), after three review rounds. The scenario passes on gemma4:31b (9/10); Gemini was not run.

- **Built:**
  - **Storage:** `_jenab_memory` in `project.db`; `user_memory` and its change log `user_memory_changes` in `registry.db`. Section names are checked (1 to 40 letters, digits, `_` or `-`) and stored in lower case, with Persian yeh and kaf.
  - **`update_memory`** as in the scope: the revision check returns the current content, the caps, and delete by empty content.
  - **Undo (decided here):** a user memory edit's before-content is kept in `registry.db`, and the project's change log entry points to it. Undo compares the current content, so a later edit to the same section blocks it. A project memory edit undoes like rows.
  - **Context:** both memories are blocks 2 and 3, rebuilt at a cut. An edit from another chat reaches the chat as a notice of at most 400 characters; a shortened notice has no revision, so the agent reads the section before it edits.
  - **UI:** *Settings → Memory* and project settings show the sections for editing; a user's edit has the source `user`. The chat chip's undo and edit state come from the backend.
  - **Runner:** `new_chat: true` on a turn, and `final: true` on `answer_contains` and `answer_lacks` (checks only the last answer).
  - **Set** `scenarios/memory`: M1, a preference given in Persian in one chat and followed in a new one; M2, a "TL;DR first" preference.
- **Scores** (5 runs each): gemma4:31b M1 5/5, M2 4/5. The miss called `list_chats` in turn 2 to read the first chat instead of using memory.
- **Open points:**
  - P2-02's undo skips a change permanently when the project is busy (rows, files and memory alike); a retry may be its own ticket.
  - After the 90-day cleanup, undo of an old user memory edit is refused as "changed since", not as past the undo window as for project changes.
  - Should a memory save ask for approval after the turn read untrusted content? Not built.
  - No live check in the app; the UI is covered by unit tests only.
  - Reports are in `scenarios/memory/results/` (git-ignored).
