# SPIKE-018 results

Run 2026-09-28 13:21, go1.26.0 windows/amd64, openai-go/v3 v3.66.0. Flags: provider="" quick=false.

Rules used: SDK retries 0, our loop retries 2 times, retry-after capped at 30s (above the cap: give up). A stream is complete only with a finish_reason. Tool list: 24 tools (SPEC 8.1), 5949 bytes of definitions.

## 0. Providers and models

- **groq** `https://api.groq.com/openai/v1/`: /models lists 11 models: `allam-2-7b`, `canopylabs/orpheus-arabic-saudi`, `canopylabs/orpheus-v1-english`, `meta-llama/llama-prompt-guard-2-22m`, `meta-llama/llama-prompt-guard-2-86m`, `openai/gpt-oss-120b`, `openai/gpt-oss-20b`, `openai/gpt-oss-safeguard-20b`, `qwen/qwen3.8-27b`, `whisper-large-v3`, `whisper-large-v3-turbo`
- **ollama** `https://ollama.com/v1/`: /models lists 17 models: `deepseek-v4-pro:0813`, `deepseek-v4.1-flash`, `gemma4:31b`, `glm-5.2`, `glm-5.3`, `glm-5.3-flash`, `gpt-oss:120b`, `gpt-oss:20b`, `kimi-k2.6`, `kimi-k2.7-code`, `kimi-k3`, `minimax-m2.7`, `minimax-m3`, `mistral-large-3:675b`, `nemotron-3-nano:30b`, `nemotron-3-super`, `nemotron-3-ultra`
- **gemini** `https://generativelanguage.googleapis.com/v1beta/openai/`: /models failed: Get "https://generativelanguage.googleapis.com/v1beta/openai/models": unexpected EOF
- **zai** `https://api.z.ai/api/paas/v4/`: /models lists 11 models: `glm-4.5`, `glm-4.5-air`, `glm-4.6`, `glm-4.7`, `glm-5`, `glm-5-turbo`, `glm-5.1`, `glm-5.2`, `glm-5.3`, `glm-5.3-flash`, `glm-5.3-flashx`
  - host: api.z.ai: /models answered 200, so open.bigmodel.cn was not tried

| Provider | Wanted | Model ID tested | In /models | Context limit (provider) | reasoning_effort sent |
|---|---|---|---|---|---|
| groq | openai/gpt-oss-120b | `openai/gpt-oss-120b` | yes | 131072 (max output 65536) (/models context_window) | low |
| groq | openai/gpt-oss-20b | `openai/gpt-oss-20b` | yes | 131072 (max output 65536) (/models context_window) | low |
| groq | qwen*27b | `qwen/qwen3.8-27b` | yes | 131072 (max output 16384) (/models context_window) | – |
| ollama | gemma4:31b | `gemma4:31b` | yes | 262144 (/api/show gemma4.context_length) | – |
| ollama | gpt-oss:120b | `gpt-oss:120b` | yes | 131072 (/api/show gptoss.context_length) | low |
| ollama | gpt-oss:20b | `gpt-oss:20b` | yes | 131072 (/api/show gptoss.context_length) | low |
| ollama | nemotron-3-nano:30b | `nemotron-3-nano:30b` | yes | 262144 (/api/show nemotron-3-nano.context_length) | – |
| ollama | nemotron-3-super | `nemotron-3-super` | yes | 262144 (/api/show nemotron_h_moe.context_length) | – |
| ollama | nemotron-3-ultra | `nemotron-3-ultra` | yes | 262144 (/api/show .context_length) | – |
| zai | glm-4.7-flash | `glm-4.7-flash` | no | 200K in / 128K out (docs) | – |
| zai | glm-4.5-flash | `glm-4.5-flash` | no | 128K in / 96K out (docs) | – |

Availability probes (one tiny request each):

