# SPIKE-025 results

Date: 2026-09-29. Harness: SPIKE-021 (validator, tasks T2-T4, reference states, mock runner, repair loop) plus the SPIKE-023 Mother fixture, in `spikes/025-skills/`. Models: nemotron-3-ultra and gemma4:31b via Ollama Cloud (one request at a time), glm-4.5-flash via Z.ai. Temperature 0.2, same prompts, tools and limits for every model (SPEC 1). 2 reps per cell; T-load under A 1 rep (cost only).

## Setup

**Conditions.**
- **A**: the whole guide v2 in the system prompt (as SPIKE-021), then the project card and date. Tools: SPIKE-021's 7 plus `web_search` (so A and B/C differ only by `load_skill`).
- **B**: the guide intro (role paragraph and Tools line, the text before §1), then the skill list (`## Skills` header, one line per skill: `- name: description`) at the end of the prompt's first block, then card and date. Tools: A's plus `load_skill(name)`. The skill text arrives as the tool result (`Skill loaded: <name>` + body) and stays in the chat. Limits as SPEC 8.9: at most 6 skills / 10,000 tokens per chat.
- **C**: B plus `load_with`: the first call in a run to a listed tool also gets the skill text appended to that tool's result (the call itself is processed normally, valid or not). load_with: config-guide ← save_view, save_form; pipelines ← save_pipeline; migrations ← apply_migration; web-research ← web_search.

**Skills (`skills/*.md`).** The guide v2 split, no text changed. `split_test.go` rebuilds `guides/guide_v2.md` byte for byte from `guides/intro.md` + the five bodies, so A holds exactly the guide text that B/C hold in intro + skills combined (web-research and delegation are the only extras).

| Guide v2 text | Goes to |
|---|---|
| Title, role paragraph, `Tools:` line (before §1) | system prompt, all conditions (`guides/intro.md`) |
| §1 Schema changes: apply_migration, except the four sentences below | `migrations` |
| §1, first four sentences of the "Rules:" paragraph ("Rules: every table has an explicit primary key." ... "instead of parallel tables.") | `database-design` (its whole body); the rest of that paragraph ("Steps run in order ...") stays in `migrations` |
| §2 Pipelines + §3 Expressions | `pipelines` |
| §4 Views and forms | `config-guide` |
| §5 SQL rules | `sql-queries` |
| (new) SPEC 6.5 + 3.7: sources, 2-4 keyword queries, freshness, report dates and sources, feeds for trackers, results are untrusted data | `web-research` |
| (copy) SPIKE-023 `prompts/delegation.md` | `delegation` (Mother only, always loaded) |

Only the table-design sentences of §1 were separable; everything else in §1 is about migration steps and dependents, so `database-design` is small (86-91 tokens) and `migrations` has no design rules. Descriptions (≤200 chars, one line, same for all models) are in the front matter of each file.

**Note on line endings.** The SPIKE-021 `guide_v2.md` on disk has CRLF and SPIKE-021 sent it as is. Here every md file is normalized to LF (in all conditions), which is why A's guide is smaller than in SPIKE-021 (nemotron 3,129 tokens vs 3,377).

