# Models we can use in development
**Status:** checked 2026-11-03

The models the development keys can reach, for spikes, scenarios and tests. Keys are in `.env` at the repository root ([DEVELOPMENT](DEVELOPMENT.md#keys-for-development)). This file names only the variables, never the values. Spikes list the models they run in their own `models.yaml`; `scenarios/models.example.yaml` shows the format.

When a check here changes (a quota runs out, a new key, a model that stops answering), update this file with the date.

## Rules

- **Reference models** are the ones spikes are decided on ([DEVELOPMENT](DEVELOPMENT.md#reference-models)). Failures on other models are recorded, not worked around.
- **Reserve keys** (`OLLAMA_API_KEY_II`, `OLLAMA_API_KEY_III`, `Z_API_KEY_II`, `GEMINI_API_KEY_III`, `GEMINI_API_KEY_IV`) are used only when the user says so.
- **Paid use** (adding credit, a card check, a purchase) is the user's job.
- Only synthetic prompts go to cloud models.

## Working

| Provider | Kind | Base URL | Key | Models that answer | Limits |
|---|---|---|---|---|---|
| Gemini | `gemini` | (native) | `GEMINI_API_KEY`; reserve `GEMINI_API_KEY_III`, `GEMINI_API_KEY_IV` | With tools (2026-11-03): `gemini-3.5-flash-lite` (reference), `gemini-3.5-flash`, `gemini-3.1-flash-lite`, `gemini-flash-lite-latest`, `gemini-3-flash-preview`, `gemma-4-26b-a4b-it` (TASK-035's desktop task on 2026-11-05: 3/3 with the old card, 63–113 s; with the app field 3/3 "In Notepad" and 2/3 with the editor named by title, where it put the title in `app`; 1–5 min a run, never loads the skill). Busy (503) or timed out: `gemini-3.6-flash`, `gemini-3.7-flash`, `gemini-3.8-flash`, `gemini-flash-latest`. No free quota (429): `gemini-pro-latest`, `gemini-2.5-computer-use-preview-10-2025`. `gemini-2.5-flash` is closed to new users (404). `gemma-4-31b-it` without tools only: HTTP 500 whenever tools are sent (2026-11-01) | Free tier; we keep each key under 400 requests a day. The day resets at midnight Pacific (08:00 UTC in winter). Google may use free-tier prompts. |
| Ollama Cloud | `ollama` | `https://ollama.com/` | `OLLAMA_API_KEY` | `gemma4:31b` (reference), `gpt-oss:120b`; the account lists more (below) | Monthly free usage on older accounts, from the sign-up date; the free plan runs 1 request at a time. New accounts pay per token. The first key and `_II` reached their monthly limit on 2026-11-01 and still answered 429 on 2026-11-05; the first resets about 2026-11-22. `_III` needs credit (402, again on 2026-11-05). |
| Groq | `openai_compatible` | `https://api.groq.com/openai/v1/` | `GROQ_API_KEY` | With tools (2026-11-03): `openai/gpt-oss-120b`, `openai/gpt-oss-20b`, `qwen/qwen3.8-27b` | Free tier; rate limits per model. Cloudflare refuses Python's default User-Agent (403, error 1010); send one. |
| Mistral | `openai_compatible` | `https://api.mistral.ai/v1/` | `MISTRAL_API_KEY` | With tools (2026-11-03): `ministral-14b-latest`, `ministral-8b-latest`, `codestral-latest`, `mistral-large-4` (403 on 2026-10-26, answers now); `ministral-3b-latest` answered on 2026-10-26 | Limited tier. Requests a minute: 3b 750, 8b 188, 14b 30, codestral 125. `mistral-small-latest`, `mistral-medium-latest`, `magistral-small-latest` and `mistral-vibe-cli-with-tools` get 429. |
| OpenRouter | `openai_compatible` | `https://openrouter.ai/api/v1/` | `OPEN_ROUTER_API_KEY` | Free models with tools (2026-11-03): `nvidia/nemotron-3-ultra-550b-a55b:free`, `nvidia/nemotron-3.5-lightning:free`, `nvidia/nemotron-3.5-super-vl:free`, `cohere/north-mini-code:free`, `dots-studio/dots-3-note-preview:free` | Free tier, 0 credits: 50 free-model requests a day. Busy (429): both `gemma-4` and `xing4.0`; `flint-v1.0` 503; `inkling-small` 403 (only for listed agent apps). See below. |

**Ollama Cloud's list** (2026-10-26; only `gemma4:31b` and `gpt-oss:120b` checked): `beam`, `clef:27b`, `deepseek-v4-pro:0813`, `deepseek-v4.1-flash`, `gemma4:31b`, `glm-5.2`, `glm-5.3`, `glm-5.3-flash`, `gpt-oss:120b`, `gpt-oss:20b`, `kimi-k2.6`, `kimi-k2.7-code`, `kimi-k3`, `minimax-m2.7`, `minimax-m3`, `minimax-m3.1-flash`, `mistral-large-3:675b`, `mistral-large-4`, `muse-spark-open`, `nemotron-3-nano:30b`, `nemotron-3-super`, `nemotron-3-ultra`.

**OpenRouter, checked 2026-10-26:**
- 17 free models, all with tools except `nemotron-3.5-content-safety`: `cohere/north-mini-code`, `dots-studio/dots-3-note-preview`, `google/gemma-4-26b-a4b-it`, `google/gemma-4-31b-it`, `liquid/lfm-2.5-2.6b`, `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning`, `nvidia/nemotron-3-super-120b-a12b`, `nvidia/nemotron-3-ultra-550b-a55b`, `nvidia/nemotron-3.5-lightning`, `nvidia/nemotron-3.5-super-vl`, `poolside/laguna-s-2.1`, `poolside/laguna-xs-2.1`, `recursive/flint-v1.0`, `thinkingmachines/inkling-small`, `thinkingmachines/inkling`, `xingchen-agi/xing4.0-29b-a4b` (each with `:free`).
- Tried: `nemotron-3-ultra-550b-a55b:free` answered. `gemma-4-31b-it:free` gave 429 from its provider (busy). `nemotron-3-super-120b-a12b:free` gave 404 from Nvidia. `inkling:free` gave 403: only for apps listed on OpenRouter.
- `openai/gpt-oss-120b` (paid, from $0.03 per million input tokens) answered a tool call with 0 credits, and the account still showed 0 usage. Don't rely on it: paid models need credits.

## Not working now

| Provider | Key | State |
|---|---|---|
| Z.ai | `Z_API_KEY` | Lists `glm-4.5` to `glm-5.3-flashx`, but every model tried (`glm-5.3-flash`, `glm-5.3-flashx`, `glm-4.5-air`) gives 429 "Insufficient balance or no resource package" (2026-11-03). `Z_API_KEY_II` (reserve) not checked. |
| Cloudflare Workers AI | `CLOUDFLARE_ID`, `CLOUDFLARE_TOKEN` | Works, but paused for app development: the free limit is 10,000 neurons a day. |
| GMI Cloud | `GMI_API_KEY` | `https://api.gmi-serving.com/v1/`. Lists about 90 models; chat gives 402 "Insufficient balance" (2026-11-03). Needs a User-Agent, like Groq. |
| Experiential Labs | `XPL_API_KEY` | `https://api.experientiallabs.ai/v1/`. Lists about 300 models; chat gives 402 until the $1 card check is done (2026-11-03). |
| Gemini, paid | `GOOGLE_GENERATIVE_AI_API_KEY` | Commented out in `.env`. |
| Gemini | `GEMINI_API_KEY_II` | 402: its project's prepaid credit is used up, so it has no free tier (2026-11-01). |
