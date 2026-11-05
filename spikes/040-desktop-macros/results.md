# SPIKE-040 results: desktop macros and recipes

Run on 2026-11-03, Windows 11, agent-desktop 1.0.2 (checked against `checksums.txt`), the same
harness and guards as SPIKE-039. Models: `gemini-3.5-flash-lite` and `ministral-14b-latest`, the
same prompt, skill and tools for both. Turns are model requests; tokens are the API's prompt and
output counts, summed over the run; time is wall time (mostly the model: Gemini's free tier
answers in 5–25 s with 503 retries, Mistral was often held back by 429s).

**Short answer: go, with one change to the design.** When the model uses a macro, a task takes
3–5 turns and 4.7k–10k prompt tokens, and the replay itself 1–16 s. By hand the same tasks took
20–30 turns and 73k–165k (Calculator), 10–13 turns and 10k–15k (the stand-in search site) and
15–18 turns and 46k–58k (Google). But the models are bad at writing the steps: 2 of the 8
written macros that were tried replayed, and one broken one replayed "fine" with the wrong
numbers, which Gemini then covered with its own arithmetic. When Jenab records the steps from
what worked and the model only names the params and the log, all 5 recorded macros that were
tried replayed right (2 with a useless log). Of the other two, one wasn't tried, and one was
broken because the run it was recorded from was itself wrong. Build the recorder,
not a step list written by the model. Recipes made no measurable difference.

## 1. What was built

- **Format** (`macro.go`): `{do, role, name, text, key, map, optional, seconds, limit}`; `do` is
  `click`, `set_value`, `toggle`, `expand`, `collapse`, `select` (an element found by role and
  exact name, case-insensitive), `press` (the 8.10 key list), `click_each` (one click per
  character of `text`, through `map`: `4=Four`), `wait` / `wait_gone` (a text) and `log` (record
  the elements of a role whose name holds a part, up to 50, link addresses dropped). `{param}`
  in `name` and `text`. 1–40 steps, at least one log. No loops, no branches.
- **Replayer** (`replay.go`): each step finds its element with `agent-desktop find --role --name
  --exact`, polling up to 10 s (5 s for an optional step), then runs the same guards as the
  model's own actions: consent, password fields, the key list, the window lock; in a browser the
  site lock and the CAPTCHA check after each action. 0.6–0.8 s per `find`.
- **Result to the model:** `done: macro X (version v) ran n steps in t s; k optional step(s)
  skipped.` plus the log, plus "It last worked 3 days ago: check the result in the window before
  you trust it." when the macro is older than a day. A failure: `failed at step k of n (step):
  reason`, the steps before it with their outcome, the log so far, a snapshot of the window with
  ids, and "Carry on by hand from here; the ids above work with act. When the task worked, save
  the fixed macro under the same name."
- **Store** (a JSON file standing in for `registry.db`): versions, runs, failures, fail streak,
  last worked, made by, untrusted; Undo per save (the notice).
- **Two ways to save:**
  - *Written* (`save_macro {name, description, params, steps}`): the model lists the steps.
    Checked by `validate()` (names, params declared and used, ref-like names refused, a log
    step), and with `-check-steps` against the session: each action's role and exact name must
    have been in one of this session's snapshots, each wait's text must have been seen.
  - *Recorded* (`-record`, `record.go`): Jenab keeps every step of the session that worked (each
    `act` the guards let through and the CLI delivered, each `wait` that found its text, the
    steps of a replay before it failed). `save_macro {name, description, params: ["query=…"], log,
    skip}` replaces each param's value in the steps with `{param}`, folds a run of keypad clicks
    that spells a value into `click_each`, marks a decline click optional, refuses a param whose
    value is in no step, and lists the stored steps in its result.
- **The saved list is pushed:** the first `list_windows` or `snapshot` of a run ends with the
  macros and recipes for that app (as `desktop_start`'s result would). Without that, Mistral
  never called `list_windows` and never saw its macro (5 of 5 editor runs in the first batch).
- **The stand-in site** (`site.go`): after Google's second CAPTCHA (section 6) the browser part
  moved to a page the harness serves on `http://127.0.0.1:8740/`, built like Google's: a cookie
  dialog on the same page ("Reject all" / "Accept all", "Alle ablehnen" / "Alle akzeptieren"), a
  search box, a results page with title-and-site links made from the query, in English or German.

## 2. Making macros (each model, each task, once, with the save instruction)

