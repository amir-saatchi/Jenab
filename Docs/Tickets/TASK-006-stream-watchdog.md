# TASK-006 — Long tool calls look like a stalled stream
**Type:** Bug
**Status:** Open
**Gate:** 3

## Goal
A model that streams a long tool call is not cut off as stalled. Found in the harness review of 2026-10-10.

## Today
- The Registry's watchdog (`provider/registry.go` `run`) restarts its timer only when a backend yields an event. After the first event the window is `Between`, 60 s (`registry.go:29`).
- No backend yields while tool-call arguments stream:
  - Anthropic keeps `input_json_delta` to itself (`anthropic/anthropic.go:236`).
  - Chat Completions keeps its tool-call deltas (`openai/chat.go:131-152`).
  - Gemini keeps its argument deltas.
  - Ollama sends a call whole.
  
  Text and thinking deltas do yield.
- So a call whose arguments take more than 60 s is cancelled as `transport` ("the stream stalled"). `ask` then sends the same request again, which stalls the same way, until the 10 minutes of retries run out. Examples:
  - A 6,000-token argument at 80 tokens/s takes 75 s.
  - A local model at 10 tokens/s stalls with a 600-token argument.
- Phase 2's `save_view`, `save_page` and `save_pipeline` send long arguments.
- The agent's `moved` mark (`agent/turn.go:380`), which the runtime panel uses, doesn't move during a long call either.

## Scope
- **Count all provider traffic.** The watchdog counts any data from the provider, not only events the backend passes on. The proposal is a heartbeat event (`EventAlive`):
  - Each backend yields it for stream chunks it keeps to itself, at most about once a second.
  - The Registry resets the timer on it and passes it up.
  - The agent updates `moved` and ignores it otherwise.
- **Ollama.** Check what `/api/chat` sends while a tool call is generated. If it sends nothing until the call is complete, a long call can't be told apart from a stall. Note the limit and the time Ollama allows in `FirstEventOllama`/`Between`.
- **Responses API.** Long reasoning without summaries has the same problem against `FirstEvent`; TASK-008 asks for summaries.

## Done when
- A fake-backend test streams only tool-argument chunks, each less than `Between` apart but longer than `Between` in total. The stream ends with the call, not a stall.
- A stream with no data for `Between` still stalls, as today.
- Each backend has a test that yields the heartbeat for tool-argument chunks.
- `go test ./...` passes.
