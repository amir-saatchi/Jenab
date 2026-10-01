# SPIKE-018 — Cloud models for development: Groq, Ollama Cloud, Gemini and Z.ai
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can the user's free Groq, Ollama Cloud, Gemini and Z.ai accounts serve as fast development and test models for Jenab, and possibly as Phase 1 providers (PROPOSAL open question 3)?

Models available to the user:
- **Groq:** GPT-OSS 120B, GPT-OSS 20B, Qwen 27B
- **Ollama Cloud (free credits):** gemma4:31b, gpt-oss:120b, gpt-oss:20b, nemotron-3-nano:30b, nemotron-3-super, nemotron-3-ultra
- **Gemini (free tier):** the models the key lists, through Gemini's OpenAI-compatible endpoint
- **Z.ai (free tier):** the free GLM Flash model(s) the key can use, through Z.ai's OpenAI-compatible endpoint

For each model:
1. Works through `openai-go/v3` with a custom base URL (SPEC 3.8)? Streaming, `finish_reason`, usage
2. Tool calling with Jenab's real tool list (SPEC 8.1): right tool, valid arguments, multi-step turns, parallel calls
3. Context: the real limit, and time to first token at 8k and 32k tokens
4. Prompt caching (cached tokens reported?) when a request only grows at the end (SPEC 3.1)
5. Rate limits: 429s, `retry-after`, and how the SPIKE-012 retry rules handle them
6. Only synthetic test prompts are sent. The whole run stays within a small token budget.

The keys are `GROQ_API_KEY`, `OLLAMA_API_KEY`, `GEMINI_API_KEY` and `Z_API_KEY`, in the user's `.env` at the repository root (git-ignored). The test program loads them itself; they are never printed or logged.

## Done when
Each model has a pass/fail row for points 1–5, and one or two are chosen as the default development models.

## Result
Run on 2026-09-28 through `openai-go/v3` v3.66.0 with each provider's OpenAI-compatible endpoint. Code and full output: `spikes/018-cloud-models/` (`results.md`). Tool tests use the 24 SPEC 8.1 tools and 11 tasks, from single calls to parallel and 3-step turns. Gemini is in appendix A of `results.md`, because the main run could not list its models.

| Model | Basic | Tools (of 11) | Time to first token, 8k / 32k | Cached tokens, turn 2 | Rate limits |
|---|---|---|---|---|---|
| Ollama `nemotron-3-ultra` | pass | **11** | 1.83 s / 3.75 s | not reported | none hit |
| Ollama `nemotron-3-super` | pass | 10 | 2.13 / 3.07 s | not reported | none hit |
| Ollama `gemma4:31b` | pass | 10 | **0.73 / 1.90 s** | 3,424 of 3,460 | none hit |
| Ollama `nemotron-3-nano:30b` | pass | 8 | 0.77 / 1.37 s | not reported | none hit |
| Ollama `gpt-oss:120b` / `20b` | pass | 8 / 7 | about 1 / 1.6 s | about 2,400 of 2,430 | none hit |
| Z.ai `glm-4.5-flash` | pass | 10 | 3.32 / 8.93 s | 3,496 of 3,520 | none hit |
| Z.ai `glm-4.7-flash` | fail | 5 | 2.48 / 6.02 s | 3,475 of 3,499 | 18 × 429 "overloaded" |
| Groq `qwen3.8-27b` | pass | 10 | 413 (5.5k: 1.12 s) | not reported | 7,000 input tokens/min |
| Groq `gpt-oss-120b` / `20b` | pass | 9 / 9 | 413 (5.5k: 0.57 / 1.04 s) | at most 1,280 | 8,000 tokens/min |
| Gemini `3.5-flash-lite` | pass | 9 | 1.17 / 29.98 s | not reported | — |
| Gemini `3.8-flash` | fail | — | 4.29 s (quick run) | — | 20 requests/day |
| Gemini `gemma-4-31b-it` | pass | 6 | errors | — | 35 × 500s and cut-off streams |

**Findings**
- **Tool misses:**
  - The gpt-oss models never make parallel tool calls: they called one tool where two were asked for.
  - The most common miss was "count the BTC rows", where models called `describe_table` before `query`. This is arguably a reasonable first step.
- **Real context limits:**
  - Ollama: 262,144 tokens for gemma4 and the nemotrons, 131,072 for gpt-oss.
  - Groq: 131,072, but the free tier's per-minute token cap rejects any request over about 7–8k tokens with a **413**.
  - Gemini flash: 1,048,576 tokens.
  - Z.ai: taken from its docs only (128k–200k).
- **Caching:** Ollama (gemma4, gpt-oss) and Z.ai report cached tokens on a request that only grows at the end. Time to first token did not reliably drop on the cached turn.
- **Rate limits:**
  - Groq sends `retry-after` (10–15 s), and the SPIKE-012 retry loop recovered.
  - Ollama Cloud and Z.ai send no rate-limit headers.
  - Gemini puts the wait (`retryDelay`) in the error body.
  - Z.ai's `glm-4.7-flash` returned 429 code 1305 ("overloaded") with no wait hint.
- **Usage** arrives in its own chunk after `finish_reason` (Groq, Ollama) or in the same chunk (Gemini, Z.ai).
- **Gemini-specific:**
  - Tool calls carry `extra_content` (a thought signature). Leaving it out on the next request gives a 400.
  - Error bodies are JSON arrays.
  - A parallel tool call can reuse an index with a new id.
  - Gemma rejects `reasoning_effort`.
- **Transport:** every provider returned an occasional `unexpected EOF`. Retries fixed them.
- **The SDK reads `OPENAI_*` environment variables** (org, project, custom headers). The test program blocked them with a header allow-list, so they never reached another host.
- **Z.ai:** `api.z.ai` worked, so `open.bigmodel.cn` was not needed. The two free flash models are not listed by `/models`, but they answer.
- **Budget:** the test used more than planned: over 1.5 million prompt tokens across all runs and reruns, 0.82 million of them in the final full run. It was all on free tiers.
- **Not tested:** paid tiers, Z.ai's real context limit, and Gemma's 32k run (network errors).

## Decision
**Decided (2026-09-28):**
- **Default development and test models:** Ollama Cloud `nemotron-3-ultra` (best at tools) and `gemma4:31b` (fastest, reports caching). The backup is Z.ai `glm-4.5-flash`.
- **Groq's free tier** is only for small requests, under about 7k tokens. **Gemini's free tier** is not a default: the daily limits are too low and it is too unreliable.
- **Phase 1 providers (PROPOSAL open question 3):** Ollama Cloud is the strongest OpenAI-compatible candidate. The final list stays open until the Phase 1 plan.
- **Provider rules** from this spike go into SPEC 3.8:
  - read the stream to its end for usage
  - treat a 413 as "shrink the request"
  - stop on daily or quota errors
  - pace requests when there are no rate-limit headers
  - retry transport errors
  - the Gemini message rules
  - ignore the SDK's `OPENAI_*` environment variables