`-nudge`: when the model acted by hand, saved nothing and answered, it got one reminder ("Before
you answer: if the task worked, save it as your instructions say"). Same text for both models.

### Written by the model

| Task | Model | Task done | Saved | Replays? | Turns | Prompt tokens |
|---|---|---|---|---|---|---|
| Editor | Gemini | yes | `write_sentence`: set_value textfield named `{sentence}` | **no**: the box has no name; the param was used as the name | 7 | 14.6k |
| Editor | Mistral (v1, no nudge) | yes | nothing | – | 3 | 5.6k |
| Editor | Mistral (v1, nudge) | yes | `write_text`: textfield named "textfield" | no (role used as name) | 6 | 12.2k |
| Editor | Mistral (v2, step check) | yes | first save refused ("no textfield named textfield"), second: set_value textfield, no name | **yes** | 7 | 14.7k |
| Calculator | Gemini (v1) | display right (14,183), no answer in 30 turns | `add_two_numbers`, no params, wrong clicks | no | 30 | 198k |
| Calculator | Gemini (v2) | **wrong**: typed 7486, answered 16,842 | `add_numbers`: the wrong digits as clicks, no params | no (replays 7486+9356 for every task) | 18 | 57.8k |
| Calculator | Mistral (v1, v2) | no: 30 steps, malformed tool calls | nothing | – | 24–30 | 102k–165k |
| Calculator | Mistral (replay batch, set 2) | yes, 9,011 | `calculate_sum(num1, num2)`: click_each, Plus, click_each, Equals, log display | **yes** (5 of 5, see 3) | 20 | 72.7k |
| Google | Mistral (v1) | yes, 5/5 | 9 steps, with a wait for "{query} - Gesucht auf Google" that had timed out in its run | not tried | 16 | 55.0k |
| Google | Mistral (v2) | yes, 5/5 | 8 steps, two waits that had timed out in its own run | **no**: failed at its step 3 (the wait) | 15 | 45.7k |
| Google | Gemini (v1) | stopped: CAPTCHA | – | – | 16 | 42.0k |
| Google | Gemini (v2) | yes, 5/5 | 6 steps: box "Suche" (German page), `log link "Wails"` (the first query's word) | **no**: stuck on a blank page, then the box | 18 | 58.5k |

Two of the eight written macros that were tried replayed. The mistakes are of a few kinds, and the step check catches
only some: a param or a role put in as the element's name; steps that failed in the model's own
run (waits that timed out) kept in; the values typed as fixed clicks instead of params; a log
filtered by the first task's words. Mistral saved after the reminder in all 6 making runs, and twice without it (its Calculator
macro, which came in a replay batch without `-nudge`, and the Date calculation fix); Gemini
never needed it.

### Recorded by Jenab

| Task | Model | Task done | Saved | Replays without the model | Turns | Prompt tokens |
|---|---|---|---|---|---|---|
| Site | Gemini | yes, 5/5 | 8 steps: address, Return, wait, Reject all (optional), box `{query}`, Search, wait "Results", log link | **yes**, 2 of 2, 10 s | 16 | 33.9k |
| Site | Mistral | yes, 5/5 | 9 steps, but its logs name elements that don't exist (`log textfield "search_box"`) and a `log option` | yes, but the log is empty: the model reads the page itself | 14 | 29.6k |
| Editor | Mistral | yes | set_value textfield `{text}`, log textfield | yes | 7 | 13.6k |
| Calculator | Mistral | no, twice (30 steps, malformed calls; display "-48 point") | – | – | 30, 30 | 121k, 153k |

Four more recorded macros came out of the breaks in section 4: Gemini's fixes for the German
page, the German Chrome UI and Calculator's Date calculation mode, and Mistral's 38-step
Calculator "fix". Every recorded macro had
the right elements and the right params; the only weak part left to the model is the log.

## 3. Replays by the model (other params; the last ones with the macro made to look 3 days old)

SPIKE-039 by hand, for comparison: editor 3–5 turns, 2.4k–4.3k tokens; Calculator 6–28 turns,
6.4k–137k tokens, right in 1 of 6; Google 16–22 turns, 22k–44k tokens.

| Task, model, macro | Runs | Right | Used the macro | Turns | Prompt tokens | Time | Old macro checked |
|---|---|---|---|---|---|---|---|
| Editor, Mistral, written | 5 | 5 | **0**: did it by hand (one set_value) | 3–4 | 5.7k–8.1k | 11–78 s | – |
| Calculator, Mistral, written | 5 | 4 | sets 3–5 (made in set 2) | by hand 20–30; with the macro **3–5** | by hand 73k–139k; with it **5.4k–9.9k** | 49–53 s with it | 2 of 2 (a snapshot) |
| Site, Gemini, recorded | 3 | 3 (one graded 0/5 for my titles, see below) | 3 | **3–4** | **4.7k–7.2k** | 32–78 s | 1 of 1 |
| Site, Mistral, recorded | 3 | 3 | 2 (run twice each); the old one: **not used** | 4 with it, 13 by hand | 6.7k with it, 27.1k by hand | 107–190 s | – (did it by hand) |
| Editor, Gemini, written | 5 | 5 | 5; the first replay failed (the param was the name), it did the task and saved a working v2 | 7, 7, then **3–4** | 15.4k, 14.2k, then **5.2k–7.4k** | 10–33 s | 2 of 2 |
| Calculator, Gemini, written (no params, the wrong digits) | 2 | **0** | 2: the replay "worked" and typed 7486 + 9356 both times | 30 (then by hand, failed); 3 | 127k; 5.2k | 18–125 s | – |

- **A macro that replays without error can still be wrong.** Gemini's Calculator macro has the
  first task's digits as fixed clicks. In run 2 the replay "worked", the display read 16,842, and
  Gemini answered 9011, its own sum. The replayer can't tell; the log (the display) showed it,
  and the model didn't compare. Recording (params from the values used) prevents this kind of
  macro; the skill should also say to compare the log with the task.
- Where the macro was used, the turns fell from 14–30 to 3–5 and the prompt tokens by 75–95%.
  The replay itself took 7–16 s.
- For a one-step task (the editor) a macro gains nothing: Mistral did the one `set_value` itself,
  in as many turns as a replay would take.
- **Old macros:** Gemini and Mistral both took a snapshot after the "last worked 3 days ago"
  note (3 of 3). But Mistral skipped an old macro altogether in 2 of 2 site runs and did the task
  by hand, though the skill says to run it first.
- Gemini's site run 1 answered every title without its last word ("…guide 1" read as list number
  1): my stand-in titles ended in a number. The answer was right; the titles were changed after it.

