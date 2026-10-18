# P2-13 — Token and accuracy benchmark
**Type:** Task
**Status:** Open
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