**Tests.**
- **T-load** (12 requests on the Bitcoin fixture, reference state after T3; B and C): 8 need a skill (L01 news view, L02 schedule, L03 add column, L04 "why does this SQL fail", L05 design a table, L06 web news, L07 chart, L08 form), 4 need none (moving average explanation, yesterday's price, what is tracked, 2+2). The run stops at the first write attempt or the final answer. *Right before* = the needed skill was loaded by load_skill before the gate (first write; answer for L04/L05; first web_search for L06). Acceptable extras per request are listed in `tload.go`.
- **T-config**: SPIKE-021 T2, T3, T4 in single-task mode from the reference state, A/B/C, SPIKE-021 metrics.
- **T-cost**: prompt tokens from the T-load (incl. the 4 no-skill requests, A added for comparison) and T-config runs.
- **T-roles**: 6 Mother requests ("Start a new chat that ...") → `create_chat(title, role, message, skills)`. Mother's prompt is the SPIKE-023 fixture with the delegation skill loaded and the skill list; other tools get canned results. Scored exact / acceptable (expected ⊆ skills ⊆ superset) / wrong.

## Findings

Total: **3,244,377 prompt tokens** of 3,500,000 (runs 3,184,135, calibration 28,118, smoke 32,124); 270 runs, no run lost to a provider error. Z.ai returned 17 transient 429s ("service may be temporarily overloaded", code 1305) and a few timeouts/EOFs; all recovered on retry. Ollama returned no errors.

1. **Self-loading before writes works.** In the five write requests of T-load (L01, L02, L03, L07, L08) every model loaded the right skill before its first write, in B and C, both reps: 60/60 runs. T-config agrees: the task's main skill was loaded before the first write in 36/36 B/C runs.
2. **No over-loading.** 0 load_skill calls in 48 no-skill T-load runs, and 0 loads outside the acceptable set in 96 need-skill runs. The only extras were acceptable ones (gemma adds sql-queries to view and pipeline work).
3. **Two requests are missed by self-loading.**
   - *L04 "why does this SQL fail"*: gemma loaded sql-queries 4/4, nemotron 1/4, glm 0/4. The others called `query`, got the guard's own error ("rejected by the SQL guard: internal name _burrow_runs") and answered from it. The answer was not scored, so this may cost nothing in practice.
   - *L06 web news*: 0/12 runs loaded web-research before searching. Under C, load_with delivered it with the first web_search result in 6/6 runs. This is the only T-load case where C differs from B.
4. **load_with helps only where the model would not load anyway.** For writes it is redundant (models had loaded the skill already, so load_with never fired in T-load writes). In T-config, C vs B is mixed with no clear direction (nemotron: C worse first-try, 67% vs 83%; glm: C better, 33% vs 17% first-try, 70% vs 55% assertions; gemma: C 83% valid vs 67%). With n=2 per cell none of these differences is solid.
5. **load_with as listed in SPEC misses T4's dependents.** apply_migration brings only `migrations`; T4 (widen BTC to BTC+ETH) also needs pipelines and config-guide to update the dependent pipeline and views. nemotron and gemma loaded all of them by hand; glm skipped config-guide in 3/4 B/C runs and then failed the view assertions ("price chart: one line per coin", "price table shows both coins"), as it also did under A.
6. **Quality under B/C vs A (T-config):**
   - nemotron: valid 100% in all three; B had fewer repair rounds (0.2 vs 1.0) and 100% assertions vs 91% under A.
   - gemma: A best. T4 under B failed both runs (repair limit) and under C one of two, with the same `for_each` item-field errors (`item.item ... has no field "url"`) that it fixed under A. T3 under B/C missed the "From filter ~90 days" assertion 3 times (never under A). Same text in both conditions; the difference is where the text sits (tool result vs system prompt). Could be chance at n=2, but it is the one model where splitting looks harmful.
   - glm: valid 83% under B and C vs 67% under A; T4 is poor in every condition (request cap or repair limit, 23-38% assertions).
7. **Cost.** The fixed first request drops by about 2.5k tokens (4.3-4.7k under A to 1.7-2.2k under B/C). Chats that need no skill cost 53-62% less per run (e.g. nemotron 5.8k → 2.7k). Single-skill work is cheaper under B for T2 and T3 by 25-50% for every model (gemma T2 under C is the exception: 15.4k vs 15.1k because of a repair round). T4 loads 3-4 skills (≈ the whole guide) and is not cheaper: nemotron B 57k vs A 77k but C 85k; gemma B/C 46-52k vs A 30k (more repair rounds). Peak request is lower under B/C except in T4 runs with repairs. For need-skill T-load runs cost per run is about the same as A (nemotron slightly higher: 7.1-7.9k vs 6.5k; gemma/glm 10-20% lower), because the skill arrives as an extra round trip.
8. **T-roles: Mother picks the skills for simple roles, and misses skills for combined roles.** Acceptable 7/12 (nemotron), 8/12 (gemma), 7/12 (glm). R4 (SQL questions) and R5 (pipeline care) were right in 12/12 runs. Two requests failed in every run of every model:
   - R3 (watch gold news daily): 6/6 gave only web-research, never pipelines. The models read "watches daily" as the chat's own job, not as a scheduled pipeline.
   - R6 (design a trades table, fill it nightly, show it): 6/6 left out migrations. They read "design a table" as database-design only, although creating a table needs apply_migration.

   Both misses point at descriptions: `migrations` starts "Changing tables", and nothing says pipelines are how things run "every day". Proposed changes (not tested): migrations "Creating and changing tables: ..."; pipelines "... Load for anything that runs on a schedule (daily, hourly)". create_chat says the chat "can load more later", so the child chat may recover the missing skill; T-roles does not test that.

## Small n and other caveats

- 2 reps per cell (T-load B/C and T-roles: 2 per model, T-config: 2 per task and condition; T-load A: 1, for cost only). At temperature 0.2 reps are often token-identical (e.g. nemotron L01 B and C, 8,183 tokens each), so the effective n is lower than the run count. Read the T-config percentages as 0, 1 or 2 out of 2.
- T-load B and C are identical until the first write (load_with fires only when a listed tool is called, and the probe stops at the first write attempt), so B vs C in T-load only differs for L06. This is by design but means T-load cannot show whether load_with changes the write itself; T-config covers that.
- T-load does not score the answers or the configs, only whether the skill was loaded in time.

## Harness issues that may bias results

- **web_search is fake** (fixed synthetic results); `feed.read` is not in the SPIKE-021 validator, so web-research mentions feeds without naming that step.
- **Mother's other tools return canned results**, and the run stops at the first create_chat / send_to_chat / subagent. Mother always had delegation loaded, as the ticket asks. `skills` is a required create_chat parameter, which may push models to name skills they would otherwise leave out.
- **The builder's skill list also shows `delegation`** (Mother-only). It was never loaded, but it costs about 30 tokens per request.
- **The intro's `Tools:` line (guide text, unchanged)** does not mention web_search or load_skill; they are only in the tool definitions. This is the same in all conditions.
- **Line endings**: LF everywhere here, CRLF in SPIKE-021. A's numbers are not directly comparable with SPIKE-021's (A is about 250 tokens smaller here).
- **Assertion counts differ between conditions**: a run that stops at the repair limit before the pipeline is saved produces fewer checks (glm A: 47 instead of 56). Compare the valid rates, not only the assertion percentages.
- Several glm runs are marked valid but stopped at the request cap: they had saved everything and kept calling tools (describe/query) until the cap.
- Z.ai's `/models` list does not include glm-4.5-flash, but chat requests with it work (same as SPIKE-021).
- Budget counts smoke tests (32k) and calibration (28k).

<!-- generated by `go run . -report`; do not edit below -->

## Token budget

Prompt tokens: 3184135 in runs (all records, incl. replaced error runs) + 28118 in calibration + 32124 in smoke tests = **3244377 of 3,500,000**. Output tokens in runs: 223561. Run records: 270 (latest per combination: 270).

By test: T-load 964597, T-config 2109447, T-roles 110091.

## Sizes (tokens, each model's own tokenizer)

Measured as the prompt_tokens difference against a one-line system prompt (as SPIKE-021). Skill rows include the `Skill loaded: <name>` line that load_skill returns. `tools_BC − tools_A` is the cost of the load_skill definition.

| Text | Chars | nemotron-3-ultra | gemma4:31b | glm-4.5-flash |
|---|---|---|---|---|
| guide_v2 | 11371 | 3129 | 3319 | 3089 |
| intro | 805 | 195 | 197 | 188 |
| skill_list | 1438 | 325 | 338 | 316 |
| skill:config-guide | 2782 | 795 | 834 | 783 |
| skill:pipelines | 5267 | 1451 | 1568 | 1450 |
| skill:migrations | 1993 | 550 | 578 | 533 |
| skill:database-design | 361 | 86 | 91 | 85 |
| skill:sql-queries | 296 | 81 | 83 | 79 |
| skill:web-research | 1116 | 290 | 287 | 280 |
| skill:delegation | 730 | 176 | 189 | 176 |
| tools_A |  | 1137 | 702 | 858 |
| tools_BC |  | 1221 | 767 | 930 |

## T-load: does the chat load the right skill by itself?

*Right before* = every needed skill was loaded before the gating action (first write attempt; for L04/L05 the answer; for L06 the first web_search). *load_with covers* (C only) = the needed skill came with the gating tool's own result. *Unneeded* = load_skill calls outside the needed + acceptable set. Runs with a provider error are left out.

| Model | Cond | Need-skill runs: right before | loaded at all (incl. load_with) | load_with covers | Unneeded loads in need-skill runs | No-skill runs without any load | Unneeded loads in no-skill runs | Errors |
|---|---|---|---|---|---|---|---|---|
| nemotron-3-ultra | B | 12/16 (75%) | 12/16 | – | 0 | 8/8 | 0 | 0 |
| nemotron-3-ultra | C | 13/16 (81%) | 15/16 | 2/16 | 0 | 8/8 | 0 | 0 |
| gemma4:31b | B | 14/16 (88%) | 14/16 | – | 0 | 8/8 | 0 | 0 |
| gemma4:31b | C | 14/16 (88%) | 16/16 | 2/16 | 0 | 8/8 | 0 | 0 |
| glm-4.5-flash | B | 12/16 (75%) | 12/16 | – | 0 | 8/8 | 0 | 0 |
| glm-4.5-flash | C | 12/16 (75%) | 14/16 | 2/16 | 0 | 8/8 | 0 | 0 |

Per request (B and C pooled; `rb` = right before, runs; skills loaded by load_skill in brackets, `+w:` = came by load_with):

| Request | Need | Gate | nemotron-3-ultra | gemma4:31b | glm-4.5-flash |
|---|---|---|---|---|---|
| L01_news_view | config-guide | write | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] | 4/4 [B:config-guide+sql-queries; C:config-guide; B:config-guide+sql-queries; C:config-guide+sql-queries] | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] |
| L02_schedule | pipelines | write | 4/4 [B:pipelines; C:pipelines; B:pipelines; C:pipelines] | 4/4 [B:pipelines; C:pipelines; B:pipelines; C:pipelines] | 4/4 [B:pipelines; C:pipelines; B:pipelines; C:pipelines] |
| L03_add_column | migrations | write | 4/4 [B:migrations; C:migrations; B:migrations; C:migrations] | 4/4 [B:migrations; C:migrations; B:migrations; C:migrations] | 4/4 [B:migrations; C:migrations; B:migrations; C:migrations] |
| L04_sql_fails | sql-queries | answer | 1/4 [B:–; C:–; B:–; C:sql-queries] | 4/4 [B:sql-queries; C:sql-queries; B:sql-queries; C:sql-queries] | 0/4 [B:–; C:–; B:–; C:–] |
| L05_design_table | database-design | answer | 4/4 [B:database-design; C:database-design; B:database-design; C:database-design] | 4/4 [B:database-design; C:database-design; B:database-design; C:database-design] | 4/4 [B:database-design; C:database-design; B:database-design; C:database-design] |
| L06_etf_news | web-research | search | 0/4 [B:–; C:+w:web-research; B:–; C:+w:web-research] | 0/4 [B:–; C:+w:web-research; B:–; C:+w:web-research] | 0/4 [B:–; C:+w:web-research; B:–; C:+w:web-research] |
| L07_chart | config-guide | write | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] |
| L08_form | config-guide | write | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] | 4/4 [B:config-guide; C:config-guide; B:config-guide; C:config-guide] |
| L09_moving_average | none | answer | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] |
| L10_price_yesterday | none | answer | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] |
| L11_what_tracked | none | answer | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] |
| L12_two_plus_two | none | answer | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] | 4/4 [B:–; C:–; B:–; C:–] |

