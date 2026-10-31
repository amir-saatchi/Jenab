# TASK-027: history cards, live

From results/live-gemini. A run is done when every check but info passed and each tool the checks need ran without an error. Stopped runs (a quota, a rate limit or the harness budget) are left out. History calls are search_history and read_messages in the follow-up.

## gemini/gemini-3.5-flash-lite

Requests the provider got (all runs, stopped ones too, titles and memory too): 268.

| follow-up | condition | done | runs with history calls | history calls/run | requests/run | prompt tokens/run | broke | stopped |
|---|---|---|---|---|---|---|---|---|
| repeat | today | 7/8 (88%) | 7/8 (88%) | 1.9 | 9.1 | 46988.0 | 0 | 0 |
| repeat | cards | 6/8 (75%) | 1/8 (12%) | 0.1 | 4.9 | 25327.4 | 0 | 0 |
| paraphrase | today | 6/8 (75%) | 6/8 (75%) | 2.9 | 8.6 | 48464.9 | 0 | 0 |
| paraphrase | cards | 6/8 (75%) (2 of them today's: no card) | 5/8 (62%) | 2.6 | 7.8 | 44758.6 | 0 | 0 |
| control | today | 8/8 (100%) | 0/8 (0%) | 0.0 | 2.2 | 10112.5 | 0 | 0 |
| control | cards | 8/8 (100%) (3 of them today's: no card) | 0/8 (0%) | 0.0 | 3.4 | 16098.9 | 0 | 0 |

## By follow-up

Done of the counted runs, today / cards; "=" is a follow-up with no card, which has no cards run.

| follow-up | test | cards | gemini-3.5-flash-lite |
|---|---|---|---|
| C1_repeat | repeat | [4] | 1/1 / 1/1 |
| C1_para | paraphrase | [16] | 1/1 / 1/1 |
| C1_ctrl | control | [4 13] | 1/1 / 1/1 |
| C2_repeat | repeat | [3] | 1/1 / 1/1 |
| C2_para | paraphrase | [8 12] | 0/1 / 0/1 |
| C2_ctrl | control | [1 8] | 1/1 / 1/1 |
| C3_repeat | repeat | [5] | 1/1 / 0/1 |
| C3_para | paraphrase | none | 1/1 / = |
| C3_ctrl | control | none | 1/1 / = |
| C4_repeat | repeat | [6] | 0/1 / 1/1 |
| C4_para | paraphrase | [6] | 1/1 / 1/1 |
| C4_ctrl | control | [8] | 1/1 / 1/1 |
| C5_repeat | repeat | [1 4] | 1/1 / 1/1 |
| C5_para | paraphrase | [11 17] | 1/1 / 1/1 |
| C5_ctrl | control | [3 12] | 1/1 / 1/1 |
| C6_repeat | repeat | [3 9] | 1/1 / 0/1 |
| C6_para | paraphrase | [5 18] | 1/1 / 1/1 |
| C6_ctrl | control | none | 1/1 / = |
| C7_repeat | repeat | [7] | 1/1 / 1/1 |
| C7_para | paraphrase | none | 1/1 / = |
| C7_ctrl | control | [10] | 1/1 / 1/1 |
| C8_repeat | repeat | [5] | 1/1 / 1/1 |
| C8_para | paraphrase | [19] | 0/1 / 0/1 |
| C8_ctrl | control | none | 1/1 / = |
