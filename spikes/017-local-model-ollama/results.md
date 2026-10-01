# SPIKE-017 results

Ollama 0.21.0, model `qwen3.5:4b`, openai-go v3.66.0, Go go1.26.0, 2026-09-28 12:56. Intel64 Family 6 Model 154 Stepping 4, GenuineIntel, 12 logical CPUs, 15.7 GiB RAM (6.7 GiB available at start).

Other programs were running on the laptop (editors, browsers, other agents); timings include that contention.

## 1. Model facts

| field | value |
|---|---|
| architecture | qwen35 (32 layers; every 4th layer is full attention, the others are linear-attention/SSM layers) |
| parameters | 4.7B (4.659865088e+09) |
| quantization | Q4_K_M |
| model context length | 262144 tokens |
| capabilities | completion, vision, tools, thinking |
| default parameters | presence_penalty 1.5 temperature 1 top_k 20 top_p 0.95 |

**Default context.** A plain /v1 request loads `qwen3.5:4b` with a context of **4096 tokens** (`/api/ps` context_length; server log: KvSize 4096, NumThreads 2). Ollama picks this default from VRAM; this laptop has none.

The Ollama tray app starts its own server with `OLLAMA_CONTEXT_LENGTH=32768 (server.log)`; its last runner used KvSize 32768. So the context a /v1 request gets depends on how the server was started, not on the request.

**Derived models** (`POST /api/create` with `{"model": "burrow-qwen3.5-4b-12k", "from": "qwen3.5:4b", "parameters": {"num_ctx": 12288, "num_thread": 10}}`, same for burrow-qwen3.5-4b-20k and burrow-qwen3.5-4b-36k). They share the weights blob, so creating one takes about a second and no disk space.

## 2. Thinking and other settings over /v1

Prompt "Say hi in three words.", max_tokens 60, base model.

| how | reasoning streamed | answer | completion tokens | finish |
|---|---|---|---|---|
| nothing set (model default) | yes (204 chars) | "" | 60 | length |
| `reasoning_effort: "none"` | no | "Greetings from you" | 4 | stop |
| `reasoning_effort: "low"` | yes (208 chars) | "" | 60 | length |
| extra field `think: false` | yes (199 chars) | "" | 60 | length |
| `/no_think` at the end of the user message | yes (228 chars) | "" | 60 | length |
| `/no_think` in the system prompt | yes (189 chars) | "" | 60 | length |
| native `/api/chat` with `think: false` | no | "Hello, how are you?" | 7 | stop |

