# SPIKE-012 results

Go go1.26.0 windows/amd64, CGO_ENABLED=0. A = Anthropic Messages mock, O = OpenAI Chat Completions mock. Ollama: ran. Total run time 777 s.

## Options

Versions and release dates from `proxy.golang.org/<module>/@latest` on 2026-09-28; licenses from the LICENSE file in the module zip.

| Option | Modules | Version | Released | License |
|---|---|---|---|---|
| official SDKs | anthropic-sdk-go + openai-go/v3 | v1.75.0 + v3.66.0 | 2026-09-22 + 2026-09-23 | MIT + Apache-2.0 |
| genkit | github.com/firebase/genkit/go | v1.13.1 | 2026-09-03 | Apache-2.0 |
| eino | cloudwego/eino + eino-ext claude, openai | v0.9.21 + claude v0.1.26, openai v0.1.13 | 2026-09-23 + 2026-09-24, 2026-04-16 | Apache-2.0 |
| any-llm-go | github.com/mozilla-ai/any-llm-go | v0.9.0 | 2026-03-09 | Apache-2.0 |
| goai | github.com/zendev-sh/goai | v0.10.4 | 2026-09-22 | MIT |

Screened out, not run: `github.com/tmc/langchaingo` v0.1.14, released 2025-10-20 (no release for 11 months), MIT.

## Summary

| Test | official SDKs | genkit | eino | any-llm-go | goai |
|---|---|---|---|---|---|
| T1 stream + 2 parallel tools (A) | pass | partial | pass | partial | pass |
| T2 image in tool result (A) | pass | pass | pass | fail | fail |
| T3 cache points + usage (A) | pass | fail | pass | fail | pass |
| T4 thinking + signature (A) | pass | pass | pass | fail | pass |
| T5 429/529/500 (A) | pass | pass | partial | partial | pass |
| T6 cancel mid-stream (A) | pass | pass | pass | pass | fail |
| T7 truncated/malformed (A) | pass | pass | fail | fail | fail |
| T1 stream + 2 parallel tools (O) | pass | pass | pass | pass | pass |
| T2 image in tool result (O) | pass | fail | fail | pass | pass |
| T3 cached_tokens usage (O) | pass | pass | pass | fail | pass |
| T5 429/529/500 (O) | pass | pass | partial | partial | pass |
| T6 cancel mid-stream (O) | pass | pass | pass | pass | fail |
| T7 truncated/malformed (O) | pass | fail | fail | fail | fail |
| T8 Ollama qwen3.5:4b round trip | pass | pass | pass | pass | pass |
| T9 modules linked / binary MB | 14 / 26.3 | 30 / 33.8 | 82 / 46.8 | 8 / 14.6 | 1 / 9.4 |

Key criterion: in all runs above, a library ran a tool on its own 0 times. (T1 also checks that no tool ran before the assistant message was written.)

genkit.Generate without WithReturnToolRequests(true): genkit called the Go tool functions 2 times itself and sent 2 requests (err `<nil>`).

## Notes per test

### T1 stream + 2 parallel tools (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 5 text, 2 tool-start, 8 arg-delta events |
| genkit | partial | argument deltas: 0 |
| eino | pass | 5 text, 2 tool-start, 6 arg-delta events |
| any-llm-go | partial | the 2 tool_results are split over 2 user messages (the API merges them; docs ask for one) |
| goai | pass | 5 text, 2 tool-start, 6 arg-delta events |

### T2 image in tool result (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | image block inside tool_result |
| genkit | pass | image block inside tool_result |
| eino | pass | image block inside tool_result |
| any-llm-go | fail | image sent, but not as an image block inside tool_result |
| goai | fail | image sent, but not as an image block inside tool_result |

### T3 cache points + usage (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 2 points: end of block 5 and last block, both requests; write/read tokens surfaced |
| genkit | fail | req 1: cache_control at [], want [.messages[2].content[0] .system[4]]; req 2: cache_control at [], want [.messages[4].content[0] .system[4]]; cache write not surfaced (got 0); cache write on turn 2 (got 0) |
| eino | pass | 2 points: end of block 5 and last block, both requests; write/read tokens surfaced |
| any-llm-go | fail | req 1: cache_control at [], want [.messages[2].content[0] .system[0]]; req 2: cache_control at [], want [.messages[4].content[0] .system[0]]; cache write not surfaced (got 0); cache read not surfaced (got 0); cache write on turn 2 (got 0) |
| goai | pass | 2 points: end of block 5 and last block, both requests; write/read tokens surfaced (5 blocks joined into 1 system block) |

