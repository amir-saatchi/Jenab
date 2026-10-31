# TASK-027: history cards, live

From results/live-gemma. A run is done when every check but info passed and each tool the checks need ran without an error. Stopped runs (a quota, a rate limit or the harness budget) are left out. History calls are search_history and read_messages in the follow-up.

## ollama-cloud/gemma4:31b

Requests the provider got (all runs, stopped ones too, titles and memory too): 454.

| follow-up | condition | done | runs with history calls | history calls/run | requests/run | prompt tokens/run | broke | stopped |
|---|---|---|---|---|---|---|---|---|
| repeat | today | 10/24 (42%) | 15/24 (62%) | 2.0 | 5.1 | 34860.7 | 0 | 0 |
| repeat | cards | 21/24 (88%) | 0/24 (0%) | 0.0 | 2.8 | 19427.0 | 0 | 0 |
| paraphrase | today | 10/24 (42%) | 13/24 (54%) | 2.0 | 4.6 | 26520.5 | 0 | 0 |
| paraphrase | cards | 15/24 (62%) (6 of them today's: no card) | 12/24 (50%) | 2.2 | 4.8 | 29594.8 | 0 | 0 |
| control | today | 24/24 (100%) | 0/24 (0%) | 0.0 | 2.2 | 10428.0 | 0 | 0 |
| control | cards | 24/24 (100%) (9 of them today's: no card) | 0/24 (0%) | 0.0 | 2.1 | 10599.0 | 0 | 0 |

## By follow-up

Done of the counted runs, today / cards; "=" is a follow-up with no card, which has no cards run.

| follow-up | test | cards | gemma4:31b |
|---|---|---|---|
| C1_repeat | repeat | [4] | 3/3 / 3/3 |
| C1_para | paraphrase | [16] | 0/3 / 3/3 |
| C1_ctrl | control | [4 13] | 3/3 / 3/3 |
| C2_repeat | repeat | [3] | 3/3 / 3/3 |
| C2_para | paraphrase | [8 12] | 0/3 / 0/3 |
| C2_ctrl | control | [1 8] | 3/3 / 3/3 |
| C3_repeat | repeat | [5] | 1/3 / 3/3 |
| C3_para | paraphrase | none | 3/3 / = |
| C3_ctrl | control | none | 3/3 / = |
| C4_repeat | repeat | [6] | 0/3 / 0/3 |
| C4_para | paraphrase | [6] | 0/3 / 0/3 |
| C4_ctrl | control | [8] | 3/3 / 3/3 |
| C5_repeat | repeat | [1 4] | 2/3 / 3/3 |
| C5_para | paraphrase | [11 17] | 1/3 / 3/3 |
| C5_ctrl | control | [3 12] | 3/3 / 3/3 |
| C6_repeat | repeat | [3 9] | 0/3 / 3/3 |
| C6_para | paraphrase | [5 18] | 3/3 / 3/3 |
| C6_ctrl | control | none | 3/3 / = |
| C7_repeat | repeat | [7] | 1/3 / 3/3 |
| C7_para | paraphrase | none | 3/3 / = |
| C7_ctrl | control | [10] | 3/3 / 3/3 |
| C8_repeat | repeat | [5] | 0/3 / 3/3 |
| C8_para | paraphrase | [19] | 0/3 / 0/3 |
| C8_ctrl | control | none | 3/3 / = |
