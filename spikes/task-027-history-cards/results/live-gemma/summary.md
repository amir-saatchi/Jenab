# TASK-027 live: gemma4:31b, 3 reps, thresholds frozen

Run on 2026-10-30, Ollama Cloud. There were 454 requests in all (285 today, 169 cards), with no quota or rate limit, no stopped runs and no broken runs. ministral-14b and Gemini are held, as Main-Session decided. results.md is the harness's raw table.

**A correction.** Two checks in the set were wrong, because a call's arguments are JSON and `>` arrives as `>`:

- C4_repeat's `HAVING COUNT(*) >= 3` failed every run in both conditions, although every call had it. In the numbers below, C4_repeat counts as done in all 6 runs.
- C4_para's check had the same bug, but no run called save_pipeline, so its numbers don't change.

Both checks are fixed in the set (`\x5cu003e`).

## By group, on the follow-ups that get a card

Follow-ups with no card are left out of this table. They are C3_para, C7_para, C3_ctrl, C6_ctrl and C8_ctrl, and they have no cards run. Their today runs: 15/15 done.

| group | carded follow-ups | condition | done | prompt tokens/run | requests/run | history calls/run |
|---|---|---|---|---|---|---|
| repeat | 8 | today | 13/24 | 34,861 | 5.1 | 2.0 |
| repeat | 8 | cards | 24/24 | 19,427 | 2.8 | 0.0 |
| paraphrase | 6 | today | 4/18 | 19,627 | 3.5 | 1.1 |
| paraphrase | 6 | cards | 9/18 | 23,726 | 3.8 | 1.4 |
| control | 5 | today | 15/15 | 11,366 | 2.3 | 0.0 |
| control | 5 | cards | 15/15 | 11,639 | 2.2 | 0.0 |

## Did a wrong or false card change the answer?

Turn 23 was read in all 3 reps of each follow-up below, with and without cards.

- **Controls with a false card** (C1, C2, C4, C5 and C7, 15 runs). The calls and answers are the same as today's, and no run acted on the carded turn. In C1_ctrl, two runs dropped a ROUND() and used another view id. The checks pass.
- **C1_para** (the card is t16, the needed turn is t9). Today, gemma offers to build the pipeline but doesn't (0/3). With the card, it searches history in every run, reads turn 9 and saves the pipeline (3/3). The wrong card led it to search, and it never acted on t16.
- **C4_para** (the card is t6, the needed turn is t12). gemma only describes a pipeline, in both conditions (0/3 each), and never mentions the pace table.
- **C8_para** (the card is t19, the needed turn is t10). gemma runs the same query in both conditions and fails the same checks (0/3 each).
- **C6_para** (the cards are t5 and t18, the needed turn is t9). Done in both conditions (3/3 each).
- **C2_para** (the needed t8 is carded, plus t12). The runs fail in both conditions and query the database instead.
- **Extra cards beside the needed turn** (C5_repeat t1, C6_repeat t9, C2_para t12, C5_para t17). No harm: C5_repeat and C6_repeat went from 2/3 and 0/3 to 3/3.

In short, on gemma, right cards made repeats reliable and cheaper: 54% to 100% done, with 44% fewer prompt tokens. False cards cost controls about 2% in tokens and changed no answer. Wrong cards on paraphrases never sent the model to the wrong turn, and one of them led it to search history.
