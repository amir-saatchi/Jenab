# SPIKE-017 — Local model: Qwen 3.5 4B on Ollama
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can Jenab be developed and tested against a local model, `qwen3.5:4b` on Ollama, on the dev laptop (i5-1235U, 16 GB RAM, Intel Iris Xe, no dedicated GPU)?

1. Context: the model's context length, Ollama's default context size, and what happens when a prompt is longer (is the start silently cut off?)
2. Speed at realistic Jenab context sizes (2k to 32k tokens): time to first token and tokens per second, with and without thinking
3. Whether Ollama reuses the unchanged prompt prefix between turns (SPEC 3.1), and how much time that saves
4. Tool calling with Jenab's real tool list (SPEC 8.1): right tool, valid arguments, multi-step turns
5. Memory use

## Done when
There is a clear answer for each point, and a rule for how Jenab talks to Ollama (context size, thinking, timeouts).

## Result
Run on 2026-09-28 with Ollama 0.21.0, through `openai-go/v3` on `/v1`. Code and full output: `spikes/017-local-model-ollama/` (`results.md`). During the 59-minute run the laptop was at 100% CPU and paging, with 0.6–1.7 GiB of RAM free, so the timings are pessimistic.

**Model:** `qwen3.5:4b` has 4.7B parameters (Q4_K_M) and a 262,144-token context. It supports tools, thinking and vision.

**Context**
- **Default context:** a plain `/v1` request gets a **4,096-token** context and 2 CPU threads. The tray app's own server starts with 32,768, so the window depends on how Ollama was started.
- **Setting it per request doesn't work:** `/v1` ignores `num_ctx` and `options`.
  - A derived model with its own `num_ctx` does work. It shares the weights and takes 0.1 s to create.
  - The alternative is the native `/api/chat`.
- **Silent truncation:** an ~8k Jenab-shaped prompt was cut to 2,622 tokens with no error, and `finish_reason` was `stop`.
  - The early history was dropped: the system prompt survived, but the first history message was lost.
  - Only the low `prompt_tokens` count shows that it happened.

**Speed, thinking off, 10 threads**

| Prompt | Time to first token | Prompt tokens/s | Output tokens/s | Total |
|---|---|---|---|---|
| ~1,750 tokens | 82 s | 21 | 4.0 | 85 s |
| ~8k tokens | — | — | — | over 6 min (timed out) |
| 16k / 32k | not run | | | about 13 / 27 min at this speed |

- **Threads:** 2 threads (the default) gave 13.7 prompt tokens/s, 10 gave 23.0. 12 threads cut output to 2.2 tokens/s.

**Settings over `/v1`**
- **Thinking:** only `reasoning_effort: "none"` turns it off. `think: false` and `/no_think` do not.
- **Output limit:** `max_completion_tokens` is ignored; only `max_tokens` works.
- **Keep-alive:** `keep_alive` is ignored.

**Prefix reuse**
- It works up to the last 512-token block before the first changed token.
  - In the tool tests, 2,560 of about 2,668 tokens were reused. The time to first token fell from 138 s to about 10–35 s.
- It is lost when:
  - the system prompt's start changes
  - another chat runs in between (Ollama keeps one slot)
  - the context size changes
- **Parallel requests** queue: the second one waited 91 s.

**Tool calling** (all 24 SPEC 8.1 tools, about 2,240 tokens, 15 tasks)

| Thinking | Tasks passed | Arguments valid by schema |
|---|---|---|
| off | 14 / 15 | 15 / 15 |
| on | 12 / 15 | 15 / 15 |

- **Thinking off:** multi-step and parallel calls were right. The one miss was "add my trade", which called `describe_table` instead of `insert`.
- **Memory:** Ollama estimates 5.4 GiB at a 4k context and 6.5 GiB at 36k. With other apps open, free RAM fell to 0.6 GiB.
- **Not tested:**
  - 16k and 32k contexts
  - the Iris Xe GPU through Vulkan
  - vision
  - a quiet machine

## Decision
**Decided (2026-09-28):**
- **What the local model is for:** on this laptop, `qwen3.5:4b` is only a **tool-loop test model with short prompts (1–3k tokens)**. It is not usable for realistic Jenab contexts.
- **Default development model:** a cloud model (SPIKE-018).
- **Rules for talking to Ollama** go into SPEC 3.8:
  - a derived model with `num_ctx` and `num_thread` set, or the native API
  - `max_tokens`, not `max_completion_tokens`
  - `reasoning_effort: "none"` when thinking is not wanted
  - one request at a time
  - timeouts in minutes
- **Rule for every provider:** check the reported `prompt_tokens` against Jenab's own estimate, and treat a large shortfall as a truncation error.
