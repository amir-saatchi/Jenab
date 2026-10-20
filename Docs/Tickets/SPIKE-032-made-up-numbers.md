# SPIKE-032 — Catch made-up numbers and let the model correct them
**Type:** Spike
**Status:** Done
**Gate:** 3
**Needs:** TASK-014

## Question
In TASK-014's `missing-features` set, asked for a 30-day BTC chart while its tools gave only today's price, Gemini made up the other 29 prices in 2 of 3 runs. Once it also saved them as an HTML chart. Prompt text for one model is out (SPEC 1).

Can the harness notice numbers that no source gave, and get the model to correct itself, the same way for every model, without flagging honest answers?

## The check (proposed on 2026-10-13)
1. **Sources:** everything the model was given: tool results in the window, the user's messages, blocks 1–5 (the date, the project card) and earlier messages.
2. **Numbers checked:** the numbers in the turn's answer and in what write tools save (`bucket_put` content).
   - Normalized first: thousands separators, decimals, `k`/`M`, `%`, currency signs, Persian and Arabic digits (5.8).
   - Left out: small integers, years and dates, list numbering, and numbers in the user's own message.
   - A number counts as sourced if it matches a source within rounding (61,240 = 61.2k = 61,240.00).
3. **Fires when:** at least K numbers have no source. K is measured, not guessed. Computed values (an average, a conversion to EUR) have no source either, so K has to let a few through.
4. **What the model gets:** an app notice, like the existing ones in square brackets, the same for every model, e.g.
   `[app check: 28 numbers in your answer appear in no tool result or message, e.g. 58.2, 59.1, 60,400. If no tool gave them, say so or get them with a tool.]`
5. **What the user sees:** a small mark on that answer, "some numbers have no source", whether or not the model corrects itself. It could be a `notice` part, like `answer_cut` (2.3, 8.3).

## Conditions
1. **None:** today's loop, to count made-up numbers and false alarms.
2. **Same turn:** when the check fires, one more request in the same turn with the notice. The correction follows the answer the user already sees.
3. **Next turn:** the notice opens the next turn instead, with no extra request.
4. **Mark only:** the check runs and marks, and the model is never told. This measures false alarms by themselves.

For each condition, K is 3, 5 and 8.

## Tests
- **Made up:** `missing-features` F2 (table) and F4 (chart). Plus new scenarios where a tool gives part of the data and the model is tempted to fill in the rest, e.g. 3 of 7 days.
- **Honest, must not fire:**
  - multi-part t3 and t4 (query results, a conversion to EUR, averages over real rows)
  - F5 (a moving average explained with an example)
  - the Persian set from TASK-011
- **Correction:** after the notice, does the model take the numbers back, get them with a tool, or argue? Does it start inventing again in the same turn?

The prototype goes in the agent behind a setting only the scenario runner sets, so the runner switches conditions. The check's verdict is written into each transcript, and a sample is labelled by hand to count misses and false alarms.

