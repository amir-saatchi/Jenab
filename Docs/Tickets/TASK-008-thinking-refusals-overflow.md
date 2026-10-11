# TASK-008 — Thinking, refusals and context overflow in the provider layer
**Type:** Bug
**Status:** Done
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

## Result
- **Responses reasoning:** with thinking on, a catalog model that reasons gets `reasoning: {summary: "auto"}`. Unknown models get the API's default.
- **Responses refusals:** a message with a `refusal` part keeps its text as the answer and ends with `StopRefused`, so the turn shows its notice.
- **Overflow:** `Classify` makes a 4xx `too_large` when the body has one of `overflowMarks`: Anthropic's "prompt is too long" and "exceed context limit", OpenAI's `context_length_exceeded` and "maximum context length" (also used by compatible APIs), and Gemini's "exceeds the maximum number of tokens". The same rule covers errors inside a stream, since every backend's stream errors go through `Classify`; the Responses stream's `context_length_exceeded` code maps to 413 directly.
  - Ollama doesn't refuse an overflow: it cuts the prompt to `num_ctx`, which the truncation check (SPEC 3.8) catches.
- **Anthropic budget:** the budget is half of `max_tokens`, at least 1,024, and always below `max_tokens`; if it can't be, thinking is off. The range that failed was `max_tokens` ≤ 1,024, not ≤ 2,048: at 1,500 and 2,048 the budget of 1,024 was already below `max_tokens`. Both now have tests, as does 1,024, which turns thinking off.
- **`reasoning_content`, decision: no change.** Both reference models use their native APIs, not Chat Completions: Gemini through the Interactions API (TASK-004) and gemma4:31b through Ollama's `/api/chat`, which sends `thinking` back already. So no reference model can show whether the field is needed. If a reference model moves to a Chat Completions API, check it then.
- **Tests:** overflow bodies in `TestClassify`, and per backend in `TestHTTPErrors` (Anthropic), `TestChatErrors` (Chat Completions) and `TestEnds` (Gemini); overflow inside the stream in `TestEnds` (Anthropic, Gemini) and `TestResponsesEnds`; `TestResponsesReasoning`; the refusal in `TestResponsesEnds`; budget rows in `TestThinkingSettings`.
- SPEC 3.8 is updated.