| Provider | Model | In /models | Result |
|---|---|---|---|
| gemini | `gemini-2.5-pro` | no | 404 [{ "error": { "code": 404, "message": "This model models/gemini-2.5-pro is no longer available to new users. Please update your code to use models/gemini-3.1-pro-preview for the latest features and improvements. We recommend you to use the Interactions API... |
| gemini | `gemini-3.1-pro-preview` | no | 429 [{ "error": { "code": 429, "message": "You exceeded your current quota, please check your plan and billing details. For more information on this error, head to: https://ai.google.dev/gemini-api/docs/rate-limits. To monitor your current usage, head to: http... |

## 1. Basic streaming

Prompt: reply "pong" and three colors; max tokens 400. TTFT = first streamed content, reasoning or tool delta.

| Model | Result | finish_reason | Text | Usage in/out | Usage chunk | cached_tokens field | TTFT | Total | Reasoning chars | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| groq openai/gpt-oss-120b | pass | stop | pong red, blue, green | 97/35 | own chunk after finish | no | 1.75 s | 1.86 s | 79 |  |
| groq openai/gpt-oss-20b | pass | stop | pong, red, blue, green | 97/43 | own chunk after finish | no | 1.50 s | 1.61 s | 103 |  |
| groq qwen/qwen3.8-27b | pass | stop | pong red, blue, green | 40/8 | own chunk after finish | no | 1.50 s | 1.61 s | 0 |  |
| ollama gemma4:31b | pass | stop | pong, red, blue, green | 41/8 | own chunk after finish | yes | 1.76 s | 2.68 s | 0 |  |
| ollama gpt-oss:120b | pass | stop | pong red, green, blue | 98/21 | own chunk after finish | yes | 0.48 s | 0.98 s | 15 |  |
| ollama gpt-oss:20b | pass | stop | pong, red, blue, green | 98/20 | own chunk after finish | yes | 15.35 s | 15.35 s | 12 |  |
| ollama nemotron-3-nano:30b | pass | stop | pong red, blue, green | 41/288 | own chunk after finish | no | 0.39 s | 2.41 s | 1129 |  |
| ollama nemotron-3-super | pass | stop | pong red, green, blue | 41/121 | own chunk after finish | no | 1.92 s | 3.00 s | 438 |  |
| ollama nemotron-3-ultra | pass | stop | pong, red, green, blue | 41/152 | own chunk after finish | no | 0.82 s | 7.22 s | 536 |  |
| zai glm-4.7-flash | FAIL: 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} | – |  | 0/0 | – | no | – | 2.35 s | 0 |  |
| zai glm-4.5-flash | pass | stop | pong, red, blue, green | 30/80 | same chunk as finish | yes | 2.59 s | 3.08 s | 301 |  |

## 2. Tool calling (24 tools)

- t1 (right tool): What columns does the prices table have?
- t2 (right tool): How many rows in prices are for the coin BTC? Count them with SQL.
- t3 (right tool): Search the web for news about the Ethereum Fusaka upgrade from the past week. I want 5 results.
- t4 (right tool): Remember for this project, in the preferences section, that prices are shown in EUR. The section's current revision is 7.
- t5 (no tool): What is 17 times 3? Just tell me.
- t6 (no tool): How do you say 'good morning' in German?
- t7 (parallel): Describe both tables, prices and coins. Call describe_table for both at once, in parallel.
- t8 (parallel): Two independent things, do both at once in parallel: list the bucket objects under reports/ and get the status of pipeline run run_81.
- t9 (2-step): Start the daily_btc pipeline now, then check the run's status and tell me how it went.
- t10 (3-step): First check the columns of the prices table, then add today's BTC price: 58,000 EUR on 2026-09-28.
- t11 (3-step): Fetch https://example.com/btc-weekly and save it to my resources as a link tagged btc.

A task passes when every step calls exactly the expected tools with schema-valid and correct arguments, and the final answer (if any) has no tool call and contains the expected fact. Fake tool results are fed back between steps.

| Model | Tasks passed | Calls schema-valid | t1 | t2 | t3 | t4 | t5 | t6 | t7 | t8 | t9 | t10 | t11 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| groq openai/gpt-oss-120b | 9/11 | 12/12 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | x | x | ok 0s | ok 0s | ok 0s |
| groq openai/gpt-oss-20b | 9/11 | 12/12 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | x | x | ok 0s | ok 0s | ok 0s |
| groq qwen/qwen3.8-27b | 10/11 | 14/14 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | x |
| ollama gemma4:31b | 10/11 | 14/14 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | x |
| ollama gpt-oss:120b | 8/11 | 11/11 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | x | x | ok 0s | ok 0s | x |
| ollama gpt-oss:20b | 7/11 | 11/12 | ok 0s | ok 0s | ok 0s | x | ok 0s | ok 0s | x | x | ok 0s | ok 0s | x |
| ollama nemotron-3-nano:30b | 8/11 | 11/11 | ok 0s | x | ok 0s | ok 0s | ok 0s | ok 0s | x | ok 0s | ok 0s | ok 0s | x |
| ollama nemotron-3-super | 10/11 | 14/14 | ok 0s | x | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s |
| ollama nemotron-3-ultra | 11/11 | 14/14 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s |
| zai glm-4.7-flash | 5/11 | 13/13 | ok 0s | x | x | ok 0s | ok 0s | x | ok 0s | ok 0s | x | x | x |
| zai glm-4.5-flash | 10/11 | 14/14 | ok 0s | x | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s |

Failures:

- groq openai/gpt-oss-120b t7: s1: called [describe_table], expected [describe_table describe_table] (finish tool_calls)
- groq openai/gpt-oss-120b t8: s1: called [bucket_list], expected [bucket_list run_status] (finish tool_calls)
- groq openai/gpt-oss-20b t7: s1: called [describe_table], expected [describe_table describe_table] (finish tool_calls)
- groq openai/gpt-oss-20b t8: s1: called [bucket_list], expected [bucket_list run_status] (finish tool_calls)
- groq qwen/qwen3.8-27b t11: s1: called [fetch_page save_link], expected [fetch_page] (finish tool_calls)
- ollama gemma4:31b t11: s1: called [fetch_page save_link], expected [fetch_page] (finish tool_calls)
- ollama gpt-oss:120b t7: s1: called [describe_table], expected [describe_table describe_table] (finish tool_calls)
- ollama gpt-oss:120b t8: s1: called [bucket_list], expected [bucket_list run_status] (finish tool_calls)
- ollama gpt-oss:120b t11: s1: called [save_link], expected [fetch_page] (finish tool_calls)
- ollama gpt-oss:20b t4: s1: update_memory args invalid: schema: jsonschema validation failed with 'mem://update_memory.json#' - at '/scope': value must be one of 'user', 'project'
- ollama gpt-oss:20b t7: s1: called [describe_table], expected [describe_table describe_table] (finish tool_calls)
- ollama gpt-oss:20b t8: s1: called [bucket_list], expected [bucket_list run_status] (finish tool_calls)
- ollama gpt-oss:20b t11: s3: answer lacks ["saved" "added" "link"] (finish stop: "Got it! The page is now stored in Resources as “BTC weekly...")
- ollama gpt-oss:20b invalid arguments: t4 update_memory: schema: jsonschema validation failed with 'mem://update_memory.json#' - at '/scope': value must be one of 'user', 'project'
- ollama nemotron-3-nano:30b t2: s1: called [describe_table], expected [query] (finish tool_calls)
- ollama nemotron-3-nano:30b t7: s1: called [], expected [describe_table describe_table] (finish length) text ""
- ollama nemotron-3-nano:30b t11: s1: called [save_link], expected [fetch_page] (finish tool_calls)
- ollama nemotron-3-super t2: s1: called [describe_table], expected [query] (finish tool_calls)
- zai glm-4.7-flash t2: s1: called [describe_table], expected [query] (finish tool_calls)
- zai glm-4.7-flash t3: s1 error: 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"}
- zai glm-4.7-flash t6: s1 error: 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"}
- zai glm-4.7-flash t9: s1: called [run_pipeline run_status], expected [run_pipeline] (finish tool_calls)
- zai glm-4.7-flash t10: s1: called [describe_table insert], expected [describe_table] (finish tool_calls)
- zai glm-4.7-flash t11: s1: called [fetch_page save_link], expected [fetch_page] (finish tool_calls)
- zai glm-4.5-flash t2: s1: called [describe_table], expected [query] (finish tool_calls)

## 3. Context size

One Burrow-shaped request per size (SPEC 3.1 blocks in the system message, a history window of query turns with tool result previews, the current turn, all 24 tools). Sizes are estimated from the chars per token measured in section 4.

| Model | Context limit | Target | Prompt tokens | TTFT | Total | Result |
|---|---|---|---|---|---|---|
| groq openai/gpt-oss-120b | 131072 (max output 65536) (/models context_window) | 8k | 0 | – | 0.34 s | ERROR 413 {"message":"Request too large for model `openai/gpt-oss-120b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 8406, please reduce your message size and try ag... |
| groq openai/gpt-oss-120b | 131072 (max output 65536) (/models context_window) | 5.5k (fallback) | 5821 | 0.57 s | 0.67 s | ok: The most recent date was 2026‑08‑16. |
| groq openai/gpt-oss-120b | 131072 (max output 65536) (/models context_window) | 32k | 0 | – | 1.53 s | ERROR 413 {"message":"Request too large for model `openai/gpt-oss-120b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 32217, please reduce your message size and try a... |
| groq openai/gpt-oss-20b | 131072 (max output 65536) (/models context_window) | 8k | 0 | – | 0.91 s | ERROR 413 {"message":"Request too large for model `openai/gpt-oss-20b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 8454, please reduce your message size and try aga... |
| groq openai/gpt-oss-20b | 131072 (max output 65536) (/models context_window) | 5.5k (fallback) | 5820 | 1.04 s | 1.14 s | ok: 2026-08-16. |
| groq openai/gpt-oss-20b | 131072 (max output 65536) (/models context_window) | 32k | 0 | – | 1.62 s | ERROR 413 {"message":"Request too large for model `openai/gpt-oss-20b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 32240, please reduce your message size and try ag... |
| groq qwen/qwen3.8-27b | 131072 (max output 16384) (/models context_window) | 8k | 0 | – | 0.74 s | ERROR 413 {"message":"Request too large for model `qwen/qwen3.8-27b` in organization `org_[masked]` service tier `on_demand` on input tokens per minute (ITPM): Limit 7000, Requested 8657, please reduce your message size and tr... |
| groq qwen/qwen3.8-27b | 131072 (max output 16384) (/models context_window) | 5.5k (fallback) | 6438 | 1.12 s | 1.48 s | ok: The most recent date in that result was 2026-08-10. |
| groq qwen/qwen3.8-27b | 131072 (max output 16384) (/models context_window) | 32k | 0 | – | 1.38 s | ERROR 413 {"message":"Request too large for model `qwen/qwen3.8-27b` in organization `org_[masked]` service tier `on_demand` on input tokens per minute (ITPM): Limit 7000, Requested 30495, please reduce your message size and t... |
| ollama gemma4:31b | 262144 (/api/show gemma4.context_length) | 8k | 7825 | 0.73 s | 0.86 s | ok: The most recent date in the last query result was 2026-08-16... |
| ollama gemma4:31b | 262144 (/api/show gemma4.context_length) | 32k | 32549 | 1.90 s | 2.07 s | ok: The most recent date in the last query result was 2026-10-26... |
| ollama gpt-oss:120b | 131072 (/api/show gptoss.context_length) | 8k | 8184 | 1.18 s | 1.18 s | ok: 2026-08-25. |
| ollama gpt-oss:120b | 131072 (/api/show gptoss.context_length) | 32k | 32161 | 1.71 s | 2.14 s | ok: 2026‑11‑28. |
| ollama gpt-oss:20b | 131072 (/api/show gptoss.context_length) | 8k | 8185 | 0.89 s | 2.13 s | ok: 2026‑08‑25. |
| ollama gpt-oss:20b | 131072 (/api/show gptoss.context_length) | 32k | 32162 | 1.48 s | 2.21 s | ok: 2026‑11‑28 |
| ollama nemotron-3-nano:30b | 262144 (/api/show nemotron-3-nano.context_length) | 8k | 7960 | 0.77 s | 2.56 s | ok: The most recent date in that result was 2026‑08‑13. |
| ollama nemotron-3-nano:30b | 262144 (/api/show nemotron-3-nano.context_length) | 32k | 33108 | 1.37 s | 4.74 s | ok, finish length, no text |
| ollama nemotron-3-super | 262144 (/api/show nemotron_h_moe.context_length) | 8k | 7959 | 2.13 s | 2.95 s | ok: The most recent date was 2026‑08‑13. |
| ollama nemotron-3-super | 262144 (/api/show nemotron_h_moe.context_length) | 32k | 33108 | 3.07 s | 3.74 s | ok: 2026-10-20 |
| ollama nemotron-3-ultra | 262144 (/api/show .context_length) | 8k | 7959 | 1.83 s | 3.50 s | ok: The most recent date was 2026-08-13. |
| ollama nemotron-3-ultra | 262144 (/api/show .context_length) | 32k | 33109 | 3.75 s | 5.80 s | ok: The most recent date was 2026-10-20. |
| zai glm-4.7-flash | 200K in / 128K out (docs) | 8k | 8510 | 2.48 s | 2.71 s | ok: The most recent date was 2026-08-22. |
| zai glm-4.7-flash | 200K in / 128K out (docs) | 32k | 32339 | 6.02 s | 6.41 s | ok: The most recent date is 2026-11-22. |
| zai glm-4.5-flash | 128K in / 96K out (docs) | 8k | 8598 | 3.32 s | 3.77 s | ok: 2026-08-22 |
| zai glm-4.5-flash | 128K in / 96K out (docs) | 32k | 32764 | 8.93 s | 10.12 s | ok: 2026-11-22 |

## 4. Prompt caching

Turn 1: a Burrow-shaped request of about 4k tokens with a fresh session nonce at the start (cold). Turn 2 and 3: the same messages plus the assistant answer and one short new user message (the request only grows at the end).

| Model | Turn 1 in / cached / TTFT | Turn 2 in / cached / TTFT | Turn 3 in / cached / TTFT | Whole run: calls with cached_tokens field / with cached > 0 / max cached / successful calls |
|---|---|---|---|---|
| groq openai/gpt-oss-120b | 2589 / 0 / 0.61 s | 2614 / 0 / 2.36 s | 2651 / 0 / 1.03 s | 16 / 16 / 1280 / 22 |
| groq openai/gpt-oss-20b | 2591 / 0 / 0.74 s | 2616 / 0 / 0.98 s | 2653 / 0 / 0.97 s | 16 / 16 / 1280 / 26 |
| groq qwen/qwen3.8-27b | 4133 / 0 / 1.42 s | 4167 / 0 / 1.66 s | 4223 / 0 / 1.24 s | 0 / 0 / 0 / 20 |
| ollama gemma4:31b | 3430 / 0 / 0.94 s | 3460 / 3424 / 0.55 s | 3488 / 3456 / 0.62 s | 42 / 32 / 3456 / 21 |
| ollama gpt-oss:120b | 2408 / 1040 / 0.60 s | 2438 / 2384 / 1.12 s | 2472 / 2416 / 1.35 s | 42 / 42 / 2416 / 21 |
| ollama gpt-oss:20b | 2407 / 1063 / 0.72 s | 2432 / 2408 / 0.87 s | 2477 / 1063 / 0.66 s | 50 / 50 / 2408 / 25 |
| ollama nemotron-3-nano:30b | 4390 / 0 / 0.54 s | 4426 / 0 / 0.57 s | 4454 / 0 / 0.79 s | 0 / 0 / 0 / 21 |
| ollama nemotron-3-super | 4390 / 0 / 5.21 s | 4423 / 0 / 1.10 s | 4482 / 0 / 2.81 s | 0 / 0 / 0 / 23 |
| ollama nemotron-3-ultra | 4389 / 0 / 1.77 s | 4421 / 0 / 2.01 s | 4455 / 0 / 2.26 s | 0 / 0 / 0 / 23 |
| zai glm-4.7-flash | 3477 / 2113 / 15.58 s | 3499 / 3475 / 1.98 s | 3527 / 3497 / 3.25 s | 28 / 28 / 3497 / 14 |
| zai glm-4.5-flash | 3494 / 2129 / 6.17 s | 3520 / 3496 / 3.03 s | 3561 / 3522 / 4.43 s | 46 / 46 / 3522 / 23 |

## 5. Rate limits

Burst tests (after the other tests of the provider): Groq sends 4 requests of about 3.5k tokens back to back (TPM limit test); Gemini sends up to 16 tiny requests back to back (RPM test); Ollama and Z.ai send 2 requests at the same time (Ollama free plan: 1 concurrent request).

| Provider | Model | Result |
|---|---|---|
| gemini | – | skipped (model stopped or missing) |
| zai | `glm-4.7-flash` | #1 failed after 3 attempt(s): 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again ... total 0.6s; #2 failed after 3 attempt(s): 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again ... total 0.6s |
| groq | `openai/gpt-oss-20b` | #1 ok after 1 attempt(s); #2 ok after 1 attempt(s); #3 ok after 1 attempt(s); #4 ok after 2 attempt(s) |
| ollama | `gpt-oss:20b` | #1 ok after 1 attempt(s) total 4.1s; #2 ok after 1 attempt(s) total 6.2s |

Every 429 and every other error status seen in the whole run (rate-limit header values only; no auth headers are read):

| Model | Request | Attempt | Status | Rate-limit headers | Wait hint in body | Action | Body (short) |
|---|---|---|---|---|---|---|---|
| gemini gemini-2.5-pro | probe | 1 | 404 | – | – | not retried | 404 [{ "error": { "code": 404, "message": "This model models/gemini-2.5-pro is no longer available to new users. Please update your code to use models/gemini-3.1-pro-preview for the latest features and improvements. We recommend you to use the Interactions API (https://ai.google.dev/gemini-api/docs/get-started).", "status": "NOT_FOUND" } } ] |
| gemini gemini-3.1-pro-preview | probe | 1 | 429 | – | GenerateContentInputTokensPerModelPerMinute-FreeTier retryDelay 52s | daily/quota limit: stop this model | 429 [{ "error": { "code": 429, "message": "You exceeded your current quota, please check your plan and billing details. For more information on this error, head to: https://ai.google.dev/gemini-api/docs/rate-limits. To monitor your current usage, head to: https://ai.dev/rate-limit. \n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_input_token_count, limit: 0, model: gemini-3.1-pro\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 0, model: gemini-3.1-pro\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 0, model: gemini-3.1-pro\n* Quota... |
| zai glm-4.7-flash | basic | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | basic | 2 | 429 | – | – | retry after 4s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | basic | 3 | 429 | – | – | retries used up | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | t3/s1 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| ollama gemma4:31b | basic | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | Post "https://ollama.com/v1/chat/completions": unexpected EOF |
| zai glm-4.7-flash | t3/s1 | 2 | 429 | – | – | retry after 4s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | t3/s1 | 3 | 429 | – | – | retries used up | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | t4/s1 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| groq openai/gpt-oss-120b | basic | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | Post "https://api.groq.com/openai/v1/chat/completions": unexpected EOF |
| groq openai/gpt-oss-20b | basic | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | Post "https://api.groq.com/openai/v1/chat/completions": unexpected EOF |
| groq qwen/qwen3.8-27b | basic | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | Post "https://api.groq.com/openai/v1/chat/completions": unexpected EOF |
| zai glm-4.7-flash | t6/s1 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | t6/s1 | 2 | 429 | – | – | retry after 4s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | t6/s1 | 3 | 429 | – | – | retries used up | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | t8/s1 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | cache/t3 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | burst/1 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | burst/2 | 1 | 429 | – | – | retry after 2s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | burst/1 | 2 | 429 | – | – | retry after 4s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | burst/2 | 2 | 429 | – | – | retry after 4s (no retry-after header, backoff) | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | burst/2 | 3 | 429 | – | – | retries used up | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| zai glm-4.7-flash | burst/1 | 3 | 429 | – | – | retries used up | 429 {"code":"1305","message":"The service may be temporarily overloaded, please try again later"} |
| groq openai/gpt-oss-20b | ctx/8k | 1 | 413 | retry-after=4 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=969 x-ratelimit-remaining-tokens=8000 x-ratelimit-reset-requests=44m38.4s x-ratelimit-reset-tokens=1ms | – | not retried | 413 {"message":"Request too large for model `openai/gpt-oss-20b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 8454, please reduce your message size and try again. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| groq openai/gpt-oss-120b | ctx/8k | 1 | 413 | retry-after=4 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=976 x-ratelimit-remaining-tokens=8000 x-ratelimit-reset-requests=34m33.6s x-ratelimit-reset-tokens=1ms | – | not retried | 413 {"message":"Request too large for model `openai/gpt-oss-120b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 8406, please reduce your message size and try again. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| groq openai/gpt-oss-20b | ctx/32k | 1 | 413 | retry-after=182 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=968 x-ratelimit-remaining-tokens=8000 x-ratelimit-reset-requests=46m4.8s x-ratelimit-reset-tokens=1ms | – | not retried | 413 {"message":"Request too large for model `openai/gpt-oss-20b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 32240, please reduce your message size and try again. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| groq openai/gpt-oss-120b | ctx/32k | 1 | 413 | retry-after=182 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=975 x-ratelimit-remaining-tokens=8000 x-ratelimit-reset-requests=36m0s x-ratelimit-reset-tokens=1ms | – | not retried | 413 {"message":"Request too large for model `openai/gpt-oss-120b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Requested 32217, please reduce your message size and try again. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| groq qwen/qwen3.8-27b | cache/t3 | 1 | 429 | retry-after=10 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=979 x-ratelimit-remaining-tokens=4142 x-ratelimit-reset-requests=30m14.4s x-ratelimit-reset-tokens=33.068s | 9.265714285s | retry after 10s | 429 {"message":"Rate limit reached for model `qwen/qwen3.8-27b` in organization `org_[masked]` service tier `on_demand` on input tokens per minute (ITPM): Limit 7000, Used 3858, Requested 4223. Please try again in 9.265714285s. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| groq qwen/qwen3.8-27b | ctx/8k | 1 | 413 | retry-after=15 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=978 x-ratelimit-remaining-tokens=8000 x-ratelimit-reset-requests=31m40.8s x-ratelimit-reset-tokens=1ms | – | not retried | 413 {"message":"Request too large for model `qwen/qwen3.8-27b` in organization `org_[masked]` service tier `on_demand` on input tokens per minute (ITPM): Limit 7000, Requested 8657, please reduce your message size and try again. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| ollama nemotron-3-ultra | t8/s1 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | context deadline exceeded |
| groq qwen/qwen3.8-27b | ctx/32k | 1 | 413 | retry-after=206 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=977 x-ratelimit-remaining-tokens=7502 x-ratelimit-reset-requests=33m7.2s x-ratelimit-reset-tokens=4.268s | – | not retried | 413 {"message":"Request too large for model `qwen/qwen3.8-27b` in organization `org_[masked]` service tier `on_demand` on input tokens per minute (ITPM): Limit 7000, Requested 30495, please reduce your message size and try again. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |
| groq openai/gpt-oss-20b | burst/4 | 1 | 429 | retry-after=15 x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=968 x-ratelimit-remaining-tokens=675 x-ratelimit-reset-requests=46m4.8s x-ratelimit-reset-tokens=54.937s | 14.655s | retry after 15s | 429 {"message":"Rate limit reached for model `openai/gpt-oss-20b` in organization `org_[masked]` service tier `on_demand` on tokens per minute (TPM): Limit 8000, Used 7325, Requested 2629. Please try again in 14.655s. Need more tokens? Upgrade to Dev Tier today at https://console.groq.com/settings/billing","type":"tokens","code":"rate_limit_exceeded"} |

Rate-limit headers on the basic request (success):

- groq openai/gpt-oss-120b: x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=996 x-ratelimit-remaining-tokens=7536 x-ratelimit-reset-requests=5m45.6s x-ratelimit-reset-tokens=3.48s
- groq openai/gpt-oss-20b: x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=989 x-ratelimit-remaining-tokens=7665 x-ratelimit-reset-requests=15m50.4s x-ratelimit-reset-tokens=2.512s
- groq qwen/qwen3.8-27b: x-ratelimit-limit-requests=1000 x-ratelimit-limit-tokens=8000 x-ratelimit-remaining-requests=996 x-ratelimit-remaining-tokens=7719 x-ratelimit-reset-requests=5m45.6s x-ratelimit-reset-tokens=2.107s
- ollama gemma4:31b: –
- ollama gpt-oss:120b: –
- ollama gpt-oss:20b: –
- ollama nemotron-3-nano:30b: –
- ollama nemotron-3-super: –
- ollama nemotron-3-ultra: –
- zai glm-4.7-flash: –
- zai glm-4.5-flash: –

## Summary

| Model | Basic | Tools | 8k TTFT | 32k TTFT | Cached tokens: cache turn 2; max in run | 429s / other errors |
|---|---|---|---|---|---|---|
| groq openai/gpt-oss-120b | pass | 9/11 | error 413 (5.5k: 0.57 s) | error 413 | not reported; max 1280 | 0 / 3 |
| groq openai/gpt-oss-20b | pass | 9/11 | error 413 (5.5k: 1.04 s) | error 413 | not reported; max 1280 | 1 / 3 |
| groq qwen/qwen3.8-27b | pass | 10/11 | error 413 (5.5k: 1.12 s) | error 413 | not reported; max 0 | 1 / 3 |
| ollama gemma4:31b | pass | 10/11 | 0.73 s | 1.90 s | 3424 of 3460; max 3456 | 0 / 1 |
| ollama gpt-oss:120b | pass | 8/11 | 1.18 s | 1.71 s | 2384 of 2438; max 2416 | 0 / 0 |
| ollama gpt-oss:20b | pass | 7/11 | 0.89 s | 1.48 s | 2408 of 2432; max 2408 | 0 / 0 |
| ollama nemotron-3-nano:30b | pass | 8/11 | 0.77 s | 1.37 s | not reported; max 0 | 0 / 0 |
| ollama nemotron-3-super | pass | 10/11 | 2.13 s | 3.07 s | not reported; max 0 | 0 / 0 |
| ollama nemotron-3-ultra | pass | 11/11 | 1.83 s | 3.75 s | not reported; max 0 | 0 / 1 |
| zai glm-4.7-flash | fail | 5/11 | 2.48 s | 6.02 s | 3475 of 3499; max 3497 | 18 / 0 |
| zai glm-4.5-flash | pass | 10/11 | 3.32 s | 8.93 s | 3496 of 3520; max 3522 | 0 / 0 |

Total prompt tokens reported by the providers: 819709. Run time: 11.6 min.

The run above lost Gemini: its `/models` call hit `unexpected EOF`, so no Gemini model ran. Appendix A is the Gemini-only rerun with the current code; Appendix B is the earlier quick Gemini run.

## Appendix A: Gemini-only run (`go run . -provider gemini`)

The network failed with DNS errors (`no such host`) near the end of this run, so the gemma 32k test, the gemma no-echo test and the flash-lite burst test did not reach the provider.

Run 2026-09-28 13:37, go1.26.0 windows/amd64, openai-go/v3 v3.66.0. Flags: provider="gemini" quick=false.

Rules used: SDK retries 0, our loop retries 2 times, retry-after capped at 30s (above the cap: give up). A stream is complete only with a finish_reason. Tool list: 24 tools (SPEC 8.1), 5949 bytes of definitions.

### 0. Providers and models

- **gemini** `https://generativelanguage.googleapis.com/v1beta/openai/`: /models lists 61 models: `antigravity-preview-05-2026`, `antigravity-preview-09-2026`, `antigravity-preview-latest`, `aqa`, `deep-research-max-preview-04-2026`, `deep-research-preview-04-2026`, `deep-research-pro-preview-12-2025`, `gemini-2.5-computer-use-preview-10-2025`, `gemini-2.5-flash`, `gemini-2.5-flash-image`, `gemini-2.5-flash-lite`, `gemini-2.5-flash-native-audio-latest`, `gemini-2.5-flash-native-audio-preview-09-2025`, `gemini-2.5-flash-native-audio-preview-12-2025`, `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro`, `gemini-2.5-pro-preview-tts`, `gemini-3-flash-preview`, `gemini-3-pro-image`, `gemini-3-pro-image-preview`, `gemini-3.1-flash-image`, `gemini-3.1-flash-image-preview`, `gemini-3.1-flash-lite`, `gemini-3.1-flash-lite-image`, `gemini-3.1-flash-lite-preview`, `gemini-3.1-flash-live-preview`, `gemini-3.1-flash-tts-preview`, `gemini-3.1-pro-preview`, `gemini-3.1-pro-preview-customtools`, `gemini-3.5-flash`, `gemini-3.5-flash-lite`, `gemini-3.5-live-translate-preview`, `gemini-3.5-transcribe`, `gemini-3.5-transcribe-live`, `gemini-3.6-flash`, `gemini-3.7-flash`, `gemini-3.8-flash`, `gemini-3.8-flash-lite-tts`, `gemini-3.8-flash-tts`, `gemini-3.8-live`, `gemini-3.8-live-extended-thinking`, `gemini-embedding-001`, `gemini-embedding-2`, `gemini-embedding-2-preview`, `gemini-flash-latest`, `gemini-flash-lite-latest`, `gemini-omni-1.1-flash`, `gemini-omni-flash-preview`, `gemini-pro-latest`, `gemini-robotics-er-2-preview`, `gemini-robotics-er-2-streaming-preview`, `gemma-4-26b-a4b-it`, `gemma-4-31b-it`, `lyria-3-clip-preview`, `lyria-3-pro-preview`, `lyria-3.5`, `lyria-realtime-exp`, `nano-banana-pro-preview`, `veo-3.1-fast-generate-preview`, `veo-3.1-generate-preview`, `veo-3.1-lite-generate-preview`

| Provider | Wanted | Model ID tested | In /models | Context limit (provider) | reasoning_effort sent |
|---|---|---|---|---|---|
| gemini | gemini-3.8-flash | `gemini-3.8-flash` | yes | 1048576 in / 65536 out (models.get) | low |
| gemini | gemini-3.5-flash-lite|gemini-3.1-flash-lite | `gemini-3.5-flash-lite` | yes | 1048576 in / 65536 out (models.get) | low |
| gemini | gemma-4-31b-it | `gemma-4-31b-it` | yes | 262144 in / 32768 out (models.get) | – |

Availability probes (one tiny request each):

| Provider | Model | In /models | Result |
|---|---|---|---|
| gemini | `gemini-2.5-pro` | yes | 404 [{ "error": { "code": 404, "message": "This model models/gemini-2.5-pro is no longer available to new users. Please update your code to use models/gemini-3.1-pro-preview for the latest features and improvements. We recommend you to use the Interactions API... |
| gemini | `gemini-3.1-pro-preview` | yes | 429 [{ "error": { "code": 429, "message": "You exceeded your current quota, please check your plan and billing details. For more information on this error, head to: https://ai.google.dev/gemini-api/docs/rate-limits. To monitor your current usage, head to: http... |

### 1. Basic streaming

Prompt: reply "pong" and three colors; max tokens 400. TTFT = first streamed content, reasoning or tool delta.

| Model | Result | finish_reason | Text | Usage in/out | Usage chunk | cached_tokens field | TTFT | Total | Reasoning chars | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | FAIL: 429 [{ "error": { "code": 429, "message": "You exceeded your current quota, please check your plan and billing details. For more information on this error, head... | – |  | 0/0 | – | no | – | 1.47 s | 0 |  |
| gemini gemini-3.5-flash-lite | pass | stop | pong, red, blue, green | 25/7 | same chunk as finish | no | 3.28 s | 3.28 s | 0 |  |
| gemini gemma-4-31b-it | pass | stop | <thought>* Task 1: Reply with the word "pong". * T... | 25/7 | same chunk as finish | no | 32.61 s | 35.56 s | 0 |  |

### 2. Tool calling (24 tools)

- t1 (right tool): What columns does the prices table have?
- t2 (right tool): How many rows in prices are for the coin BTC? Count them with SQL.
- t3 (right tool): Search the web for news about the Ethereum Fusaka upgrade from the past week. I want 5 results.
- t4 (right tool): Remember for this project, in the preferences section, that prices are shown in EUR. The section's current revision is 7.
- t5 (no tool): What is 17 times 3? Just tell me.
- t6 (no tool): How do you say 'good morning' in German?
- t7 (parallel): Describe both tables, prices and coins. Call describe_table for both at once, in parallel.
- t8 (parallel): Two independent things, do both at once in parallel: list the bucket objects under reports/ and get the status of pipeline run run_81.
- t9 (2-step): Start the daily_btc pipeline now, then check the run's status and tell me how it went.
- t10 (3-step): First check the columns of the prices table, then add today's BTC price: 58,000 EUR on 2026-09-28.
- t11 (3-step): Fetch https://example.com/btc-weekly and save it to my resources as a link tagged btc.

A task passes when every step calls exactly the expected tools with schema-valid and correct arguments, and the final answer (if any) has no tool call and contains the expected fact. Fake tool results are fed back between steps.

| Model | Tasks passed | Calls schema-valid | t1 | t2 | t3 | t4 | t5 | t6 | t7 | t8 | t9 | t10 | t11 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | 0/0 | 0/0 | – | – | – | – | – | – | – | – | – | – | – |
| gemini gemini-3.5-flash-lite | 9/11 | 13/13 | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | ok 0s | x | x |
| gemini gemma-4-31b-it | 6/11 | 10/10 | ok 0s | x | ok 0s | x | ok 0s | x | x | ok 0s | x | ok 0s | ok 0s |

Gemini thought signatures (sent back unchanged in the runs above):

- gemini gemini-3.8-flash: tool calls carried `extra_content` (thought signature): no.
- gemini gemini-3.5-flash-lite: tool calls carried `extra_content` (thought signature): yes. t9 again without sending it back: failed: s2 error: 400 [{ "error": { "code": 400, "message": "Function call is missing a thought_signature in functionCall parts. This is required for tools to work correctly, and missing thought_signature may lead to degraded model performance. Additional data, function call `default_api:run_pipeline` , position 2. P...
- gemini gemma-4-31b-it: tool calls carried `extra_content` (thought signature): yes. t9 again without sending it back: failed: s1 error: Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host

Failures:

- gemini gemini-3.5-flash-lite t10: s1: called [describe_table insert], expected [describe_table] (finish stop)
- gemini gemini-3.5-flash-lite t11: s1: called [save_link], expected [fetch_page] (finish stop)
- gemini gemini-3.5-flash-lite: in 3 response(s) parallel tool calls came with the same index and different ids (handled: a new id starts a new call)
- gemini gemma-4-31b-it t2: s1: called [describe_table], expected [query] (finish stop)
- gemini gemma-4-31b-it t4: s1 error: stream ended without finish_reason (truncated)
- gemini gemma-4-31b-it t6: s1 error: 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ]
- gemini gemma-4-31b-it t7: s1 error: stream ended without finish_reason (truncated)
- gemini gemma-4-31b-it t9: s2 error: stream ended without finish_reason (truncated)
- gemini gemma-4-31b-it: in 1 response(s) parallel tool calls came with the same index and different ids (handled: a new id starts a new call)

### 3. Context size

One Burrow-shaped request per size (SPEC 3.1 blocks in the system message, a history window of query turns with tool result previews, the current turn, all 24 tools). Sizes are estimated from the chars per token measured in section 4.

| Model | Context limit | Target | Prompt tokens | TTFT | Total | Result |
|---|---|---|---|---|---|---|
| gemini gemini-3.5-flash-lite | 1048576 in / 65536 out (models.get) | 8k | 7820 | 1.17 s | 2.17 s | ok: The most recent date in the DOT prices query was 2026-08-16. |
| gemini gemini-3.5-flash-lite | 1048576 in / 65536 out (models.get) | 32k | 32410 | 29.98 s | 31.17 s | ok: The most recent date in the last query result was 2026-10-26... |
| gemini gemma-4-31b-it | 262144 in / 32768 out (models.get) | 8k | 0 | – | 120.00 s | ERROR context deadline exceeded |
| gemini gemma-4-31b-it | 262144 in / 32768 out (models.get) | 32k | 0 | – | 11.67 s | ERROR Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |

### 4. Prompt caching

Turn 1: a Burrow-shaped request of about 4k tokens with a fresh session nonce at the start (cold). Turn 2 and 3: the same messages plus the assistant answer and one short new user message (the request only grows at the end).

| Model | Turn 1 in / cached / TTFT | Turn 2 in / cached / TTFT | Turn 3 in / cached / TTFT | Whole run: calls with cached_tokens field / with cached > 0 / max cached / successful calls |
|---|---|---|---|---|
| gemini gemini-3.8-flash | – | – | – | 0 / 0 / 0 / 0 |
| gemini gemini-3.5-flash-lite | 3450 / 0 / 31.69 s | 3471 / 0 / 1.06 s | 3518 / 0 / 1.08 s | 0 / 0 / 0 / 20 |
| gemini gemma-4-31b-it | 3450 / 0 / 40.47 s | 3512 / 0 / 33.32 s | 3642 / 1972 / 34.38 s | 4 / 4 / 1972 / 16 |

### 5. Rate limits

Burst tests (after the other tests of the provider): Groq sends 4 requests of about 3.5k tokens back to back (TPM limit test); Gemini sends up to 16 tiny requests back to back (RPM test); Ollama and Z.ai send 2 requests at the same time (Ollama free plan: 1 concurrent request).

| Provider | Model | Result |
|---|---|---|
| gemini | `gemini-3.5-flash-lite` | #1 failed after 3 attempt(s): Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp:... |

Every 429 and every other error status seen in the whole run (rate-limit header values only; no auth headers are read):

| Model | Request | Attempt | Status | Rate-limit headers | Wait hint in body | Action | Body (short) |
|---|---|---|---|---|---|---|---|
| gemini gemini-2.5-pro | probe | 1 | 404 | – | – | not retried | 404 [{ "error": { "code": 404, "message": "This model models/gemini-2.5-pro is no longer available to new users. Please update your code to use models/gemini-3.1-pro-preview for the latest features and improvements. We recommend you to use the Interactions API (https://ai.google.dev/gemini-api/docs/get-started).", "status": "NOT_FOUND" } } ] |
| gemini gemini-3.1-pro-preview | probe | 1 | 429 | – | GenerateContentInputTokensPerModelPerDay-FreeTier retryDelay 0s | daily/quota limit: stop this model | 429 [{ "error": { "code": 429, "message": "You exceeded your current quota, please check your plan and billing details. For more information on this error, head to: https://ai.google.dev/gemini-api/docs/rate-limits. To monitor your current usage, head to: https://ai.dev/rate-limit. \n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_input_token_count, limit: 0, model: gemini-3.1-pro\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 0, model: gemini-3.1-pro\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 0, model: gemini-3.1-pro\n* Quota... |
| gemini gemini-3.8-flash | basic | 1 | 429 | – | GenerateRequestsPerDayPerProjectPerModel-FreeTier retryDelay 59s | daily/quota limit: stop this model | 429 [{ "error": { "code": 429, "message": "You exceeded your current quota, please check your plan and billing details. For more information on this error, head to: https://ai.google.dev/gemini-api/docs/rate-limits. To monitor your current usage, head to: https://ai.dev/rate-limit. \n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 20, model: gemini-3.8-flash\nPlease retry in 59.281060615s.", "status": "RESOURCE_EXHAUSTED", "details": [ { "@type": "type.googleapis.com/google.rpc.Help", "links": [ { "description": "Learn more about Gemini API quotas", "url": "https://ai.google.dev/gemini-api/docs/rate-limits" } ] }, { "@type": "type.go... |
| gemini gemma-4-31b-it | basic | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t1/s1 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t1/s1 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t2/s1 | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t3/s1 | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t3/s1 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t4/s1 | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t4/s1 | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemini-3.5-flash-lite | t9/s2 | 1 | 400 | – | – | not retried | 400 [{ "error": { "code": 400, "message": "Function call is missing a thought_signature in functionCall parts. This is required for tools to work correctly, and missing thought_signature may lead to degraded model performance. Additional data, function call `default_api:run_pipeline` , position 2. Please refer to https://ai.google.dev/gemini-api/docs/thought-signatures for more details.", "status": "INVALID_ARGUMENT" } } ] |
| gemini gemma-4-31b-it | t4/s1 | 3 | transport/stream | – | – | retries used up | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t6/s1 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t6/s1 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t6/s1 | 3 | 500 | – | – | retries used up | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t7/s1 | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t7/s1 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t7/s1 | 3 | transport/stream | – | – | retries used up | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t9/s2 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t9/s2 | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t9/s2 | 3 | transport/stream | – | – | retries used up | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | t10/s2 | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t10/s2 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t10/s3 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | context deadline exceeded |
| gemini gemma-4-31b-it | t10/s3 | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": unexpected EOF |
| gemini gemma-4-31b-it | t11/s3 | 1 | 500 | – | – | retry after 2s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | t11/s3 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | cache/t1 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | cache/t1 | 2 | 500 | – | – | retry after 4s (no retry-after header, backoff) | 500 [{ "error": { "code": 500, "message": "Internal error encountered.", "status": "INTERNAL" } } ] |
| gemini gemma-4-31b-it | ctx/8k | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | ctx/8k | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | stream ended without finish_reason (truncated) |
| gemini gemma-4-31b-it | ctx/8k | 3 | transport/stream | – | – | retries used up | context deadline exceeded |
| gemini gemma-4-31b-it | ctx/32k | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | context deadline exceeded |
| gemini gemma-4-31b-it | ctx/32k | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemma-4-31b-it | ctx/32k | 3 | transport/stream | – | – | retries used up | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemma-4-31b-it | t9/s1 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemma-4-31b-it | t9/s1 | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemma-4-31b-it | t9/s1 | 3 | transport/stream | – | – | retries used up | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemini-3.5-flash-lite | burst/1 | 1 | transport/stream | – | – | retry after 2s (no retry-after header, backoff) | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemini-3.5-flash-lite | burst/1 | 2 | transport/stream | – | – | retry after 4s (no retry-after header, backoff) | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |
| gemini gemini-3.5-flash-lite | burst/1 | 3 | transport/stream | – | – | retries used up | Post "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions": dial tcp: lookup generativelanguage.googleapis.com: no such host |

Rate-limit headers on the basic request (success):

- gemini gemini-3.8-flash: –
- gemini gemini-3.5-flash-lite: –
- gemini gemma-4-31b-it: –

### Summary

| Model | Basic | Tools | 8k TTFT | 32k TTFT | Cached tokens: cache turn 2; max in run | 429s / other errors |
|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | fail | 0/0 | – | – | –; max 0 | 1 / 0 |
| gemini gemini-3.5-flash-lite | pass | 9/11 | 1.17 s | 29.98 s | not reported; max 0 | 0 / 4 |
| gemini gemma-4-31b-it | pass | 6/11 | error (transport/stream) | error (transport/stream) | not reported; max 1972 | 0 / 35 |

Total prompt tokens reported by the providers: 117819. Run time: 28.2 min.

## Appendix B: earlier quick Gemini run (`go run . -quick -provider gemini`)

Older code (no transport retries, `gemini-2.5-pro` still in the model list). Made before `gemini-3.8-flash` ran out of its 20 requests per day.

Run 2026-09-28 12:40, go1.26.0 windows/amd64, openai-go/v3 v3.66.0. Flags: provider="gemini" quick=true.

Rules used: SDK retries 0, our loop retries 2 times, retry-after capped at 30s (above the cap: give up). A stream is complete only with a finish_reason. Tool list: 24 tools (SPEC 8.1), 5949 bytes of definitions.

### 0. Providers and models

- **gemini** `https://generativelanguage.googleapis.com/v1beta/openai/`: /models lists 61 models: `antigravity-preview-05-2026`, `antigravity-preview-09-2026`, `antigravity-preview-latest`, `aqa`, `deep-research-max-preview-04-2026`, `deep-research-preview-04-2026`, `deep-research-pro-preview-12-2025`, `gemini-2.5-computer-use-preview-10-2025`, `gemini-2.5-flash`, `gemini-2.5-flash-image`, `gemini-2.5-flash-lite`, `gemini-2.5-flash-native-audio-latest`, `gemini-2.5-flash-native-audio-preview-09-2025`, `gemini-2.5-flash-native-audio-preview-12-2025`, `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro`, `gemini-2.5-pro-preview-tts`, `gemini-3-flash-preview`, `gemini-3-pro-image`, `gemini-3-pro-image-preview`, `gemini-3.1-flash-image`, `gemini-3.1-flash-image-preview`, `gemini-3.1-flash-lite`, `gemini-3.1-flash-lite-image`, `gemini-3.1-flash-lite-preview`, `gemini-3.1-flash-live-preview`, `gemini-3.1-flash-tts-preview`, `gemini-3.1-pro-preview`, `gemini-3.1-pro-preview-customtools`, `gemini-3.5-flash`, `gemini-3.5-flash-lite`, `gemini-3.5-live-translate-preview`, `gemini-3.5-transcribe`, `gemini-3.5-transcribe-live`, `gemini-3.6-flash`, `gemini-3.7-flash`, `gemini-3.8-flash`, `gemini-3.8-flash-lite-tts`, `gemini-3.8-flash-tts`, `gemini-3.8-live`, `gemini-3.8-live-extended-thinking`, `gemini-embedding-001`, `gemini-embedding-2`, `gemini-embedding-2-preview`, `gemini-flash-latest`, `gemini-flash-lite-latest`, `gemini-omni-1.1-flash`, `gemini-omni-flash-preview`, `gemini-pro-latest`, `gemini-robotics-er-2-preview`, `gemini-robotics-er-2-streaming-preview`, `gemma-4-26b-a4b-it`, `gemma-4-31b-it`, `lyria-3-clip-preview`, `lyria-3-pro-preview`, `lyria-3.5`, `lyria-realtime-exp`, `nano-banana-pro-preview`, `veo-3.1-fast-generate-preview`, `veo-3.1-generate-preview`, `veo-3.1-lite-generate-preview`

| Provider | Wanted | Model ID tested | In /models | Context limit (provider) | reasoning_effort sent |
|---|---|---|---|---|---|
| gemini | gemini-3.8-flash | `gemini-3.8-flash` | yes | 1048576 in / 65536 out (models.get) | low |
| gemini | gemini-3.5-flash-lite|gemini-3.1-flash-lite | `gemini-3.5-flash-lite` | yes | 1048576 in / 65536 out (models.get) | low |
| gemini | gemini-2.5-pro | `gemini-2.5-pro` | yes | 1048576 in / 65536 out (models.get) | low |

### 1. Basic streaming

Prompt: reply "pong" and three colors; max tokens 400. TTFT = first streamed content, reasoning or tool delta.

| Model | Result | finish_reason | Text | Usage in/out | Usage chunk | cached_tokens field | TTFT | Total | Reasoning chars | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | pass | stop | pong, red, blue, green | 25/7 | same chunk as finish | no | 2.02 s | 2.02 s | 0 |  |
| gemini gemini-3.5-flash-lite | pass | stop | pong, red, blue, green | 25/7 | same chunk as finish | no | 1.02 s | 1.02 s | 0 |  |
| gemini gemini-2.5-pro | FAIL: 404 [{ "error": { "code": 404, "message": "This model models/gemini-2.5-pro is no longer available to new users. Please update your code to use models/gemini-3.... | – |  | 0/0 | – | no | – | 1.02 s | 0 |  |

### 2. Tool calling (24 tools)

- t1 (right tool): What columns does the prices table have?
- t2 (right tool): How many rows in prices are for the coin BTC? Count them with SQL.
- t3 (right tool): Search the web for news about the Ethereum Fusaka upgrade from the past week. I want 5 results.
- t4 (right tool): Remember for this project, in the preferences section, that prices are shown in EUR. The section's current revision is 7.
- t5 (no tool): What is 17 times 3? Just tell me.
- t6 (no tool): How do you say 'good morning' in German?
- t7 (parallel): Describe both tables, prices and coins. Call describe_table for both at once, in parallel.
- t8 (parallel): Two independent things, do both at once in parallel: list the bucket objects under reports/ and get the status of pipeline run run_81.
- t9 (2-step): Start the daily_btc pipeline now, then check the run's status and tell me how it went.
- t10 (3-step): First check the columns of the prices table, then add today's BTC price: 58,000 EUR on 2026-09-28.
- t11 (3-step): Fetch https://example.com/btc-weekly and save it to my resources as a link tagged btc.

A task passes when every step calls exactly the expected tools with schema-valid and correct arguments, and the final answer (if any) has no tool call and contains the expected fact. Fake tool results are fed back between steps.

| Model | Tasks passed | Calls schema-valid | t1 | t2 | t3 | t4 | t5 | t6 | t7 | t8 | t9 | t10 | t11 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | 3/3 | 3/3 | ok 0s | – | – | – | ok 0s | – | – | – | ok 0s | – | – |
| gemini gemini-3.5-flash-lite | 3/3 | 3/3 | ok 0s | – | – | – | ok 0s | – | – | – | ok 0s | – | – |
| gemini gemini-2.5-pro | 0/0 | 0/0 | – | – | – | – | – | – | – | – | – | – | – |

Gemini thought signatures (sent back unchanged in the runs above):

- gemini gemini-3.8-flash: tool calls carried `extra_content` (thought signature): yes.
- gemini gemini-3.5-flash-lite: tool calls carried `extra_content` (thought signature): yes.
- gemini gemini-2.5-pro: tool calls carried `extra_content` (thought signature): no.

Failures:


### 3. Context size

One Burrow-shaped request per size (SPEC 3.1 blocks in the system message, a history window of query turns with tool result previews, the current turn, all 24 tools). Sizes are estimated from the chars per token measured in section 4.

| Model | Context limit | Target | Prompt tokens | TTFT | Total | Result |
|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | 1048576 in / 65536 out (models.get) | 8k | 10065 | 4.29 s | 4.29 s | ok: The most recent date in the LTC query result above was 2026-... |
| gemini gemini-3.5-flash-lite | 1048576 in / 65536 out (models.get) | 8k | 10064 | 1.71 s | 1.71 s | ok: The most recent date in the LTC prices query result was 2026... |

### 4. Prompt caching

Turn 1: a Burrow-shaped request of about 4k tokens with a fresh session nonce at the start (cold). Turn 2 and 3: the same messages plus the assistant answer and one short new user message (the request only grows at the end).

| Model | Turn 1 in / cached / TTFT | Turn 2 in / cached / TTFT | Turn 3 in / cached / TTFT | Whole run: calls with cached_tokens field / with cached > 0 / max cached / successful calls |
|---|---|---|---|---|
| gemini gemini-3.8-flash | 3450 / 0 / 3.90 s | 3472 / 0 / 10.48 s | – | 0 / 0 / 0 / 9 |
| gemini gemini-3.5-flash-lite | 3451 / 0 / 2.14 s | 3472 / 0 / 1.51 s | – | 0 / 0 / 0 / 9 |
| gemini gemini-2.5-pro | – | – | – | 0 / 0 / 0 / 0 |

### 5. Rate limits

Burst tests (after the other tests of the provider): Groq sends 4 requests of about 2.5k tokens back to back (TPM limit test); Gemini sends up to 16 tiny requests back to back (RPM test); Ollama sends 2 requests at the same time (free plan: 1 concurrent request).

| Provider | Model | Result |
|---|---|---|

Every 429 and every other error status seen in the whole run (rate-limit header values only; no auth headers are read):

| Model | Request | Attempt | Status | Rate-limit headers | Wait hint in body | Action | Body (short) |
|---|---|---|---|---|---|---|---|
| gemini gemini-2.5-pro | basic | 1 | 404 | – | – | not retried | 404 [{ "error": { "code": 404, "message": "This model models/gemini-2.5-pro is no longer available to new users. Please update your code to use models/gemini-3.1-pro-preview for the latest features an... |

Rate-limit headers on the basic request (success):

- gemini gemini-3.8-flash: –
- gemini gemini-3.5-flash-lite: –
- gemini gemini-2.5-pro: –

### Summary

| Model | Basic | Tools | 8k TTFT | 32k TTFT | Cached tokens (turn 2) | 429s / errors |
|---|---|---|---|---|---|---|
| gemini gemini-3.8-flash | pass | 3/3 | 4.29 s | – | not reported | 0 / 0 |
| gemini gemini-3.5-flash-lite | pass | 3/3 | 1.71 s | – | not reported | 0 / 0 |
| gemini gemini-2.5-pro | fail | 0/0 | – | – | – | 0 / 1 |

Total prompt tokens reported by the providers: 53038. Run time: 1.5 min.

## Recommendation

- Default development and test models: Ollama Cloud `nemotron-3-ultra` (11/11 tool tasks, 8k TTFT 1.83 s, 32k TTFT 3.75 s) and `gemma4:31b` (10/11, 0.73 s / 1.90 s, 3424 of 3460 prompt tokens cached on turn 2).
- Second choice: Z.ai `glm-4.5-flash` (10/11, 3.32 s / 8.93 s, caching reported, no 429s). `glm-4.7-flash` was overloaded (429 code 1305) during this run.
- Groq free tier: only for small requests. Every 8k and 32k request got 413: the free tier allows 8000 tokens per minute (TPM) for gpt-oss and 7000 input tokens per minute for qwen.
- Gemini free tier: `gemini-3.5-flash-lite` works for small tests (9/11), but 32k TTFT was 29.98 s. `gemini-3.8-flash` allows 20 requests per day. `gemma-4-31b-it` returned many 500 errors and truncated streams.
- Phase 1 provider: Ollama Cloud is the best candidate of the four (OpenAI-compatible, 262144-token contexts for gemma4 and nemotron, cached tokens reported). It sends no rate-limit headers, so Burrow must pace on its own.

secret scan: clean