### T4 thinking + signature (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | thinking + signature byte-identical, before tool_use |
| genkit | pass | thinking + signature byte-identical, before tool_use |
| eino | pass | thinking + signature byte-identical, before tool_use |
| any-llm-go | fail | thinking config sent: map[budget_tokens:4096 type:enabled]; assistant block 0 = {"id":"toolu_T","input":{"city":"Berlin"},"name":"get_weather","type":"tool_use"}; tool_use not after thinking |
| goai | pass | thinking + signature byte-identical, before tool_use |

### T5 429/529/500 (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 429: 3 tries/2.0s, 529: 3 tries/1.4s, 500: 3 tries/1.3s |
| genkit | pass | 429: 3 tries/2.0s, 529: 3 tries/1.3s, 500: 3 tries/1.3s |
| eino | partial | 429: 3 tries/2.0s, 529: 3 tries/1.2s, 500: 3 tries/1.3s; retries cannot be set to 0 |
| any-llm-go | partial | 429: 3 tries/2.0s, 529: 3 tries/1.4s, 500: 3 tries/1.3s; retries cannot be set to 0 |
| goai | pass | 429: 3 tries/2.0s, 529: 3 tries/7.6s, 500: 3 tries/6.0s |

### T6 cancel mid-stream (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 10 runs: returned within 190 µs, server saw close within 210 µs; err `context canceled` |
| genkit | pass | 10 runs: returned within 335 µs, server saw close within 190 µs; err `context canceled` |
| eino | pass | 10 runs: returned within 245 µs, server saw close within 257 µs; err `context canceled` |
| any-llm-go | pass | 10 runs: returned within 189 µs, server saw close within 237 µs; err `[anthropic] provider_error: context canceled` |
| goai | fail | 10 runs: returned within 385 µs, server saw close within 338 µs; err `context canceled`; nil error (partial message looks complete) in 4 runs; connection left open in 0 runs |

### T7 truncated/malformed (A)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | all 4 variants return an error |
| genkit | pass | all 4 variants return an error |
| eino | fail | EOF without end: silent |
| any-llm-go | fail | EOF without end: silent |
| goai | fail | EOF without end: silent, malformed JSON: silent |

### T1 stream + 2 parallel tools (O)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 5 text, 2 tool-start, 6 arg-delta events |
| genkit | pass | 5 text, 2 tool-start, 6 arg-delta events |
| eino | pass | 5 text, 2 tool-start, 6 arg-delta events |
| any-llm-go | pass | 5 text, 2 tool-start, 6 arg-delta events |
| goai | pass | 5 text, 2 tool-start, 6 arg-delta events |

### T2 image in tool result (O)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | tool text + image in a user message after it (our rule) |
| genkit | fail | image dropped from the tool result |
| eino | fail | image put inside the tool message; the API rejects it (400) |
| any-llm-go | pass | tool text + image in a user message after it (our rule) |
| goai | pass | tool text + image in a user message after it (our rule) |

### T3 cached_tokens usage (O)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | no cache points in this API; cached_tokens=3000 surfaced |
| genkit | pass | no cache points in this API; cached_tokens=3000 surfaced |
| eino | pass | no cache points in this API; cached_tokens=3000 surfaced |
| any-llm-go | fail | cached_tokens not surfaced ([{Input:3100 Output:4 CacheWrite:0 CacheRead:0}]) |
| goai | pass | no cache points in this API; cached_tokens=3000 surfaced |

### T5 429/529/500 (O)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 429: 3 tries/2.0s, 500: 3 tries/1.3s |
| genkit | pass | 429: 3 tries/2.0s, 500: 3 tries/1.4s |
| eino | partial | 429: 1 tries/0.0s, 500: 1 tries/0.0s; status/retry-after not all reachable |
| any-llm-go | partial | 429: 3 tries/2.0s, 500: 3 tries/1.2s; retries cannot be set to 0 |
| goai | pass | 429: 3 tries/2.0s, 500: 3 tries/7.0s |

