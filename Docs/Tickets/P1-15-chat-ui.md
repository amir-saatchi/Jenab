# P1-15 — Chat UI
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-14, P1-11
**Requirements:** R-14, R-18, R-19, R-116, R-118, N-01, N-02

## Goal
Chatting in the app: streaming, parts, the composer and everything that waits for the user (SPEC 5.8, 8.3, 8.8, 3.9).

## Scope
- **Parts:**
  - Markdown with `remark-gfm` and the Typeset styling (5.8)
  - thinking, tool calls with previews, notices, skill chips
- **Streaming:** the SPIKE-022 rules: tokens drawn once per frame, memoised blocks, lazy loading.
- **Composer:**
  - the model picker and approval-level chips
  - *Stop*, and writing during a turn
  - starter prompts
- **Waiting for the user:**
  - the approval card and the question form
  - the waiting bar above the composer
  - the chat-list badge and the desktop notification
- **No provider yet:** the provider card when a message is sent without a provider. The message is kept and sent once a provider is connected (3.9).
- **Text:** right-to-left text in messages and inputs (5.8).
- **Turn footer:** the footer, with *Inspect* added by P1-17.

## Done when
- With SPIKE-022's method, streaming stays at 60 fps (N-01) and a 200-message chat opens within 100 ms (N-02).
- The mockups' Persian and mixed-text cases show correctly.
- A message sent before any provider is set up is sent after *Connect*.
