# SPIKE-018 — Cloud models for development: Groq, Ollama Cloud, Gemini and Z.ai

Throwaway code for [SPIKE-018](../../Docs/Tickets/SPIKE-018-cloud-models.md).

```bash
go run . > results.md                  # 11.6 minutes and about 0.82M prompt tokens in the last run (free tiers)
go run . -quick > quick.md             # basic, 3 tool tasks, 2 cache turns, 8k only; about 4 minutes
go run . -provider zai                 # only some providers (groq, ollama, gemini, zai)
go run . -list                         # only list the models each provider reports
go run . -scan                         # only the secret scan of this folder
```

The keys are `GROQ_API_KEY`, `OLLAMA_API_KEY`, `GEMINI_API_KEY` and `Z_API_KEY` in the repo-root `.env` (git-ignored). Only `loadEnv` reads that file; the values stay in memory and go only in the `Authorization` header to the provider's own host (the Gemini `models.get` call uses the documented `x-goog-api-key` header). All output passes through a redacting writer, and at the end the program scans every file in this folder for the key values and prints only `secret scan: clean` or the file names. Only synthetic prompts are sent.

- `env.go`: the `.env` parser, redaction (keys and Groq organization IDs) and the secret scan
- `providers.go`: base URLs, wanted models (Z.ai: the two free Flash models from the pricing page; `open.bigmodel.cn` is tried only if `api.z.ai` answers 401), the openai-go client per provider, a middleware that refuses other hosts, strips headers the SDK takes from `OPENAI_*` environment variables, and records rate-limit headers and error bodies; context limits from `/models`, Gemini `models.get` or Ollama `/api/show`
- `stream.go`: one streamed call with the SPIKE-012 rules (finish_reason required, `ctx.Err()`, valid tool JSON, `include_usage`), SDK retries off, our retry loop (2 retries, `retry-after` capped at 30 s), a pacer per model, and the log of every 429 and error status
- `tools.go`: the 24 SPEC 8.1 tools as JSON schemas (validated with santhosh-tekuri/jsonschema) and the 11 tool tasks
- `context.go`: the synthetic Burrow-shaped request (SPEC 3.1 blocks, a history window of query turns with tool result previews, the current turn)
- `main.go`: the run per model (basic, tools, cache, context), burst tests per provider, Gemini thought-signature check
- `report.go`: the tables in `results.md`
- `results.md`: output of the last run

Providers run in parallel. Groq and Gemini models run in parallel too (their limits are per model); Ollama and Z.ai models run one after another (Ollama's free plan allows 1 concurrent request). A model is stopped after a daily or quota error, a 401/403/404, or 4 failed requests in a row.

The Gemini free tier allowed only 20 requests per day for `gemini-3.8-flash`; the earlier trial runs used them up, so the last full run stops that model at its first tool task. The last full run also lost Gemini to an `unexpected EOF` on `/models`, so `results.md` has two appendices: A is a `-provider gemini` rerun (it hit DNS errors near the end), B is the `-quick -provider gemini` run made before the quota ran out (older code, no transport retries). The Recommendation section at the end of `results.md` was written by hand after the runs, followed by the output of `go run . -scan`.