## 4. Breaks

| Break | Without the model | Model | Handover | Fixed version saved | The fix |
|---|---|---|---|---|---|
| Site without the cookie dialog | ok, the optional step skipped, 15 s | – | – | – | – |
| Site page in German | fails at the search box ("Suche"), snapshot returned | Gemini | carried on by hand, 5/5, 12 turns, 28.5k | **yes**, v2 | works in German (19 s), **now fails in English** |
| Chrome's own UI in German | fails at step 1 (address bar "Adress- und Suchleiste") | Gemini (1st) | took over, but Return failed twice with STALE_REF (focus not confirmed), 20 turns, no answer | – | Calculator ran at the same time: my harness, not the macro |
| | | Gemini (alone) | 5/5, 14 turns, 31.8k | **yes**, v2 | German address bar; would now fail in English |
| | | Mistral | the macro marked old: skipped it, 5/5 by hand | – | – |
| | | Mistral (macro fresh) | carried on by hand, 5/5, 14 turns, 30.0k | **no** | – |
| Calculator in Scientific / Programmer | ok, same button names, 9–10 s | – | – | – | – |
| Calculator in Date calculation | fails at step 1 (no "Seven") | Gemini | switched to Standard itself, 971, 20 turns, 71.4k | **yes**, v2: Open Navigation, select "Standard Calculator", click_each `{num1}`, Plus, `{num2}`, Equals | **works in Standard and in Date calculation** (15–16 s); its log is 9 useless `log button` steps |
| | | Mistral | answered 971, but the display read **112.9** (a guess) | **yes, worse**: 38 steps, no params, its mistakes four times | replaced the working v1 |

