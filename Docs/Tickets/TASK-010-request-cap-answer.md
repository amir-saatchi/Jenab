# TASK-010 — A turn at its request cap still answers
**Type:** Bug
**Status:** Open
**Gate:** 3

## Goal
A turn that reaches `turn_max_requests` always ends with something the user can read, and its last request keeps the prompt cache. Found in the harness review of 2026-10-10.

## Today
- SPEC 8.3 says: at the cap, "one last request goes out without tools, so the user always gets an answer. A tool call in that answer is not run".
- In the code, `last := t.requests+1 >= TurnMaxRequests` (`agent/turn.go:75`). The last request drops the tools and adds the `lastRequest` line (`agent/context.go:99-106`).
- Some models answer it with tool calls anyway, or write calls as text. Then:
  - `runTools` closes the calls as "not run" (`turn.go:431-436`) and the turn ends.
  - The user gets no text and no notice, because `cut()` runs only for an answer without calls (`turn.go:158-166`).
- Dropping the tools changes the start of the request (Anthropic orders tools → system → messages). The largest request of a long turn misses the cache and writes it again.
- `provider.Request` has no `ToolChoice` field (SPIKE-030).
- SPEC 8.3 doesn't say whether the last request is the 25th or the 26th. The code counts it in the 25.

## Scope
- **No text, a notice.** When the last answer has no text, the turn ends with a notice, such as `[the turn reached its limit of 25 requests before the model answered. Send a message to continue.]`. Use `answer_cut` with this text, or a new kind. There is no further request, which could loop.
- **`ToolChoice` in `provider.Request`.** The last request keeps the same tools with "none" where the API has it:
  - Anthropic `tool_choice: {type: "none"}`
  - OpenAI `tool_choice: "none"`
  - Gemini `mode: NONE`

  Ollama has no `tool_choice` (SPIKE-030), so it keeps dropping the tools. This is a protocol detail inside each backend, not a rule for one model (SPEC 1). Check that each API accepts it with tool calls in the history.
- **SPEC 8.3.** The spec says the last request counts in `turn_max_requests`, and gains the notice.

## Done when
- Fake-provider tests:
  - A model that keeps calling tools at the cap ends the turn with the notice.
  - The request at the cap has the same tools as the one before, plus `tool_choice` none, in Anthropic, OpenAI and Gemini.
  - Ollama's request at the cap has no tools.
- `go test ./...` passes.
