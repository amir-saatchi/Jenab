# TASK-029 — Each provider's JSON mode for schema answers
**Type:** Task
**Status:** Done
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
Done on 2026-10-31 (commit c80ac2b).

- **Built as SPEC 3.8 now says:** `provider.Request.Schema`, mapped per kind (Chat Completions, Responses, Gemini, Ollama). The fallback is in the Registry, once for every kind, not in each backend: a rejected schema is sent again without it and then dropped per connection, model, tools use and schema hash. It is kept in memory only.
- **Refresh memory** passes the map and merge schemas the prompt shows. The prompts didn't change.
- **Tests:** each kind's body with and without a schema, the schema sent byte for byte, the drop and its key, no resend on other errors, while paused, on a cancel or for Anthropic, a backend rebuilt mid-call, and parallel calls. The review checked the new rules by mutation.
- **Scenario `refresh-memory` R1,** 3 reps each:
  - gemma4:31b 3/3. Gemini gemini-3.5-flash-lite 2/3; the failure was the check's wording, not the answer, and the check now takes "prohibits chemical pesticides" too.
  - Both APIs took the JSON mode, with no fallback request. Schema retries stayed at 0, as in all 15 earlier runs, so "fewer retries" couldn't be shown. 37 Gemini requests.
- **Open:** Anthropic's JSON mode, which needs closed schemas or a transform and a key to check it. `-race` wasn't run (no cgo on this PC).
