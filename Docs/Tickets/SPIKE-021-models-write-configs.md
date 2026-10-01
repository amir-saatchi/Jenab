# SPIKE-021 — Can the models write valid configs?
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can the development models (SPIKE-018) write Jenab's configs so that they pass validation (SPEC 10), using only the error lists to fix them? This is the proposal's main risk: "LLMs write invalid pipelines or views".

1. **Schemas:** draft JSON Schemas for pipelines (SPEC 6), views and forms (SPEC 5), and structured migration steps (SPEC 7.5, 8.2). They are checked against the SPEC 9 example.
2. **Tasks:** the Bitcoin example (SPEC 9):
   - the tables, as migration steps
   - the `daily_btc` pipeline
   - the three views and the form
   - "also track Ethereum" (9.5), as a migration with updated dependents
3. **Loop:**
   - The model gets the SPEC-style guide, the schema summary and the task.
   - It returns the config through the save tools.
   - Validation returns the complete error list with paths (SPEC 10).
   - It gets at most 4 repair rounds.
4. **Measured per model:** first-try valid rate, rounds to valid, error types, tokens used, and whether the result does what was asked (checked by hand-written assertions, not only by the schema).
5. **Context:** do the guide, the schemas and the history fit comfortably inside the history window (SPEC 3.6)?

Models: Ollama Cloud `nemotron-3-ultra` and `gemma4:31b`, and Z.ai `glm-4.5-flash`, all on free tiers with the keys in `.env`.

## Done when
Each model has pass rates and round counts for each task, with the most common errors. There is a decision on the guide and schema changes, and on whether the default development models are good enough.

## Result
Run on 2026-09-28. Code, schemas, both guides and all transcripts are in `spikes/021-models-write-configs/` (`results.md`, `results/runs.jsonl`).

**Setup:**
- One agent with the save tools, a migration tool and a read-config tool. It combines the main agent and the schema agent.
- Validation returns the full error list with paths, and the agent gets at most 4 repair rounds.
- Each task starts from the correct state of the previous one. There were 3 runs per model and task, plus one chained run per model.
- Temperature was the provider default.
- The SPEC 9 reference configs pass the draft schemas and all 32 hand-written checks unchanged.

**Guide v1, 3 runs per cell** (first-try valid / valid in the end / checks passed in valid runs):

| Model | T1 tables | T2 `daily_btc` | T3 views + form | T4 "track Ethereum" | Speed |
|---|---|---|---|---|---|
| `nemotron-3-ultra` | 3/3 · 3/3 · 100% | 0/3 · 3/3 (1–2 rounds) · 97% | 3/3 · 3/3 · 100% | 0/3 · 3/3 (1–3 rounds) · 97% | slow: T4 273–692 s |
| `gemma4:31b` | 3/3 · 3/3 · 100% | 3/3 · 3/3 · 100% | 3/3 · 3/3 · 100% | 0/3 · **0/3** (repair limit) | fast: 2–27 s |
| `glm-4.5-flash` | 3/3 · 3/3 · 100% | 0/3 · **1/3** · 100% | 0/3 · 3/3 (1 round) · 100% | 0/3 · **1/3** · 11/13 | 22–241 s |

