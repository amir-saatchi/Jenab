# TASK-004 — Gemini through its native API
**Type:** Task
**Status:** Open
**Gate:** 3

## Goal
The `gemini` kind uses Gemini's native API through Google's Go SDK, `google.golang.org/genai`, instead of its OpenAI-compatible endpoint. Gemini is the main reference model (DEVELOPMENT.md), so its backend should be the best one Google offers.

## Why
- Google calls the OpenAI-compatible layer "still in beta" and recommends the native API for anyone not already using the OpenAI libraries.
- On that layer, thinking settings and thought summaries are only partly available through `extra_body`. The native API gives `thinkingConfig` with thought summaries, `responseJsonSchema`, and the thinking and cached token counts.
- OpenClaw and Hermes Agent both use the native Gemini API.
- It fits SPEC 1 and 3.8. `gemini` is already its own kind, so only its backend changes, and the official SDK is used as for Anthropic and OpenAI. Nothing is detected from URLs.

## Scope
- **First, the dependency.** Measure the SDK before using it: added modules and the `cmd/desktop` binary size. It pulls in grpc, protobuf and cloud auth, mainly for Vertex AI. If the cost is too high, write a small client for the REST API instead, as for Ollama. Adding it needs the user's OK.
- **A new backend**, `internal/provider/gemini`. It streams text, thinking, tool calls, usage and the stop reason, and it passes the `provider` contract tests.
- **Thought signatures** are kept on the parts and sent back unchanged (SPEC 2.3, 3.8). The `extra_content` handling in the OpenAI backend is then only for compatible APIs.
- **Thinking:** `Thinking` maps to `thinkingConfig`, with thought summaries on. Some models can't turn thinking off; then the model's lowest level is used.
- **The output schema** from SPIKE-030 maps to `responseJsonSchema`.
- **Errors** map to SPEC 3.8's kinds: `retryDelay`, `PerDay` as `quota`, and 400s on signatures.
- **SPEC 3.8 and 3.9** are updated, and so are the smoke tests and `scenarios/models.example.yaml`.

## Done when
- The smoke and chain tests pass on `gemini-3.5-flash-lite`, including thinking and a 3-step tool chain.
- The scenario runner passes on Gemini as before.
- The dependency cost is written here.
