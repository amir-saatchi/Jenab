# SPIKE-021 results

Can the free dev models write complete, valid Burrow configs through tools? Short answer: yes for tables, one pipeline and views. With the validator's error messages and at most 4 repair rounds, all three models get T1–T3 valid in the end, except glm on T2 (1 of 3). The hard part is T4, a migration that must also update its dependent configs. Under guide v1, only nemotron-3-ultra solved T4 reliably (3/3 valid, but never on the first try). Guide v2 fixed most of that for nemotron and glm.

## Setup

- **Models:**
  - nemotron-3-ultra and gemma4:31b via Ollama Cloud, one request at a time.
  - glm-4.5-flash via Z.ai.
  - gpt-oss:120b was skipped to save budget.
  - Temperature 0.2 on every request; reasoning effort left at the provider default; max output 8192 tokens.
- **Tools:** apply_migration, save_pipeline, save_view, save_form, describe_table, query, get_config.
- **System prompt:** the guide, the generated project card and today's date.
- **Validation:** writes go through the draft JSON Schemas (`schemas/`, 2020-12) and the SPEC 10 Go checks.
  - A rejected write returns the error list, each item with line:column and a SPEC path. Nothing is saved.
  - Limits per task: at most 4 repair rounds (a 5th failed batch stops the task), one nudge if the model stops without saving, and 16 requests.
- **Tasks:** T1–T4 follow SPEC 9:
  - T1: the tables
  - T2: the daily_btc pipeline
  - T3: five views and forms
  - T4: add ETH, which needs a migration plus the updated pipeline and views
  - In **single** runs, each task starts from the reference state of the previous task (the SPEC 9 configs plus seeds: 10 days of BTC prices, 4 news items, 3 keywords).
  - The **chained** run does T1..T4 in one conversation from an empty project. It uses the SPEC 3.6 history window (8 turns / 24k tokens, cut to 3; tool results from older turns cut to 6,000-char previews). Seeds are added after T1.
- **Assertions:** after each task the saved pipeline is dry-run twice on a copy of the database:
  - http.get and web.search return synthetic data inside the process, and llm.select takes the first N items. There are no real network calls.
  - Views are opened with their filter defaults, and a form submit is simulated.
  - There are 4 / 10 / 5 / 13 assertions for T1..T4.
  - Caveat: when nothing was saved, only the "saved" assertion is counted, so assertion % flatters runs that never saved anything.
- **Metrics:**
  - *Failed batch:* one model response that contains at least one rejected write.
  - *Valid:* every needed artifact is saved and the last write of each is OK.
  - *First-try valid:* valid with 0 failed batches.
  - *Repair rounds:* failed batches in valid runs.
- **Runs:** 3 runs per model × task with guide v1, plus one chained run per model. Guide v2 was run on a subset (1–2 runs; see below).

## Per model × task (guide v1, single tasks, 3 runs each)

| Model | T1 first-try / rounds / assertions | T2 | T3 | T4 | All: first-try, valid, assertions |
|---|---|---|---|---|---|
| nemotron-3-ultra | 100% / 0 / 100% | 0% / 1.7 / 97% | 100% / 0 / 100% | 0% / 2.0 / 97% | 50%, 100%, 98% |
| gemma4:31b | 100% / 0 / 100% | 100% / 0 / 100% | 100% / 0 / 100% | 0% / never valid / 15% | 75%, 75%, 66% |
| glm-4.5-flash | 100% / 0 / 100% | 0% / 3.0 (valid 1/3) / 83% | 0% / 1.0 / 100% | 0% / 1.0 (valid 1/3) / 33% | 25%, 67%, 64% |

**Chained (v1, 1 run each):**
- **nemotron-3-ultra:** all 4 tasks valid, 26/32 assertions.
  - 4 of the 6 misses are a harness artifact. Nemotron named the column `price_usd`, and the seed insert still used `price`, so no seeds were added. That failed the "seeds kept", T3 "10 seeded prices" and T4 "10 BTC rows copied" checks.
  - `seed()` and the T2 check now use the model's price column. The run was not repeated (budget).
- **gemma4:31b:** T1–T3 first-try; T4 hit the repair limit again (21/32).
- **glm-4.5-flash:** all valid, 29/32; T4 was first-try in the chain.

**Speed:** for T4, nemotron is slow (434 s per run on average, up to 692 s). gemma is very fast (4–25 s per task). glm takes 28–204 s.

## Guide v1 vs v2

v2 is `guides/guide_v2.md` (11,519 chars vs 9,998) plus two harness changes: YAML error hints, and the project card listing which configs use which tables. It adds:

