# NVIDIA_API_KEY check

**Checked:** 2026-11-09. Requests sent: **47** (well under the 80 budget and under 40 a minute).

## Does the key work?

Yes. `GET /v1/models` answers 200, and 11 chat models answered a plain prompt.

- **Base URL:** `https://integrate.api.nvidia.com/v1/`. NVIDIA's [LLM API reference](https://docs.api.nvidia.com/nim/reference/llm-apis) gives the host `https://integrate.api.nvidia.com` and the path `POST /v1/chat/completions`.
- **Kind:** `openai_compatible`. Chat, `tools` and `tool_choice` work in the OpenAI format. Streaming (SSE) works.
- **Key:** `NVIDIA_API_KEY`, sent as `Authorization: Bearer`.

Quirks:
- **No User-Agent needed.** Python's default User-Agent got 200, unlike Groq and GMI.
- **`/v1/models` lists more than the key can call.** 10 of the listed models answer 404 "Function ... Not found for account". The 404 body contains an account ID, so don't copy raw errors into docs or logs.
- **Busy models hang.** Some models send nothing at all, even with streaming, until the client gives up (tested to 240 s). Retrying sometimes helps. Set a client timeout; don't wait forever.
- **No rate-limit headers.** Responses carry no `x-ratelimit-*` or credit headers. Only a 429 tells you when you hit the limit.

## Free-tier rules