- **Overall:** 50 % of v1 runs were valid on the first try, and 81 % after repairs.
- **When runs failed:** every run that hit the repair limit was repeating the same error, not making progress.
- **Chained run** (all four tasks in one conversation):
  - nemotron and glm got all four tasks valid.
  - gemma failed T4 again.
  - Scores were nemotron 26/32 (4 misses came from a bug in the spike's own test setup), gemma 21/32 and glm 29/32.

**Errors** (186 error lines in v1):
- 48 % bad references, mostly misusing `for_each`, `item` and `each`
- 14 % semantic, e.g. row actions without `rows_from`
- 13 % a stored dependent not updated in a migration
- 11 % YAML, e.g. a map literal inside `${{ }}`
- 5 % bad expressions, e.g. `rows.map(.keyword)` or a made-up `| flatten`
- 4 % schema errors, 4 % unknown fields

**Context:**
- The guide is about 2,800–2,900 tokens (v1) or 3,100–3,500 (v2); tool definitions add 600–970.
- The peak request was 15,084 prompt tokens.
- The largest chained history was about 3.9k tokens, so the 24k window (SPEC 3.6) was never close.

**Guide v2** (a subset of 1–2 runs per cell, so these are hints, not results). It added the list of configs that use each table to the project card, YAML hints in errors, a `copy_data` recipe and a longer `for_each` section:
- glm:
  - T2 valid went from 1/3 to 2/2 runs, with 37k → 23k prompt tokens.
  - T4 went from 1/3 to 2/2 valid.
- nemotron: T4 was valid on the first try (1 run), with 54k → 18k tokens.
- gemma:
  - T4 became valid after 2 rounds.
  - **T2 got worse, from 3/3 to 0/2.** The longer `for_each` section made it loop over a one-item list.

**Tokens:** 1.92 million prompt tokens in total (cap 2.0 million). No quota or rate-limit stops. The secret scan was clean.

**SPEC gaps found** (the SPEC 9 configs themselves are valid):
1. **Migration steps (8.2):** no field names the step type. `unique`, `create_index` and `drop_index` are ambiguous.
2. **Dependents (8.2):**
   - The schema agent gets only the columns each dependent uses, not the full configs it must rewrite. A read-config tool was used in 7 of 7 T4 transcripts.
   - The spike merged the main agent and schema agent, so the "send dependents with the migration" rule itself wasn't tested separately.
3. **`for_each` (6.3):** it's not stated where `item` is allowed or how `each` is used in later steps. This caused the largest error group.
4. **Missing function:** there is no `flatten` or `concat` to merge lists made per item (6.4).
5. **Bitcoin example:**
   - SPEC 9.3 has no news view.
   - In 9.5, `news` keeps `url` as its key, so one article can't belong to two coins.
   - `DEFAULT 'BTC'` silently labels rows.
   - "all six steps" is really 7 steps plus 2 dependent updates.
   - It doesn't say whether copied rows keep `_run_id`.
6. **Smaller points:**
   - Filter `options: {query}` is shown only for forms (5.2).
   - `http.get` has its own `timeout` next to the step `timeout`.
   - The "no run in 9 years" cron check is only described.

## Decision
**Decided (2026-09-28):**
1. **Keep the loop as designed:** save through tools, complete error lists with paths, 4 repair rounds. 81 % of runs ended valid. It fits the context budget easily.
2. **Guide and harness changes that worked:**
   - The project card lists which configs use each table.
   - A `read_config(id)` tool for the main agent and the schema agent.
   - YAML hints in error messages (e.g. "quote this value").
   - Short recipes for `copy_data` and for building rows with `transform.map`.
   - A short `for_each` section that starts with "no `for_each` unless you loop over a real list".
3. **SPEC fixes:**
   - 6.3: exactly where `item` works and how `each` is read.
   - 6.4: add `flatten()`.
   - 8.2: add an `op` field to each migration step, and give the schema agent the full dependent configs.
   - 9.3: add a news table view.
   - 9.5:
     - `news` gets the key `(coin, url)` through `rebuild_table`, with no silent default.
     - Copied rows keep `_run_id`.
     - Correct the step count.
   - 5.2: allow `options: {query}` on view filters.
   - 6.5: drop `http.get`'s own `timeout`.
   - 10: implement the cron check.
4. **Model-neutral:**
   - The loop, guide, schemas, tools and error messages are the same for every model.
   - Nothing above the provider layer knows which model runs. The provider layer (SPEC 3.8) only handles protocol details, such as error codes, field names and rate limits.
   - A guide or schema change is kept only if it helps, or is neutral, on every tested model. If one model gets worse (as gemma did with v2), the change is reworked, not split per model.
   - The spike's loop had no per-model code. Its only provider-specific code was Ollama's `max_tokens` field and a retry on Z.ai's "overloaded" error.
5. **Which models test it:**
   - The free dev models are stand-ins. They're chosen because they're weak and free: a config format that they can write, a stronger model can write too.
   - nemotron-3-ultra, gemma4:31b and glm-4.5-flash all stay in the test set.
   - Before Phase 5, the same tests also run on at least one frontier model (for example Claude or GPT) with the user's key. That run isn't done yet.
6. **Before Phase 2** (schema agent): build guide v3 from these changes and rerun T2 and T4 with 3 runs per model. A config the models still can't write reliably gets a simpler shape or a higher-level step, not a longer guide.
7. **Early-stop nudge:** the spike added one "Not finished: … not saved yet" message when a model stopped without saving. This is model-neutral too. SPEC 3 gets it as a general orchestrator rule, at most once per turn.

Doc changes (made in SPEC v0.5): SPEC 1 (model-neutral rule), 3.2 (dependents in the project card), 3.8 (protocol only), 5.2, 6.3, 6.4, 6.5, 8.1, 8.2, 9.2, 9.3, 9.5, 10 (repair loop, early-stop notice) and the changelog. PROPOSAL: the risk row "LLMs write invalid pipelines or views" gets the measured numbers.
