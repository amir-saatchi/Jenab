# SPIKE-030 — Structured output: JSON that matches a schema, from any model
**Type:** Spike
**Status:** Done
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

## Result
Runs of 2026-10-06; tables in `spikes/030-structured-output/results.md`.
- **Complete:** gemini-3.5-flash-lite, gemma4:31b and nemotron-3-ultra, every method and task, 3 reps, thinking off (468 runs), and `native` with thinking on (117 runs).
- **Partial:** glm-4.5-flash, 101 of 156 runs. Z.ai answered `429` code 1302 to many requests, and to all 4 requests of a later check while nothing else was running. GLM took 30 to 180 s per call, and some calls went past the 5-minute limit.

Thinking off, all models:

| Method | Valid first | Valid after retry | Right | Made-up values |
|---|---|---|---|---|
| tool | 88% | 90% | 65% | 7 |
| tool-forced | 81% | 83% | 56% | 6 |
| text | 94% | 97% | 71% | 0 |
| native | 97% | 97% | 71% | 3 |

Without GLM, every method is 97–100% valid after the retry. `native` was valid on the first try in every run. `text` failed the first try 5 times (a wrong type, a wrong item count), and the retry fixed all 5.

- **Tools are worse:**
  - They have the fewest right answers and the most made-up values.
  - Ollama's `/api/chat` has no `tool_choice`.
  - On the extract, wide and inject tasks, all 12 of GLM's tool calls had arguments that were not valid JSON. Its other calls on those tasks hit rate limits or timeouts. Those are the schemas with nullable fields.
- **Text around the JSON:** Gemma and GLM put text around the JSON in every `text` and `native` answer, even with `format` set. The lenient parser reads them.
- **Wrong answers** come from the content, not the format:
  - one speaker's company: 29 of 36 runs wrote "freiberuflich" instead of `null`;
  - `sd_card_reader` given as `false` where the page says nothing;
  - long-en's date order.
- **Thinking on (`native`):**
  - Gemini: 85% → 92% right, at the same time.
  - Gemma: 74% → 79%, at 3× the time.
  - Nemotron: 67% → 69%.
  - 5 of the Ollama runs went past 5 minutes.
- **Long input:** long-en is about 15,000 tokens, not near `max_input_tokens`.

Two app problems showed up:
- A tool call whose arguments are not valid JSON is a `Transport` error (SPEC 3.8). The request is sent again unchanged, and the model never learns what was wrong.
- The backends check the arguments before the finish reason. A tool call cut off by `max_tokens` is reported as invalid JSON, not as a max-tokens stop.

## Decision
**Decided (2026-10-08):**
- **Method:** `llm.select`, `llm.extract` and `llm.decide` put the schema in the prompt. `provider.Request` gets an output schema, and each backend maps it to the API's JSON mode: `response_format` `json_schema`, or Ollama's `format`. A backend without a JSON mode ignores it. The prompt is the same for every model; only the provider layer differs.
- **Reading the answer:** Go reads the first JSON value even with text around it, and checks it against the schema and the index checks.
- **When the API rejects the JSON mode,** the backend drops it and remembers that for the endpoint and model. The prompt alone carries the schema from then on, as Hermes Agent does.
- **No `tool_choice` field** in `provider.Request`.
- **Retry:** one retry is enough. The retry message lists the schema errors with their paths, at most 8.
- **Broken responses** are handled in the same general way for every model, in TASK-003. Bad tool-call JSON, unknown tools and invalid arguments go back to the model as a tool error. A tool call cut off at `max_tokens` is reported as max tokens.
- **GLM stays partial.** Its failures were rate limits, slow calls and broken JSON, and Jenab doesn't tune for them.
- **Reference models** for spikes and tests are now Gemini `gemini-3.5-flash-lite` and Ollama Cloud `gemma4:31b` (DEVELOPMENT.md). This replaces the SPIKE-018 choice. The research behind it:
  - OpenCode, OpenClaw and Hermes Agent all keep one generic loop, and all three send bad tool calls back to the model as errors. All three also added prompt text per model family; Jenab doesn't.
  - Gemini's free tier allows about 1,500 requests a day; a third-party figure, since Google now shows the limits only in AI Studio. Ollama Cloud now uses monthly credits, and its free plan runs 1 request at a time.
