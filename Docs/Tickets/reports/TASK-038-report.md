# TASK-038 report: desktop macros and recipes (SPEC 8.11)

Branch `claude/dazzling-elgamal-396dd2`, head `c32de8c` (c32de8caa44193cd951c0339dcc99978dae9957d). Nothing under `Docs/` was changed.
This file is not committed.

## 1. What was built

**Recorder** (`internal/desktop/macro.go`, `session.go`, `snapshot.go`)
- Each session keeps the steps that worked, by role and exact name as agent-desktop reported them, with the values used:
  - each `desktop_act` the guards allowed and the CLI delivered
  - each `desktop_wait` that found its text
  - the steps of a replay before it failed
- A save after `desktop_end` in the same turn still finds the session (a model often ends first, then saves).

**`desktop_save_macro {name, description, params, log, skip}`** (`macro_tools.go`)
- `params` are `name=value` with the value used this time. Jenab puts `{name}` where the value was. A param whose value is in no step is refused: "the value of X ("v") is in none of the steps".
- Keypad clicks that spell a value fold into one `click_each` step. A click on a decline button ("Reject all", "Alle ablehnen" …) becomes optional.
- `skip` leaves out recorded steps by number. The tool result lists the recorded steps with numbers when a save leaves some out.
- **The log is checked.** Each log step must match an element in the session's last snapshot or the window now; the others are left out with a note. If no log step is left, Jenab logs the elements whose value changed in the last action, by the part of the name that stayed ("Display is" of "Display is 72"). The window itself is left out.
- **Saving again after a failed replay** lines the new steps up with the old ones (same `do` and role, in order) and keeps both names as alternatives: `["Addieren", "Plus"]`.
- **A version that has worked must keep its params:** a save that drops one is refused.
- 1 to 40 steps, checked by the same `ParseSteps` that Settings uses.

**`desktop_run_macro {name, params}`**
- Finds the macro for the session's app (and site, for a browser).
- Checks that the params are complete and known, fills them in, and replays.
- Every step goes through the session's `do()`, so all of 8.10 applies:
  - the picked window and its lock
  - the deny list, consent, password and key rules
  - the CAPTCHA stop
  - the first key press asks
- **At Strict:** one card (kind `desktop_macro`) lists the steps with the params filled in. At Standard and Auto there is no card.
- **On success:** "Done: macro X (version N) ran K steps in T s", plus the log.
- **The age note** ("It last worked 3 days ago: check the result in the window with a snapshot…") shows only when the macro last worked more than 24 h ago.
- **On failure:**
  - the failed step and the steps before it
  - the log so far
  - a snapshot with ids (the same text as `desktop_snapshot`)
  - "carry on by hand"
  - after the third failure in a row: a note that the macro was removed

**`desktop_save_recipe {name, text}`**
- At most 1,200 characters.
- Recipes for any browser are stored under app `browser`.
- Two recipes ship with Jenab (`Shipped`): `calculator` and `web_search`. They go into the store once per start, and a new text replaces the old by name.

**`desktop_start`'s result**
- Ends with the macros (up to 20) and recipes (up to 10) for the app and site, without their age.
- Recipes with the same name are listed once, the user's first, then the agent's, then the shipped one.

**Saves without asking**
- Each save adds a `desktop_saved` notice with the steps or text, Undo/Redo and Edit (opens Settings › Desktop).
- One reminder per session: an agent that acted in the window and saved nothing gets one user-role notice before it answers, through a new `Env.BeforeAnswer` hook in the turn, which allows one more request.

**Storage** (`internal/store/desktop.go`, `registry.db`): four tables.
- `desktop_macros`: versions, and alternatives in the steps.
- `desktop_recipes`: source `app`, `user` or `agent`.
- `desktop_saved_changes`: before and after JSON, for Undo.
- `desktop_macro_runs`: the last 20 per macro, lengths only.

**Removal** (`PruneDesktop`, run at every `desktop_start` and when Settings › Desktop opens)
- A macro is removed after 3 failed replays in a row, or after 90 days unused.
- An agent's recipe is removed after a year unused.
- Removed rows and Undo rows are deleted after 30 days.
- Shipped recipes and the user's recipes are never removed. The user's edit of a shipped recipe is saved as their own copy.

**Settings › Desktop** (`frontend/src/settings/desktop.tsx`; Windows only)
- Lists each app's macros (steps, params, last 5 runs) and recipes.
- You can edit a macro's description, params and steps (as JSON, checked as a save is), delete it, write or edit a recipe, and see what was removed in the last 30 days.
- The app services are in `internal/app/desktop.go`.

