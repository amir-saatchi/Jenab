# TASK-038 — Desktop macros and recipes
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-035, SPIKE-040

## Goal
Build SPEC 8.11: the agent saves what worked in a desktop session as a macro Jenab replays, or as a recipe, and reuses it. SPIKE-040's `results.md` (section 8) has the design, the skill text and the schema.

## Scope
1. **Recorder:** in each desktop session, keep every step that worked (acts the guards allowed and the app received, waits that found their text, the steps of a replay before it failed), by role and exact name, in memory for that session only.
2. **`desktop_save_macro {name, description, params, log, skip}`:**
   - Jenab puts `{param}` where each value was, folds keypad clicks that spell a value into one step, makes a decline click optional, and refuses a param whose value is in no step.
   - It checks the log: each log step must match an element in the last snapshot. With no usable log, Jenab logs the elements whose value changed in the last action.
   - Saving over a macro: the new steps are lined up with the old ones and both names kept as alternatives, and a version of a macro that has worked must keep its params.
3. **`desktop_run_macro {name, params}`:** replays through the same 8.10 guards, finds each element by role and a name or its alternatives, and returns the log. Older than a day: the result says when it last worked and to check the window. A failure returns the failed step, the steps before it, the log and a snapshot with ids. At Strict, one card listing the steps with the params filled in.
4. **`desktop_save_recipe {name, text}`:** at most 1,200 characters. Two shipped recipes: Calculator and a web search (texts in SPIKE-040's results).
5. **`desktop_start`'s result** lists the macros and recipes for the picked app and site, without their age.
6. **Saved without asking:** a notice with *Undo* in the chat, like memory saves (TASK-030). One reminder per session when the agent acted, saved nothing and is about to answer.
7. **Storage:** `desktop_macros`, `desktop_recipes`, `desktop_saved_changes`, `desktop_macro_runs` in `registry.db`. Removal: a macro after 3 failed replays in a row or 90 days unused, an agent's recipe after a year unused, *Undo* rows after 30 days; shipped and user-edited recipes never.
8. **Settings › Desktop:** per app, macros (steps, params, last runs, failures) and recipes; edit and delete; what was removed in the last 30 days.
9. **Skill:** the `desktop` skill gains the macro and recipe lines, the same for every model.

## Not in scope
- Loops, branches, sharing macros between computers, macOS and Linux.

## Rules
- Tests use the fake runner. Live checks only on windows the test opens (the test editor, Calculator, Chrome with a scratch profile on a local stand-in site); no live Google.
- No new Go dependencies without asking.

## Done when
- Go tests cover the recorder, params and folding, the log check, alternatives on a fix, keeping params, replay through the guards, the age note, failure handover, the Strict card, *Undo* and removal.
- Live on `gemini-3.5-flash-lite` and `ministral-14b-latest`: make, then replay 3 times with other params, for the test editor, Calculator and the stand-in site; break once (another language or mode) and fix. Requests and tokens next to SPIKE-040's.
