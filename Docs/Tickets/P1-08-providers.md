# P1-08 — Providers and the model catalog
**Type:** Feature
**Status:** Open
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
