# Models we can use in development
**Status:** checked 2026-10-26

The models the development keys can reach, for spikes, scenarios and tests. Keys are in `.env` at the repository root ([DEVELOPMENT](DEVELOPMENT.md#keys-for-development)). This file names only the variables, never the values. Spikes list the models they run in their own `models.yaml`; `scenarios/models.example.yaml` shows the format.

When a check here changes (a quota runs out, a new key, a model that stops answering), update this file with the date.

## Rules

- **Reference models** are the ones spikes are decided on ([DEVELOPMENT](DEVELOPMENT.md#reference-models)). Failures on other models are recorded, not worked around.
- **Reserve keys** (`OLLAMA_API_KEY_II`, `Z_API_KEY_II`) are used only when the user says so.
- **Paid use** (adding credit, a card check, a purchase) is the user's job.
- Only synthetic prompts go to cloud models.

## Working

| Provider | Kind | Base URL | Key | Models that answer | Limits |
|---|---|---|---|---|---|
| Gemini | `gemini` | (native) | `GEMINI_API_KEY` | `gemini-3.5-flash-lite` (reference), `gemma-4-31b-it`, `gemma-4-26b-a4b-it` | Free tier, about 1,500 requests a day, reset at midnight Pacific. Google may use free-tier prompts. Used up on 2026-10-26. |
| Ollama Cloud | `ollama` | `https://ollama.com/` | `OLLAMA_API_KEY` | `gemma4:31b` (reference), `gpt-oss:120b`; the account lists more (below) | Monthly credits; the free plan runs 1 request at a time. |
| Groq | `openai_compatible` | `https://api.groq.com/openai/v1/` | `GROQ_API_KEY` | `openai/gpt-oss-120b`; also lists `openai/gpt-oss-20b`, `qwen/qwen3.8-27b` | Free tier; rate limits per model. |
| Mistral | `openai_compatible` | `https://api.mistral.ai/v1/` | `MISTRAL_API_KEY` | `ministral-14b-latest`, `ministral-8b-latest`, `ministral-3b-latest`, `codestral-latest` | Limited tier. Requests a minute: 3b 750, 8b 188, 14b 30, codestral 125. `mistral-small/medium` and `magistral` get 429 (0 a minute); `mistral-large` gets 403. |
| OpenRouter | `openai_compatible` | `https://openrouter.ai/api/v1/` | `OPEN_ROUTER_API_KEY` | Free models (`:free`): `nvidia/nemotron-3-ultra-550b-a55b:free` answered | Free tier, no credits: 50 free-model requests a day. See below. |

**Ollama Cloud's list** (2026-10-26; only `gemma4:31b` and `gpt-oss:120b` checked): `beam`, `clef:27b`, `deepseek-v4-pro:0813`, `deepseek-v4.1-flash`, `gemma4:31b`, `glm-5.2`, `glm-5.3`, `glm-5.3-flash`, `gpt-oss:120b`, `gpt-oss:20b`, `kimi-k2.6`, `kimi-k2.7-code`, `kimi-k3`, `minimax-m2.7`, `minimax-m3`, `minimax-m3.1-flash`, `mistral-large-3:675b`, `mistral-large-4`, `muse-spark-open`, `nemotron-3-nano:30b`, `nemotron-3-super`, `nemotron-3-ultra`.

**OpenRouter, checked 2026-10-26:**
- 17 free models, all with tools except `nemotron-3.5-content-safety`: `cohere/north-mini-code`, `dots-studio/dots-3-note-preview`, `google/gemma-4-26b-a4b-it`, `google/gemma-4-31b-it`, `liquid/lfm-2.5-2.6b`, `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning`, `nvidia/nemotron-3-super-120b-a12b`, `nvidia/nemotron-3-ultra-550b-a55b`, `nvidia/nemotron-3.5-lightning`, `nvidia/nemotron-3.5-super-vl`, `poolside/laguna-s-2.1`, `poolside/laguna-xs-2.1`, `recursive/flint-v1.0`, `thinkingmachines/inkling-small`, `thinkingmachines/inkling`, `xingchen-agi/xing4.0-29b-a4b` (each with `:free`).
- Tried: `nemotron-3-ultra-550b-a55b:free` answered. `gemma-4-31b-it:free` gave 429 from its provider (busy). `nemotron-3-super-120b-a12b:free` gave 404 from Nvidia. `inkling:free` gave 403: only for apps listed on OpenRouter.
- `openai/gpt-oss-120b` (paid, from $0.03 per million input tokens) answered a tool call with 0 credits, and the account still showed 0 usage. Don't rely on it: paid models need credits.

## Not working now

| Provider | Key | State |
|---|---|---|
| Z.ai | `Z_API_KEY` | `glm-4.5-flash` gave "Insufficient balance or no resource package" on 2026-10-26. `Z_API_KEY_II` (reserve) not checked. |
| Cloudflare Workers AI | `CLOUDFLARE_ID`, `CLOUDFLARE_TOKEN` | Works, but paused for app development: the free limit is 10,000 neurons a day. |
| GMI Cloud | `GMI_API_KEY` | `https://api.gmi-serving.com/v1/`. 402 until the account has a $10 balance (2026-10-09). |
| Experiential Labs | `XPL_API_KEY` | `https://api.experientiallabs.ai/v1/`. The free tier needs a $1 card check; `claude-fable-5.1` needs a purchase (2026-10-09). |
| Gemini, paid | `GOOGLE_GENERATIVE_AI_API_KEY` | Commented out in `.env`. |