**Skill:** the desktop skill gains the three tools and a "Saved macros and recipes" section. It is the same text for every model.

**Approvals:** kind `desktop_macro`: ask at Strict, no card at Standard and Auto.

## 2. Checks

- `go build ./...`, `go vet ./...`, `go test -p 2 -short ./...`: pass (GOMEMLIMIT=1500MiB).
- gofmt on changed `.go` files (CRLF-aware): clean.
- `go run ./cmd/jenab-scenarios check scenarios`: pass.
- `wails3 task check:bindings`: no diff.
- In `frontend/`: `bun run typecheck`, `bun test` (127 pass), `bun run build`: pass.
- New Go tests (also `TestEndChat`, `TestAutomaticLog`, `TestSaveTheSameStepsAgain`, and `TestClear` checks `ChatReset`):
  - `internal/desktop/macro_test.go`: recorder, folding and params, unused param refused, optional decline, log check, automatic log, replay with other params, age note, failure handover and fix with alternatives, fix keeps params, guards in a replay, Strict card, param checks, 3 failures remove, notice and Undo, one reminder, step format, alignment.
  - `internal/store/desktop_test.go`: versions and Undo, runs and removal, prune, recipe sources.
  - `internal/agent/desktop_test.go`: approval rows, reminder before the answer.
  - `internal/app/desktop_test.go`: Settings and the chip's Undo.

**Later changes from the live runs** (all with Go tests)
- Saving the same steps and params again is a no-op: "Not saved again: macro X (version N) already has these steps. Next time, run it with desktop_run_macro instead of doing the task by hand." The same goes for a recipe with the same text. No notice, and the session counts as saved, so there is no reminder.
- A log step with no name must be an element whose text the last action changed. Otherwise it is left out with a note asking for a part of the result element's name. `log button` matched every button in the window.
- The automatic log leaves out the window and elements whose text didn't change (a focus change alone).
- `desktop.Manager.EndChat(p, c)` and `agent.Deps.ChatReset`, at Main-Session's request. The orchestrator calls it after Clear and after a chat is deleted, so a new turn with the same number can't reuse a dropped turn's desktop session. `cmd/desktop` wires it to `EndChat` on Windows. Main-Session wires rewind and undo rewind (TASK-018) at merge. Tests: `TestEndChat` (desktop) and `TestClear` (agent).
- The live test, `internal/agent/desktop_macro_live_test.go` (Windows build tag, `JENAB_DESKTOP_LIVE=1`). It also gained request pacing (`JENAB_DESKTOP_PACE_MS`) in `desktop_live_test.go`.

## 3. Live runs

**Setup**
- `TestDesktopMacroLive`, agent-desktop 1.2.0 (pinned SHA).
- Windows the test opens itself:
  - SPIKE-039's test editor
  - Calculator (a new window; the test puts Standard mode back and closes it)
  - Chrome with a fresh profile in the scratchpad (`--lang=en-US`) on a stand-in search site on 127.0.0.1, in English and German, with a consent dialog
- Approval level Standard.

**Phases per app**
- make (set 0)
- replays with other params (sets 1, 2)
- a replay with the clock 3 days on (set 3, the age note)
- break (set 4: Calculator in Date calculation mode, the site in German)
- after the fix (set 5, back to normal)

The editor has no mode or language, so it had no break. Then come no-model replays: a scripted model calls `desktop_start` then `desktop_run_macro` with the set's values, so they cost no requests.

**Grading**
- Editor: the sentence once in the editor.
- Calculator: the display (`Display is …`) equals the sum.
- Site: the 5 first titles in order in the answer, from all of the turn's text.

No consent was accepted in any run (the stand-in counts accepts, and the test fails on one). There was no CAPTCHA. The test closed only what it started: Calculator by WM_CLOSE, Chrome by WM_CLOSE and then `taskkill /T` of its own PID.

**Requests used**
- gemini-3.5-flash-lite: 152, from 08:01 UTC (cap 400 a day).
- ministral-14b-latest: about 200, paced at 2.5 s (under 30 a minute).
- A first try of each model stopped early on a test bug. Gemini's used no requests; Mistral's used 8, on one editor make, which are counted in the 200.

### gemini-3.5-flash-lite

