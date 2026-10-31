# TASK-027: history cards, live

From results/live-ministral. A run is done when every check but info passed and each tool the checks need ran without an error. Stopped runs (a quota, a rate limit or the harness budget) are left out. History calls are search_history and read_messages in the follow-up.

## mistral/ministral-14b-latest

Requests the provider got (all runs, stopped ones too, titles and memory too): 384.

| follow-up | condition | done | runs with history calls | history calls/run | requests/run | prompt tokens/run | broke | stopped |
|---|---|---|---|---|---|---|---|---|
| repeat | today | 4/24 (17%) | 4/24 (17%) | 0.4 | 5.1 | 37108.9 | 0 | 0 |
| repeat | cards | 20/24 (83%) | 8/24 (33%) | 0.8 | 3.7 | 26809.8 | 0 | 0 |
| paraphrase | today | 0/24 (0%) | 0/24 (0%) | 0.0 | 1.4 | 6733.1 | 0 | 0 |
| paraphrase | cards | 3/24 (12%) (6 of them today's: no card) | 6/24 (25%) | 0.5 | 2.4 | 12282.2 | 0 | 0 |
| control | today | 24/24 (100%) | 0/24 (0%) | 0.0 | 2.1 | 11105.0 | 0 | 0 |
| control | cards | 24/24 (100%) (9 of them today's: no card) | 4/24 (17%) | 0.2 | 2.4 | 13061.8 | 0 | 0 |

## By follow-up

Done of the counted runs, today / cards; "=" is a follow-up with no card, which has no cards run.

| follow-up | test | cards | ministral-14b-latest |
|---|---|---|---|
| C1_repeat | repeat | [4] | 0/3 / 3/3 |
| C1_para | paraphrase | [16] | 0/3 / 0/3 |
| C1_ctrl | control | [4 13] | 3/3 / 3/3 |
| C2_repeat | repeat | [3] | 1/3 / 3/3 |
| C2_para | paraphrase | [8 12] | 0/3 / 0/3 |
| C2_ctrl | control | [1 8] | 3/3 / 3/3 |
| C3_repeat | repeat | [5] | 0/3 / 3/3 |
| C3_para | paraphrase | none | 0/3 / = |
| C3_ctrl | control | none | 3/3 / = |
| C4_repeat | repeat | [6] | 0/3 / 3/3 |
| C4_para | paraphrase | [6] | 0/3 / 0/3 |
| C4_ctrl | control | [8] | 3/3 / 3/3 |
| C5_repeat | repeat | [1 4] | 0/3 / 3/3 |
| C5_para | paraphrase | [11 17] | 0/3 / 3/3 |
| C5_ctrl | control | [3 12] | 3/3 / 3/3 |
| C6_repeat | repeat | [3 9] | 0/3 / 0/3 |
| C6_para | paraphrase | [5 18] | 0/3 / 0/3 |
| C6_ctrl | control | none | 3/3 / = |
| C7_repeat | repeat | [7] | 3/3 / 2/3 |
| C7_para | paraphrase | none | 0/3 / = |
| C7_ctrl | control | [10] | 3/3 / 3/3 |
| C8_repeat | repeat | [5] | 0/3 / 3/3 |
| C8_para | paraphrase | [19] | 0/3 / 0/3 |
| C8_ctrl | control | none | 3/3 / = |
