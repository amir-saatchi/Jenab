# TASK-004 — Gemini through its native API
**Type:** Task
**Status:** Done
**Gate:** 3

## Goal
The `gemini` kind uses Gemini's native API instead of its OpenAI-compatible endpoint. Gemini is the main reference model (DEVELOPMENT.md), so its backend should be the best one Google offers.

## Why
- Google calls the OpenAI-compatible layer "still in beta" and recommends the native API for anyone not already using the OpenAI libraries.
- On that layer, thinking settings and thought summaries are only partly available through `extra_body`. The native API gives thinking levels with thought summaries, a JSON output schema, and the thinking and cached token counts.
- OpenClaw and Hermes Agent both use the native Gemini API.
- It fits SPEC 1 and 3.8. `gemini` is already its own kind, so only its backend changes, Nothing is detected from URLs.

## Scope
- **First, the dependency.** Measure the SDK before using it: added modules and the `cmd/desktop` binary size. It pulls in grpc, protobuf and cloud auth, mainly for Vertex AI. If the cost is too high, write a small client for the REST API instead, as for Ollama. Adding it needs the user's OK.
- **A new backend**, `internal/provider/gemini`. It streams text, thinking, tool calls, usage and the stop reason, and it passes the `provider` contract tests.
- **Thought signatures** are kept on the parts and sent back unchanged (SPEC 2.3, 3.8). The `extra_content` handling in the OpenAI backend is then only for compatible APIs.
- **Thinking:** `Thinking` maps to the thinking level, with thought summaries on. Some models can't turn thinking off; then the model's lowest level is used.
- **The output schema** from SPIKE-030 maps to `response_format`.
- **Errors** map to SPEC 3.8's kinds: `retryDelay`, `PerDay` as `quota`, and 400s on signatures.
- **SPEC 3.8 and 3.9** are updated, and so are the smoke tests and `scenarios/models.example.yaml`.

## Dependency cost
Measured on 2026-10-08 with `google.golang.org/genai` v1.73.0. Two test programs were built with the release flags (`-trimpath -ldflags="-w -s"`). Both use the OpenAI and Anthropic SDKs and the `golang.org/x` packages Jenab already links; the second also streams one Gemini request.

| | Size | Go packages |
|---|---|---|
| without genai | 19.3 MB | 302 |
| with genai | 32.7 MB | 506 |
| added | **+13.4 MB** | **+204** |

- `bin/jenab.exe` is 58 MB, so genai would add about 23 %.
- **New modules:** genai, grpc, protobuf, `genproto/googleapis/rpc`, `cloud.google.com/go`, `cloud.google.com/go/auth`, `compute/metadata`, s2a-go, enterprise-certificate-proxy, opencensus, groupcache, gorilla/websocket, `golang.org/x/crypto` and go-cmp.
- **Why grpc is linked even with only an API key:** genai always imports `cloud.google.com/go/auth/httptransport`, which imports s2a-go, which imports grpc and protobuf. This is for Vertex AI and Google Cloud credentials, which Jenab doesn't use.

## goai
[goai](https://github.com/zendev-sh/goai) (MIT, from 2026-03, mostly one maintainer) is a Go SDK for 25+ providers on plain `net/http`, so it is light. Read on 2026-10-08; it doesn't fit as Jenab's provider layer:
- It decides behaviour from model-name prefixes: the Gemini 3 thinking level, Claude version checks, OpenAI reasoning-model renames (SPEC 1).
- It doesn't flag a stream that ends without a finish reason, and doesn't parse Gemini's `retryDelay` or quota errors. Its errors have no kinds.
- It ignores a blocked prompt (`promptFeedback`) and uses `responseSchema`, not `responseJsonSchema`.
- It has its own retries and tool loop.

Its Gemini code (`provider/google/google.go`) is a useful reference: the URL and SSE stream, the thought signature on the part that holds the `functionCall`, the `functionResponse` shape, and the finish-reason table.

## Done when
- The smoke and chain tests pass on `gemini-3.5-flash-lite`, including thinking and a 3-step tool chain.
- The scenario runner passes on Gemini as before.
- The dependency cost is written here.

## Decision
- **No SDK.** genai adds 13.4 MB and grpc for features Jenab doesn't use. `internal/provider/gemini` is a small client on `net/http`, like the Ollama one.
- **The Interactions API**, not `generateContent` (the user's choice, 2026-10-08). It is Google's newer API; the docs now lead with it. Its stream has typed steps (`thought`, `model_output`, `function_call`) and a final status.

## Result
Done on 2026-10-08. SPEC 3.8 has the details.
- **Stream:** text, thought summaries, signatures, tool calls (arguments arrive as deltas), usage and the end status. A stream without `interaction.completed` is a cut-off.
- **Signatures** are stored as `{"google":{"thought_signature":…}}`, the shape the compatible API already uses. Other backends skip them.
- **Thinking off** without model names: `minimal`, then `low`, then no level. `gemini-3.8-flash` refuses `minimal` with a 400; the backend then uses `low` for that model.
- **Thinking on** first sent no level, so the model's default applied. A SPIKE-031 run found that this is `minimal` on `gemini-3.5-flash-lite`, which doesn't think (fixed 2026-10-09). On a logic puzzle: no level and `minimal` gave 0 thought tokens, `medium` 2,738 and `high` 5,756; no level got it wrong. Thinking on now sends `medium`, then `high`, then `low`. Flash-lite sent no thought summaries even at `high`, so the usage now reports thought tokens on their own.
- **Errors:** HTTP errors go through the shared classifier (`retryDelay`, `PerDay`). Error events and a `failed` status map their code: `resource_exhausted` to `rate_limited`, `unavailable` or `internal` to `overloaded`.
- **Tests:** unit tests on recorded stream shapes. Live on `gemini-3.5-flash-lite`: the smoke test with thinking on and off and the 3-step chain pass, on the native and the compatible API. The `skill-loading` scenarios passed 12/12; one run hit the free tier's 15 requests a minute and the retry recovered.
- **Not done:** the output schema. `provider.Request` has no schema field yet; it maps to `response_format` when one is added.
- The compatible API still works as an `openai_compatible` provider. The smoke tests keep it as `geminicompat`.
