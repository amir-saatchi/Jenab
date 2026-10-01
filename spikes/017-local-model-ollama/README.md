# SPIKE-017 — Local model: Qwen 3.5 4B on Ollama

Throwaway code for [SPIKE-017](../../Docs/Tickets/SPIKE-017-local-model-ollama.md).

```bash
# a server with debug logging, so the reuse table can show Ollama's cache-slot lines
OLLAMA_DEBUG=1 OLLAMA_HOST=127.0.0.1:11434 ollama serve 2> serve.log &
go run . -serverlog serve.log > results.md          # about an hour (59 min in the last run)
go run . -quick -serverlog serve.log > results.md   # fewer sizes and tasks (not timed)
```

Needs Ollama with `qwen3.5:4b` pulled. Without `-serverlog` everything runs, but the cache-slot and truncation columns stay empty. The program creates `burrow-qwen3.5-4b-12k`, `-20k` and `-36k` (derived models with `num_ctx` and `num_thread`, sharing the weights) and deletes them at the end (`-keep` keeps them). Progress goes to stderr.

- `main.go`: the sections in order: model facts and default context, thinking and other settings over /v1, truncation, CPU threads, speed per context size, prefix reuse, tool calling, memory
- `client.go`: the /v1 client with openai-go/v3, used like Burrow's provider layer (streaming, `include_usage`, retries 0, complete streams only); measures time to first token
- `ollama.go`: native `/api/*` calls (show, ps, create, delete, chat), memory sampling (tasklist + GlobalMemoryStatusEx) and server-log parsing
- `prompt.go`: builds Burrow-shaped requests of a given size (SPEC 3.1 order, tool previews as in 3.7), with a code word at the start of the system prompt and a ticket number in the first history message
- `tools.go`: the 24 main agent tools of SPEC 8.1 as JSON schemas, and argument validation with santhosh-tekuri/jsonschema/v6
- `tasks.go`: the 15 tool-calling tasks, simulated tool results and scoring
- `results.md`: output of the last run

Notes:

- Timings are wall-clock on a laptop that was also running editors, browsers and other agents, with little free RAM. Treat them as this laptop's numbers, not the model's.
- On Windows, stopping `ollama serve` leaves its `ollama.exe runner` child running (and holding the model's memory); stop the runner too.
- Sizes that would take longer than the 6-minute request timeout, judged from the rate measured at the previous size, are not run; the table says so.
- Two output fixes were made after the last run and are not in `results.md`: the reuse row "A turn 6" says "TTFT includes the reload", but the model had not unloaded (/v1 ignores `keep_alive`, as section 2 shows); and the truncation table shows only the first truncation log line. The server log of that run also had the runner line `truncating input prompt limit=4096 prompt=7822 keep=4 new=4096` for the one-system-message case.