- **Free for prototyping only.** "Members of the NVIDIA Developer Program have free access to NIM API endpoints for prototyping." It is "for prototyping, research, development and testing purposes only". Production needs an NVIDIA AI Enterprise licence. ([General NIM FAQ](https://docs.api.nvidia.com/nim/docs/product))
- **No fixed end date.** Access lasts "through the duration of program membership" (same FAQ).
- **Rate limit, not credits.** NVIDIA staff on the forum (2026-04-29) said the limit depends "on model, use-case and the amount of current overall traffic", and that there is no way to raise it on the free tier. On 2026-05-13 they named "the 40 rpm rate limit". ([forum thread](https://forums.developer.nvidia.com/t/api-rate-limit-increase-is-not-granted-by-requesting-it-here/368420)) Third-party guides say the 40 a minute is shared across all models. The old 1,000 / 5,000 credit system is gone, though the trial terms still mention credits.
- **Prompts may be used by NVIDIA.** The [NVIDIA API Trial Terms of Service](https://assets.ngc.nvidia.com/products/api-catalog/legal/NVIDIA%20API%20Trial%20Terms%20of%20Service.pdf) (dated 2025-10-07), section 3.3: NVIDIA collects "User Content and Generated Content to improve NVIDIA products and services, including AI models". Use is logged for security and abuse checks. Section 1.1: trial use only, not in production. Section 2.6: no confidential or sensitive data. Our synthetic-prompts rule already covers this.
- **Free vs paid.** There is no paid tier on the site. The [model catalog](https://build.nvidia.com/models) (105 models) marks each model "Free Endpoint" (hosted, callable with the key) or "Downloadable" only (self-host). Every model that answered is marked "Free Endpoint". The 404 models are listed by the API but have no free endpoint in the catalog (they are older or replaced).

## Models listed by `/v1/models`: 84

Chat or text models in bold are the ones probed.

- **Google (9):** `codegemma-1.1-7b`, `codegemma-7b`, `deplot`, **`diffusiongemma-26b-a4b-it`**, `gemma-2b`, **`gemma-3-12b-it`**, **`gemma-3-4b-it`**, **`gemma-4-31b-it`**, `recurrentgemma-2b`
- **OpenAI (1):** **`openai/gpt-oss-20b`** (no `gpt-oss-120b`)
- **Qwen:** none
- **NVIDIA Nemotron and other chat (19):** `cosmos-reason2-8b`, `ising-calibration-1.5-31b`, `llama-3.1-nemotron-51b-instruct`, **`llama-3.1-nemotron-70b-instruct`**, **`llama-3.1-nemotron-ultra-253b-v1`**, `llama3-chatqa-1.5-70b`, `mistral-nemo-minitron-8b-8k-instruct`, `nemotron-3-nano-omni-30b-a3b-reasoning`, **`nemotron-3-super-120b-a12b`**, **`nemotron-3-ultra-550b-a55b`**, **`nemotron-3.5-bolt-e4b`**, **`nemotron-3.5-lightning-30b-a3b`**, **`nemotron-3.5-super-vl-120b-a12b`**, `nemotron-4-340b-instruct`, **`nemotron-nano-3-30b-a3b`**, `neva-22b`, `riva-translate-4b-instruct`, `riva-translate-4b-instruct-v2`, `vila` (all `nvidia/`)
- **NVIDIA safety and reward (5):** `llama-3.1-nemoguard-8b-content-safety`, `llama-3.1-nemoguard-8b-topic-control`, `llama-3.1-nemotron-safety-guard-8b-v3`, `nemotron-3.5-content-safety`, `nemotron-4-340b-reward`
- **NVIDIA embedding, parsing, other (12):** `ai-synthetic-video-detector`, `embed-qa-4`, `llama-3.2-nemoretriever-1b-vlm-embed-v1`, `llama-3.2-nv-embedqa-1b-v1`, `llama-nemotron-embed-vl-1b-v2`, `nemotron-3-embed-1b`, `nemotron-3.5-embed-1b`, `nemotron-3.5-embed-8b`, `nemotron-parse`, `nemotron-parse-2.0`, `nv-embedqa-mistral-7b-v2`, `nvclip`
- **Mistral (7):** `mistralai/codestral-22b-instruct-v0.1`, **`mistralai/mistral-7b-instruct-v0.3`**, `mistralai/mistral-large`, **`mistralai/mistral-large-2-instruct`**, **`mistralai/mistral-large-4-1t-a52b`**, `mistralai/mixtral-8x22b-v0.1`, **`nv-mistralai/mistral-nemo-12b-instruct`**
- **Meta (5):** `codellama-70b`, **`llama-3.2-90b-vision-instruct`**, `llama-guard-4-12b`, `llama2-70b`, **`muse-glimmer-30b`**
- **DeepSeek (2):** `deepseek-coder-6.7b-instruct`, **`deepseek-v4.1-flash`**
- **Kimi (1):** **`moonshotai/kimi-k2.6`**. The catalog shows `kimi-k3.1-pro` as a free endpoint, but the key's list doesn't have it.
- **GLM (1):** **`z-ai/glm-5.3-flash`**
- **MiniMax (1):** **`minimaxai/minimax-m3.1-flash`**
- **Others (21):** `01-ai/yi-large`, `adept/fuyu-8b`, `ai21labs/jamba-1.5-large-instruct`, `aisingapore/sea-lion-7b-instruct`, `bigcode/starcoder2-15b`, `databricks/dbrx-instruct`, `ibm/granite-3.0-3b-a800m-instruct`, `ibm/granite-3.0-8b-instruct`, `ibm/granite-34b-code-instruct`, `ibm/granite-8b-code-instruct`, `microsoft/kosmos-2`, `microsoft/phi-3-vision-128k-instruct`, **`microsoft/phi-3.5-moe-instruct`**, **`poolside/laguna-xs-2.1`**, `snowflake/arctic-embed-l`, **`stepfun-ai/step-5-preview`**, `writer/palmyra-creative-122b`, `writer/palmyra-fin-70b-32k`, `writer/palmyra-med-70b`, `writer/palmyra-med-70b-32k`, `zyphra/zamba2-7b-instruct`

## Probe results

Chat prompt: "What is 2+3? Answer with the number only." Tool prompt: "What time is it in UTC? Use the tool." with one `get_time(timezone)` function. Every answer was correct ("5", or `get_time({"timezone":"UTC"})`).

| Model | Chat | Tool call | Latency (chat / tool) | Notes |
|---|---|---|---|---|
| `nvidia/nemotron-3.5-bolt-e4b` | 200 | 200, calls `get_time` | 0.4 s / 0.5 s | Fastest |
| `nvidia/nemotron-3.5-lightning-30b-a3b` | 200 | 200, calls `get_time` | 1.8 s / 1.0 s | |
| `nvidia/nemotron-3.5-super-vl-120b-a12b` | 200 | 200, calls `get_time` | 0.9 s / 1.0 s | |
| `nvidia/nemotron-3-ultra-550b-a55b` | 200 | 200, calls `get_time` | 1.1 s / 4.6 s | |
| `mistralai/mistral-large-4-1t-a52b` | 200 | 200, calls `get_time` | 21.4 s / 1.6 s | Slow first call |
| `meta/muse-glimmer-30b` | 200 | 200, calls `get_time` | 7.2 s / 4.5 s | |
| `poolside/laguna-xs-2.1` | 200 | 200, calls `get_time` | 0.4 s / 16.4 s | |
| `stepfun-ai/step-5-preview` | timeout (90 s), then 200 | 200, calls `get_time` | 10 s (streamed) / 22.8 s | Reasoning model |
| `google/diffusiongemma-26b-a4b-it` | timeout (90 s), then 200 | timeout (240 s) | 8.5 s (streamed) / – | Busy |
| `minimaxai/minimax-m3.1-flash` | timeout (90 s), then 200 | timeout (240 s) | 239 s to the first chunk / – | Too slow to use |
| `google/gemma-4-31b-it` | timeout (90 s, then 240 s) | not sent | – | "Free Endpoint", no reply |
| `openai/gpt-oss-20b` | timeout (90 s, then 240 s) | not sent | – | "Free Endpoint", no reply |
| `deepseek-ai/deepseek-v4.1-flash` | timeout (90 s, then 240 s) | not sent | – | "Free Endpoint", no reply |
| `z-ai/glm-5.3-flash` | timeout (90 s, then 240 s) | not sent | – | "Free Endpoint", no reply |
| `meta/llama-3.2-90b-vision-instruct` | timeout (90 s, then 240 s) | not sent | – | "Free Endpoint", no reply |
| `google/gemma-3-12b-it` | 404 | not sent | 0.4 s | Not found for account |
| `google/gemma-3-4b-it` | 404 | not sent | 0.4 s | Not found for account |
| `moonshotai/kimi-k2.6` | 404 | not sent | 0.4 s | Not found for account |
| `mistralai/mistral-large-2-instruct` | 404 | not sent | 0.5 s | Not found for account |
| `mistralai/mistral-7b-instruct-v0.3` | 404 | not sent | 0.4 s | Not found for account |
| `nv-mistralai/mistral-nemo-12b-instruct` | 404 | not sent | 0.4 s | Not found for account |
| `nvidia/nemotron-3-super-120b-a12b` | 404 | not sent | 0.4 s | Not found for account. OpenRouter's `:free` copy also got 404 from Nvidia. |
| `nvidia/nemotron-nano-3-30b-a3b` | 404 | not sent | 0.4 s | Not found for account |
| `nvidia/llama-3.1-nemotron-ultra-253b-v1` | 404 | not sent | 0.4 s | Not found for account |
| `nvidia/llama-3.1-nemotron-70b-instruct` | 404 | not sent | 0.4 s | Not found for account |
| `microsoft/phi-3.5-moe-instruct` | 404 | not sent | 0.4 s | Not found for account |

No 402, 403, 429 or 5xx came back. The timeouts look like busy shared queues, not a quota.

## Suggested row for Docs/MODELS.md ("Working")

| Provider | Kind | Base URL | Key | Models that answer | Limits |
|---|---|---|---|---|---|
| NVIDIA (build.nvidia.com) | `openai_compatible` | `https://integrate.api.nvidia.com/v1/` | `NVIDIA_API_KEY` | With tools (2026-11-09): `nvidia/nemotron-3.5-bolt-e4b`, `nvidia/nemotron-3.5-lightning-30b-a3b`, `nvidia/nemotron-3.5-super-vl-120b-a12b`, `nvidia/nemotron-3-ultra-550b-a55b`, `mistralai/mistral-large-4-1t-a52b`, `meta/muse-glimmer-30b`, `poolside/laguna-xs-2.1`, `stepfun-ai/step-5-preview` (10–23 s). Chat only, slow: `google/diffusiongemma-26b-a4b-it`, `minimaxai/minimax-m3.1-flash` (4 min). No reply in 240 s: `gemma-4-31b-it`, `gpt-oss-20b`, `deepseek-v4.1-flash`, `glm-5.3-flash`, `llama-3.2-90b-vision-instruct`. `/v1/models` lists 84; models without a "Free Endpoint" in the catalog give 404 (`kimi-k2.6`, `nemotron-3-super-120b-a12b`, `gemma-3`, older Mistral and Llama) | Free for Developer Program members, prototyping only, no end date. About 40 requests a minute (NVIDIA staff on the forum; no headers say so). No credits. NVIDIA may use prompts to improve its models. Busy models can hang with no reply; set a timeout. |

## Files

The scripts and raw results are in this session's scratchpad, not the repo. Nothing in the repo was changed apart from this report.
