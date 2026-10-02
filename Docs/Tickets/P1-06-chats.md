# P1-06 — Chats: storage, Mother chat, titles and roles
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-04, P1-19
**Requirements:** R-10, R-11, R-12

## Goal
`chats.db` and the chat model (SPEC 2.3, 8.6).

## Scope
- **Storage:** `store/chats.go` with `ChatsDB` and its format steps, on the P1-03 store and the P1-19 types (CODE-OUTLINE `store`). `Search` comes with P1-07.
- **Tables:** `chats`, `messages`, `message_parts` as in 2.3. `session_notes` and `review_chunks` are created now and used in Phase 2.
- **Parts:** all part types; `thinking` parts kept with their signature (3.8); large outputs as `ref` plus preview.
- **Writes:** streamed text is written when a part completes; the assistant message and its `tool_call` part are written before the tool runs (2.3).
- **Mother chat:** created with the project, `created_by` = `app`; it can be cleared but not archived or deleted.
- **Titles:** generated after the first turn (P1-10), fixed once the user sets one, unique within the project.
- **Roles:** at most 500 tokens; a change is recorded with its source. Undo for role changes comes with the change log (Phase 2).
- **Model:** `chats.model`; new chats use `default` (3.9).
- **Sequence:** a counter per chat for snapshot plus sequence (Q32).

## Done when
- Integration tests for every table and rule above.
- Killing the process while a part streams loses only that part.
- Every new project has its Mother chat, and it survives *Clear*.