## Models
The ones available now, with the same prompts for all (SPEC 1):
- `glm-4.5-flash` on Z.ai
- a Groq model, picked at the start from what the key lists (`gpt-oss-20b` passed P1-08's smoke test)
- `gemma4:31b` and `gpt-oss:120b` on Ollama Cloud
- `gemini-3.5-flash-lite` when its shared free quota allows

## Measures
- Made-up answers caught, and honest answers flagged (false alarms), per K.
- After the notice: corrected, unchanged or worse.
- Extra requests, tokens and time, only where the check fires.
- How long the check takes on a long answer and a long window.

## Done when
Each condition has results per model, with at least 3 reps per cell. There is a decision on the check (8.3), K, same turn or next turn, the notice text, and the mark on the answer (2.3). Or the decision is that the check isn't worth its false alarms.

## Round 1 (2026-10-13)
The code is committed on the branch `worktree-agent-af0e299be34cd117e` (commit dd91c98), not merged into main: the check (`internal/agent/grounding.go`), the `grounding` scenario set, and `spikes/032-made-up-numbers/` with `results.md` and the labels. About 335 runs on glm-4.5-flash, gemma4:31b and gpt-oss:120b. Groq stopped on 429s, and Gemini was left out to keep its quota.

- **How it works:** the numbers in the answer and in what write tools save are matched against every number in the request, within rounding. Dates, years, small whole numbers, list numbering and names like `v3` are skipped. It fires at K or more numbers with no source.
- **Catching:** at K=3, 9 of 12 made-up answers. All of them came from glm; gemma and gpt-oss made up none.
- **Misses:** a real number on the wrong day, and a made-up file the model read back, which then counted as a source.
- **False alarms:** 13 of 249 honest answers, all worked examples.
- **Same turn:** the model admitted the numbers 4 of 4 times, but fixed the saved file only 2 of 4. Each firing cost 1–6 extra requests and 12–63 s. Next turn: the one notice was ignored. No model took back a correct number.
- **Fixes before the app:** keep numbers the model wrote unsourced when read back; count each value once; don't flag numbers quoted from the notice; make the correction fix the saved file; decide on worked examples.

## Round 2: correct answers full of numbers
Round 1's honest answers had at most 2 computed numbers, so K=3 let them through. The check can't tell a computed number from a made-up one. Test answers that are correct but full of numbers:

- a computed table, such as a 7-day moving average over 30 real prices
- several conversions in one answer (7 prices to EUR)
- what-ifs and worked examples
- general knowledge ("the supply cap is 21 million")
- a saved script or config with constants (`timeout: 5000`)
- percentages written another way (`0.052` and `5.2%`)

Run them on gemma4:31b and gpt-oss:120b, which made nothing up in round 1, so any firing is a false alarm. Then compare three ways to cut false alarms, the same for every model:
1. Numbers a tool computed count as sourced (a calculator tool, or `query` in Phase 2).
2. Two thresholds: a low one for the user's mark, a high one, or any number in a saved file, for telling the model.
3. Mark only: the model is never told.

Done when: false alarms per case and per fix are known, and the decision says which mode, if any, goes in the app.

## Result
Done on 2026-10-20 (commit 2e46851), after one review round for round 2. **Decision: no number check in v1.** The details are in `spikes/032-made-up-numbers/results.md`.

- **Round 2:** 9 honest scenarios full of numbers (the `grounding-honest` set) on gemma4:31b and gpt-oss:120b, 3 reps, 162 runs.
  - **Mark only, K=3:** marks 24 of 54 honest answers. No K separates them from made-up ones: round 1's made-up answers had 3–6 numbers without a source, these have 3–21.
  - **Telling the model** (same turn): all 23 told runs were false alarms, and 7 did harm. The model took back correct numbers, redid a correct answer wrong, or stopped to ask the user. It cost 1.47× the requests and 1.73× the prompt tokens.
  - **Fix 1, computing through `query`,** halves the marks, but it is a prompt change, and it made gemma refuse a general-knowledge question in 3 of 3 runs.
  - **Fix 2, a higher threshold for telling,** can't catch round 1's made-up answers and still tells on honest ones.
- **Round 1's fixes** are in the check:
  - a number saved without a source isn't a source when it is read back;
  - each value counts once;
  - the notice's own numbers are exempt;
  - saved files are checked after a correction;
  - a percent matches its fraction;
  - Persian is read like English.
- **Why nothing goes in v1:** telling harms honest work, and marking flags about half of correct answers full of numbers. The check also still has open points: the read-back tie is a SQL heuristic, its state is in memory only, and a repeat of quoted numbers is exempt.
- **What stays:** the check remains in the code, spike-only. Only the scenario runner sets `agent.Deps.Grounding`, so it can still measure made-up numbers in scenario runs.
- **Later:** revisit when tool-computed numbers count as sourced through the data tools, and when catching can be measured on more than one model that makes numbers up (round 1 had only glm-4.5-flash).