- how to send dependents
- a copy_data recipe, and that a NOT NULL add_column needs a default
- a worked for_each / `each` example
- "build rows with transform.map, not map literals"
- YAML quoting rules
- the function-call style (`lower(s)`, `map(x, .f)`)

| Model | Task | v1 (3 runs): first-try / valid / assertions / prompt tok per run | v2: runs, first-try / valid / assertions / prompt tok per run |
|---|---|---|---|
| glm-4.5-flash | T2 | 0% / 33% / 83% / 36.7k | 2 runs: 0% / 100% / 100% / 23.0k |
| glm-4.5-flash | T4 | 0% / 33% / 33% / 94.4k | 2 runs: 50% / 100% / 58% / 71.0k |
| nemotron-3-ultra | T2 | 0% / 100% / 97% / 22.4k | 1 run: 0% / 100% / 100% / 21.1k |
| nemotron-3-ultra | T4 | 0% / 100% / 97% / 54.2k | 1 run: 100% / 100% / 92% / 18.5k |
| gemma4:31b | T2 | 100% / 100% / 100% / 8.4k | 2 runs: 0% / **0%** / 0% / 28.6k |
| gemma4:31b | T4 | 0% / 0% / 15% / 42.0k | 1 run: 0% / 100% (2 rounds) / 69% / 34.6k |

**Takeaways:**
- **What helped:** v2 fixed the T4 dependents problem. "Dependent not updated" was 13% of v1 errors (24 lines) and 0 for nemotron under v2. It also fixed glm's T2 map-literal loop and made T4 valid for all three models.
- **What regressed:** gemma's T2 got worse under v2. Both v2 runs built a one-item transform.map (`items: [1]`), ran web.search with for_each over it, and then referenced `item.item.title` five times. The long for_each section taught a pattern that simple cases don't need.
- **Fix for v3:** open the section with "no for_each unless you loop over a real list; build a search string with `join(map(steps.k.rows, .keyword), ' ')` right in `with`".
- **Caveat:** the v2 samples are small (1–2 runs).

## Top error types (v1, all error lines in rejected writes; 186 lines)

1. **Bad refs, 48%.**
   - Common forms: the for_each `each` shape (`item.item`, `steps.x.each` treated as rows), `item` used outside for_each/transform.map, and row fields that are not table columns.
   - Example: `steps[2].with.fields.coin: item is only available in steps with for_each, transform.map fields and transform.filter conditions`.
2. **Semantic (SPEC 10 checks, SQL at apply time), 14%.**
   - Example: `row actions need rows_from { table, key }`.
   - Example: a copy_data that hits `UNIQUE constraint failed: btc_news.url`.
3. **Dependent not updated, 13%.**
   - Example: `stored view btc_price_chart (not sent with this migration) would break: no such table: btc_prices`.
4. **YAML, 11%.**
   - Almost all are `mapping values are not allowed in this context`, from a map literal inside `${{ }}` or an unquoted `: ` in a plain value.
5. **Bad expression, 5%.**
   - Method-call style, e.g. `join(steps.k.rows.map(.keyword), ' OR ')`.
   - A made-up `| flatten`.
6. **Schema / unknown field, 4% each.**
   - Examples: missing `default_sort`; `backoff: exponential`; `fields` on db.insert.

The full tables with examples are below.

## Sizes

- **Guide v1:** 9,998 chars, measured with each model's own tokenizer:
  - 2,934 tokens on gemma
  - 2,738 on glm
  - 2,774 on nemotron
- **Guide v2:** 11,519 chars = 3,098 / 3,489 / 3,377 tokens.
- **Tool definitions:** 588–967 tokens.
- **Peak single request:** 15,084 prompt tokens (nemotron T4 v1). Chained peak: 12,142 (gemma T4).
- **History window:** the largest history block in the chained run was 15,550 chars (about 3.9k tokens, glm at T4). The SPEC 3.6 window (8 turns / 24k tokens) never had to cut. The 24k budget has more than enough room for this project.
- **Tokens:** 1,923,072 prompt tokens in total, of 2.0M:
  - 1,715,390 in task runs
  - 23,442 in calibration
  - 184,240 in aborted and smoke runs (`results/smoke.jsonl`)
  - Output: 150,059 tokens.

## SPEC 9 reference and SPEC inconsistencies

The SPEC 9.1–9.5 example configs (tables, pipeline, the four 9.3 views, the 9.5 migration) validate against the draft schemas and Go checks without changes. With them, all 32 assertions pass (`go run . -ref`). Found while encoding them:

