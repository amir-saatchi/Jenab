# TASK-080 — What the model reads of a failed turn's notice
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-071

## Goal
A note in TASK-071's report ("Seen, not changed"). When a failed turn is retried, the next request holds the `turn_failed` notice as a user message, such as `[the turn stopped: the model "default" is not set up. Connect a provider in Settings, or pick another model.]`. That text is written for the user (8.3), not the model.

## Scope
- `failure` (`internal/agent/turn.go`) writes the notice. Every notice part is then sent as text by each provider (`anthropic`, `gemini`, `ollama`, `openai` chat and responses). Only `skill_loaded` is kept from the model, by sitting in the tool message (8.9).
- So after *Retry*, the model reads advice addressed to the user: connect a provider, add credit, use Retry. It may answer that advice instead of the user's message, or tell the user to do what they just did.
- **Decide what the model reads** and say why in the report. Either:
  - nothing, as for `skill_loaded`;
  - or a short line of fact without the advice, such as `[the last request failed: <what>; the turn was retried]`.
  
  Some failures may be worth knowing, such as tool calls that couldn't run three times in a row; others, such as a model not set up, are not.
- The same for every model and provider (no model-specific setup): one place decides it, not each provider. What the user sees in the chat is unchanged.
- A `turn_failed` notice from an earlier turn, not retried, is history like any other. Treat it the same way, and say what you chose.
- If the SPEC needs a line (8.3, *When a provider fails*), put the wording in the report; don't edit Docs/.

## Done when
- A test: after a turn fails and is retried, the request the provider gets holds what you chose, not the user's advice. It fails on the old code.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done. The model reads a line of fact in place of a `turn_failed` notice: `[the turn stopped here on an error, and the user retried it]` when a message of the same turn follows it or nothing does, and `[the turn stopped here on an error]` when the next message is a later turn's. No cause is named: a tool call's error is already in its result, and the other causes are the user's settings. `shape` (`internal/agent/context.go`) decides it, so no provider changed; the stored notice and what the user sees are unchanged. SPEC 8.3 has the line.

A test, failing on the old code. Report: [reports/TASK-080-report.md](reports/TASK-080-report.md).
