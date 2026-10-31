# P2-13 — Token and accuracy benchmark
**Type:** Task
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P2-05, P2-09, P2-10, P2-11, TASK-013
**Requirements:** N-42

## Goal
Measure whether Jenab's structured memory beats a plain chat harness on a long task (PROPOSAL 11).

## Scope
- **The task:** a scripted two weeks of the Bitcoin project as a scenario set: set up the tables, add data on several days, change the goal (add Ethereum), and ask follow-up questions about earlier work. The data comes from fixtures, not live pages.
- **Two harnesses, same model and messages:**
  - Jenab with tables, memory, notes and cuts.
  - A plain chat harness: the whole history in every request, no tables or memory, compacted only when it hits the window.
- **Measures:** prompt tokens per turn, accuracy on the follow-up questions (each answer checked against the fixture), the prompt cache hit rate per chat, and the time per turn.
- **Tuning:** the history window sizes, memory caps and notes caps (SPEC 3.6, PROPOSAL 13 question 5), with the change and the effect recorded.
- **Models:** the reference models, 3 reps each.

## Done when
- A report with both harnesses side by side, and a decision on the default sizes. N-42 holds, or the report says where it doesn't and why.

## Result
Done on 2026-10-31 (commit 44c9d6d). N-42 doesn't hold on this set: Jenab is as accurate, but uses more tokens. TASK-033 follows up.

- **The set:** `scenarios/bench-bitcoin`, 20 turns over 12 scripted days (5-16 October 2026), a fake page a day, a daily "usual", a weekend catch-up, Ethereum added on day 8, and 9 follow-up questions checked against the pages. Committed before any scored run; two checks were widened afterwards for right answers scored wrong ("1.10 %", "**12** days").
- **Plain harness:** `-harness jenab,plain` in the scenario runner. Plain sends block 1 alone and the whole history with tool results as they came, dropping the oldest turns only past 0.8 of the window; it has no data tools, memory, notes or cards. Same model, messages and loop. The app never uses it.
- **Side by side** (3 reps, 51 checks a lane):

  | Model | Checks, Jenab / plain | Prompt tokens a turn, Jenab / plain | First request at turn 20 | Requests a turn | Seconds a turn |
  |---|---|---|---|---|---|
  | gemma4:31b | 51 / 51 | 23,787 / 12,275 | 10,229 / 13,558 | 2.87 / 1.50 | 6.4 / 4.0 |
  | gemini-3.5-flash-lite | 51 / 50 | 37,466 / 9,962 | 10,358 / 11,673 | 4.12 / 1.63 | 15.9 / 6.5 |
  | ministral-14b (extra) | 40 / 48 | 25,790 / 13,825 | 9,997 / 14,838 | 2.98 / 1.55 | 10.4 / 4.5 |

  Cache reads: Gemini 67 % / 53 %, ministral 15 % / 10 %; Ollama Cloud reports none. The scripted day clock can't expire a provider's cache, so both are optimistic.
- **Why Jenab costs more:** about 4,900 tokens of tool definitions and block 1 in every request against plain's 900, and data work adds rounds (fetch, insert, answer), so 2.9-4.1 requests a turn against 1.5-1.6. Jenab's history stays small and its first request at turn 20 is already smaller, but in two weeks plain's history only reaches 12-15k tokens. From gemma, not measured: plain passes Jenab around turn 23.
- **Accuracy:** the plain lane missed an average it worked out in its head (Gemini once, ministral 3 times), where Jenab's query got it right. ministral did worse in Jenab: a wrong Ethereum start date and a headline not found in its tables. In gemma's tuning runs Jenab twice left Ethereum out of "the usual", because the session notes never took the change.
- **Tuning** (gemma, 2 reps each): `history_min_turns: 1` cost 56 % more tokens and lost answers; 2, 5 with `history_max_turns: 10`, and `history_max_tokens: 8000` stayed within noise. The daily cut comes first, so the max sizes never bound. Memory ended at 70 tokens and notes at 45, far below the caps. **Defaults kept** (SPEC 3.6).
- **Requests:** 1,033 Ollama Cloud (the second key), 352 Gemini, 287 Mistral.
