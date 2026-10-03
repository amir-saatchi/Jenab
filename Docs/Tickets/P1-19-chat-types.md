# P1-19 — Chat types
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-02
**Requirements:** R-10, R-90

## Goal
`internal/chat`: the chat types every layer shares (CODE-OUTLINE `chat`, Q5, Q7). Providers (P1-08) and chat storage (P1-06) both use them, so they come first, in a ticket of their own.

## Scope
- **Types:** `Chat`, `Message`, `Part` and its kinds (`Text`, `Thinking`, `ToolCall`, `ToolResult`, `Image`, `Notice`, `Approval`, `Question`), `Usage`, `SessionNote`, and the `Kind`, `Role`, `PartKind` and `State` values.
- **Events (Q32):** `Delta`, `PartDone` and `Status`, each with its IDs and the chat's sequence number.
- **`Part.Validate`:** exactly one field set, and it matches `Kind`.
- **`Thinking`** keeps its signature and **`ToolCall`** its provider extras (Gemini's signature) exactly, so both are sent back unchanged (SPEC 3.8).
- **JSON** for every type, since parts are stored as JSON in `chats.db` (2.3) and sent to the frontend.
- Types only: no storage and no provider code. `chat` imports only `id`.
- `Approval` and `Question` start with the fields SPEC 8.8 lists; P1-11 may change them until the first release.

## Done when
- `Validate` tests for every part kind, including the wrong ones.
- JSON round-trip tests for every part kind; a thinking signature and Gemini's extras come back byte for byte.
- `go vet` passes, and nothing in `chat` imports `store`, `provider` or `agent`.

## Result
- `internal/chat`: `chat.go` (chats, messages, usage, session notes), `part.go` (parts and their checks), `event.go` (`Delta`, `PartDone`, `Status`, `Waiting`). 6 tests; `go vet` and `go test ./...` pass on Windows.
- **JSON names are snake_case**, as in SPEC 2.3 (`tool_call`, `created_by`, `title_fixed`). Empty part fields are left out.
- **Provider extras are a string, not `json.RawMessage`.** `encoding/json` rewrites a `RawMessage` when it writes it: it removes spaces and escapes `<`, `>` and `&`. A string comes back byte for byte. Thinking signatures are strings too. Tool-call arguments stay `json.RawMessage`, so the frontend gets an object; providers accept the compacted form.
- **`Validate`** checks exactly one field and that it matches `Kind`, then each kind's own rules: a tool call has an ID, a name and valid JSON arguments (a cut-off stream fails here), an approval has at least 2 options, and a question form has 1–4 questions with distinct headers and 2–4 options each. `Message.Validate` names the bad part. A mutation check (allowing two fields) made the tests fail, as it should.
- `Text.Stopped` marks text kept after *Stop*, and `ToolResult.IsError` also covers "cancelled by user" (8.3). `Thinking.Redacted` holds a redacted block's data.
- `NoticeKind` lists the notices SPEC names (finish, scheduled run, page opened, memory changed, approval level, early stop); readers accept unknown kinds.
- A `Plan` part waits for SPIKE-028.
