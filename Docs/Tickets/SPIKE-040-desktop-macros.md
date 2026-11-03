# SPIKE-040 — Desktop macros and recipes
**Type:** Spike
**Status:** Open
**Gate:** 3
**Needs:** SPIKE-039

## Question
In SPIKE-039 a Google search took 16–22 turns and 22k–44k tokens, every time. Can the agent save what worked and reuse it, so the next time costs a fraction? The app ships the way to save and replay, not the procedures: the agent makes them by doing the task first.

## Design to test (the user's decisions, 2026-11-03)
- **Two kinds:**
  - **Recipe:** a short text on how to do a task in an app ("type the words into the search box, press Return, read the first five results, skip ads"). The model still looks at each step. Some come with the app; the agent adds its own. They last longer than macros.
  - **Macro:** steps Jenab replays without the model, which gets only the result.
- **A macro's steps:**
  - find an element by role and name, never by snapshot id;
  - an action: click, set value, toggle, expand, select, a key from the 8.10 list;
  - wait for a text, or for one to go;
  - optional steps, for a dialog that may not appear (a consent dialog);
  - **log steps** that record what an element shows, so the agent can see how far a replay got and what was on screen;
  - parameters filled in at each run (`{query}`, `{a}`, `{b}`); no loops or branches.
- **Saved without asking:** a skill tells the agent to save a macro after a task worked, and to use one when it exists. The chat shows a notice with *Undo*, like memory saves (TASK-030).
- **Age:** a macro keeps when it last worked. After a day, the replay's result says so ("last worked 3 days ago: check the result") and the agent checks before trusting it. Macros that keep failing, or go unused for a long time, are removed; the spike proposes when.
- **Failure:** a replay stops at the failing step and returns its log and a snapshot, so the agent carries on by hand and saves a fixed version.
- **Storage:** tables in `registry.db`, app-wide like user memory: name, app and site, description, parameters, steps or text, made, last worked, runs, failures, and a mark when made in a turn that read untrusted content.
- **Safety without asking:** a replay keeps every 8.10 rule: the window picked in a card each session, the lock, the deny list, password and consent refusal, the key list, and each action asked at Strict. Screen text stays untrusted. The steps are in Settings, where macros can be deleted.

## Scope
Its own module in `spikes/040-desktop-macros/`, reusing SPIKE-039's harness code (copied).
1. **Format and replayer:** the macro format above, replayed through agent-desktop's command line (`find` by role and name).
2. **Tasks:** a Google search (the first five results for `{query}`), a Calculator sum (`{a}` + `{b}`), a sentence in the test editor.
3. **Making them:** each model does each task once with the save instruction. Is a macro saved, and does it replay?
4. **Replays:** 5 per task and model with other parameters: success, tokens, turns and time, next to SPIKE-039's runs by hand.
5. **Breaking them:** Calculator in another mode, Chrome with the consent dialog and without, the page in German and in English. Does the handover work, and does the agent save a fixed version?
6. **Recipes:** the agent writes one after a task; the next run with it and without it: tokens and turns.
7. **Proposal:** the skill text, the tools (save, run, list), the schema, when macros are removed, which recipes should come with the app, and SPEC text.

## Rules
- As in SPIKE-039: only windows the test opens (a test editor, Calculator, Chrome with a fresh profile in the scratch folder), never the user's windows, profile or accounts; close what the spike opened.
- Live google.com searches, as in SPIKE-039's Chrome test: decline consent only; a CAPTCHA stops the run, and two stop the batch.
- agent-desktop 1.0.2 as in SPIKE-039, checked against its `checksums.txt`; ask before downloading it again.
- Models: `gemini-3.5-flash-lite` (`GEMINI_API_KEY`, under 400 requests a day) and `ministral-14b-latest` (`MISTRAL_API_KEY`, paced under 30 a minute). Ask before any other key.
- No new dependencies in Jenab's `go.mod`.

## Done when
- The numbers above are recorded, with the failures.
- A go or no-go, and if go, the design for the build ticket.