### T6 cancel mid-stream (O)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 10 runs: returned within 148 µs, server saw close within 163 µs; err `context canceled` |
| genkit | pass | 10 runs: returned within 1.3 ms, server saw close within 196 µs; err `stream error: context canceled` |
| eino | pass | 10 runs: returned within 153 µs, server saw close within 160 µs; err `failed to receive stream chunk: context canceled` |
| any-llm-go | pass | 10 runs: returned within 193 µs, server saw close within 204 µs; err `[local] provider_error: context canceled` |
| goai | fail | 10 runs: returned within 229 µs, server saw close within 211 µs; err `context canceled`; nil error (partial message looks complete) in 4 runs; connection left open in 0 runs |

### T7 truncated/malformed (O)

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | all 4 variants return an error |
| genkit | fail | EOF without end: silent |
| eino | fail | EOF without end: silent |
| any-llm-go | fail | EOF without end: silent |
| goai | fail | EOF without end: silent, malformed JSON: silent |

### T8 Ollama qwen3.5:4b round trip

| Option | Result | Note |
|---|---|---|
| official SDKs | pass | 2 requests, 157 s, args {"city":"Berlin","unit":"C"}, answer "It's 18 C and sunny in Berlin right now." |
| genkit | pass | 2 requests, 165 s, args {"city":"Berlin"}, answer "It's 18°C and sunny in Berlin right now." |
| eino | pass | 2 requests, 119 s, args {"city":"Berlin"}, answer "Berlin is currently 18°C and sunny." |
| any-llm-go | pass | 2 requests, 101 s, args {"city":"Berlin"}, answer "It's currently 18°C and sunny in Berlin." |
| goai | pass | 2 requests, 113 s, args {"city":"Berlin"}, answer "The weather in Berlin right now is 18 C and sunny." |

## T5 errors in detail

Mock always returns the status. "default" = library defaults; "retries=0" = after asking the library for no retries.

