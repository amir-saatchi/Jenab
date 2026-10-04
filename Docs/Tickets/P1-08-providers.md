# P1-08 — Providers and the model catalog
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-02, P1-19
**Requirements:** R-90, R-119, N-21, N-40, N-43, N-44

## Goal
All five Phase 1 providers behind one `Provider` interface, and the model catalog (SPEC 3.8, 3.9).

## Scope
- **Interface:** streams text, tool calls, thinking, usage and the stop reason (3.8).
- **Packages:** `provider/anthropic`; `provider/openai` for OpenAI, Gemini, OpenAI-compatible and Ollama; `provider/fake` for tests.
- **Every rule in 3.8:** complete streams only, stall timeouts, retries with capped `retry-after`, `ProviderError`, the silent-truncation check, the Ollama and Gemini rules, the header allow-list, `OPENAI_*` ignored.
- **Registry:** builds clients from settings and keys, and passes every call through the global `limit.Gate` and one Gate per provider (`provider_max_parallel_calls`). A background call takes its provider's slot first, so it never holds a global slot while its provider is busy.
- **Errors (3.8):** the error kinds, the waits, and one pause per provider that halves its background limit and raises it again after 20 calls in a row that succeed.
- **Retry after text was shown** (CODE-OUTLINE open point 1): decided in SPEC 8.3. A retry replaces the text of the cut-off try.
- **Catalog:** `models.json` built in; *Connect* stores the key, reads the provider's model list and turns on the catalog models; *Other models*; the context window from Ollama `/api/show` or from the user; the `default` and `fast` aliases (3.9).

## Done when
- Tests per provider on recorded streams: a complete answer, tool calls, a cut-off stream, 413, a quota error, Gemini's `extra_content`.
- Stall timeouts are tested with `synctest`.
- `synctest` tests: a 429 pauses every call to that provider until its wait ends; a quota error is never retried; the limit halves and comes back.
- A test shows no key in logs, errors or requests other than the auth header.
- A manual smoke test with the development keys (synthetic prompts only) passes on Gemini, Groq, Ollama Cloud and Z.ai.

## Result
- **`internal/provider`:** the `Provider` interface, `Registry`, the catalog (`models.json`), `Guard` (host check, header allow-list, error bodies kept redacted) and `Classify`. `backends.All()` maps each kind to its backend.
- **Registry:**
  - Every call passes the global Gate and its provider's Gate; a background call takes its provider's slot first.
  - A rate limit or overload pauses the provider: every call waits it out and first gets an `EventWait`. The background limit halves once per pause and rises by one after 20 calls in a row succeed.
  - Stall timeouts: 2 minutes to the first event (10 for Ollama), then 60 s between events. Time the caller spends on an event doesn't count.
  - The truncation check compares the reported input tokens with an estimate of the request.
  - `Connect` lists the models with the new key and only then stores it as `provider:<name>`. A 404 or 405 model list means the provider has none (`NoModelList`): the key is stored and the user types the models.
  - Base-URL placeholders such as Cloudflare's `{account_id}`: the values are checked (letters, digits, `-`, `_`), stored as `provider:<name>:<field>` and redacted like keys.
  - `Presets` for the *Connect* form give a name, kind and base URL only. Nothing is set up when the app ships, so the `ollama: 1` default became a rule by kind: a local Ollama gets 1.
- **Backends:**
  - `provider/anthropic`: the Messages API, with the SDK sending a body built here. All tool results go in one user message, a tool's image goes inside its result, thinking is sent back unchanged, and other providers' thinking is dropped. Thinking: `adaptive` for catalog models, `enabled` with a budget for models marked `thinking_budget` (Claude Haiku 4.5), otherwise the API's default.
  - `provider/openai`: the **Responses API** for OpenAI (its newest models take tools only there) and Chat Completions for Gemini and OpenAI-compatible APIs. This changes the ticket's plan, which had Chat Completions for all.
  - `provider/ollama`: the **native `/api/chat`** instead of the OpenAI SDK, because `/v1` ignores `num_ctx` (SPIKE-017). It sends `num_ctx`, `keep_alive` 30 minutes, and `think` only to models whose `/api/show` lists thinking.
  - `provider/fake` for tests.
- **Changes on the way:**
  - "billing" is no longer a quota mark: Gemini's per-minute 429s say it too.
  - A stream error event is turned into an error by the Anthropic SDK, so it is read from the SDK's error.
  - Explicit options alone don't stop the Anthropic SDK reading `ANTHROPIC_CUSTOM_HEADERS`, which could add an `Authorization` header that the allow-list lets through. `WithoutEnvironmentDefaults` stops it, and a test checks this.
  - An assistant message with only tool calls is sent with `content: []`: Cloudflare refuses a missing `content`, and Z.ai refuses `""` with a 429 "overloaded".
- **Tests:**
  - Recorded streams for every backend: complete answers, tool calls with their send-back, cut-off streams, bad tool JSON, 413, quota, Gemini's `extra_content` and Anthropic's thinking settings.
  - `synctest` for the pause, quota and the limit halving and coming back. Stall timeouts are tested too.
  - Key-leak tests run with the SDKs' environment variables set.
  - Mutation checks: 5 on the registry, 13 on *Connect*, placeholders and the local-Ollama limit, 2 on the OpenAI backends, 7 on Anthropic and 10 on Ollama. Each made the tests fail.
- **Smoke test** (`go test -tags smoke -run Smoke ./internal/provider/backends/`, synthetic prompts): a tool call and its answer passed on Gemini (`gemini-3.5-flash-lite`, its signature sent back), Groq and Ollama Cloud (`gpt-oss-20b`, over `/v1` and the native API) and Z.ai (`glm-4.7-flash`, which Z.ai's model list leaves out). Cloudflare Workers AI (`llama-3.3-70b-instruct-fp8-fast`, OpenAI-compatible endpoint, the account ID as a placeholder) passed with `content: ""`; it is to be rerun with `content: []`.
- **Not done here:** Anthropic and OpenAI have no development keys, so they were tested on recorded streams only. Choosing a local Ollama model's `num_thread` is left open.