For no-skill requests the fraction counts runs with no load at all.

## T-config: SPIKE-021 T2, T3, T4 under A, B, C

Single-task mode from the reference state; metrics as SPIKE-021 (valid = every needed artifact saved and the last write of each OK; first-try = valid with no failed batch; repair rounds = failed batches in valid runs). *Skills* = loads per run (load_skill `s:`, load_with `w:`); *before 1st write* = the task's main skill (T2 pipelines, T3 config-guide, T4 migrations) was loaded before the first write call.

| Model | Task | Cond | Runs | First-try valid | Valid | Repair rounds (valid runs) | Assertions | Prompt tok/run | Requests/run | Peak request | Main skill before 1st write | Skills loaded | Not done |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| nemotron-3-ultra | T2 | A | 2 | 0% | 100% | 2.0 | 95% (19/20) | 27.7k | 5.0 | 6659 | – | – / – |  |
| nemotron-3-ultra | T2 | B | 2 | 50% | 100% | 0.5 | 100% (20/20) | 18.7k | 5.0 | 4927 | 2/2 | s:pipelines / s:pipelines |  |
| nemotron-3-ultra | T2 | C | 2 | 100% | 100% | 0.0 | 100% (20/20) | 13.9k | 4.0 | 4499 | 2/2 | s:pipelines / s:pipelines s:database-design |  |
| nemotron-3-ultra | T3 | A | 2 | 100% | 100% | 0.0 | 100% (10/10) | 38.7k | 7.0 | 6337 | – | – / – |  |
| nemotron-3-ultra | T3 | B | 2 | 100% | 100% | 0.0 | 100% (10/10) | 28.0k | 8.0 | 4351 | 2/2 | s:config-guide / s:config-guide |  |
| nemotron-3-ultra | T3 | C | 2 | 100% | 100% | 0.0 | 100% (10/10) | 29.5k | 8.5 | 4267 | 2/2 | s:config-guide / s:config-guide |  |
| nemotron-3-ultra | T4 | A | 2 | 50% | 100% | 1.0 | 85% (22/26) | 77.1k | 8.5 | 12774 | – | – / – |  |
| nemotron-3-ultra | T4 | B | 2 | 100% | 100% | 0.0 | 100% (26/26) | 57.0k | 8.0 | 9691 | 2/2 | s:migrations s:database-design s:pipelines s:config-guide / s:migrations s:pipelines s:config-guide |  |
| nemotron-3-ultra | T4 | C | 2 | 0% | 100% | 1.5 | 100% (26/26) | 85.2k | 9.5 | 13378 | 2/2 | s:migrations s:database-design s:pipelines s:config-guide / s:migrations s:database-design s:pipelines s:config-guide |  |
| nemotron-3-ultra | **all** | A | 6 | 50% | 100% | 1.0 | 91% (51/56) | 47.8k | 6.8 | 12774 | – |  |  |
| nemotron-3-ultra | **all** | B | 6 | 83% | 100% | 0.2 | 100% (56/56) | 34.6k | 7.0 | 9691 | 6/6 |  |  |
| nemotron-3-ultra | **all** | C | 6 | 67% | 100% | 0.5 | 100% (56/56) | 42.9k | 7.3 | 13378 | 6/6 |  |  |
| gemma4:31b | T2 | A | 2 | 0% | 100% | 1.0 | 100% (20/20) | 15.1k | 3.0 | 5709 | – | – / – |  |
| gemma4:31b | T2 | B | 2 | 100% | 100% | 0.0 | 100% (20/20) | 9.1k | 3.0 | 3976 | 2/2 | s:pipelines s:sql-queries / s:pipelines s:sql-queries |  |
| gemma4:31b | T2 | C | 2 | 0% | 100% | 1.0 | 100% (20/20) | 15.4k | 4.5 | 4599 | 2/2 | s:pipelines s:sql-queries / s:pipelines s:sql-queries |  |
| gemma4:31b | T3 | A | 2 | 50% | 100% | 0.5 | 100% (10/10) | 12.5k | 2.5 | 5466 | – | – / – |  |
| gemma4:31b | T3 | B | 2 | 100% | 100% | 0.0 | 80% (8/10) | 7.7k | 3.0 | 3444 | 2/2 | s:config-guide / s:config-guide |  |
| gemma4:31b | T3 | C | 2 | 100% | 100% | 0.0 | 90% (9/10) | 7.7k | 3.0 | 3431 | 2/2 | s:config-guide / s:config-guide |  |
| gemma4:31b | T4 | A | 2 | 0% | 100% | 1.5 | 73% (19/26) | 30.1k | 4.5 | 9641 | – | – / – |  |
| gemma4:31b | T4 | B | 2 | 0% | 0% | – | 15% (4/26) | 46.1k | 6.0 | 11744 | 2/2 | s:migrations s:database-design s:config-guide s:pipelines / s:migrations s:database-design s:config-guide s:pipelines | repair limit, repair limit |
| gemma4:31b | T4 | C | 2 | 0% | 50% | 3.0 | 50% (13/26) | 52.3k | 7.0 | 12018 | 2/2 | s:migrations s:database-design s:pipelines s:config-guide s:sql-queries / s:migrations s:database-design s:config-guide s:pipelines s:sql-queries | repair limit |
| gemma4:31b | **all** | A | 6 | 17% | 100% | 1.0 | 88% (49/56) | 19.2k | 3.3 | 9641 | – |  |  |
| gemma4:31b | **all** | B | 6 | 67% | 67% | 0.0 | 57% (32/56) | 20.9k | 4.0 | 11744 | 6/6 |  | repair limit, repair limit |
| gemma4:31b | **all** | C | 6 | 33% | 83% | 1.0 | 75% (42/56) | 25.1k | 4.8 | 12018 | 6/6 |  | repair limit |
| glm-4.5-flash | T2 | A | 2 | 0% | 50% | 4.0 | 82% (9/11) | 44.1k | 8.5 | 6981 | – | – / – | repair limit |
| glm-4.5-flash | T2 | B | 2 | 50% | 100% | 1.5 | 75% (15/20) | 27.3k | 7.5 | 5964 | 2/2 | s:pipelines / s:pipelines |  |
| glm-4.5-flash | T2 | C | 2 | 0% | 100% | 1.5 | 100% (20/20) | 29.7k | 8.0 | 6005 | 2/2 | s:pipelines / s:pipelines |  |
| glm-4.5-flash | T3 | A | 2 | 0% | 100% | 2.0 | 100% (10/10) | 55.3k | 11.0 | 5947 | – | – / – |  |
| glm-4.5-flash | T3 | B | 2 | 0% | 100% | 2.5 | 100% (10/10) | 43.5k | 13.0 | 4602 | 2/2 | s:config-guide / s:config-guide |  |
| glm-4.5-flash | T3 | C | 2 | 50% | 100% | 1.0 | 100% (10/10) | 37.3k | 11.5 | 4601 | 2/2 | s:config-guide / s:config-guide |  |
| glm-4.5-flash | T4 | A | 2 | 0% | 50% | 2.0 | 38% (10/26) | 89.9k | 14.0 | 13015 | – | – / – | repair limit |
| glm-4.5-flash | T4 | B | 2 | 0% | 50% | 3.0 | 23% (6/26) | 82.6k | 16.0 | 12018 | 2/2 | s:migrations s:pipelines / s:migrations s:pipelines | request cap, request cap |
| glm-4.5-flash | T4 | C | 2 | 50% | 50% | 0.0 | 35% (9/26) | 73.3k | 16.0 | 8898 | 2/2 | s:migrations s:pipelines / s:migrations s:pipelines s:config-guide | request cap, request cap |
| glm-4.5-flash | **all** | A | 6 | 0% | 67% | 2.5 | 62% (29/47) | 63.1k | 11.2 | 13015 | – |  | repair limit, repair limit |
| glm-4.5-flash | **all** | B | 6 | 17% | 83% | 2.2 | 55% (31/56) | 51.1k | 12.2 | 12018 | 6/6 |  | request cap, request cap |
| glm-4.5-flash | **all** | C | 6 | 33% | 83% | 1.0 | 70% (39/56) | 46.8k | 11.8 | 8898 | 6/6 |  | request cap, request cap |

