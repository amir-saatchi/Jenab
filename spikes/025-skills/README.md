# SPIKE-025: do models load the right skills?

Throwaway code for [SPIKE-025](../../Docs/Tickets/SPIKE-025-skills.md). It takes the SPIKE-021 harness (validator, tasks, tools, providers) and the SPIKE-023 Mother fixture and tests the SPEC 8.9 skills draft on the development models: condition A (the whole guide v2 in the system prompt, as in SPIKE-021) against B (a skill list plus `load_skill`) and C (B plus `load_with`).

```bash
go test ./...                       # offline: the guide split gives guide v2 back byte for byte; SPIKE-021 validator checks
go run . -check                     # offline: skill files and limits, split check, prompt sizes, SPEC 9 reference check
go run . -list                      # prints the system prompts of A, B/C and Mother (no network)
go run . -calib -plan all           # the whole spike: calibration, then phases 1-5 (resumable)
go run . -tests tload -conds B -reps 1 -items L01,L09 -models gemma4:31b   # a subset
go run . -smoke -tests tload -conds C -items L02 -reps 1                  # smoke run into results/smoke.jsonl
go run . -report                    # rewrite the measured part of results.md from results/*.jsonl
go run . -scan                      # secret scan of this folder (prints file names only)
```

`-plan all` runs, per provider lane (Ollama models one after another, Z.ai in parallel):
1. T-load and T-roles, B and C, rep 1
2. T-config T2-T4, A, B and C, rep 1
3. T-load under A, rep 1 (for the cost comparison only)
4. T-config, rep 2
5. T-load and T-roles, rep 2

A combination (test, model, condition, item, rep) that is already in `results/runs.jsonl` without an error is skipped, so an interrupted run is resumed with the same command. The prompt-token budget counts `results/runs.jsonl`, `results/calib.jsonl` and `results/smoke.jsonl`; the program stops 20k below 3,500,000.

## Keys and safety

- `OLLAMA_API_KEY` and `Z_API_KEY` come from the repo-root `.env`. Only `loadEnv` (`env.go`, unchanged from SPIKE-021) reads it; it reads only the key names it knows, and values stay in memory.
- Keys go only into the `Authorization` header, and only to `ollama.com` and `api.z.ai` (`providers.go`: host lock and outgoing-header allow-list). `OPENAI_*` variables are removed from the process environment.
- Everything written goes through the SPIKE-018 redaction. `-scan` checks the folder.
- Only synthetic prompts are sent. `web_search` returns fixed synthetic results; generated pipelines are only validated and run as a mock dry run (SPIKE-021).
- A 429 or "limit" error that persists after the retries stops every model of that provider (`stream.go`).

## Files

- **Skills (`skills/*.md`):** SPEC 8.9 format (front matter `name`, `description`, `load_with`; body).
  - `config-guide`, `pipelines`, `migrations`, `database-design`, `sql-queries`: guide v2 split without changes (see results.md for what moved where). `split_test.go` checks that `guides/intro.md` + the five bodies rebuild `guides/guide_v2.md` byte for byte.
  - `web-research` (new, from SPEC 6.5 and 3.7) and `delegation` (the SPIKE-023 guidance v1, verbatim).
- **Guides (`guides/`):** `guide_v2.md` (SPIKE-021, with LF line endings) and `intro.md` (its text before section 1, which stays in every condition).
- **Prompts (`prompts/`):** SPIKE-023's `base.md`, `mother.md`, `rule.md`, `card.md`, `chats.md` and `delegation.md`, for Mother.
- **Code:**
  - `skills.go`: skill parsing and limits, the skill list, `rebuildGuide`
  - `agent.go`: the SPIKE-021 loop with conditions A/B/C, `load_skill`, `load_with`, events, and the T-load probe mode (stops at the first write attempt or the final answer)
  - `tload.go`: the 12 T-load requests and their scoring; the fake `web_search`
  - `troles.go`: Mother's prompt and tools (`create_chat` with `skills`), the 6 T-roles requests and their scoring
  - `calib.go`: token sizes per model
  - `report.go`: the tables in results.md
  - `main.go`: flags, phases, lanes, resume; `refcheck.go`: the SPIKE-021 reference check
  - Unchanged from SPIKE-021: validator, schemas, migrations, tasks, reference state, mock runner, `env.go`. `providers.go`/`stream.go` gained only the provider-wide stop on persistent quota errors. `tools.go` gained `web_search` and `load_skill`.
- **Output (`results/`):** `runs.jsonl` (one record per run), `calib.jsonl`, `smoke.jsonl`, `transcripts/` (redacted), `run_all.log`.