| app | phase | ok | requests | input tokens (+cached) | macro |
|---|---|---|---|---|---|
| editor | make | yes | 5 | 15.0k (+5.7k) | saved write_sentence v1 |
| editor | replay 1 | yes | 5 | 13.4k (+9.1k) | ran, 2 s |
| editor | replay 2 | yes | 5 | 12.5k (+10.8k) | ran, 3 s |
| editor | replay, 3 days old | yes | 5 | 15.2k (+7.6k) | ran, 2 s |
| Calculator | make | yes | 26 | 62.0k (+68.6k) | reached the turn's request limit, so the save calls were cut off |
| Calculator | replay 1 | yes | 12 | 28.9k (+29.7k) | by hand, then saved calculator_add v1 |
| Calculator | replay 2 | yes | 6 | 15.5k (+11.0k) | ran, 11 s |
| Calculator | replay, 3 days old | yes | 5 | 12.6k (+9.5k) | ran, 8 s |
| Calculator | break (Date mode) | **no** | 26 | 61.3k (+77.1k) | failed at step 1 ("found no button named One"); didn't recover by hand, saved no fix |
| Calculator | after the fix | yes | 5 | 12.4k (+9.4k) | ran v1, 11 s |
| site | make | yes | 16 | 35.3k (+46.8k) | saved search_findit v1 |
| site | replay 1 | yes | 5 | 12.5k (+10.3k) | ran, 8 s |
| site | replay 2 | yes | 6 | 14.5k (+14.1k) | ran, 9 s |
| site | replay, 3 days old | yes | 6 | 15.4k (+12.6k) | ran, 13 s |
| site | break (German) | yes | 12 | 26.3k (+33.5k) | failed at step 4 (no "Search" field); carried on by hand and saved v2 |
| site | after the fix | yes | 7 | 16.6k (+15.5k) | ran v2, 18 s, skipped the optional German decline |

**Gemini's fix of the site macro (v2)** kept both languages as alternatives:
- `set_value textfield "Suche" or "Search" {query}`
- `click button "Suchen" or "Search"`
- "Reject all" and "Alle ablehnen" both optional
- params `query`, `url`

v2 then worked in English. **The fix-with-alternatives path works as designed.**

**No-model replays of Gemini's macros**
- editor: ok, 3 s.
- Calculator set 1: the display was right (2,363), but the test graded it as failed. calculator_add's log was `log button` (every button), so the check found no display in the result. This led to the stricter no-name log rule above.
- Calculator set 4 (Date mode): failed at step 1 with the snapshot handover, as expected.
- site: not replayed. The macro has a `url` param that the test's sets don't fill.

### ministral-14b-latest

| app | phase | ok | requests | input tokens (+cached) | macro |
|---|---|---|---|---|---|
| editor | make | yes | 7 | 19.1k (+22.3k) | saved write_sentence v1 |
| editor | replay 1 | yes | 7 | 35.7k (+8.8k) | by hand; saved v2 |
| editor | replay 2 | yes | 7 | 32.0k (+12.4k) | by hand; saved v3 |
| editor | replay, 3 days old | yes | 7 | 22.6k (+21.9k) | by hand; saved v4 |
| Calculator | make | no | 10 | 35.6k (+33.8k) | save refused: the params were in no step |
| Calculator | replay 1 | no | 11 | 43.5k (+34.3k) | save refused |
| Calculator | replay 2 | no | 14 | 61.7k (+46.0k) | saved add_numbers v2 with no params |
| Calculator | replay, 3 days old | no | 9 | 19.7k (+44.2k) | save refused |
| Calculator | break (Date mode) | no | 12 | 22.6k (+56.8k) | never left Date mode |
| Calculator | after the fix | no | 25 | 89.7k (+115.8k) | save refused |
| site | make | yes | 14 | 25.6k (+71.4k) | saved search_local_site v1 |
| site | replay 1 | yes | 16 | 38.1k (+76.4k) | by hand; saved v2 |
| site | replay 2 | yes* | 15 | 32.6k (+74.6k) | by hand; saved v3 |
| site | replay, 3 days old | yes | 18 | 39.3k (+89.6k) | by hand; saved v4 |
| site | break (German) | yes | 15 | 33.7k (+72.3k) | by hand; saved v5 |
| site | after the fix | yes | 13 | 26.3k (+65.7k) | by hand; saved v6 |

\* Graded 0/5 in the run, because the last message was "The task is complete." and the titles were in an earlier message. It is 5/5 on the regrade; the grader now reads all of the turn's text.

**What Mistral did**
- It never called `desktop_run_macro` (0 of 16 turns), though `desktop_start` listed its macros each time. Instead it saved a new version every time, which led to the identical-save no-op above.
- **Calculator:** it typed into the display with `set_value`, clicked digits that didn't spell the params, and answered with its own sum. So the save check refused its params, and one save without params froze wrong digits ("One Two Plus Nine Five Decimal Zero Equals"). Jenab refused the bad params as designed. It did not refuse a macro without params made from a failed task (see open points).