1. **SPEC 8.2:** migration steps have no named discriminator field. The spike uses `op`. The step names are also ambiguous: `unique` vs `create_index` / `drop_index`.
2. **SPEC 8.1 and the agents:**
   - 8.1 lists no tool for reading a stored config. The spike added `get_config`, and every T4 transcript used it (7 of 7).
   - The main agent has no migration tool. Migrations and dependent updates belong to the schema agent, but T4 needs both in one change. The spike merges them into `apply_migration(steps, pipelines, views)`.
3. **SPEC 6.3:** the form of `if` / `for_each` is not specified: what `item` is, and what shape `steps.x.each` has. That gap caused the largest error class.
4. **No flatten / concat function:** there is no way to merge per-item lists (e.g. one http.get per coin, then one insert). Models invented `| flatten`. The spike's workaround is `db.insert_many` with for_each over `steps.x.each` and `rows: ${{ item.items }}`.
5. **SPEC 9.3 has no news view.** T3 asks for btc_news_table, so the reference adds one in the 9.3 style.
6. **SPEC 9.5:**
   - news keeps `url` as the primary key, so the same article cannot be stored for both coins.
   - `add_column coin NOT NULL DEFAULT 'BTC'` quietly labels a missing coin as BTC.
   - The text says "all six steps", but the migration has 7 steps plus 2 dependent updates.
   - It does not say whether moved rows keep `_run_id`. The T4 check requires it: all 4 v2 T4 runs and 1 v1 run dropped it, and each is counted as a miss.
7. **SPEC 5.2:** filter `options: { query }` is shown only for forms, not for view filters.
8. **http.get timeout:** `with.timeout` duplicates the step-level `timeout`.
9. **Cron:** SPEC 6's "no run in 9 years" cron check was not implemented. It would need a cron-next implementation.
10. **YAML pitfalls:** `${{ { a: b } }}` map literals and unquoted `: ` inside plain values break YAML. The guide has to teach quoting, or the error message must explain it (v2 hint).

## Recommendation

1. **Keep the tool-based write-plus-repair loop with positional errors.** It carries the models:
   - 81% of v1 single runs end valid even though only 50% are valid on the first try.
   - 4 repair rounds are enough. Every repair-limit stop was the same error repeated (glm's map literal 5 times, gemma's `item.item`), not progress being cut off.
2. **Ship the v2 elements that worked,** with the for_each section rewritten as described above:
   - the dependents list in the project card and the `get_config` read tool
   - the copy_data recipe
   - the YAML hints in error messages
   - "rows via transform.map"
3. **Fix the SPEC gaps:**
   - name the step discriminator
   - define `for_each` / `each` / `item`
   - add `flatten` (or let insert_many take `each`)
   - give the main agent a way to apply migrations together with dependents (or define the hand-off to the schema agent)
   - change the 9.5 news key to (url, coin)
   - say whether `_run_id` survives copy_data
4. **Dev model choice:**
   - **nemotron-3-ultra** is the most reliable config writer: 100% valid under v1, and T4 first-try with v2. It is slow.
   - **gemma4:31b** is the best for fast iteration on simple configs, but it is weak at multi-step migrations and sensitive to guide wording.
   - **glm-4.5-flash** needs the most repair rounds but improves strongly with v2.
   - Rerun the comparison with a v3 guide before settling on it.

## Notes

- **Aborted attempts:**
  - the first phase-A attempt, restarted after the T1 prompt got "only the tables for now"
  - a glm T1 run, killed
  - a nemotron chained run, killed by a session stall
  - These are in `results/smoke.jsonl` and counted in the budget.
- **Strict assertions:**
  - The T4 check "news: 5 new items" fails when a model picks 5 per coin (10 rows). The prompt is ambiguous.
  - The "_run_id kept" check is stricter than the prompt.
- **Budget reserve:** before the last v2 runs, the stop rule gained a 20k reserve so that one more request cannot cross the cap.
- **Rate limits:** there were no rate-limit or quota stops. There was one `unexpected EOF` from Ollama, which was retried.

<!-- generated by `go run . -report`; do not edit below -->

## Token budget

Prompt tokens: 1715390 in task runs + 23442 in calibration + 184240 in the smoke test = **1923072 of 2,000,000**. Output tokens in task runs: 150059. Task runs recorded: 57.

## Guide and request sizes

