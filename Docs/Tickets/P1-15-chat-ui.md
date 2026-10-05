# P1-15 — Chat UI
**Type:** Feature
**Status:** Done
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

## Result
- **Packages,** pinned: `@shadcn/react` 0.3.1 (MessageScroller, Questionnaire), react-markdown, remark-gfm, rehype-highlight. The chat components and `typeset.css` were copied from the mockups.
- **Go additions:**
  - `SettingsService.Models`, `Presets` and `Connect` for the model picker and the *Connect* form. `Connect` turns on the provider's catalog models and points `default` and `fast` at it when they point at no connected provider.
  - `provider.Registry.Connect` needs a key for every kind but Ollama.
  - `SystemService.Notify` sends a desktop notification through the Wails notifications service. A click shows the window and sends `app:open`.
- **Thread:** the snapshot, then `chat:part` and `chat:status`, with seq checks. A gap, or the end of a turn, reads the snapshot again. *Earlier messages* loads older turns as it scrolls into view.
- **Streaming:** the streaming part lives outside React and is drawn once per frame. Rows are memoised. Markdown is parsed block by block, and the renderer loads lazily (100 KB gzip).
- **Parts:** Markdown with code highlighting and tables. Thinking, folded. Tool chips with a preview, unfolding to the input and output. Notices, skill chips, pictures from the bucket. Links open in the browser; pictures load only from the bucket.
- **Composer:** Enter sends. A message written during a turn shows "Joins at the next step", below the streaming answer. *Stop*, the model picker (aliases marked, *Connect a provider…*), the approval-level chip and drafts kept per chat. Mother's empty chat shows the starters.
- **Waiting:** the approval card (Deny takes a note) and the question form (number keys, *Other*). Both turn into a record once answered. The waiting bar shows while the card is out of view; *Show* scrolls to it. Also the retry bar and the failed-turn card. The chat list and the rail show *Waiting for you*. A desktop notification is sent when a chat starts waiting off screen.
- **No provider:** a turn that fails without a provider shows the *Connect* card. After *Connect*, the kept message is retried.
- **Right-to-left text:** `dir="auto"` on messages, inputs and Markdown blocks. Lists, items and tables take the direction of their first strong letter. Each question choice takes its own direction.
- **Done-when checks:**
  - Measured in the built app (WebView2, Windows 11) with SPIKE-022's method, over the DevTools protocol, against a local fake model at 100 tokens/s:
    - **Streaming (N-01):** 59.7–59.9 fps, frame p99 17 ms, no long tasks. Key → paint p95 27–32 ms, max 49 ms.
    - **Opening a 200-message chat (N-02),** with a key every 30 ms: the longest task is 59–71 ms and key → paint at most 82 ms. The snapshot holds the last 30 turns, and the first rows paint in a frame of about 85 ms.
  - Persian and mixed text, as in mockup screen 12: messages, lists, tables, a Persian question and its choices.
  - A message sent with no provider ran after *Connect* (server mode, scratch data).
  - One desktop notification was sent on this machine and is in Windows' notification history.
- **Tests:** `bun test` for the thread and stream rules, the rows, the Markdown blocks, the first-rows budget and text direction. Go tests for `Connect`, `Presets`, `Models`, the aliases and the key rule.
- **Not done:**
  - Pictures in the composer.
  - *Inspect* in the turn footer (P1-17).
  - Clicking a notification to open its chat was not tried by hand.
  - Loading older turns was not measured.