- **The handover works:** in 5 of 6 model runs the model picked up from the snapshot in the
  failure result and finished the task. (The one that didn't failed for a harness reason.)
- **The fix replaces the old version, so it flips:** a macro fixed for the German page fails on
  the English one, and back. One fix (Calculator's mode switch) was general; the language and UI
  fixes weren't.
- A bad run can overwrite a good macro (Mistral's 38 steps over its working `calculate_sum`).
- The mode a test puts Calculator in is put back to Standard when the window closes (Calculator
  keeps its mode between launches). A `click` on the mode doesn't switch it; `select` does.

## 5. Recipes

Each model's own recipe from its making run (`-mode recipes`: the recipe in the saved list, no
macro tools), next to the same task without one (`-mode plain`, SPIKE-039's prompt).

| Task | Model | Without a recipe | With its recipe | Right |
|---|---|---|---|---|
| Site | Gemini | 13 turns, 14.1k | 13 turns, 15.3k / 13 turns, 15.4k | 3 of 3 |
| Site | Mistral | 10 turns, 9.9k | 11 turns, 12.2k / 10 turns, 11.0k | 3 of 3 |
| Calculator | Gemini | 18–30 turns, 58k–198k in this spike, 0 of 2 (one had the display right but no answer after 30 turns); SPIKE-039: 1 of 3 | 15 turns, 30.2k, **right** | 1 of 1 |
| Calculator | Mistral | 20–30 turns, 73k–165k, 1 of 6 right | 21 and 29 turns, 79k and 82k, **wrong both times**: the display read 12,340 and 41,913, the answer was its own sum | 0 of 2 |

The recipes are the models' own: "Navigate to …, reject cookies if present, type the query into
the search box, click Search, and read the resulting links" (Gemini), and for Calculator "Click
the buttons for the first number, then Plus, then the buttons for the second number, and finally
Equals. Read the result from the display static text."

- **A recipe saves nothing where the model already does the task well** (the site: the same turns
  and tokens, a few hundred tokens more for the text).
- Where the model struggles, its own recipe doesn't fix it: Mistral's Calculator runs stayed
  wrong. Gemini's one Calculator run with its recipe was its best (15 turns, right), but that is
  one run.
- So recipes are for what a model can't see in the snapshot: an app's traps (Calculator's mode
  needs `select`, *Keep on top* loses the window), not the steps it can read off the screen.
  Agent recipes are cheap to keep as the fallback text when a macro breaks; the ones that ship
  with the app should be few and written by hand (section 8).
- **Grading:** a Calculator run counts as right only if the display shows the sum. Mistral
  answered the right number three times with a wrong display (here twice, and in the Date
  calculation break); the first grader counted those as right.

## 6. Google and CAPTCHAs

Two CAPTCHAs, so the batch stopped and the browser part moved to the stand-in site:
1. Gemini's first making run, at its step 16 (the evidence wasn't kept then).
2. A no-model replay of a hand-written control macro, right after its Return, about 40 minutes
   later (searches at least a minute apart). The two replays just before it ended on
   `about:blank` and on Chrome's error page.

`out/captchas.txt` keeps them; the harness now refuses Google runs once it holds two. Google
runs: 4 by a model (3 answered 5/5 by hand, 1 CAPTCHA), 5 no-model replays (only the first,
hand-written one reached a results page, and its log of 20 links held one result: raised to 50).

## 7. Requests

- `gemini-3.5-flash-lite`: 310 requests on 2026-11-03 (Pacific day, counted by the harness; the
  cap was 370). The key checks made earlier that day for Docs/MODELS.md are not in this count.
- `ministral-14b-latest`: 546 requests (UTC day), including the retries after 429s; always 4 s
  apart or more.
- No other key was used. Nothing was downloaded after agent-desktop 1.0.2.
- Not done: the stronger models for making (`gemini-3.5-flash`, `mistral-large-4`). The finding
  that matters (the step list is the weak part) doesn't depend on the model, and the recorder
  takes it away from the model.

## 8. Proposal for the build ticket

### Go, with these changes to the tested design

1. **Jenab records the steps; the model names them.** No step list from the model. Jenab keeps
   every step of the desktop session that worked, by role and exact name as agent-desktop
   reported them, with the values used: each `desktop_act` the guards allowed and the CLI
   delivered, each `desktop_wait` that found its text, the steps of a replay before it failed.
   `desktop_save_macro` gives the name, a description, the params as `name=value` with the value
   used this time, what to log, and the numbers of recorded steps to leave out. Jenab puts
   `{param}` where the value was, folds keypad clicks that spell a value into `click_each`, makes
   a decline click optional, and refuses a param whose value is in no step. This fixed every
   kind of mistake the written macros had (section 2).
2. **Jenab checks the log** too (untested): each log step must match at least one element in the
   session's last snapshot, duplicates are dropped, and with no usable log Jenab logs the
   elements whose value changed in the last recorded action (Calculator's display, the editor's
   box). Weak logs were the one fault left in recorded macros (Mistral's site, Gemini's
   Calculator fix).
3. **A fix keeps the old version's names as alternatives** (untested). When a replay failed and
   the model saves the same macro again, Jenab lines the new steps up with the old ones (same
   `do` and role, in order) and keeps both names: `"name": ["Search", "Suche"]`; `find` takes
   the first that is there. Without that, the language fixes flipped (section 4).
4. **A fix can't lose the params:** a save over a macro that has worked must keep its params
   (Mistral's 38-step version without params replaced a working one).
5. **The age is in the replay result only, not in the list.** The list says what exists; the
   result says "last worked 3 days ago: check the window". Mistral skipped old macros when the
   list showed their age (2 of 2), though the skill said to run them.
6. **The saved list is pushed** at the end of `desktop_start`'s result (macros and recipes for
   that app and site). Mistral never fetched it.
7. **Strict:** a replay is one card listing its steps with the params filled in, then runs
   without a card per step. The first key press card (8.10) stays. At Standard and Auto, no card.

### Skill text (for the desktop skill, 8.9)

```
Saved macros and recipes:
- desktop_start also shows the macros and recipes saved for this app.
- If a macro fits the task, run it with desktop_run_macro first, with the task's values as
  params. It replays its steps without you and returns its log. If the result says the macro
  is old, check the result in the window with a snapshot before you answer.
- If a replay fails, it returns its log and a snapshot of the window: carry on by hand from
  there. When the task worked, save the macro again under the same name.
- A recipe is a short note on how to do a task in this app. Follow it, but still check each step.
- When you did a task by hand and it worked, save it before you answer. Don't ask the user:
  they see a notice and can undo it.
  1. desktop_save_macro with a name, a one-sentence description, the values that change from
     task to task as params (each with the value you used this time), and log: the role (and a
     part of the name) of the elements that hold what the user asked for. Jenab takes the steps
     from your actions that worked, in order, and the result lists them. If one of them was a
     mistake, save again with its number in skip.
  2. desktop_save_recipe with two to five sentences on how to do this task in this app, for
     when the macro breaks.
```

This is `recordSkill` in `record.go` with the 8.10 tool names. Mistral saved after the one
reminder; keep the reminder in Jenab (once per desktop session, when the model acted, saved
nothing and is about to answer), the same text for every model.

### Tools

| Tool | Arguments | Result |
|---|---|---|
| `desktop_run_macro` | `name`, `params` (`["query=…"]`) | `done: … ran n steps in t s` and the log, plus the age note; or `failed at step k of n (…): reason`, the steps before it, the log so far, a snapshot with ids, "carry on by hand" |
| `desktop_save_macro` | `name`, `description`, `params` (`name=value`), `log` (`role`, `name` part, `limit`), `skip` | the stored steps; the notice goes to the chat |
| `desktop_save_recipe` | `name`, `text` (≤ 1,200 characters) | the notice goes to the chat |

No list tool: the list is in `desktop_start`'s result, the full list in Settings. All three work
only inside a desktop session, so a replay always has a picked, locked window.

### Step format

As built (section 1), with `name` a string or a list of alternatives. 1–40 steps, at least one
log step, `{param}` in `name` and `text`, params lower_snake_case. No loops or branches: none of
the three tasks needed one.

### Schema (`registry.db`, app-wide like user memory)

```sql
CREATE TABLE desktop_macros (
  id             INTEGER PRIMARY KEY,
  app            TEXT NOT NULL,              -- process name as agent-desktop reports it: chrome.exe, CalculatorApp.exe
  site           TEXT NOT NULL DEFAULT '',   -- browser: the origin; '' for other apps
  name           TEXT NOT NULL,              -- lower_snake_case
  description    TEXT NOT NULL,
  params         TEXT NOT NULL,              -- JSON array of names
  steps          TEXT NOT NULL,              -- JSON array of steps
  version        INTEGER NOT NULL DEFAULT 1,
  made_at        TEXT NOT NULL,
  made_by_model  TEXT NOT NULL,
  made_in_chat   TEXT,
  untrusted      INTEGER NOT NULL DEFAULT 1, -- element names came from the screen: every recorded macro
  last_worked_at TEXT,                       -- the save (the task worked) and each replay that finished
  last_run_at    TEXT,
  runs           INTEGER NOT NULL DEFAULT 0,
  failures       INTEGER NOT NULL DEFAULT 0,
  fail_streak    INTEGER NOT NULL DEFAULT 0,
  UNIQUE (app, site, name)
);

CREATE TABLE desktop_recipes (
  id            INTEGER PRIMARY KEY,
  app           TEXT NOT NULL,
  site          TEXT NOT NULL DEFAULT '',
  name          TEXT NOT NULL,
  text          TEXT NOT NULL,               -- at most 1,200 characters
  source        TEXT NOT NULL CHECK (source IN ('app', 'agent', 'user')),
  app_version   TEXT,                        -- source 'app': the Jenab version that shipped it
  made_at       TEXT NOT NULL,
  made_by_model TEXT,
  untrusted     INTEGER NOT NULL DEFAULT 0,
  last_used_at  TEXT,
  uses          INTEGER NOT NULL DEFAULT 0,
  UNIQUE (app, site, name, source)
);

-- Undo for the notice, like user_memory_changes (3.3)
CREATE TABLE desktop_saved_changes (
  id      INTEGER PRIMARY KEY,
  at      TEXT NOT NULL,
  kind    TEXT NOT NULL CHECK (kind IN ('macro', 'recipe')),
  item_id INTEGER NOT NULL,
  before  TEXT,                              -- JSON of the old row; NULL when the save made it
  chat_id TEXT
);

-- The last 20 replays per macro, for Settings. No screen text (8.10's log rule): lengths only.
CREATE TABLE desktop_macro_runs (
  id        INTEGER PRIMARY KEY,
  macro_id  INTEGER NOT NULL REFERENCES desktop_macros(id) ON DELETE CASCADE,
  version   INTEGER NOT NULL,
  at        TEXT NOT NULL,
  ok        INTEGER NOT NULL,
  failed_at INTEGER,                         -- step number
  reason    TEXT,                            -- the replayer's words ("no textfield named …"), names clipped
  skipped   INTEGER NOT NULL DEFAULT 0,
  seconds   REAL NOT NULL,
  log_chars INTEGER NOT NULL DEFAULT 0
);
```

The session's recorded steps live in memory, for that session only.

### Removal

- A macro goes after **3 failed replays in a row** (`fail_streak`), or **90 days** without a run.
  The third failure's result says so; Settings lists what went in the last 30 days.
- Agent recipes: after **1 year** unused. Recipes that come with the app (`source = 'app'`):
  never; a Jenab update replaces them by name. Recipes the user wrote or edited: never.
- Undo rows: 30 days.
- Settings › Desktop: per app, the macros (steps, params, last runs, failures) and recipes;
  edit, delete.

### Recipes to ship with the app

Only where SPIKE-039 and this spike saw models fail without one:
- **Calculator:** "The digits are separate buttons named Zero to Nine, with Plus, Minus and
  Equals. The result is the statictext 'Display is …'. For another mode, click Open Navigation
  and select the mode's option (a click doesn't switch it). Don't use Keep on top: the window
  drops out of the list."
- **A web search in a browser:** "Set the address bar to the site and press Return. If a cookie
  dialog appears, decline it. Type the words into the page's search box and press Return or its
  search button, wait for the results, and read titles and sites from the result links; skip ads
  and don't open them."

### SPEC text (a new 8.11 after 8.10)

> ### 8.11 Desktop macros and recipes (SPIKE-040)
>
> An agent can save what worked in a desktop session (8.10) and reuse it.
>
> - **Macro:** steps Jenab replays in the picked window without the model. Jenab records each step
>   of the session that worked, by role and exact name; `desktop_save_macro` names the macro,
>   marks the values that change (`{query}`, `{a}`) and says what to log. A replay finds each
>   element by role and name, keeps every 8.10 rule (the lock, the deny list, the consent,
>   password and key rules, the CAPTCHA stop) and returns its log. A replay that fails returns
>   the step, its log and a snapshot, and the agent carries on by hand; saving again keeps the
>   old names as alternatives.
> - **Recipe:** a short text on how to do a task in an app; the model follows it and checks each
>   step. Some come with Jenab; the agent adds its own.
> - **Saved without asking:** the chat shows a notice with *Undo* (like 3.3's memory saves). At
>   Strict a replay asks once, in a card that lists its steps.
> - `desktop_start` lists the macros and recipes for the picked app (and site). A macro that last
>   worked more than a day ago says so in its result, and the agent checks the window.
> - Stored in `registry.db` (`desktop_macros`, `desktop_recipes`); a macro goes after 3 failed
>   replays in a row or 90 days unused, an agent recipe after a year unused. Settings › Desktop
>   shows, edits and deletes them.
> - Screen text stays untrusted: a macro's element names come from the screen, so every macro is
>   marked untrusted; params come only from the model's call.
