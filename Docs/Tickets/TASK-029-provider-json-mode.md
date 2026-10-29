# TASK-029 — Each provider's JSON mode for schema answers
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** SPIKE-030, P2-11

## Goal
SPIKE-030 decided that schema answers put the schema in the prompt and also pass it to each API's JSON mode. Refresh memory (P2-11) built only the prompt side: a lenient reader, a schema check and one retry. SPIKE-030 measured that at 94% valid on the first try and 97% after the retry. Add the provider side, so fewer answers need the retry.

## Scope
1. `provider.Request` gets an output schema. Each backend maps it: `response_format` `json_schema` (OpenAI and OpenAI-compatible), Gemini's response schema, Ollama's `format`. A backend without a JSON mode ignores it.
2. When an API rejects it, the backend drops it and remembers that for the endpoint and model, so the prompt alone carries the schema from then on.
3. The prompt stays the same for every model. No guessing from the base URL: the provider kind decides.
4. Refresh memory's map and merge calls pass their schemas.

## Done when
- Tests per backend with recorded or fake responses: the schema is sent, a rejection is dropped and remembered, and a backend without a JSON mode works as today.
- The `refresh-memory` scenario on the reference models gives the same results or better, with fewer retries.

## Result
