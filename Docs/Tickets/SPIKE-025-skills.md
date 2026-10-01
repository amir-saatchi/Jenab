# SPIKE-025 — Do models load the right skills?
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can skills (SPEC 8.9) replace the guide that is always in the prompt, on the test models, with the same prompts for all (SPEC 1)?
1. Does a chat load the right skill by itself, from the skill list?
2. Does the `load_with` trigger make up for models that don't?
3. Are configs as valid with skills as with the whole guide in the prompt, and how many tokens does it save?
4. Does Mother pick the right skills when it creates a role chat?

## Setup
- **Models:** the SPIKE-018 development models (nemotron-3-ultra, gemma4:31b, glm-4.5-flash). Same prompts for all.
- **Harness:** the SPIKE-021 harness, validator and tasks, in `spikes/025-skills/`. SPIKE-023 scenarios where useful.
- **Skills:**
  - The SPIKE-021 guide v2, split without changes into `config-guide`, `pipelines`, `migrations`, `database-design` and `sql-queries`.
  - Plus `web-research` (new, from SPEC 6.5) and `delegation` (the SPIKE-023 guidance v1), so the list has 7 skills.
- **Conditions:**
  - **A:** the whole guide in the system prompt, as in SPIKE-021 (the baseline).
  - **B:** only the skill list, plus `load_skill`.
  - **C:** B plus `load_with`.

## Tests
1. **Self-loading:** about 12 short requests, 8 that need a skill (e.g. "add a view of the news", "why does this query fail?") and 4 that don't ("what is a moving average?"). Under B and C:
   - Is the right skill loaded before the first action it's needed for?
   - Are unneeded skills loaded?
2. **Config quality:** SPIKE-021 T2 (pipeline), T3 (views) and T4 (add ETH: migration plus dependents) under A, B and C. Measured: first-try valid, valid after repairs, repair rounds, prompt tokens per task.
3. **Cost:** prompt tokens per request and per run under A, B and C, also for requests that write no config.
4. **Role skills:** about 6 Mother requests that create a role chat (e.g. "a chat that redesigns the tables"). Does `create_chat` get the right skills?

## Done when
Each test has pass rates per model and condition. There is a decision on:
- whether skills replace the guide in the prompt
- whether `load_with` is needed
- the skill list format and the skill descriptions

## Result
Run on 2026-09-29: 270 runs, 2 reps per cell, 3.24M of 3.5M prompt tokens. No run was lost to a provider error. Details are in `spikes/025-skills/results.md`.

The guide v2 was split without text changes; a test rebuilds it byte for byte from the skills.

| Text | Tokens (nemotron / gemma / glm) |
|---|---|
| Whole guide v2 | 3,129 / 3,319 / 3,089 |
| Skill list (7 skills) | 325 / 338 / 316 |
| Largest skill, `pipelines` | 1,451 / 1,568 / 1,450 |

1. **Self-loading (T-load):** the right skill was loaded before the action that needed it.

   | Model | B | C |
   |---|---|---|
   | nemotron | 12/16 | 13/16 |
   | gemma | 14/16 | 14/16 |
   | glm | 12/16 | 12/16 |

   - **Writes:** every model loaded the right skill before its first write in 60/60 runs.
   - **Extras:** no model loaded a skill it didn't need, in 144 runs.
   - **Misses:**
     - Web news: 0/12 runs loaded `web-research` before searching. Its description is the only one without "Load before …".
     - "Why does this SQL fail": only gemma loaded `sql-queries`. The other models answered from the SQL guard's own error message.
2. **`load_with`:** it only mattered where models didn't load a skill themselves. On `web_search` it delivered `web-research` in 6/6 runs. On writes it never fired, because the skill was already loaded. In T-config, C against B shows no clear direction.
3. **Config quality (T-config, T2–T4; valid, then first-try valid, then prompt tokens per run):**

   | Model | A (whole guide) | B (skills) | C (skills + `load_with`) |
   |---|---|---|---|
   | nemotron | 6/6 · 3/6 · 47.8k | 6/6 · 5/6 · 34.6k | 6/6 · 4/6 · 42.9k |
   | gemma | 6/6 · 1/6 · 19.2k | 4/6 · 4/6 · 20.9k | 5/6 · 2/6 · 25.1k |
   | glm | 4/6 · 0/6 · 63.1k | 5/6 · 1/6 · 51.1k | 5/6 · 2/6 · 46.8k |

   - **nemotron and glm:** as good as or better than A.
   - **gemma, the one concern:** T4 (widen to ETH) failed 2/2 under B and 1/2 under C, with `for_each` errors it fixed under A. At n=2 this may be chance.
   - **T4 dependents:** it needs `migrations`, `pipelines` and `config-guide`. glm often skipped `config-guide` and failed the view checks, as it also did under A.
4. **Cost:**
   - The first request of every turn drops from 4.3–4.7k tokens to 1.7–2.2k.
   - Chats that need no skill cost 54–61% less per run.
   - T2 and T3 cost 25–50% less.
   - T4 loads 3–4 skills, about the whole guide, and costs no less.
5. **Role skills (T-roles):** Mother gave acceptable skills in 7–8 of 12 runs per model. Simple roles (SQL questions, pipeline care) were right in 12/12. Two combined roles failed in every run:
   - "Watch gold news daily": `pipelines` was missing.
   - "Design a trades table, fill it nightly": `migrations` was missing.

   Both misses point at the descriptions. `migrations` starts with "Changing tables", and nothing says pipelines are how things run on a schedule.

**Caveats:**
- n is small, and reps at temperature 0.2 are often identical.
- T-load checks only when skills load; it doesn't score the answers.
- `web_search` and Mother's other tools return canned results.
- `skills` was a required `create_chat` parameter.
- The builder's skill list also showed `delegation`, which is for Mother only.

## Decision
**Decided (2026-09-29):**
- **Skills replace the guide in the prompt.** They are as good or better on two of three models, and much cheaper per turn.
  - The system prompt keeps the role paragraph and the tool list, plus the skill list.
  - gemma's T4 result is the open risk. It joins the scenario-runner set (TASK-001) with at least 5 reps, and must pass before Phase 2, where migrations arrive.
- **Keep `load_with` as a safety net.** It adds a skill only if the chat hasn't loaded it yet, so it costs nothing when models load skills themselves.
  - It stays on the SPEC 8.9 tools (`web_search` and `read_feed` are where it mattered).
  - `request_schema_change` also brings `pipelines` and `config-guide` when the change has dependent pipelines or views.
- **Skill list format:** one line per skill, `- name: description`, at the end of block 1. It takes about 45 tokens per skill.
  - The list shows only the skills a chat can use, so `delegation` appears in Mother only.
- **Descriptions:** every description says when to load the skill: "Load before `<tool>`" or "Load to …".
  - `migrations`: "Creating and changing tables …"
  - `pipelines`: "… Load for anything that runs on a schedule (daily, hourly)."
  - `web-research`: "Load before `web_search` or `read_feed`."
  - These wordings are untested. The scenario runner rechecks T-load L06 and T-roles R3/R6 with them.
- **Role skills:** `create_chat(..., skills)` stays, and `skills` becomes optional. The child chat has its own skill list and `load_skill`, so it can load a skill Mother missed.
- **Small skills:** `database-design` (about 90 tokens) and `sql-queries` (about 80) stay separate. `database-design` gets the SPEC 8.2 table guidelines in Phase 2.

Doc changes (made in SPEC v0.6): 8.9 (no longer a draft; skill list, descriptions, `load_with` rules, results), 8.6 (`skills` in `create_chat` is optional). TASK-001 gets the SPIKE-025 set.