Measured per model as the prompt_tokens difference against a one-line system prompt (so each model's own tokenizer and chat template).

| Model | Guide | Guide chars | Guide tokens | Tool definitions tokens (7 tools) | Peak request (prompt tokens, any run) | Peak request, chained |
|---|---|---|---|---|---|---|
| gemma4:31b | v1 | 9998 | 2934 | 588 | 10472 | 12142 |
| glm-4.5-flash | v1 | 9998 | 2738 | 739 | 12089 | 9919 |
| glm-4.5-flash | v2 | 11519 | 3098 | 739 | 10792 | 0 |
| gemma4:31b | v2 | 11519 | 3489 | 588 | 9413 | 0 |
| nemotron-3-ultra | v1 | 9998 | 2774 | 967 | 15084 | 12054 |
| nemotron-3-ultra | v2 | 11519 | 3377 | 967 | 7631 | 0 |

Chained conversation: history block (previous turns kept in the window) at the start of each task, in characters (about chars/4 tokens), and cuts made by the SPEC 3.6 rule (8 turns or 24k tokens, cut to 3):

| Model | Guide | T1 | T2 | T3 | T4 | Cuts |
|---|---|---|---|---|---|---|
| glm-4.5-flash | v1 | 0 | 2451 | 11034 | 15550 | 0 |
| nemotron-3-ultra | v1 | 0 | 1740 | 4833 | 8597 | 0 |
| gemma4:31b | v1 | 0 | 1607 | 4045 | 7125 | 0 |

## Results, guide v1, single tasks (each from the reference state of the previous task)

| Model | Task | Runs | First-try valid | Valid in the end | Repair rounds (valid runs, mean) | Assertions passed | Prompt tok/run | Output tok/run | Requests/run | Peak request | Time/run | Not done |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| nemotron-3-ultra | T1 | 3 | 100% | 100% | 0.0 | 100% (12/12) | 8342 | 735 | 2.0 | 4409 | 32 s |  |
| nemotron-3-ultra | T2 | 3 | 0% | 100% | 1.7 | 97% (29/30) | 22433 | 2417 | 4.7 | 5759 | 161 s |  |
| nemotron-3-ultra | T3 | 3 | 100% | 100% | 0.0 | 100% (15/15) | 35288 | 1449 | 7.3 | 5610 | 79 s |  |
| nemotron-3-ultra | T4 | 3 | 0% | 100% | 2.0 | 97% (38/39) | 54216 | 10419 | 6.3 | 15084 | 434 s |  |
| **nemotron-3-ultra** | **all** | 12 | **50%** | **100%** | 0.9 | **98%** (94/96) | 30069 | 3755 | 5.1 | 15084 | 176 s |  |
| gemma4:31b | T1 | 3 | 100% | 100% | 0.0 | 100% (12/12) | 7928 | 351 | 2.0 | 4201 | 5 s |  |
| gemma4:31b | T2 | 3 | 100% | 100% | 0.0 | 100% (30/30) | 8360 | 522 | 2.0 | 4446 | 4 s |  |
| gemma4:31b | T3 | 3 | 100% | 100% | 0.0 | 100% (15/15) | 8625 | 700 | 2.0 | 4748 | 4 s |  |
| gemma4:31b | T4 | 3 | 0% | 0% | – | 15% (6/39) | 41966 | 6162 | 6.0 | 10472 | 25 s | repair limit ×3 |
| **gemma4:31b** | **all** | 12 | **75%** | **75%** | 0.0 | **66%** (63/96) | 16720 | 1933 | 3.0 | 10472 | 9 s | repair limit ×3 |
| glm-4.5-flash | T1 | 3 | 100% | 100% | 0.0 | 100% (12/12) | 7763 | 768 | 2.0 | 4119 | 28 s |  |
| glm-4.5-flash | T2 | 3 | 0% | 33% | 3.0 | 83% (10/12) | 36730 | 2637 | 8.0 | 6118 | 115 s | repair limit ×2 |
| glm-4.5-flash | T3 | 3 | 0% | 100% | 1.0 | 100% (15/15) | 41838 | 1292 | 9.3 | 5395 | 53 s |  |
| glm-4.5-flash | T4 | 3 | 0% | 33% | 1.0 | 33% (13/39) | 94356 | 5598 | 15.3 | 12089 | 204 s | repair limit ×1, request cap ×1 |
| **glm-4.5-flash** | **all** | 12 | **25%** | **67%** | 0.9 | **64%** (50/78) | 45172 | 2573 | 8.7 | 12089 | 100 s | repair limit ×3, request cap ×1 |

### Guide v1, chained conversation (T1..T4 in one chat, from an empty project)

| Model | Task | Runs | First-try valid | Valid in the end | Repair rounds (valid runs, mean) | Assertions passed | Prompt tok/run | Output tok/run | Requests/run | Peak request | Time/run | Not done |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| nemotron-3-ultra | T1 | 1 | 100% | 100% | 0.0 | 100% (4/4) | 8345 | 778 | 2.0 | 4409 | 23 s |  |
| nemotron-3-ultra | T2 | 1 | 100% | 100% | 0.0 | 90% (9/10) | 9949 | 1234 | 2.0 | 5234 | 37 s |  |
| nemotron-3-ultra | T3 | 1 | 100% | 100% | 0.0 | 60% (3/5) | 36873 | 1465 | 6.0 | 6617 | 66 s |  |
| nemotron-3-ultra | T4 | 1 | 0% | 100% | 2.0 | 77% (10/13) | 37853 | 8106 | 4.0 | 12054 | 275 s |  |
| **nemotron-3-ultra** | **all** | 4 | **75%** | **100%** | 0.5 | **81%** (26/32) | 23255 | 2895 | 3.5 | 12054 | 100 s |  |
| gemma4:31b | T1 | 1 | 100% | 100% | 0.0 | 100% (4/4) | 7928 | 351 | 2.0 | 4201 | 6 s |  |
| gemma4:31b | T2 | 1 | 100% | 100% | 0.0 | 100% (10/10) | 9470 | 530 | 2.0 | 5001 | 5 s |  |
| gemma4:31b | T3 | 1 | 100% | 100% | 0.0 | 100% (5/5) | 11285 | 752 | 2.0 | 6087 | 8 s |  |
| gemma4:31b | T4 | 1 | 0% | 0% | – | 15% (2/13) | 45984 | 6709 | 5.0 | 12142 | 49 s | repair limit ×1 |
| **gemma4:31b** | **all** | 4 | **75%** | **75%** | 0.0 | **66%** (21/32) | 18666 | 2085 | 2.8 | 12142 | 17 s | repair limit ×1 |
| glm-4.5-flash | T1 | 1 | 100% | 100% | 0.0 | 100% (4/4) | 20461 | 640 | 5.0 | 4307 | 23 s |  |
| glm-4.5-flash | T2 | 1 | 0% | 100% | 4.0 | 90% (9/10) | 33821 | 2170 | 6.0 | 6706 | 70 s |  |
| glm-4.5-flash | T3 | 1 | 0% | 100% | 1.0 | 100% (5/5) | 52414 | 1140 | 7.0 | 8016 | 43 s |  |
| glm-4.5-flash | T4 | 1 | 100% | 100% | 0.0 | 85% (11/13) | 18159 | 1875 | 2.0 | 9919 | 58 s |  |
| **glm-4.5-flash** | **all** | 4 | **50%** | **100%** | 1.2 | **91%** (29/32) | 31213 | 1456 | 5.0 | 9919 | 48 s |  |

### Guide v1: error categories (all error lines in failed write calls)

| Category | Errors | nemotron-3-ultra | gemma4:31b | glm-4.5-flash | Examples |
|---|---|---|---|---|---|
| bad refs | 90 (48%) | 12 | 37 | 41 | glm-4.5-flash T4 apply_migration: `pipelines[0]: line 26:15 steps[2].with.fields.coin: item is only available in steps with for_each, transform.map fields and transform.filter conditions (this step has no for_each)`<br>glm-4.5-flash T4 apply_migration: `pipelines[0]: line 28:16 steps[2].with.fields.price: item is only available in steps with for_each, transform.map fields and transform.filter conditions (this step has no for_each)`<br>glm-4.5-flash T4 apply_migration: `pipelines[0]: line 24:13 steps[2].with.rows: rows have a field "body", but table prices has no such column (did you mean "coin"?) (columns: coin, date, price, _run_id, _fetched_at)` |
| semantic | 26 (14%) | 0 | 2 | 24 | glm-4.5-flash T3 save_view: `line 13:5 row_actions[0]: row actions need rows_from { table, key }`<br>glm-4.5-flash T4 apply_migration: `steps[3].from: SQLite: constraint failed: UNIQUE constraint failed: btc_news.url (1555)`<br>glm-4.5-flash T4 apply_migration: `steps[3].from: SQLite: constraint failed: UNIQUE constraint failed: btc_news.url (1555)` |
| dependent not updated | 24 (13%) | 15 | 3 | 6 | nemotron-3-ultra T4 apply_migration: `stored view btc_news_table (not sent with this migration) would break: query: SQLite: no such table: btc_news`<br>nemotron-3-ultra T4 apply_migration: `stored view btc_price_chart (not sent with this migration) would break: query: SQLite: no such table: btc_prices`<br>nemotron-3-ultra T4 apply_migration: `stored view btc_price_table (not sent with this migration) would break: query: SQLite: no such table: btc_prices` |
| yaml | 20 (11%) | 3 | 3 | 14 | nemotron-3-ultra T2 save_pipeline: `line 45:0 (top level): YAML: line 45: mapping values are not allowed in this context`<br>glm-4.5-flash T2 save_pipeline: `line 49:0 (top level): YAML: line 49: mapping values are not allowed in this context`<br>glm-4.5-flash T2 save_pipeline: `line 49:0 (top level): YAML: line 49: mapping values are not allowed in this context` |
| bad expression | 9 (5%) | 2 | 3 | 4 | glm-4.5-flash T2 save_pipeline: `line 23:14 steps[2].with.query: expression "join(steps.get_watch_keywords.rows.map(.keyword), ' OR ')" does not compile: unexpected token Operator(".") (1:40)`<br>glm-4.5-flash T2 save_pipeline: `line 23:14 steps[2].with.query: expression "join(steps.get_watch_keywords.rows.map(.keyword), ' OR ')" does not compile: unexpected token Operator(".") (1:40)`<br>glm-4.5-flash T2 save_pipeline: `line 23:14 steps[2].with.query: expression "join(steps.get_watch_keywords.rows, ' OR ', .keyword)" does not compile: unexpected token Operator(".") (1:45)` |
| schema | 8 (4%) | 2 | 3 | 3 | glm-4.5-flash T3 save_view: `line 1:1 (top level): missing required field "default_sort"`<br>gemma4:31b T4 apply_migration: `steps[1]: missing required field "column"`<br>glm-4.5-flash T2 save_pipeline: `line 15:16 steps[0].retry.backoff: 'exponential' is not valid go-duration: not a Go duration like 500ms, 60s, 5m, 2h` |
| unknown field | 7 (4%) | 0 | 3 | 4 | glm-4.5-flash T4 apply_migration: `pipelines[0]: line 25:7 steps[2].with.fields: unknown field "fields"`<br>gemma4:31b T4 apply_migration: `steps[1].columns: unknown field "columns"`<br>glm-4.5-flash T4 apply_migration: `pipelines[0]: line 25:7 steps[2].with.fields: unknown field "fields"` |
| bad SQL | 1 (1%) | 0 | 0 | 1 | glm-4.5-flash T4 apply_migration: `steps[4].from: SQLite: no such table: btc_prices` |
| wrong step type | 1 (1%) | 0 | 0 | 1 | glm-4.5-flash T2 save_pipeline: `line 14:10 steps[1].use: value must be one of 'http.get', 'http.download', 'web.search', 'html.extract', 'json.extract', 'transform.map', 'transform.filter', 'db.query', 'db.insert', 'db.insert_many', 'db.update', 'llm.select', 'llm.extract', 'bucket.put', 'bu...` |

### Guide v1: failed assertions

| Model | Task | Assertion | Failed runs | First reason |
|---|---|---|---|---|
| gemma4:31b | T4 | all 10 BTC rows copied with coin BTC | 4 | no prices table |
| gemma4:31b | T4 | btc_prices dropped | 4 | btc_prices still exists |
| gemma4:31b | T4 | fetches the BTC and the ETH URL | 4 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| gemma4:31b | T4 | news search covers both coins | 4 | queries ["bitcoin ETF halving regulation news" "bitcoin ETF halving regulation news"] |
| gemma4:31b | T4 | news table has coin; old news are BTC | 4 | table btc_news: -1 of 4 seeded rows with coin BTC |
| gemma4:31b | T4 | news table view shows the coin | 4 | rows 9 coin column false err <nil> |
| gemma4:31b | T4 | news: 5 new items, each with coin BTC/ETH and a reason | 4 | no news table with coin |
| gemma4:31b | T4 | price chart: one line per coin (series_by) | 4 | series_by "" coins map[] err <nil> |
| gemma4:31b | T4 | price table shows both coins | 4 | coins map[] err <nil> |
| gemma4:31b | T4 | prices today: BTC 65000.5 and ETH 3200.25, one row each | 4 | no prices table |
| gemma4:31b | T4 | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 4 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| glm-4.5-flash | T2 | pipeline daily_btc saved | 2 | not saved (saved: ) |
| glm-4.5-flash | T2 | search mentions bitcoin and every watch keyword | 1 | queries ["Bitcoin news" "Bitcoin news"], missing [ETF halving regulation] |
| glm-4.5-flash | T4 | all 10 BTC rows copied with coin BTC | 2 | no prices table |
| glm-4.5-flash | T4 | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | 0 of 10 seeded rows found |
| glm-4.5-flash | T4 | btc_prices dropped | 2 | btc_prices still exists |
| glm-4.5-flash | T4 | fetches the BTC and the ETH URL | 3 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| glm-4.5-flash | T4 | news search covers both coins | 3 | queries ["bitcoin ETF halving regulation news" "bitcoin ETF halving regulation news"] |
| glm-4.5-flash | T4 | news table has coin; old news are BTC | 3 | table btc_news: -1 of 4 seeded rows with coin BTC |
| glm-4.5-flash | T4 | news table view shows the coin | 2 | rows 9 coin column false err <nil> |
| glm-4.5-flash | T4 | news: 5 new items, each with coin BTC/ETH and a reason | 4 | no news table with coin |
| glm-4.5-flash | T4 | price chart: one line per coin (series_by) | 2 | series_by "" coins map[] err <nil> |
| glm-4.5-flash | T4 | price table shows both coins | 2 | coins map[] err <nil> |
| glm-4.5-flash | T4 | prices today: BTC 65000.5 and ETH 3200.25, one row each | 2 | no prices table |
| glm-4.5-flash | T4 | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 2 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| nemotron-3-ultra | T2 | btc_prices: one row today = 65000.5, seeds kept | 1 | today rows 1 price 0, older rows 0 |
| nemotron-3-ultra | T2 | search mentions bitcoin and every watch keyword | 1 | queries ["ETF OR halving OR regulation" "ETF OR halving OR regulation"], missing [] |
| nemotron-3-ultra | T3 | btc_price_chart: line chart, date on x, price on y, 10 points | 1 | 0 rows |
| nemotron-3-ultra | T3 | btc_price_table: 10 seeded prices newest first, From filter ~90 days, Refresh runs daily_btc | 1 | 0 rows |
| nemotron-3-ultra | T4 | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | -1 of 10 seeded rows found |
| nemotron-3-ultra | T4 | news: 5 new items, each with coin BTC/ETH and a reason | 1 | 10 new rows, 10 with coin and reason |
| nemotron-3-ultra | T4 | prices today: BTC 65000.5 and ETH 3200.25, one row each | 1 | BTC rows 1 (0), ETH rows 1 (0) |
| nemotron-3-ultra | T4 | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | missing price |

## Results, guide v2, single tasks (each from the reference state of the previous task)

| Model | Task | Runs | First-try valid | Valid in the end | Repair rounds (valid runs, mean) | Assertions passed | Prompt tok/run | Output tok/run | Requests/run | Peak request | Time/run | Not done |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| nemotron-3-ultra | T2 | 1 | 0% | 100% | 1.0 | 100% (10/10) | 21098 | 1639 | 4.0 | 5965 | 77 s |  |
| nemotron-3-ultra | T4 | 1 | 100% | 100% | 0.0 | 92% (12/13) | 18462 | 5008 | 3.0 | 7631 | 229 s |  |
| **nemotron-3-ultra** | **all** | 2 | **50%** | **100%** | 0.5 | **96%** (22/23) | 19780 | 3323 | 3.5 | 7631 | 153 s |  |
| gemma4:31b | T2 | 2 | 0% | 0% | – | 0% (0/2) | 28580 | 2509 | 5.0 | 6954 | 16 s | repair limit ×2 |
| gemma4:31b | T4 | 1 | 0% | 100% | 2.0 | 69% (9/13) | 34574 | 3665 | 5.0 | 9413 | 20 s |  |
| **gemma4:31b** | **all** | 3 | **0%** | **33%** | 2.0 | **60%** (9/15) | 30578 | 2894 | 5.0 | 9413 | 18 s | repair limit ×2 |
| glm-4.5-flash | T2 | 2 | 0% | 100% | 1.0 | 100% (20/20) | 22959 | 1345 | 5.0 | 5448 | 57 s |  |
| glm-4.5-flash | T4 | 2 | 50% | 100% | 1.0 | 58% (15/26) | 71043 | 3566 | 12.5 | 10792 | 129 s |  |
| **glm-4.5-flash** | **all** | 4 | **25%** | **100%** | 1.0 | **76%** (35/46) | 47001 | 2455 | 8.8 | 10792 | 93 s |  |

### Guide v2, chained conversation (T1..T4 in one chat, from an empty project)

| Model | Task | Runs | First-try valid | Valid in the end | Repair rounds (valid runs, mean) | Assertions passed | Prompt tok/run | Output tok/run | Requests/run | Peak request | Time/run | Not done |
|---|---|---|---|---|---|---|---|---|---|---|---|---|

### Guide v2: error categories (all error lines in failed write calls)

| Category | Errors | nemotron-3-ultra | gemma4:31b | glm-4.5-flash | Examples |
|---|---|---|---|---|---|
| bad refs | 25 (74%) | 0 | 20 | 5 | glm-4.5-flash T2 save_pipeline: `line 40:13 steps[4].with.rows: rows have a field "published", but table btc_news has no such column (columns: url, date, title, reason, _run_id, _fetched_at)`<br>glm-4.5-flash T2 save_pipeline: `line 40:13 steps[4].with.rows: rows have a field "snippet", but table btc_news has no such column (columns: url, date, title, reason, _run_id, _fetched_at)`<br>glm-4.5-flash T2 save_pipeline: `line 44:13 steps[5].with.rows: rows have a field "published", but table btc_news has no such column (columns: url, date, title, reason, _run_id, _fetched_at)` |
| dependent not updated | 5 (15%) | 0 | 1 | 4 | glm-4.5-flash T4 apply_migration: `stored view btc_price_chart (not sent with this migration) would break: query: SQLite: no such table: btc_prices`<br>glm-4.5-flash T4 apply_migration: `stored view btc_price_table (not sent with this migration) would break: query: SQLite: no such table: btc_prices`<br>glm-4.5-flash T4 apply_migration: `stored view btc_price_chart (not sent with this migration) would break: query: SQLite: no such table: btc_prices` |
| bad expression | 2 (6%) | 0 | 2 | 0 | gemma4:31b T2 save_pipeline: `line 30:16 steps[3].with.fields.query: unclosed ${{ (expressions are written as ${{ ... }})`<br>gemma4:31b T4 apply_migration: `pipelines[0]: line 54:14 steps[7].with.items: expression "map(steps.news_filter.each, .items) | flatten" does not compile: unexpected token EOF (1:45)` |
| yaml | 2 (6%) | 1 | 1 | 0 | gemma4:31b T4 apply_migration: `pipelines[0]: line 57:0 (top level): YAML: line 57: mapping values are not allowed in this context (a plain value on that line contains ": ", often a map literal inside ${{ }}; put the whole value in double quotes, or build the rows with transform.map)`<br>nemotron-3-ultra T2 save_pipeline: `line 45:0 (top level): YAML: line 45: mapping values are not allowed in this context (a plain value on that line contains ": ", often a map literal inside ${{ }}; put the whole value in double quotes, or build the rows with transform.map)` |

### Guide v2: failed assertions

| Model | Task | Assertion | Failed runs | First reason |
|---|---|---|---|---|
| gemma4:31b | T2 | pipeline daily_btc saved | 2 | not saved (saved: ) |
| gemma4:31b | T4 | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | -1 of 10 seeded rows found |
| gemma4:31b | T4 | news table has coin; old news are BTC | 1 | table btc_news: 0 of 4 seeded rows with coin BTC |
| gemma4:31b | T4 | news: 5 new items, each with coin BTC/ETH and a reason | 1 | 10 new rows, 10 with coin and reason |
| gemma4:31b | T4 | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | missing _run_id, _fetched_at |
| glm-4.5-flash | T4 | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 2 | 0 of 10 seeded rows found |
| glm-4.5-flash | T4 | mock run succeeds twice (rerun safe) | 1 | run 1 steps[2] (save_price) failed: db.insert_many: rows is not a list |
| glm-4.5-flash | T4 | news search covers both coins | 2 | queries ["ETF halving regulation news" "ETF halving regulation news"] |
| glm-4.5-flash | T4 | news table has coin; old news are BTC | 1 | table btc_news: -1 of 4 seeded rows with coin BTC |
| glm-4.5-flash | T4 | news table view shows the coin | 1 | rows 4 coin column false err <nil> |
| glm-4.5-flash | T4 | news: 5 new items, each with coin BTC/ETH and a reason | 1 | no news table with coin |
| glm-4.5-flash | T4 | price chart: one line per coin (series_by) | 1 | series_by "" coins map[BTC:true] err <nil> |
| glm-4.5-flash | T4 | price table shows both coins | 1 | coins map[BTC:true] err <nil> |
| glm-4.5-flash | T4 | prices today: BTC 65000.5 and ETH 3200.25, one row each | 1 | BTC rows 0 (0), ETH rows 0 (0) |
| nemotron-3-ultra | T4 | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | 0 of 10 seeded rows found |