**No-model replays of Mistral's macros**
- editor write_sentence v4: ok, 2 s.
- site search_local_site v6, English: 5/5, 13 s.
- site v6, German: failed at step 4 ("found no textfield named Search") with the snapshot handover, as designed.
- Calculator: not replayed (no params).

### Next to SPIKE-040

SPIKE-040 measured turns and prompt tokens. A Jenab request here is one model turn, so the numbers compare.

| | SPIKE-040 by hand | SPIKE-040 with a macro | TASK-038 Gemini with a macro | TASK-038 by hand (make / Mistral) |
|---|---|---|---|---|
| editor | 3–5 turns, 2.4k–4.3k | 3–5 turns, 4.7k–9.9k | 5 requests, 12.5k–15.2k | 5–7 requests, 15k–36k |
| Calculator | 6–28 turns | 3–5 turns | 5–6 requests, 12.4k–15.5k | 12–26 requests (Gemini), 29k–62k |
| search | 16–22 turns, 22k–44k (Google) | 3–5 turns | 5–7 requests, 12.5k–16.6k | 13–18 requests, 26k–39k (stand-in) |
| total requests | Gemini 310, Mistral 546 | | Gemini 152 | Mistral about 200 |

Jenab's input per request is higher than the spike's. It includes Jenab's system prompt, the desktop skill and the tool list, which the spike's bare loop didn't have.

**The saving is in requests:**
- Calculator: from 12–26 down to 5–6.
- search: from 16 down to 5–7.
- editor: the same, since a macro there is a single step.

A replay's own time was 2–18 s.

## 4. Open points

1. **Mistral ignores saved macros**, as in SPIKE-040. It does the task by hand and saves again. Recorded as a model failure; there is no model-specific workaround. Only the no-op re-save limits the noise.
2. **A model can save a macro from a failed task.** With no params, Jenab can't tell. Options:
   - refuse a save in a turn where the last check failed (Jenab has no such check today)
   - let the first replay's result decide whether the macro is kept
3. **Calculator Date-mode recovery.** Neither model got back to Standard mode after the replay's handover. The handover's snapshot shows the mode menu, but the models didn't use `select`. Maybe the Calculator recipe should say how to switch modes.
4. **The turn's request limit** cut off Gemini's saves after a long make run (26 requests). Maybe allow the save calls one more request, as the reminder already gets.
5. **An agent's recipe can shadow a shipped recipe of the same name.** Gemini saved its own `calculator`, and the agent's recipe is listed first. The order is user, agent, shipped, as designed, but a newer shipped text is then hidden.
6. **Undo of a macro save puts back the old version's run counts too.** That is by design (the row comes back as it was), but worth a line in the SPEC.
7. **A macro needs no log.** Most models gave one, and the automatic log covers the rest, but a macro with an empty log can only report "Done".
8. **The no-model replay of a site macro with a `url` param** needs the test to fill it. The test doesn't map it yet.
9. **Rule incident:** while checking formatting I once ran `bunx prettier --check`. prettier is not in package.json. It only read files and wrote nothing, but it broke the "no bunx for packages not in package.json" rule. It didn't happen again.
10. **Main-Session's note** (desktop sessions keyed by turn number could be reused after a Clear or rewind) is handled by `EndChat`/`ChatReset` from Clear and Delete. The rewind and undo-rewind calls are left to Main-Session at merge. I didn't merge or touch TASK-018's branch.

## 5. Proposed SPEC 8.11 changes

- **Log steps:** a log step with no name must be an element whose text the last action changed; otherwise it is left out with a note. The automatic log leaves out the window and elements whose text didn't change.
- **Same save again:** saving the same steps and params again (or a recipe with the same text) is a no-op with the note "Not saved again … run it with desktop_run_macro". It counts as a save for the reminder.
- **Failed step note:** say "found no ROLE named NAME in N s", and give the missing log steps' reason.
- **Session lifetime:** a chat's desktop session ends on Clear, rewind, undo rewind and when the chat is deleted (`EndChat`). 8.10 should say this too.
- **Live test grading:** grade on all of the turn's assistant text, not only the last message.
- **Points 2–4 and 6 above** need a decision.

## 6. Head

`claude/dazzling-elgamal-396dd2` at `c32de8c`: one commit on top of `8d9d286`, signed off, not pushed.
