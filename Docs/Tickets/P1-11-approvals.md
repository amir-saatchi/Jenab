# P1-11 — Approvals and questions
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-10
**Requirements:** R-18, R-19, R-117

## Goal
The approval card, the question form and the approval levels, on the Go side (SPEC 8.8). The UI is in P1-15.

## Scope
- **Parts:** `approval` and `question`. The turn waits without holding a transaction.
- **Closing:** if the user writes instead, the card closes as *Deny* with the message as the note; *Stop* also closes it.
- **Records:** in `_jenab_approvals`, with the answer and its source (`user` or `auto`).
- **Levels:** Strict, Standard and Auto per project in `_jenab_meta`; the default comes from `config.yaml`; a change applies from the next tool call and adds a notice.
- **`approve(t, spec, needs)`** in `agent` (CODE-OUTLINE 7).
- **`ask_user`:** 1–4 questions with options and *Other*; the answer returns to the agent.
- **Later:** no Phase 1 tool needs an approval yet (migrations come in Phase 2, hosts in Phase 4), so the tests use a test tool that does.

## Done when
Tests cover:
- wait, approve, and deny with a note
- deny by writing a message, and *Stop* while waiting
- each level on the test tool
- an `ask_user` answer reaching the agent
