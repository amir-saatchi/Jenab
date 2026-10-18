# TASK-014 — The agent offers features the app doesn't have
**Type:** Bug
**Status:** Done
**Gate:** 3
**Needs:** TASK-011

## Goal
The agent offers and tries only what its tools can do. When no tool can do what the user asks, it says so plainly. Found while testing the app on 2026-10-13.

## Today
- `base.md` says a project has "tables in a local database, pipelines that fetch data, and views" (`internal/agent/prompts/base.md:1`). Phase 1 has no tools for any of them: only `fetch_page`, the bucket tools, history search, `list_chats`, `load_skill` and `ask_user`.
- A test chat (Mother, gemini-3.5-flash-lite) went like this:
  - Its greeting offered to "set up tables, fetch data via pipelines, or build views".
  - Asked to build them, it saved `schemas/users.sql` and `pipelines/fetch_users.py` with `bucket_put`. The script writes to a `project.db` in whatever folder it runs from.
  - It then made up a "Tables section" and a "Views section", and told the user to run Python in a terminal.
- Nothing tells the model what to do when no tool fits.
- TASK-011 made the background rule and the delegation guidance follow the tools, but left `base.md` as it was: rewording it for every set cut gemma's L02 from 12/12 to 10/21.

## Scope
1. **Block 1 follows the tools.** `base.md` names tables, pipelines and views only when tools for them are registered. A tool says what it belongs to (for example a field in `tool.Spec`); the prompt builder doesn't list tool names. The scenario sets have fake tools for all three, so their prompt stays as it is.
2. **When no tool fits.** One line, the same for every model: when no tool can do what the user asks, say so and say what you can do instead. Don't describe screens, menus or steps you can't see.
3. **A scenario set with Phase 1's tools only.** It asks for a table, a pipeline and a view, plus a plain question. Checks:
   - No `bucket_put` of SQL or scripts posing as the setup.
   - The answer says the app can't do it yet.
   - The answer names no app sections or menus.

## Not in scope
- The table, pipeline and view tools themselves (Phase 2).

## Done when
- A test in `agent` checks block 1 with and without the table, pipeline and view tools.
- The new set passes on both reference models, with 3 reps.
- skill-loading and multi-part don't get worse (`-baseline`).
- `go test ./...` passes.

## Result
Block 1 says what this version has, and what to do when no tool fits (SPEC 3.1).
- **Areas.** `tool.Spec` has `Areas` (`tool.Tables`, `Pipelines`, `Views`). The first line names the areas some tool has and says the others aren't here yet. With Phase 1's tools: "Jenab is a desktop app where a user keeps a project. This version has no tables, pipelines, or views yet." The scenario sets mark their fake tools with `areas:`, so their line is as before. A test checks each set on a fixture has all three.
- **When no tool fits,** one line: say so and say what you can do; a file you save doesn't run or update itself; don't make up data no tool gave you; don't describe screens, menus or steps you can't see.
- **Why "not yet" and not just leaving the areas out.** Without the words, both models read "table" and "pipeline" as plain requests and built them from files: CSVs, Python scripts, a `pipelines/*.json` "scheduled daily", an invented 30-day chart. The no-tool line alone didn't stop it: F2 to F4 passed 2/9 on Gemini and 5/9 on gemma. With the "not yet" sentence: 12/12 on both, then the full set below.
- **New set `missing-features`:** Phase 1's tools only, no card or fixture. A greeting, a table, a pipeline, a chart, a plain question and a note to save. Checks: no SQL, scripts, configs or HTML saved as the setup; the answer says it can't; no screens or menus; no made-up series of prices. `fetch_page` is a fake with the real tool's text (a fake without `description` and `schema` takes the real one's). New check `answer_lacks`.
- **Live runs,** 3 reps:

  | Set | gemini-3.5-flash-lite | gemma4:31b | last report (1 rep) |
  |---|---|---|---|
  | missing-features | 15/18 | 18/18 | - |
  | skill-loading | 32/36 | 36/36 | 11/12, 11/12 |
  | multi-part | 27/27 (rerun on 2026-10-16, after the quota reset) | 27/27 | 9/9, 9/9 |

  - Gemini in missing-features: F4 (chart) 1/3. It still makes up 30 days of prices and once saved them as an HTML chart. F2 (table) 2/3: once it saved a CSV "table" without saying the app has none.
  - Gemini in skill-loading: L04 0/3, as before (0/1, and 3/4 failed on TASK-011's old code). L02 2/3: once it saved the pipeline before loading `pipelines`. gemma's L02, which the old rewording hurt, is 3/3.
- **Also fixed:** `TestScriptedRun` was flaky: the "between" message came 50 ms after the first, and a slow first turn made it land during the turn. It now comes after 1 s; 20/20 runs pass.
- **Tests:** `TestBlockOneAreas` (the line with no, one, two and three areas), `TestBlockOnePhase1` (the "not yet" sentence and the no-tool line with the app's tools), the tables line and the real `fetch_page` text in `TestRunSet`, and an unknown area in `TestLoadErrors`.
- **Made-up chart data:** a prompt line for one model is out (SPEC 1). [SPIKE-032](SPIKE-032-made-up-numbers.md) tests a harness check instead.