| Option | API | Status | Attempts (default) | Time (default) | Attempts (retries=0) | What our code can read | Error text |
|---|---|---|---|---|---|---|---|
| official SDKs | anthropic | 429 | 3 | 2.00 s | 1 | status 429, type "rate_limit_error", retry-after 1s | `POST "http://127.0.0.1:6341/v1/messages": 429 Too Many Requests (Request-ID: req_mock01) {...` |
| official SDKs | anthropic | 529 | 3 | 1.38 s | 1 | status 529, type "overloaded_error" | `POST "http://127.0.0.1:6351/v1/messages": 529  (Request-ID: req_mock01) {"error":{"message...` |
| official SDKs | anthropic | 500 | 3 | 1.34 s | 1 | status 500, type "api_error" | `POST "http://127.0.0.1:6369/v1/messages": 500 Internal Server Error (Request-ID: req_mock0...` |
| genkit | anthropic | 429 | 3 | 2.01 s | 1 | status 429, type "rate_limit_error", retry-after 1s | `POST "http://127.0.0.1:6380/v1/messages": 429 Too Many Requests (Request-ID: req_mock01) {...` |
| genkit | anthropic | 529 | 3 | 1.34 s | 1 | status 529, type "overloaded_error" | `POST "http://127.0.0.1:6386/v1/messages": 529  (Request-ID: req_mock01) {"error":{"message...` |
| genkit | anthropic | 500 | 3 | 1.25 s | 1 | status 500, type "api_error" | `POST "http://127.0.0.1:6392/v1/messages": 500 Internal Server Error (Request-ID: req_mock0...` |
| eino | anthropic | 429 | 3 | 2.00 s | 3 | status 429, type "rate_limit_error", retry-after 1s | `create new streaming message fail: POST "http://127.0.0.1:6398/v1/messages": 429 Too Many ...` |
| eino | anthropic | 529 | 3 | 1.17 s | 3 | status 529, type "overloaded_error" | `create new streaming message fail: POST "http://127.0.0.1:6410/v1/messages": 529  (Request...` |
| eino | anthropic | 500 | 3 | 1.28 s | 3 | status 500, type "api_error" | `create new streaming message fail: POST "http://127.0.0.1:6421/v1/messages": 500 Internal ...` |
| any-llm-go | anthropic | 429 | 3 | 2.00 s | 3 | status 429, type "rate_limit_error", retry-after 1s | `[anthropic] rate_limit: POST "http://127.0.0.1:6429/v1/messages": 429 Too Many Requests (R...` |
| any-llm-go | anthropic | 529 | 3 | 1.42 s | 3 | status 529, type "overloaded_error" | `[anthropic] provider_error: POST "http://127.0.0.1:6444/v1/messages": 529  (Request-ID: re...` |
| any-llm-go | anthropic | 500 | 3 | 1.26 s | 3 | status 500, type "api_error" | `[anthropic] provider_error: POST "http://127.0.0.1:6452/v1/messages": 500 Internal Server ...` |
| goai | anthropic | 429 | 3 | 2.00 s | 1 | status 429, type "rate_limit_error", retry-after 1s | `goai: 2 retries exhausted: mock rate_limit_error` |
| goai | anthropic | 529 | 3 | 7.57 s | 1 | status 529, type "overloaded_error" | `goai: 2 retries exhausted: mock overloaded_error` |
| goai | anthropic | 500 | 3 | 6.00 s | 1 | status 500, type "api_error" | `goai: 2 retries exhausted: mock api_error` |
| official SDKs | openai | 429 | 3 | 2.00 s | 1 | status 429, type "rate_limit_exceeded", retry-after 1s | `POST "http://127.0.0.1:4511/v1/chat/completions": 429 Too Many Requests {"code":"rate_limi...` |
| official SDKs | openai | 500 | 3 | 1.29 s | 1 | status 500, type "server_error" | `POST "http://127.0.0.1:4517/v1/chat/completions": 500 Internal Server Error {"code":"serve...` |
| genkit | openai | 429 | 3 | 2.01 s | 1 | status 429, type "rate_limit_exceeded", retry-after 1s | `stream error: POST "http://127.0.0.1:4523/v1/chat/completions": 429 Too Many Requests {"co...` |
| genkit | openai | 500 | 3 | 1.44 s | 1 | status 500, type "server_error" | `stream error: POST "http://127.0.0.1:4532/v1/chat/completions": 500 Internal Server Error ...` |
| eino | openai | 429 | 1 | 0.00 s | 1 | status 429, type "rate_limit_exceeded", retry-after not reachable | `error, status code: 429, status: 429 Too Many Requests, message: mock rate_limit_exceeded` |
| eino | openai | 500 | 1 | 0.00 s | 1 | status 500, type "server_error" | `error, status code: 500, status: 500 Internal Server Error, message: mock server_error` |
| any-llm-go | openai | 429 | 3 | 2.00 s | 3 | status 429, type "rate_limit_exceeded", retry-after 1s | `[local] rate_limit: POST "http://127.0.0.1:4546/v1/chat/completions": 429 Too Many Request...` |
| any-llm-go | openai | 500 | 3 | 1.25 s | 3 | status 500, type "server_error" | `[local] provider_error: POST "http://127.0.0.1:4570/v1/chat/completions": 500 Internal Ser...` |
| goai | openai | 429 | 3 | 2.00 s | 1 | status 429, type "rate_limit_exceeded", retry-after 1s | `goai: 2 retries exhausted: mock rate_limit_exceeded` |
| goai | openai | 500 | 3 | 7.00 s | 1 | status 500, type "server_error" | `goai: 2 retries exhausted: mock server_error` |

## T7 truncated and malformed streams in detail

