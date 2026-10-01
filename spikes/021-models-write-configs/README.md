# SPIKE-021: can the dev models write valid Burrow configs?

Throwaway code for SPIKE-021. It drafts JSON Schemas for pipelines, views and migration steps, adds the SPEC 10 Go checks, and lets free-tier models build the SPEC 9 Bitcoin project through Burrow-style tools. Then it measures how often the configs are valid and whether they do what was asked.

```bash
go run . -all                       # everything: reference check, calibration, 3 runs x 4 tasks, 1 chained run, results.md
go run . -ref                       # only the SPEC 9 reference check (no network)
go test ./...                       # validator checks on broken variants of the reference configs (no network)
go run . -runs 1,2,3 -chained 1     # model runs only (appends to results/runs.jsonl)
go run . -guide v2 -runs 1 -tasks T2,T3,T4 -models gemma4:31b,glm-4.5-flash   # improved guide on a subset
go run . -report                    # regenerate the measured part of results.md from results/*.jsonl
go run . -scan                      # secret scan of this folder
```

A full `-all` run with three models uses about 1.4M prompt tokens and takes about 1.5 hours on the free tiers. Most of the time is nemotron-3-ultra on T4, 3 to 12 minutes per run.

The spike's own runs used 1,923,072 prompt tokens in total, split into:
- the v1 runs
- the v2 subset
- calibration
- aborted attempts

The program keeps a running total of prompt tokens, including earlier invocations read from `results/*.jsonl`. It stops 20k below the 2.0M cap, so the next request cannot cross it. The results already in `results/` count toward that total. To reproduce from scratch, move `results/*.jsonl` aside first. The spike itself ran in phases, with Ollama and Z.ai in parallel lanes. Its logs are in `results/phase*.log`.

Results and conclusions are in `results.md`. The part above the marker is hand-written; `-report` regenerates the part below it.

## Keys and safety

- **Keys:** `OLLAMA_API_KEY` and `Z_API_KEY` come from the repo-root `.env` (git-ignored). Only `loadEnv` reads that file.
- **Where keys go:** only into the `Authorization` header, and only to `ollama.com` and `api.z.ai`. The client middleware refuses any other host and strips headers that are not on an allow-list. `OPENAI_*` variables are removed from the process environment.
- **Output:** everything written, including results and transcripts, goes through the SPIKE-018 redaction.
- **No real network calls from generated pipelines:** they are only validated. The assertions run them as a mock dry run: `http.get` and `web.search` return synthetic data inside the process, `llm.select` picks the first items, and db steps run on a `VACUUM INTO` copy of the project database.

## Files

- **Schemas (`schemas/`):** the draft JSON Schemas (2020-12).
  - `pipeline.schema.json` (SPEC 6)
  - `view.schema.json` (table/chart/form, SPEC 5)
  - `migration.schema.json` (structured steps, SPEC 7.5/8.2; the step kind is in `op`)
- **Loading and schema check:**
  - `load.go`: the SPEC 1 YAML loader (no anchors, aliases, merge keys or tags; line:column per JSON pointer; SPEC paths)
  - `formats.go`: the custom formats (cron, IANA time zone, Go duration, http URL) and date defaults
  - `validate.go`: schema validation with leaf errors only, error categories, and the issue list with paths
- **Go checks:**
  - `sqlguard.go`: the SQL guard (text check plus EXPLAIN opcode scan)
  - `sandbox.go`: the expression sandbox (from SPIKE-005)
  - `shapes.go`: reference and shape checks on expressions (step ids, earlier steps, outputs, item fields, inputs)
  - `check_pipeline.go`, `check_view.go`: the SPEC 10 checks that the schemas cannot express
  - `migrate.go`: migration steps in one transaction, the schema guard, and re-validation of every stored view and pipeline (dependents are sent in the same call)
- **Project and tools:**
  - `project.go`: the test project (SQLite via modernc, writer plus guarded reader, project card)
  - `tools.go`: the tools `apply_migration`, `save_pipeline`, `save_view`, `save_form`, `describe_table`, `query` and `get_config`
- **Agent:**
  - `agent.go`: the loop (guide + card as system prompt, at most 4 repair rounds, one nudge, 16 requests per task), token tracking, and the SPEC 3.6 history window for the chained run
  - `guides/guide_v1.md`, `guides/guide_v2.md`: the config guides given to the models
- **Tasks and scoring:**
  - `tasks.go`: tasks T1 to T4 and their assertions
  - `reference.go`: the correct state after each task (SPEC 9 configs), the seeds, and the state builder
  - `runner.go`: the mock dry run, opening views, and simulated form submits
- **Providers:** `providers.go`, `stream.go`, `env.go` are from SPIKE-018, trimmed to Ollama Cloud and Z.ai. Temperature 0.2 is sent; reasoning effort is left at the provider default.
- **Reporting:** `report.go` builds the tables in `results.md`.
- **Output (`results/`):**
  - `runs.jsonl`: one record per task run
  - `calib.jsonl`: guide and tool sizes per model
  - `transcripts/`: redacted conversations
  - `smoke.jsonl`: aborted first attempts (counted in the budget)
