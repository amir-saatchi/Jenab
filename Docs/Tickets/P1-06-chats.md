# P1-06 — Chats: storage, Mother chat, titles and roles
**Type:** Feature
**Status:** Done
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

## Result
- **`store/chats.go`:** `ChatsDB` on `chats.db`, format step 1 with `chats`, `messages`, `message_parts`, `session_notes`, `review_chunks` and `role_changes`.
  - `message_parts.content` is the part's own field as JSON. `ref` and `preview` repeat a large output's bucket key and preview, and an image's key.
  - `AppendMessage` writes a message with its parts so far in one transaction. `AppendPart` adds one finished part and returns its index.
  - `Messages` reads turns in one read transaction, with the `seq` they are current at.
- **Sequence:** `chats.seq` goes up by one with every write to the chat, in the same transaction, and every write returns it (Q32).
- **Mother chat:** `EnsureMother` runs when a project is created and at every open, so a crash between the two files can't leave a project without one. One Mother per file is enforced by an index. `Archive`, `DeleteChat` and `SetRole` refuse it with `ErrMother`. `Clear` works on any chat: it removes messages, notes and review results, and keeps title, role and model.
- **Titles:** unique within the project, ignoring case. A generated title is skipped once the title is fixed, and gets " 2", " 3"… when another chat has it. A title set by the user or Mother fails with `ErrTitleTaken`.
- **Roles:** at most 500 tokens (4 bytes a token). Each change is a `role_changes` row with its source.
- **Session notes:** `SaveNotes` uses the memory revision rule (`ErrConflict`) and the 2,500-token cap.
- **Project:** `Project.Chats` opens and closes with `project.db`, read-only after damage. A missing `chats.db` is made new even then, since it holds nothing to protect; the snapshot offer for damaged files stays as in 2.7. `Activity` reports the chats writer too.
- **Left for later:** FTS and `Search` (P1-07); generating titles (P1-10); answering approval and question parts (P1-11); `ui_state`, `default_page` and `memory_reviewed_up_to` writers (5.9, Phase 2); undo of role changes (Phase 2).
- **Tests:**
  - Every table and rule: Mother, titles, roles, all part types round-tripped (thinking signature, Gemini `extra` byte for byte), ref and preview, turn ranges, sequence on every write, Clear, delete cascades, notes revisions, read-only.
  - Crash test: a worker streams text parts into the Mother chat and is killed at random. Every part it reported is there, every stored part is whole, and `seq` equals the number of writes. The project crash test also writes chats and checks recovery.
  - Mutation checks: 47 on the tables, titles, roles, parts, notes, the sequence and the project wiring. 45 made the tests fail; two missed ones gave tests for Mother sorting first and for the Mother chat made at create. The other two change nothing: `title_fixed OR ?` when the title isn't fixed, and sorting parts by `seq` when the unique index already returns them in order.