| Option | API | Variant | Result |
|---|---|---|---|
| official SDKs | anthropic | EOF without end | error after 1.7 ms: `stream ended before message_stop / finish_reason` |
| official SDKs | anthropic | connection reset | error after 1.2 ms: `unexpected EOF` |
| official SDKs | anthropic | malformed JSON | error after 1.2 ms: `unexpected end of JSON input` |
| official SDKs | anthropic | error event | error after 914 µs: `POST "http://127.0.0.1:4443/v1/messages": 200 OK {"error":{"message":"Overloaded","type":"...` |
| official SDKs without our end check | anthropic | EOF without end | no error, text "" |
| genkit | anthropic | EOF without end | error after 5.5 ms: `anthropic stream ended without a message_stop event` |
| genkit | anthropic | connection reset | error after 4.2 ms: `unexpected EOF` |
| genkit | anthropic | malformed JSON | error after 4.3 ms: `unexpected end of JSON input` |
| genkit | anthropic | error event | error after 4.8 ms: `POST "http://127.0.0.1:4453/v1/messages": 200 OK {"error":{"message":"Overloaded","type":"...` |
| eino | anthropic | EOF without end | no error, text "Hel" |
| eino | anthropic | connection reset | error after 873 µs: `unexpected EOF` |
| eino | anthropic | malformed JSON | error after 1.1 ms: `unexpected end of JSON input` |
| eino | anthropic | error event | error after 900 µs: `POST "http://127.0.0.1:4461/v1/messages": 200 OK {"error":{"message":"Overloaded","type":"...` |
| any-llm-go | anthropic | EOF without end | no error, text "Hel" |
| any-llm-go | anthropic | connection reset | error after 2.0 ms: `[anthropic] provider_error: unexpected EOF` |
| any-llm-go | anthropic | malformed JSON | error after 1.2 ms: `[anthropic] provider_error: unexpected end of JSON input` |
| any-llm-go | anthropic | error event | error after 887 µs: `[anthropic] provider_error: POST "http://127.0.0.1:4469/v1/messages": 200 OK {"error":{"me...` |
| goai | anthropic | EOF without end | no error, text "Hel" |
| goai | anthropic | connection reset | error after 940 µs: `reading stream: unexpected EOF` |
| goai | anthropic | malformed JSON | no error, text "Hel" |
| goai | anthropic | error event | error after 1.0 ms: `Overloaded` |
| official SDKs | openai | EOF without end | error after 971 µs: `stream ended before message_stop / finish_reason` |
| official SDKs | openai | connection reset | error after 862 µs: `unexpected EOF` |
| official SDKs | openai | malformed JSON | error after 1.1 ms: `unexpected end of JSON input` |
| official SDKs | openai | error event | error after 863 µs: `received error while streaming: {"code":null,"message":"The server is overloaded","param":...` |
| official SDKs without our end check | openai | EOF without end | no error, text "Hel" |
| genkit | openai | EOF without end | no error, text "Hel" |
| genkit | openai | connection reset | error after 2.6 ms: `stream error: unexpected EOF` |
| genkit | openai | malformed JSON | error after 2.6 ms: `stream error: unexpected end of JSON input` |
| genkit | openai | error event | error after 3.5 ms: `stream error: received error while streaming: {"code":null,"message":"The server is overlo...` |
| eino | openai | EOF without end | no error, text "Hel" |
| eino | openai | connection reset | error after 1.2 ms: `failed to receive stream chunk: unexpected EOF` |
| eino | openai | malformed JSON | error after 1.1 ms: `failed to receive stream chunk: unexpected end of JSON input` |
| eino | openai | error event | error after 706 µs: `failed to receive stream chunk: error, The server is overloaded` |
| any-llm-go | openai | EOF without end | no error, text "Hel" |
| any-llm-go | openai | connection reset | error after 736 µs: `[local] provider_error: unexpected EOF` |
| any-llm-go | openai | malformed JSON | error after 826 µs: `[local] provider_error: unexpected end of JSON input` |
| any-llm-go | openai | error event | error after 788 µs: `[local] provider_error: received error while streaming: {"code":null,"message":"The server...` |
| goai | openai | EOF without end | no error, text "Hel" |
| goai | openai | connection reset | error after 590 µs: `reading stream: unexpected EOF` |
| goai | openai | malformed JSON | no error, text "Hel" |
| goai | openai | error event | error after 1.3 ms: `The server is overloaded` |

## T9 dependency weight

Each program in `weight/` streams one request through the Anthropic and the OpenAI-compatible provider of that option. `go build` with default flags, CGO_ENABLED=0.

| Option | Modules in `go list -m all` | Edges in `go mod graph` | Modules linked (`go version -m`) | Binary (MB) | Builds with CGO_ENABLED=0 | Official SDKs in its own graph |
|---|---|---|---|---|---|---|
| (baseline: net/http only) | 0 | 2 | 0 | 8.4 | yes |  |
| official SDKs | 76 | 140 | 14 | 26.3 | yes | anthropics/anthropic-sdk-go v1.75.0, openai/openai-go/v3 v3.66.0 |
| genkit | 163 | 335 | 30 | 33.8 | yes | anthropics/anthropic-sdk-go v1.23.0, openai/openai-go v1.8.2 |
| eino | 158 | 929 | 82 | 46.8 | yes | anthropics/anthropic-sdk-go v1.56.0 |
| any-llm-go | 76 | 130 | 8 | 14.6 | yes | anthropics/anthropic-sdk-go v1.26.0, openai/openai-go v1.12.0 |
| goai | 4 | 7 | 1 | 9.4 | yes |  |
