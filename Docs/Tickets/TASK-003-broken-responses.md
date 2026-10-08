# TASK-003 — Broken responses: the same handling for every model
**Type:** Task
**Status:** Done
**Gate:** 3

## Goal
When a model sends a broken tool call, the model is told what is wrong and can fix it on the next request. Nothing is resent blind, and nothing depends on which model runs (SPEC 1). Decided in SPIKE-030; OpenCode, OpenClaw and Hermes Agent all do this.

## Today
- An unknown tool and arguments that fail the schema already go back as tool errors (`agent/turn.go` `runTool`, `tool.decodeArgs`).
- Tool-call arguments that are not valid JSON end the stream with a `transport` error (SPEC 3.8). The turn then sends the same request again, and the model never learns what was wrong. In SPIKE-030, GLM did this on every extract task.
- The backends check the arguments before the finish reason. So a tool call cut off by `max_tokens` is reported as invalid JSON, not as a max-tokens stop.

## Scope
- **Order of checks in every backend:**
  1. no end marker or `finish_reason`: `transport`, retried as today;
  2. `max_tokens` (`length`): the stop is `max_tokens`. The cut-off tool call is dropped, and the turn shows its cut notice;
  3. a complete stream with arguments that are not valid JSON: a normal tool call, marked as bad.
- **A bad call** is not run. Its result is a tool error with the parser's message, e.g. "the arguments are not valid JSON: unexpected end at offset 412. Send the call again with valid JSON."
- **Stored and sent back:** decide how a bad call is kept in `chats.db` and replayed. Each API needs valid arguments in the assistant message: Anthropic an object, OpenAI a string. One option is to keep the raw text in a new field and send `{}`.
- **A limit:** after 3 requests in a row in one turn whose tool calls all failed to run (bad JSON, unknown tool, or wrong arguments), the turn stops with `turn_failed`. The count resets on a call that runs.
- **SPEC 3.8 and 8.3** are updated: invalid arguments in a complete stream are no longer `transport`.

## Not in scope
- Repairing JSON, or guessing a tool from a similar name.
- Turning tool calls written as text into real calls. This could come later, and only for an exact tool name, as in OpenClaw.

## Done when
- Fake-provider tests cover each case for each backend: no end marker, `max_tokens` in a tool call, bad JSON, an unknown tool, 3 bad calls in a row.
- A scenario with a fake provider shows the model getting the error and the next call running.
- `go test ./...` passes.

## Result
- `provider.ToolCall` builds every backend's calls. Arguments that are not valid JSON go in `ToolCall.Invalid`, and `Args` is `{}`, so the call is stored and sent back like any other.
- Every backend checks the finish reason first: `length` or `max_tokens` is a `max_tokens` stop, also with tool calls. The turn drops the cut-off calls; without other calls it shows the cut notice.
- A call with `Invalid` set is not run. Its result is, for example, "the arguments are not valid JSON: unexpected end of JSON input, at byte 8 of 8. Send the call again with valid JSON."
- `tool.ErrArgs` marks argument errors. 3 requests in a row with only such calls, or unknown tools, stop the turn with `turn_failed`.
- Tests: each backend (no end marker, bad JSON, a call cut off at the limit), `provider.ToolCall`, and the agent (`TestBadToolJSON`, `TestCutToolCall`, `TestBadCallsStop`). The agent tests run on the fake provider: the model gets the error and its next call runs.