| extra /v1 field | effect |
|---|---|
| `max_completion_tokens: 20` (what openai-go's MaxCompletionTokens sends) | 190 completion tokens, finish stop |
| `max_tokens: 20` (openai-go's deprecated MaxTokens) | 20 completion tokens, finish length |
| `options: {num_ctx: 8192}` and `num_ctx: 8192` | loaded context after the request: 4096 (err <nil>) |
| `keep_alive: "30m"` | model expires in 5 min after the request (server default is 5 min) (err <nil>) |

## 3. Default context and truncation

Both requests go to the base model over /v1 with the default context (4,096 unless the server sets another), thinking off.

| prompt | built for (estimate) | prompt_tokens reported | TTFT | answer | code word | ticket | server log |
|---|---|---|---|---|---|---|---|
| Burrow-shaped, ~8k: code word at the start of the system prompt, ticket in the first history message | ~8000 | 2622 | 240.9 s | "PELICAN-47, trunc." | yes | no | `truncating input messages which exceed context length" truncated=2` |
| one ~12k system message, code word only at its start | ~12000 | – | – | error: context deadline exceeded | | | `truncating input messages which exceed context length" truncated=1` |

The response has `finish_reason: stop` and no error or warning field; only prompt_tokens being lower than the prompt shows the cut. The same ~8k prompt on burrow-qwen3.5-4b-12k (num_ctx 12288) is the 8k row of section 4.

## 3b. CPU threads

Native /api/chat, base model, a ~700-token prompt, `think: false`, num_predict 16; each num_thread value reloads the model, so nothing is cached. Timings are Ollama's own (prompt_eval_duration, eval_duration).

| num_thread | NumThreads in log | load | prompt tokens | prompt tok/s | output tok/s |
|---|---|---|---|---|---|
| not set (Ollama default) | 2 | 5.3 s | 517 | 13.7 | 5.1 |
| 4 | 4 | 5.3 s | 517 | 16.3 | 5.3 |
| 8 | 8 | 5.2 s | 517 | 21.1 | 5.3 |
| 10 | 10 | 5.0 s | 517 | 23.0 | 5.5 |
| 12 | 12 | 5.3 s | 517 | 23.7 | 2.2 |

## 4. Speed

Burrow-shaped requests (SPEC 3.1: system prompt, user and project memory, project card, session notes, history turns with query/page previews of up to 1,500 tokens, current turn). No tool list in these requests (see section 6 for its size). Streaming over /v1. Each thinking-off request starts with a new request id, so nothing is reused. max_tokens = 200 with thinking off, 400 with thinking on (thinking uses the same budget).

- TTFT = time to the first streamed token (reasoning or answer). With thinking off this is almost all prompt processing, so prompt tok/s = prompt tokens / TTFT.
- The thinking-on request repeats the same prompt right after the thinking-off one, so its TTFT shows a reused prefix (section 5), not a cold start.
- "code / ticket" = the answer contains the code word (start of the system prompt) / the ticket number (first history message).

| size | model (num_ctx) | thinking | prompt tokens | TTFT | prompt tok/s | output tokens | output tok/s | first answer token | total | code / ticket | finish |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 2k | burrow-qwen3.5-4b-12k | off | 1750 | 81.6 s | 21.4 | 14 | 4.0 | 81.6 s | 85.2 s | yes / yes | stop |
| 2k | burrow-qwen3.5-4b-12k | on | 1748 | 57.5 s | 30.4 | 388 | 3.7 | 157.5 s | 162.0 s | yes / yes | stop |
| 8k | burrow-qwen3.5-4b-12k | off | – | – | – | – | – | – | 360.3 s | – | context deadline exceeded |
| 16k | burrow-qwen3.5-4b-20k | off | – | – | – | – | – | – | – | – | not run: at 20 tok/s it needs about 13 min |
| 32k | burrow-qwen3.5-4b-36k | off | – | – | – | – | – | – | – | – | not run: at 20 tok/s it needs about 27 min |

Thinking on was only run up to 8k: prompt processing is the same, and the time budget does not allow it at 16k.

## 5. Prefix reuse between turns

A ~1.3k-token Burrow-shaped chat on burrow-qwen3.5-4b-12k, thinking off, max 60 tokens. Turn 2 = turn 1 + the assistant answer + a short user message, as SPEC 3.1 intends. "cache slot" is Ollama's debug log line `loading cache slot … used=N` (tokens taken from the cache).

| step | prompt tokens | TTFT | cache slot (server log) | note |
|---|---|---|---|---|
| A turn 1 (cold) | 999 | 49.1 s | used 0 of 999 |  |
| A turn 2 (turn 1 + answer + short message) | 1080 | 28.1 s | used 512 of 1080 |  |
| A turn 2 again (identical request) | 1080 | 28.1 s | used 512 of 1080 |  |
| A turn 2 with a changed first line of the system prompt | 1084 | 53.7 s | used 0 of 1084 |  |
| B turn 1 (another chat, in between) | 999 | 50.3 s | used 0 of 999 |  |
| A turn 3 (after B) | 1102 | 55.6 s | used 0 of 1102 |  |
| A turn 4 and B turn 2 sent together: A | 1127 | 28.7 s | used 512 of 1127; used 0 of 1031 | total 43.6 s |
| … B (sent 0.2 s later) | 1031 | 91.1 s | used 512 of 1127; used 0 of 1031 | total 95.2 s; OLLAMA_NUM_PARALLEL=1, so B waits for A |
| A turn 5 with `keep_alive: "5s"` | 1199 | 55.2 s | used 0 of 1199 |  |
| A turn 6 after the model unloaded | 1242 | 31.7 s | used 512 of 1242 | model unloaded 90 s after turn 5: false; TTFT includes the reload |
| A turn 7 on burrow-qwen3.5-4b-20k (other num_ctx) | 1254 | 64.3 s | used 0 of 1254 | different num_ctx → model reload, TTFT includes it |

## 6. Tool calling

All 24 tools of SPEC 8.1 with JSON schemas, on burrow-qwen3.5-4b-12k over /v1 (streaming). Arguments are checked with santhosh-tekuri/jsonschema/v6 against the tool's schema, then against what the task needs (table name, values, enum choice, revision). Single-tool tasks check the first response only; multi-step tasks run the loop with simulated tool results up to the final answer.

System prompt + one user message: 422 prompt tokens without tools (TTFT 19.8 s).

### thinking off (`reasoning_effort: none`), max_tokens 400

| task | kind | prompt | pass | right tool | args valid (schema) | args right | requests | time | first TTFT | prompt tokens | completion tokens | notes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| T01 | single | What columns does the trades table have? | yes | yes | yes | yes | 1 | 138.0 s | 138.0 s | 2666 | 27 |  |
| T02 | single | How many ETH rows are in the prices table? | yes | yes | yes | yes | 1 | 15.7 s | 15.3 s | 2668 | 35 |  |
| T03 | single | Add my trade: I bought 0.1 BTC on 2026-09-01 at 57… | no | no | yes | yes | 1 | 16.9 s | 16.7 s | 2691 | 27 | called describe_table |
| T04 | single | Search the web for news about Ethereum ETF flows f… | yes | yes | yes | yes | 1 | 27.0 s | 26.9 s | 2672 | 57 |  |
| T05 | single | Please remember this for all my projects: I want p… | yes | yes | yes | yes | 1 | 35.5 s | 35.3 s | 2673 | 68 |  |
| T06 | single | Run the daily_btc pipeline now. | yes | yes | yes | yes | 1 | 21.8 s | 21.5 s | 2666 | 28 |  |
| T07 | single | I also want to track the daily trading volume in t… | yes | yes | yes | yes | 1 | 23.5 s | 23.5 s | 2678 | 39 |  |
| T08 | single | The page preview stopped early. Read more of cache… | yes | yes | yes | yes | 1 | 31.4 s | 31.4 s | 2688 | 51 |  |
| T09 | single | Did we talk about exchange fees in any other chat … | yes | yes | yes | yes | 1 | 25.0 s | 24.9 s | 2672 | 38 |  |
| T10 | no tool | What is 17 times 23? | yes | yes | yes | yes | 1 | 14.3 s | 9.7 s | 2668 | 13 |  |
| T11 | no tool | Thanks, that's all for today! | yes | yes | yes | yes | 1 | 15.8 s | 9.6 s | 2666 | 13 |  |
| T12 | multi-step | How many BTC trades did I make in September 2026? … | yes | yes | yes | yes | 2 | 53.2 s | 15.1 s | 2685 | 110 |  |
| T13 | multi-step | Start the daily_btc pipeline and tell me whether t… | yes | yes | yes | yes | 3 | 49.7 s | 12.9 s | 2672 | 79 |  |
| T14 | parallel | Describe both the prices table and the trades tabl… | yes | yes | yes | yes | 1 | 20.3 s | 13.0 s | 2675 | 53 | 2 call(s) in one response |
| T15 | parallel | List the bucket objects under reports/ and under i… | yes | yes | yes | yes | 1 | 21.1 s | 13.5 s | 2672 | 56 | 2 call(s) in one response |

**thinking off (`reasoning_effort: none`):** pass 14/15, right tool (or rightly no tool) 14/15, schema-valid args 15/15, right args 15/15, 8.5 min.

### thinking on (model default), max_tokens 800

| task | kind | prompt | pass | right tool | args valid (schema) | args right | requests | time | first TTFT | prompt tokens | completion tokens | notes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| T01 | single | What columns does the trades table have? | yes | yes | yes | yes | 1 | 19.4 s | 6.0 s | 2664 | 56 |  |
| T10 | no tool | What is 17 times 23? | yes | yes | yes | yes | 1 | 36.8 s | 5.9 s | 2666 | 116 |  |
| T12 | multi-step | How many BTC trades did I make in September 2026? … | yes | yes | yes | yes | 2 | 89.9 s | 6.9 s | 2683 | 253 |  |
| T14 | parallel | Describe both the prices table and the trades tabl… | yes | yes | yes | yes | 1 | 37.5 s | 7.1 s | 2673 | 110 | 2 call(s) in one response |
| T05 | single | Please remember this for all my projects: I want p… | yes | yes | yes | yes | 1 | 93.0 s | 6.9 s | 2671 | 298 |  |
| T03 | single | Add my trade: I bought 0.1 BTC on 2026-09-01 at 57… | no | no | yes | yes | 1 | 40.8 s | 8.2 s | 2689 | 125 | called describe_table |
| T13 | multi-step | Start the daily_btc pipeline and tell me whether t… | no | no | yes | no | 2 | 50.2 s | 6.5 s | 2670 | 127 | called [run_pipeline]; no final answer |
| T11 | no tool | Thanks, that's all for today! | yes | yes | yes | yes | 1 | 24.6 s | 6.1 s | 2664 | 71 |  |
| T02 | single | How many ETH rows are in the prices table? | yes | yes | yes | yes | 1 | 29.5 s | 6.2 s | 2666 | 90 |  |
| T04 | single | Search the web for news about Ethereum ETF flows f… | yes | yes | yes | yes | 1 | 44.1 s | 6.4 s | 2670 | 142 |  |
| T06 | single | Run the daily_btc pipeline now. | yes | yes | yes | yes | 1 | 23.8 s | 6.2 s | 2664 | 68 |  |
| T07 | single | I also want to track the daily trading volume in t… | no | no | yes | yes | 1 | 43.2 s | 6.8 s | 2676 | 137 | called describe_table |
| T08 | single | The page preview stopped early. Read more of cache… | yes | yes | yes | yes | 1 | 175.7 s | 8.1 s | 2686 | 553 |  |
| T09 | single | Did we talk about exchange fees in any other chat … | yes | yes | yes | yes | 1 | 42.1 s | 7.4 s | 2670 | 114 |  |
| T15 | parallel | List the bucket objects under reports/ and under i… | yes | yes | yes | yes | 1 | 38.0 s | 7.6 s | 2670 | 96 | 2 call(s) in one response |

**thinking on (model default):** pass 12/15, right tool (or rightly no tool) 12/15, schema-valid args 15/15, right args 14/15, 13.1 min.

| mode | pass | right tool | schema-valid args | right args | time |
|---|---|---|---|---|---|
| thinking off (`reasoning_effort: none`) | 14/15 | 14/15 | 15/15 | 15/15 | 8.5 min |
| thinking on (model default) | 12/15 | 12/15 | 15/15 | 14/15 | 13.1 min |

"args valid" and "args right" count as no when no call was made for a task that needs one.

## 7. Memory

Runner working set = the largest ollama.exe process (tasklist). /api/ps size is Ollama's own estimate. Ollama log values are from the runner's load lines.

| when | num_ctx | /api/ps size | runner working set | system available | KV cache (log) | total (log) |
|---|---|---|---|---|---|---|
| base model, default context | 4096 | 5.4 GiB | 5.1 GiB | 1.7 GiB | 1.4 GiB | 5.4 GiB |
| after 2k request | 12288 | 5.6 GiB | 3.1 GiB | 1.5 GiB | 1.6 GiB | 5.6 GiB |
| after 8k request | 12288 | 5.6 GiB | 3.1 GiB | 0.7 GiB | – | – |
| burrow-qwen3.5-4b-20k loaded, no request | 20480 | 5.9 GiB | 4.5 GiB | 0.8 GiB | 1.9 GiB | 5.9 GiB |
| burrow-qwen3.5-4b-36k loaded, no request | 36864 | 6.5 GiB | 6.3 GiB | 0.6 GiB | 2.4 GiB | 6.5 GiB |

Total run time: 59.0 min.
