# SPIKE-012 — LLM SDKs and the agent loop
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
What should talk to the LLM providers: the official SDKs behind our own `Provider` interface, or a Go agent framework (LangChainGo, Genkit, Eino, any-llm-go, or a Go counterpart of the Vercel AI SDK)?

The orchestrator owns each turn (SPEC 2.3, 3.1). It writes the assistant message and its `tool_call` part to `chats.db` before any tool runs, and records every change for undo. The provider layer must allow that. It must support:
1. Streaming text and tool calls, including parallel tool calls
2. Cancelling a request mid-stream
3. Anthropic cache points at chosen places (SPEC 3.1), plus cache and token counts in the response
4. Images in tool results
5. OpenAI-compatible and local models (Ollama, LM Studio) through a custom base URL
6. Errors, rate limits and retries that the app can show

## Done when
Each option runs a small tool-using loop against mock servers (and a local model if one is installed), and one approach is chosen.

## Result
Run on 2026-09-28. Code and full output are in `spikes/012-llm-sdk-agent-loop/` (`results.md`).

**How it was tested:**
- Mock servers sent real-format Anthropic and OpenAI streams (A and O in the table).
- Every option also ran one real tool-call round trip on the local Ollama model `qwen3.5:4b`.
- LangChainGo was left out: its last release was 11 months ago.

| Test | Official SDKs + our `Provider` | Genkit | Eino | any-llm-go | goai (Vercel-AI-SDK style) |
|---|---|---|---|---|---|
| Version (date) | anthropic v1.75.0, openai/v3 v3.66.0 (2026-09) | v1.13.1 (2026-09-03) | v0.9.21 (2026-09-23) | v0.9.0 (2026-03-09) | v0.10.4 (2026-09-22) |
| Stream + 2 parallel tool calls | pass | A: no argument deltas | pass | A: tool results split | pass |
| Image in tool result | pass | O: image dropped | O: HTTP 400 | A: fail | A: fail |
| Anthropic cache points + usage | pass | no `cache_control` | pass | fail | 5 system blocks merged into 1 |
| Thinking signature sent back | pass | pass | pass (needs private keys) | fail | pass |
| 429 / 529 / 500 retries | pass | pass | retries can't be turned off | retries can't be turned off | pass, slow (529: 7.6 s) |
| Cancel mid-stream | pass | pass | pass | pass | nil error in 4 of 10 runs |
| Truncated or malformed stream | pass* | fail | fail | fail | fail |
| Ollama round trip | pass | pass | pass | pass | pass |
| Modules linked / binary size | 14 / 26.3 MB | 30 / 33.8 MB | 82 / 46.8 MB | 8 / 14.6 MB | 1 / 9.4 MB |

*Only with our own end-of-stream check. Without it, both SDKs return no error for a stream cut off early.

**Findings**
- **Tools:** no library ran a tool on its own, except Genkit by default. Without `WithReturnToolRequests(true)` it called our tools twice itself.
- **The frameworks pin old SDKs underneath:**
  - Genkit: anthropic v1.23.0, openai v1.8.2
  - Eino: anthropic v1.56.0
  - any-llm-go: anthropic v1.26.0, openai v1.12.0
- **SDK retries:**
  - 3 tries by default; 0 retries means 1 try.
  - A 429 with `retry-after: 1s` took 2.0 s.
  - Status, error type and `retry-after` can be read, but the SDK does not cap `retry-after`.
- **Cancel:** every option returned within 1.3 ms, and the server saw the connection close within 0.34 ms.
- **cgo:** every option builds without it.
- **Not tested:**
  - real cloud APIs
  - the OpenAI Responses API
  - `redacted_thinking`
  - LM Studio
  - Bedrock and Vertex
  - HTTP/2 cancel
  - the graph and agent features of Eino and Genkit

## Decision
**Decided (2026-09-28):** the official SDKs (`anthropic-sdk-go`, `openai-go/v3`) behind Jenab's own `Provider` interface. No agent framework: each one failed at least one thing Jenab needs, and they pin old SDK versions. The rules Go adds are in SPEC 3.8:
- a complete stream needs its end event
- `ctx.Err()` is checked after the stream
- tool arguments are validated as JSON when the call ends
- Anthropic tool results go in one user message
- OpenAI tool images go in a user message after the tool messages
- thinking is sent back unchanged
- cache points are placed as in 3.1, and cache tokens are stored
- `include_usage` is set on OpenAI streams
- the retry count is ours, and `retry-after` is capped
- errors map to `ProviderError`
- the message is written before any tool runs
