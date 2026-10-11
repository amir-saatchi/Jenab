# TASK-008 — Thinking, refusals and context overflow in the provider layer
**Type:** Bug
**Status:** Open
**Gate:** 3

## Goal
The provider layer asks for what the agent expects and reports what happened, the same way for every API. Found in the harness review of 2026-10-10.

## Today
- **The Responses API never asks for reasoning.** `rRequest.Reasoning` exists, but `body()` never sets it (`openai/responses.go:245-252`). So:
  - `req.Thinking` is ignored, no reasoning summaries stream, and OpenAI models show no thinking in the chat.
  - During long reasoning nothing is sent, so `FirstEvent` can fire (TASK-006).
- **Responses refusals look like answers.** A `refusal` content part is added to the text (`responses.go:186-187`) and the stop is normal. The turn never shows its refused notice (`agent/turn.go` `cut`).
- **Context overflow is `bad_request`.** `provider.Classify` makes only HTTP 413 `too_large` (`provider/error.go:94-109`). These are all 400s:
  - Anthropic's "prompt is too long"
  - OpenAI's `context_length_exceeded`, also inside a Responses stream (`responses.go` `failed`)
  - Gemini's token-limit errors

  So the user sees "refused the request" instead of the too-large guidance, and SPEC 3.8's "the context is shrunk" can never start.
- **The Anthropic thinking budget can be too large.** `max(1024, MaxTokens/2)` (`anthropic/anthropic.go:350`) is at or above `max_tokens` when 0 < MaxTokens ≤ 2048, which Anthropic refuses. No caller does this today: the title request has Thinking off.
- **Chat Completions drops `reasoning_content` in replays.** Some thinking models on compatible APIs return `reasoning_content` and may expect it back on assistant messages with tool calls in the same turn. The code sends no thinking back, by design (`openai/chat.go:240-247`).

## Scope
- **Responses reasoning.** Send `reasoning: {summary: "auto"}` when `req.Thinking` is set and the catalog says the model reasons, the same rule as Anthropic's `Known`.
- **Responses refusals.** A refusal part sets `StopRefused`.
- **Overflow.** `Classify` makes an overflow 400 `too_large`, from each API's own error code or wording: a list per API kind, like `quotaMarks`. This includes the in-stream `context_length_exceeded`.
- **Anthropic budget.** The budget is always below `max_tokens`. When that leaves less than 1,024 tokens, thinking is off.
- **`reasoning_content`.** Check on the reference models only, and record what free models do without working around it.
  - If a reference model fails or answers worse without it, store the field's name with the thinking part and send it back under that name.
  - This is driven by the response, not guessed from the URL.

## Not in scope
- Shrinking the context after `too_large` (SPEC 3.8). That gets its own ticket once the error is classified.

## Done when
- Tests:
  - Responses: the request has `reasoning` with Thinking on; a refusal stream gives `StopRefused`.
  - Each API: its overflow error, in the body and in the stream, is `too_large`.
  - Anthropic: `MaxTokens` 1500 and 2048 give a valid request.
- The `reasoning_content` check is recorded here, with its decision.
- `go test ./...` passes.
