# SPIKE-012 — LLM SDKs and the agent loop

Throwaway code for [SPIKE-012](../../Docs/Tickets/SPIKE-012-llm-sdk-agent-loop.md).

```bash
go run . > results.md                 # about 13 minutes; about 11 of them are Ollama on CPU
go run . -ollama=false -weight=false  # mock tests only, about 1.5 minutes
```

Every option sits behind the same small `Provider` interface, and the same agent loop and tests run on each one. The mock servers are local `httptest` servers that send real-format SSE for the Anthropic Messages API and the OpenAI Chat Completions API. No cloud API key is used. T8 needs Ollama at `http://localhost:11434` with `qwen3.5:4b`; it is skipped if Ollama is not there.

- `types.go`: the `Provider` interface Burrow would own (parts, events, usage, `ProviderError`)
- `loop.go`: the agent loop: stream, write the assistant message and its tool_call parts to a fake chats.db, then run tools; also places the two SPEC 3.1 cache points
- `mock.go`: the mock server plus Anthropic and OpenAI SSE writers
- `tests.go`: T1–T7 (tools, images, cache points, thinking, errors and retries, cancel, broken streams)
- `ollama.go`: T8, one real tool-call round trip against Ollama's OpenAI-compatible endpoint
- `weight.go`: T9, runs `go list -m all`, `go mod graph` and `go build` in each `weight/` module
- `opt_sdk.go`: option 1, `anthropic-sdk-go` + `openai-go/v3` behind our interface (all our own code)
- `opt_genkit.go`: option 2, Genkit (`genkit.Generate` with `WithReturnToolRequests(true)`)
- `opt_eino.go`: option 3, Eino ChatModel components (claude, openai), no graph or agent
- `opt_anyllm.go`: option 4, any-llm-go
- `opt_goai.go`: option 5, GoAI (`StreamText` with tools that have no `Execute`)
- `weight/<option>/`: one minimal module per option (plus a net/http baseline) that streams one request per provider
- `clock.go`: high-resolution timer (copied from SPIKE-005); `clockdiff.go`: difference of two stamps
- `results.md`: output of the last run

All frameworks are in one module here, so Go picks the newest shared SDK versions (for example anthropic-sdk-go v1.75.0 for all of them). The `weight/` modules show which SDK versions each framework asks for by itself.
