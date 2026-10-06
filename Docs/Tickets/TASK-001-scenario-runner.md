# TASK-001 — Scenario runner
**Type:** Task
**Status:** Done
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

This also closes P1-12's last check: SPIKE-025's skill-loading set passes on two models.

The turn inspector (SPEC 8.4) is a separate Phase 1 task.

## Result
- **`jenab-scenarios`** (`cmd/jenab-scenarios`): `run`, `check` and `list`. A dir is a set or a folder of sets.
  - `run` takes `-model`, `-scenario`, `-reps`, `-scale`, `-timeout`, `-parallel`, `-out` and `-keep`.
  - Providers come from a models file (`scenarios/models.example.yaml`). Each one names the environment variable for its key, and `-env` reads a .env file. Nothing is built in; models of a `paid` provider run only when named.
  - Keys are never printed, and reports and transcripts are scanned for them and redacted.
- **`internal/scenario`:** each run gets a new project and the real orchestrator, system prompt, skills and built-in tools. Fake tools from the set's files replace the tools the app doesn't have yet. Tools of kind `sql`, `describe` and `config` read a fixture database and stored configs; the rest answer from rules, with delays.
  - Messages with `at` are sent during a turn. Cards and forms are answered like a user without an opinion: approve once, pick the recommended option.
  - Scenarios that need what the app doesn't have yet are marked `needs: [background]` and skipped with that reason.
- **Assertions:** `text_before_tool`, `tool_called`, `tool_not_called`, `max_calls`, `max_prompt_tokens`, `answer_contains`, `answer_delivered`, `no_repeat`, `max_status_polls`, `skill_loaded`, `skill_loaded_before`, `only_skills`, `created_skills`, `after_message`, `no_background`, `max_turns` and `no_false_running`. A check with `test: info` is reported but never fails a run.
- **Report** per set in `<set>/results/`: runs passed, checks per model, tokens and time per run, a table of scenarios, the changes since the last report, the failures, and one transcript per run.
- **Agent addition:** `agent.Deps.Card`, the project card as block 4 (SPEC 3.1). Phase 2 builds it from the schema; the runner gives a fixture's.
- **Sets** in `scenarios/`: `multi-part` (SPIKE-023, 20 scenarios, 9 run now), `skill-loading` (SPIKE-025 T-load, 12) and `role-skills` (SPIKE-025 T-roles, 6).
- **First run,** 2026-10-06, on gemma4:31b (Ollama Cloud) and glm-4.5-flash (Z.ai), 1 rep:

  | Set | gemma4:31b | glm-4.5-flash |
  |---|---|---|
  | multi-part | 9/9 | 9/9 |
  | skill-loading | 11/12 | 11/12 |
  | role-skills | 4/6 | 4/6 |

  - skill-loading: the skill was loaded before the action in 7/7 (gemma) and 6/7 (glm) runs, and no skill was loaded when none was needed in 8/8. This closes P1-12's last check.
  - glm's L04 and both models' R3 and R6 failed as in SPIKE-025: glm answered "why does this SQL fail" without sql-queries, R3 got no pipelines and R6 no migrations.
  - gemma's L02 broke: after its one save it repeated the same `save_pipeline` call 17 times, until the truncation guard stopped the turn ("read 4941 input tokens of about 9937"). L03 repeated the same way until the request cap. The provider's prompt count grew by about 18 tokens per request while each call was about 435, so Ollama Cloud may drop the earlier tool calls from what gemma sees. SPIKE-025 stopped at the first write, so it never saw this.
- **Checks:** a run on `provider/fake` covers the card, the skill list, the fake tools on the fixture, the checks, the report, the diff and that no key reaches a file. Further tests cover a Mother run with a message during the turn, a timeout, model and .env selection, 22 load errors and the command. A test loads every set in the repo.
- **Not done:**
  - SPIKE-021's config tasks (T1�T4), `config_valid` and `max_repair_rounds` come with Phase 2, when the app checks configs; the user's decision. So does T4 on gemma with 5 reps.
  - The 11 SPIKE-023 scenarios that need background work run when the app has it (8.3); their checks are kept in the files.
  - In CI, `go test` loads every set. A run on models needs the keys as secrets, and Actions are blocked for now.
