# SPIKE-040 — Desktop macros and recipes
**Type:** Spike
**Status:** Done
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

## Result
Done on 2026-11-03 with agent-desktop 1.0.2. Code and numbers in `spikes/040-desktop-macros/` (`results.md`). **Go, with one change:** Jenab records the steps; the model only names the macro, its params and what to log. SPEC 8.11 has the design; TASK-038 builds it.

- **A macro saves a lot.** With one, a task took 3–5 requests and 4.7k–10k prompt tokens; the replay itself 1–16 s. By hand: Calculator 20–30 requests and 73k–165k tokens, the stand-in search site 10–13 and 10k–15k, Google 15–18 and 46k–58k.
- **Models write bad steps.** Of 8 macros the models wrote, 2 replayed. Faults: wrong digits, no params, useless log steps. One broken Calculator macro replayed "fine", and Gemini covered it with its own arithmetic.
- **Recorded steps work.** All 5 macros Jenab recorded that were tried replayed right; 2 had a useless log. One was not tried; one was recorded from a wrong run (Mistral's 38-step Calculator "fix" with no params).
- **Per model:**
  - gemini-3.5-flash-lite: the recorded site macro 3/3 (3–4 requests, 4.7k–7.2k); its written editor macro failed once, was fixed, then 3–4 requests; its written Calculator macro 0/2.
  - ministral-14b: its written Calculator macro 4/5 (3–5 requests, 5.4k–9.9k); the recorded site macro in 4 requests. It skipped old macros when the list showed their age.
- **Age:** every run with a macro older than a day checked the window with a snapshot.
- **Breaks:** no consent dialog, and Calculator in Scientific and Programmer modes, were fine. German pages: Gemini fixed the macro, but the fix then failed in English, so a fix should keep both names. German Chrome: Gemini fixed it; Mistral did the task by hand and saved nothing. Calculator's Date mode: Gemini's fix works in all modes; Mistral's was wrong and replaced its working macro. Handover from a failed replay to work by hand: 5 of 6.
- **Recipes:** no gain on the site (Gemini 13 requests and about 15k with or without; Mistral 10–11 and 10k–12k). Calculator with a recipe: Gemini 1/1, Mistral 0/2. Hints, not a cost cut: ship Calculator and web search only.
- **Google:** two CAPTCHAs (a fresh profile, consent declined only) stopped the Google runs, as the rules say; the browser part moved to a local stand-in search site (consent on or off, English or German). The model runs before that: 15–18 requests, 46k–58k tokens.
- **Not tried:** Jenab checking the log steps, names kept as alternatives on a fix, and keeping params on a fix. They are in TASK-038.
- **Requests:** Gemini 310 (`GEMINI_API_KEY`), Mistral 546 with 429 retries (`MISTRAL_API_KEY`).
