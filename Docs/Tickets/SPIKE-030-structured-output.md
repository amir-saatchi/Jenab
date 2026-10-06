# SPIKE-030 — Structured output: JSON that matches a schema, from any model
**Type:** Spike
**Status:** Open (before Phase 2)
**Gate:** 3

## Question
`llm.select` and `llm.extract` (SPEC 6.5), and later `llm.decide`, need an answer that matches a schema. SPEC 6.5 says the output is checked against its schema and retried once, but not how the app asks for it. Jenab has no structured output today.

How should Jenab get schema-valid JSON from any model, with the same prompt for every model (SPEC 1)? Only the provider layer may differ, for protocol reasons.

## Methods
1. **Submit tool:** one tool, `submit`, whose parameters are the output schema. The model must call it. Go checks the arguments against the schema; on an error it sends the error back once and asks again.
2. **JSON in the text:** the schema is in the prompt. Go reads the first JSON value from the answer, also inside a code fence or after a sentence, checks it, and retries once the same way.
3. **The provider's own mode:** `response_format` with `json_schema` where the API has it, Ollama's `format` with the schema. Same check and retry. This shows whether a path per provider is worth it.

For method 1, check whether each provider accepts a required tool call (`tool_choice`). `provider.Request` has no field for it yet, so the harness adds one; without it the prompt asks for the call.

## Tasks
Synthetic data, with the right answers fixed before any run. Inputs in English, German and Persian, as in SPIKE-027.
- **select:** 30 news items; pick the 5 about ETH, ranked, each with a `reason`. Only indexes and added fields come back (SPEC 6.5).
- **extract:** a product or event page to an object with strings, numbers, a date, an enum, a list and optional fields. Some fields are missing in the page and must come back `null`, not made up.
- **decide:** 20 short items, each with a yes/no, a pick-one and a score from 1 to 5, in one call.
- **Hard cases:** a schema with 40 fields; nested arrays of objects; long input near `max_input_tokens`; input text that says to ignore the format and answer in prose.

## Models
- The SPIKE-018 development models: gemma4:31b and nemotron-3-ultra (Ollama Cloud, native API) and glm-4.5-flash (Z.ai).
- gemini-3.5-flash-lite, for a provider with a full `json_schema` mode.
- Thinking off, as pipeline steps send it. For the best method only, also with thinking on.
- At least 3 reps per cell. The models are confirmed with the user before the runs.

## Setup
- **Harness:** `spikes/030-structured-output/`. It calls the app's own provider registry and backends, so it tests our code, and the app's schema checker (`santhosh-tekuri/jsonschema`, already used for tool arguments).
- **Safety:** keys from `.env`, only in auth headers to each provider's own host. Only synthetic text is sent. A quota error stops that provider.

## Measures
- **Valid:** schema-valid on the first try, and after the one retry.
- **Right:** the answer matches the fixed answers: the chosen items and their order, extracted values, the decisions; made-up values for missing fields.
- **Failures, by kind:**
  - no tool call, or text instead of the call
  - JSON that doesn't parse
  - a missing field or a wrong type
  - an index out of range or repeated
  - a value outside the enum
- **Cost:** tokens and time per call, retries included.
- **Provider support:** which APIs accept a required tool call or a `json_schema` mode, and their errors, mapped to SPEC 3.8's kinds.

## Done when
Every method has results on every model for every task, with at least 3 reps. There is a decision:
- the method for `llm.select`, `llm.extract` and `llm.decide` (SPEC 6.5)
- whether the provider's own mode is used where it exists, or one method for all
- whether `provider.Request` gets a field to require a tool call
- the retry rule: what the retry message holds, and whether one retry is enough
