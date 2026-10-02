# P1-19 — Chat types
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-02
**Requirements:** R-10, R-90

## Goal
`internal/chat`: the chat types every layer shares (CODE-OUTLINE `chat`, Q5, Q7). Providers (P1-08) and chat storage (P1-06) both use them, so they come first, in a ticket of their own.

## Scope
- **Types:** `Chat`, `Message`, `Part` and its kinds (`Text`, `Thinking`, `ToolCall`, `ToolResult`, `Image`, `Notice`, `Approval`, `Question`), `Usage`, `SessionNote`, and the `Kind`, `Role`, `PartKind` and `State` values.
- **Events (Q32):** `Delta`, `PartDone` and `Status`, each with its IDs and the chat's sequence number.
- **`Part.Validate`:** exactly one field set, and it matches `Kind`.
- **`Thinking`** keeps its signature and **`ToolCall`** its provider extras (Gemini's signature) as raw bytes, so both are sent back unchanged (SPEC 3.8).
- **JSON** for every type, since parts are stored as JSON in `chats.db` (2.3) and sent to the frontend.
- Types only: no storage and no provider code. `chat` imports only `id`.
- `Approval` and `Question` start with the fields SPEC 8.8 lists; P1-11 may change them until the first release.

## Done when
- `Validate` tests for every part kind, including the wrong ones.
- JSON round-trip tests for every part kind; a thinking signature and Gemini's extras come back byte for byte.
- `go vet` passes, and nothing in `chat` imports `store`, `provider` or `agent`.
