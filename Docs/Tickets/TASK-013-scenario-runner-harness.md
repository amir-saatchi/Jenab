# TASK-013 — Scenario runner: measure the harness, not only the model
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-001

## Goal
The scenario runner shows whether a harness change made things better or worse: cache use, request counts and stops, in any language, without API keys where possible. Found in the harness review of 2026-10-10.

## Today
- **No offline mode.** The CLI runs real models only (`cmd/jenab-scenarios/main.go:208`). A scripted model exists only in Go tests (`scenario/run_test.go`, `acceptance_test.go`). CI is blocked, so changes go unchecked unless someone runs the sets with keys.
- **No settings per scenario.** `Options.Settings` exists, but neither YAML nor the CLI sets it (`scenario/run.go:41`). So the request cap and history limits can't be tested.
- **Cache use is invisible.** Input, cache-read and cache-write tokens are added into one number (`scenario/record.go:117`), though the context is designed around a stable prefix (SPEC 3.1).
- **No checks on harness events.** Nothing checks for a cap hit, a bad-call stop, cuts or retries. `Run.Requests` is recorded, but there is no `max_requests` check.
- **Reps without spread.** The report gives pass counts and means only. The diff compares with the newest report, not a chosen baseline (`report.go:224-227`).
- **English-only checks:**
  - `no_false_running` matches English phrases (`assert.go:67`).
  - `skill_loaded` parses the chip's English text (`record.go:81`).
- **`after_message` never fails.** It doesn't fail when the message lands between turns (`assert.go:425-438`), so the t3 scenarios may not test joining a turn at all.
- **Skipped scenarios aren't checked.** In skipped (`needs`) scenarios, unknown assertion types aren't checked (`scenario.go:415-417`). This covers 11 of the 20 multi-part scenarios.

## Scope
In order:
1. **Tokens.** The record keeps input, cache read, cache write and output per request. The report shows each model's cache-read share.
2. **Settings.** `_set.yaml` and each scenario can set `context` and `llm` keys.
3. **Checks:**
   - `max_requests`
   - `no_turn_failed`
   - `cache_read_share` (minimum, `test: info` by default)
4. **`after_message`.** It gains `when: during | between`, which fails when the message lands in the wrong place.
5. **Language-neutral checks:**
   - `skill_loaded` reads the notice kind, not its text.
   - `no_false_running` takes its phrase list from the set, so a Persian set brings its own.
6. **Report.** With 3 or more reps it shows min and max per model and per check. `-baseline <report.json>` picks what the diff compares with.
7. **Offline.** `-model scripted:<file>` plays scripted answers from YAML through the real orchestrator, for regression runs without keys.
8. **Skipped scenarios.** Their assertion types are always checked.

## Not in scope
- Stop, Retry and injected provider errors in scenarios. Go tests in `agent` cover them; add them only if a behaviour turns out to depend on the model.

## Done when
- Each item has a test in `internal/scenario`.
- The skill-loading set runs with `-model scripted:` and no keys.
- One real run of multi-part and skill-loading on the reference models shows the cache-read share in the report.
- `go test ./...` passes.

## Result
All eight items, each with a test in `internal/scenario`.
1. **Tokens.** Each run records input, cache read and cache write. The transcript's request table has a column for each, and the report's model table has a Cache read column.
2. **Settings.** `settings` in `_set.yaml` and in a scenario takes `context` and `llm` keys of config.yaml; the scenario's go over the set's. `llm.providers` and `llm.models` are refused, and so is a bad key or value (`config.ParseSettings`).
3. **Checks.** `max_requests`, `no_turn_failed` and `cache_read_share` (`min`; `test: info` unless given).
4. **`after_message`** takes `when: during | between`. The four t3 scenarios use `when: during`.
5. **Language-neutral.** `skill_loaded` reads the skill_loaded notice's skill. A set's `false_running` regexes replace the English ones.
6. **Report.** With 3 or more reps, runs and checks show the least and the most of a rep's pass share. `-baseline <report.json>` picks what the changes compare with.
7. **Offline.** `-model scripted:<file>` plays a script's replies through the real orchestrator, with no models file and no keys. Reports go to `<set>/results/scripted`. `scenarios/skill-loading/_offline.yaml` passes 12/12 (`TestOfflineScript`).
8. **Skipped scenarios.** Every assertion type is checked. A check that comes with a later feature (`background_started` and the like) needs that feature in `needs`.

Also: a run out of time reports "took longer" wherever it hit, also while the project opens or while waiting for a message's time.

Real run on the reference models (reps 1):

| Set | Model | Passed | Cache read |
|---|---|---|---|
| multi-part | gemini-3.5-flash-lite | 9/9 (11 skipped) | 3 % |
| multi-part | gemma4:31b | 9/9 (11 skipped) | 0 % |
| skill-loading | gemini-3.5-flash-lite | 11/12 | 46 % |
| skill-loading | gemma4:31b | 11/12 | 0 % |

- t3_a, t3_b and t3_d pass `when: during` on both, so their second message does join the turn.
- L04 fails on both, as on the old code (TASK-011).
- Ollama Cloud reports no cache tokens, so gemma shows 0 %. gemini's 3 % on multi-part isn't explained yet; each run is a new project with few requests, so few can reuse a cached prefix. Worth a look when cache use matters.
