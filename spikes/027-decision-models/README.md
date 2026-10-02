# SPIKE-027: decision models

Throwaway code for [SPIKE-027](../../Docs/Tickets/SPIKE-027-decision-models.md). It asks Cloudflare's Clef and Clef-flash typed questions, and asks the same questions to two LLMs for comparison. Go standard library only.

```bash
go run . -plan main -sets weak         # one set on all four models
go run . -plan all                     # everything (resumable: finished calls are skipped)
go run . -plan burst -models clef-flash
go run . -report                       # rewrite the measured part of results.md
```

Steps (`-plan`): `main` (every set, one call per item and model), `repeat` (30 items 4 more times, Clef only), `size`, `cutoff` (how much of the state is read), `counts`, `errors`, `images`, `burst`.

## Keys and safety

- `CLOUDFLARE_ID`, `CLOUDFLARE_TOKEN`, `OLLAMA_API_KEY` and `Z_API_KEY` come from the repo-root `.env`. Only `loadEnv` (`env.go`) reads it, and only those names. Values stay in memory.
- Keys go only in the `Authorization` header, and only to `api.cloudflare.com`, `ollama.com` and `api.z.ai` (`send` refuses other hosts). Cloudflare's account ID has to be in its URL path, so URLs are never printed or written.
- Everything printed or written goes through `redact`.
- Only synthetic text is sent. The `errors` step sends one call with a made-up token to read the 401 body.
- A quota or daily-limit error, or a 429 that outlives 3 tries, stops every later call to that provider. Ollama and Z.ai run one call at a time.

## Files

- `types.go`: questions, answers, records, the answer parsers and scoring
- `client.go`: the Clef and LLM calls, retries, provider stops
- `data_classify.go`: 40 news items in English, German and Persian with their labels; the steer items
- `data_decide.go`: skills (SPIKE-025's requests plus new ones), routing (SPIKE-023's fixture), re-ranking, untrusted text, weak spots
- `scale.go`: state size, question and option counts, error cases, chart images, bursts
- `report.go`: the tables in `results.md`
- `results/`: `runs.jsonl` (one record per call), `run.log`, `probe.jsonl` (the first 8 calls), `schema-*.json` (the models' schemas from Cloudflare)
