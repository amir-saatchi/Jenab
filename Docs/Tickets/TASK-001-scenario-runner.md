# TASK-001 — Scenario runner
**Type:** Task
**Status:** Open
**Gate:** 3 (built in Phase 1)

## Goal
A headless tool that tests agent behaviour across models on the real orchestrator (SPEC 8.4). It is not a separate playground. The same scenarios run on every model in the test set.

## Scope
- **Scenario file (YAML):**
  - a fixture project
  - scripted user messages, with an optional `at` time for messages sent during a turn
  - fake or recorded tool results, with delays
  - assertions
- **Assertions, first set:**
  - `text_before_tool`
  - `tool_called` / `tool_not_called`
  - `config_valid` (SPEC 10)
  - `max_repair_rounds`
  - `max_prompt_tokens`
  - `answer_contains`
  - `no_repeat` (the finish turn doesn't repeat earlier text)
  - `max_status_polls` (no repeated `task_status` / `run_status` calls for the same ID; SPIKE-023)
  - `answer_delivered` (the user gets the rest of the answer, also when the request cap is hit)
  - `skill_loaded_before` (a skill is loaded before a given tool call or the answer; SPIKE-025)
- **Models:**
  - the free development models (SPIKE-018)
  - any frontier model the developer has a key for, from the keychain
  - Keys follow SPEC 6.7 and are never written to reports.
- **Report per model:**
  - pass rate per assertion, repair rounds, prompt and output tokens, time
  - a diff against the previous run
- **Scenario sets to start with:**
  - SPIKE-021's config tasks (T1–T4)
  - SPIKE-023's requests with several parts and Mother's delegation, as a standing set (`spikes/023-multi-part-requests/scenarios/`)
  - SPIKE-025's skill-loading requests, Mother's role-skill requests and T4 under skills, as a standing set (`spikes/025-skills/`). T4 on gemma runs with at least 5 reps before Phase 2.
  - The Phase 2 benchmark (PROPOSAL 11) is added later.
- **Where it runs:** from the command line and in CI. Paid models only run on request.

## Done when
`jenab-scenarios run <dir>` runs the SPIKE-021, SPIKE-023 and SPIKE-025 sets on at least two models and writes a report. A change to the guide or system prompt can be checked on every model with one command.

The turn inspector (SPEC 8.4) is a separate Phase 1 task.