### T-config: failed assertions

| Model | Task | Cond | Assertion | Failed runs | First reason |
|---|---|---|---|---|---|
| gemma4:31b | T3 | B | btc_price_table: 10 seeded prices newest first, From filter ~90 days, Refresh runs daily_btc | 2 | no date filter defaulting to ~90 days ago |
| gemma4:31b | T3 | C | btc_price_table: 10 seeded prices newest first, From filter ~90 days, Refresh runs daily_btc | 1 | no date filter defaulting to ~90 days ago |
| gemma4:31b | T4 | A | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 2 | 0 of 10 seeded rows found |
| gemma4:31b | T4 | A | news table has coin; old news are BTC | 2 | table btc_news: 0 of 4 seeded rows with coin BTC |
| gemma4:31b | T4 | A | news: 5 new items, each with coin BTC/ETH and a reason | 2 | 2 new rows, 0 with coin and reason |
| gemma4:31b | T4 | A | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | missing _run_id, _fetched_at |
| gemma4:31b | T4 | B | all 10 BTC rows copied with coin BTC | 2 | no prices table |
| gemma4:31b | T4 | B | btc_prices dropped | 2 | btc_prices still exists |
| gemma4:31b | T4 | B | fetches the BTC and the ETH URL | 2 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| gemma4:31b | T4 | B | news search covers both coins | 2 | queries ["bitcoin ETF halving regulation news" "bitcoin ETF halving regulation news"] |
| gemma4:31b | T4 | B | news table has coin; old news are BTC | 2 | table btc_news: -1 of 4 seeded rows with coin BTC |
| gemma4:31b | T4 | B | news table view shows the coin | 2 | rows 9 coin column false err <nil> |
| gemma4:31b | T4 | B | news: 5 new items, each with coin BTC/ETH and a reason | 2 | no news table with coin |
| gemma4:31b | T4 | B | price chart: one line per coin (series_by) | 2 | series_by "" coins map[] err <nil> |
| gemma4:31b | T4 | B | price table shows both coins | 2 | coins map[] err <nil> |
| gemma4:31b | T4 | B | prices today: BTC 65000.5 and ETH 3200.25, one row each | 2 | no prices table |
| gemma4:31b | T4 | B | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 2 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| gemma4:31b | T4 | C | all 10 BTC rows copied with coin BTC | 1 | no prices table |
| gemma4:31b | T4 | C | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | 0 of 10 seeded rows found |
| gemma4:31b | T4 | C | btc_prices dropped | 1 | btc_prices still exists |
| gemma4:31b | T4 | C | fetches the BTC and the ETH URL | 1 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| gemma4:31b | T4 | C | news search covers both coins | 1 | queries ["bitcoin ETF halving regulation news" "bitcoin ETF halving regulation news"] |
| gemma4:31b | T4 | C | news table has coin; old news are BTC | 1 | table btc_news: -1 of 4 seeded rows with coin BTC |
| gemma4:31b | T4 | C | news table view shows the coin | 1 | rows 9 coin column false err <nil> |
| gemma4:31b | T4 | C | news: 5 new items, each with coin BTC/ETH and a reason | 2 | no news table with coin |
| gemma4:31b | T4 | C | price chart: one line per coin (series_by) | 1 | series_by "" coins map[] err <nil> |
| gemma4:31b | T4 | C | price table shows both coins | 1 | coins map[] err <nil> |
| gemma4:31b | T4 | C | prices today: BTC 65000.5 and ETH 3200.25, one row each | 1 | no prices table |
| gemma4:31b | T4 | C | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| glm-4.5-flash | T2 | A | pipeline daily_btc saved | 1 | not saved (saved: ) |
| glm-4.5-flash | T2 | A | search mentions bitcoin and every watch keyword | 1 | queries ["Bitcoin news" "Bitcoin news"], missing [ETF halving regulation] |
| glm-4.5-flash | T2 | B | btc_news: 5 new rows with url, title, date, reason (no duplicates after rerun) | 1 | 0 new rows, 0 complete |
| glm-4.5-flash | T2 | B | btc_prices row has _run_id (runtime fill) | 1 | row without _run_id |
| glm-4.5-flash | T2 | B | btc_prices: one row today = 65000.5, seeds kept | 1 | today rows 0 price 0, older rows 10 |
| glm-4.5-flash | T2 | B | mock run succeeds twice (rerun safe) | 1 | run 1 steps[1] (prepare_price_data) failed: transform.map: items is not a list |
| glm-4.5-flash | T2 | B | search mentions bitcoin and every watch keyword | 1 | queries [], missing [ETF halving regulation] |
| glm-4.5-flash | T4 | A | all 10 BTC rows copied with coin BTC | 1 | no prices table |
| glm-4.5-flash | T4 | A | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | 0 of 10 seeded rows found |
| glm-4.5-flash | T4 | A | btc_prices dropped | 1 | btc_prices still exists |
| glm-4.5-flash | T4 | A | fetches the BTC and the ETH URL | 1 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| glm-4.5-flash | T4 | A | news search covers both coins | 2 | queries ["ETF halving regulation news" "ETF halving regulation news"] |
| glm-4.5-flash | T4 | A | news table has coin; old news are BTC | 1 | table btc_news: -1 of 4 seeded rows with coin BTC |
| glm-4.5-flash | T4 | A | news table view shows the coin | 2 | rows 9 coin column false err <nil> |
| glm-4.5-flash | T4 | A | news: 5 new items, each with coin BTC/ETH and a reason | 1 | no news table with coin |
| glm-4.5-flash | T4 | A | price chart: one line per coin (series_by) | 2 | series_by "" coins map[] err <nil> |
| glm-4.5-flash | T4 | A | price table shows both coins | 2 | coins map[] err <nil> |
| glm-4.5-flash | T4 | A | prices today: BTC 65000.5 and ETH 3200.25, one row each | 1 | no prices table |
| glm-4.5-flash | T4 | A | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| glm-4.5-flash | T4 | B | all 10 BTC rows copied with coin BTC | 1 | no prices table |
| glm-4.5-flash | T4 | B | btc_prices dropped | 1 | btc_prices still exists |
| glm-4.5-flash | T4 | B | fetches the BTC and the ETH URL | 2 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| glm-4.5-flash | T4 | B | mock run succeeds twice (rerun safe) | 1 | run 1 steps[1] (price_data) failed: GET https://api.example.com/etf/daily: status 404 |
| glm-4.5-flash | T4 | B | news search covers both coins | 2 | queries ["bitcoin ETF halving regulation news" "bitcoin ETF halving regulation news"] |
| glm-4.5-flash | T4 | B | news table has coin; old news are BTC | 2 | table btc_news: -1 of 4 seeded rows with coin BTC |
| glm-4.5-flash | T4 | B | news table view shows the coin | 2 | rows 9 coin column false err <nil> |
| glm-4.5-flash | T4 | B | news: 5 new items, each with coin BTC/ETH and a reason | 2 | no news table with coin |
| glm-4.5-flash | T4 | B | price chart: one line per coin (series_by) | 2 | series_by "" coins map[] err <nil> |
| glm-4.5-flash | T4 | B | price table shows both coins | 2 | coins map[] err <nil> |
| glm-4.5-flash | T4 | B | prices today: BTC 65000.5 and ETH 3200.25, one row each | 2 | no prices table |
| glm-4.5-flash | T4 | B | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| glm-4.5-flash | T4 | C | all 10 BTC rows copied with coin BTC | 1 | no prices table |
| glm-4.5-flash | T4 | C | btc_prices dropped | 1 | btc_prices still exists |
| glm-4.5-flash | T4 | C | fetches the BTC and the ETH URL | 2 | fetched [https://api.example.com/btc/daily https://api.example.com/btc/daily] |
| glm-4.5-flash | T4 | C | news search covers both coins | 2 | queries ["ETF halving regulation news" "ETF halving regulation news"] |
| glm-4.5-flash | T4 | C | news table has coin; old news are BTC | 1 | table btc_news: -1 of 4 seeded rows with coin BTC |
| glm-4.5-flash | T4 | C | news table view shows the coin | 1 | rows 9 coin column false err <nil> |
| glm-4.5-flash | T4 | C | news: 5 new items, each with coin BTC/ETH and a reason | 2 | 20 new rows, 20 with coin and reason |
| glm-4.5-flash | T4 | C | price chart: one line per coin (series_by) | 2 | series_by "" coins map[BTC:true] err <nil> |
| glm-4.5-flash | T4 | C | price table shows both coins | 2 | coins map[BTC:true] err <nil> |
| glm-4.5-flash | T4 | C | prices today: BTC 65000.5 and ETH 3200.25, one row each | 2 | BTC rows 1 (65000.5), ETH rows 0 (0) |
| glm-4.5-flash | T4 | C | prices(coin, date, price): key (coin, date), _run_id/_fetched_at | 1 | no prices table (tables: btc_news, btc_prices, watch_keywords) |
| nemotron-3-ultra | T2 | A | search mentions bitcoin and every watch keyword | 1 | queries ["ETF OR halving OR regulation" "ETF OR halving OR regulation"], missing [] |
| nemotron-3-ultra | T4 | A | all 10 BTC rows copied with coin BTC (price and _run_id kept) | 1 | 0 of 10 seeded rows found |
| nemotron-3-ultra | T4 | A | news table has coin; old news are BTC | 1 | table news: 0 of 4 seeded rows with coin BTC |
| nemotron-3-ultra | T4 | A | news: 5 new items, each with coin BTC/ETH and a reason | 2 | 10 new rows, 10 with coin and reason |

## T-cost: prompt tokens per request and per run

*First request* = system prompt + tools + user message (the fixed cost of a chat turn). T-load runs stop at the first write attempt or the final answer.

| Model | Cond | T-load need-skill: first request / per request / per run | T-load no-skill: first request / per request / per run | T-config: first request / per run (T2, T3, T4) |
|---|---|---|---|---|
| nemotron-3-ultra | A | 4.7k / 4.7k / 6.5k (n=8) | 4.6k / 4.7k / 5.8k (n=4) | 4.7k / 27.7k, 38.7k, 77.1k |
| nemotron-3-ultra | B | 2.1k / 2.6k / 7.1k (n=16) | 2.1k / 2.1k / 2.7k (n=8) | 2.2k / 18.7k, 28.0k, 57.0k |
| nemotron-3-ultra | C | 2.1k / 2.7k / 7.9k (n=16) | 2.1k / 2.1k / 2.7k (n=8) | 2.2k / 13.9k, 29.5k, 85.2k |
| gemma4:31b | A | 4.4k / 4.5k / 5.6k (n=8) | 4.4k / 4.4k / 5.5k (n=4) | 4.5k / 15.1k, 12.5k, 30.1k |
| gemma4:31b | B | 1.7k / 2.1k / 4.5k (n=16) | 1.7k / 1.7k / 2.1k (n=8) | 1.7k / 9.1k, 7.7k, 46.1k |
| gemma4:31b | C | 1.7k / 2.1k / 4.6k (n=16) | 1.7k / 1.7k / 2.1k (n=8) | 1.7k / 15.4k, 7.7k, 52.3k |
| glm-4.5-flash | A | 4.3k / 4.4k / 8.2k (n=8) | 4.3k / 4.3k / 6.5k (n=4) | 4.3k / 44.1k, 55.3k, 89.9k |
| glm-4.5-flash | B | 1.8k / 2.3k / 6.6k (n=16) | 1.7k / 1.8k / 2.7k (n=8) | 1.8k / 27.3k, 43.5k, 82.6k |
| glm-4.5-flash | C | 1.8k / 2.3k / 7.4k (n=16) | 1.7k / 1.8k / 2.7k (n=8) | 1.8k / 29.7k, 37.3k, 73.3k |

## T-roles: does Mother give a new role chat the right skills?

*exact* = the skills equal the expected set; *acceptable* = expected ⊆ skills ⊆ acceptable superset; *wrong* = a needed skill missing, an extra outside the superset, an unknown name, or no create_chat.

| Model | Runs | Exact | Acceptable (incl. exact) | Wrong | No create_chat | Errors |
|---|---|---|---|---|---|---|
| nemotron-3-ultra | 12 | 4/12 | 7/12 | 5/12 | 0 | 0 |
| gemma4:31b | 12 | 4/12 | 8/12 | 4/12 | 0 | 0 |
| glm-4.5-flash | 12 | 6/12 | 7/12 | 5/12 | 0 | 0 |

| Request | Expected | Acceptable superset | nemotron-3-ultra | gemma4:31b | glm-4.5-flash |
|---|---|---|---|---|---|
| R1_redesign_tables | database-design, migrations | database-design, migrations, sql-queries, pipelines, config-guide | r1 acceptable: config-guide, database-design, migrations, pipelines<br>r2 exact: database-design, migrations | r1 acceptable: database-design, migrations, sql-queries<br>r2 acceptable: database-design, migrations, sql-queries | r1 exact: database-design, migrations<br>r2 wrong: database-design |
| R2_dashboards | config-guide | config-guide, sql-queries | r1 acceptable: config-guide, sql-queries<br>r2 wrong: config-guide, database-design, sql-queries | r1 acceptable: config-guide, sql-queries<br>r2 acceptable: config-guide, sql-queries | r1 exact: config-guide<br>r2 acceptable: config-guide, sql-queries |
| R3_gold_news_daily | pipelines, web-research | pipelines, web-research, database-design, migrations, sql-queries | r1 wrong: web-research<br>r2 wrong: web-research | r1 wrong: web-research<br>r2 wrong: web-research | r1 wrong: web-research<br>r2 wrong: web-research |
| R4_sql_questions | sql-queries | sql-queries, config-guide | r1 exact: sql-queries<br>r2 exact: sql-queries | r1 exact: sql-queries<br>r2 exact: sql-queries | r1 exact: sql-queries<br>r2 exact: sql-queries |
| R5_pipeline_care | pipelines | pipelines, sql-queries | r1 exact: pipelines<br>r2 acceptable: pipelines, sql-queries | r1 exact: pipelines<br>r2 exact: pipelines | r1 exact: pipelines<br>r2 exact: pipelines |
| R6_exchange_trades | database-design, migrations, pipelines, config-guide | database-design, migrations, pipelines, config-guide, sql-queries | r1 wrong: config-guide, database-design, pipelines<br>r2 wrong: config-guide, database-design, pipelines | r1 wrong: config-guide, database-design, pipelines, sql-queries<br>r2 wrong: config-guide, database-design, pipelines, sql-queries | r1 wrong: database-design, pipelines<br>r2 wrong: config-guide, database-design, pipelines |

Mother's own load_skill calls in T-roles: nemotron-3-ultra R1 r1: database-design; glm-4.5-flash R1 r2: database-design.

## Provider errors (runs left out or rerun)

None.
