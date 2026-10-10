# SPIKE-031 runs

One row per graph. *Valid* counts define and expand calls: on the first try / after the retry / not at all. *Next* is how often the model's `next` named a thought that could be expanded. *Spread* is the mean standard deviation of sibling weights. *Dups* are pairs of titles with mostly the same words. *Index* is the size of the index in the last expand call, in characters.

| Graph | Problem | Model | Mode | Thoughts | Kinds (sol/step/crit/merge) | Dead ends | Depth | Calls | Tokens in / out | Time | Valid | Failures | Next | Finish | Reweights | Spread | Dups | Index |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 7 | b1-exam | ollama-cloud/gemma4:31b | one | 8 | 1/4/2/0 | 0 | 4 | 10 | 15561 / 3710 | 33s | 6/2/0 | no_tool_call 2 | 4/7 | model | 0 | 0.09 | 1 | 517 |
| 6 | b1-exam | ollama-cloud/gemma4:31b | siblings | 16 | 3/9/3/0 | 0 | 4 | 8 | 9568 / 4084 | 31s | 6/0/0 | – | 5/5 | budget | 1 | 0.06 | 0 | 883 |
| 16 | b1-exam | zai/glm-4.5-flash | one | 16 | 1/13/1/0 | 0 | 4 | 18 | 26700 / 11641 | 6m2s | 16/0/0 | – | 11/15 | budget | 0 | 0.01 | 0 | 996 |
| 15 | b1-exam | zai/glm-4.5-flash | siblings | 16 | 3/7/4/1 | 0 | 3 | 8 | 11378 / 8978 | 4m41s | 5/1/0 | check 1 | 5/5 | budget | 0 | 0.08 | 0 | 794 |
| 12 | bakery | ollama-cloud/gemma4:31b | one | 16 | 1/11/3/0 | 0 | 4 | 18 | 27968 / 5318 | 53s | 15/1/0 | no_tool_call 1 | 5/15 | budget | 0 | 0.05 | 1 | 1579 |
| 10 | bakery | ollama-cloud/gemma4:31b | siblings | 16 | 3/10/2/0 | 0 | 3 | 8 | 9972 / 4261 | 36s | 6/0/0 | – | 5/5 | budget | 0 | 0.08 | 0 | 1289 |
| 20 | bakery | zai/glm-4.5-flash | one | 16 | 1/14/0/0 | 0 | 4 | 18 | 32849 / 13297 | 6m14s | 16/0/0 | – | 10/15 | budget | 0 | 0.00 | 1 | 1404 |
| 19 | bakery | zai/glm-4.5-flash | siblings | 16 | 3/8/4/0 | 0 | 3 | 8 | 10004 / 8157 | 4m52s | 6/0/0 | – | 5/5 | budget | 0 | 0.08 | 1 | 999 |
| 5 | db-move | ollama-cloud/gemma4:31b | one | 5 | 1/2/1/0 | 0 | 4 | 7 | 7679 / 2834 | 19s | 5/0/0 | – | 3/4 | model | 0 | 0.00 | 0 | 310 |
| 4 | db-move | ollama-cloud/gemma4:31b | siblings | 16 | 3/9/3/0 | 1 | 4 | 9 | 10531 / 3475 | 38s | 7/0/0 | – | 6/6 | budget | 0 | 0.10 | 0 | 933 |
| 14 | db-move | zai/glm-4.5-flash | one | 16 | 1/11/3/0 | 0 | 4 | 19 | 29846 / 28613 | 16m21s | 15/1/1 | no_tool_call 1, schema 1, max_tokens 1 | 9/16 | budget | 0 | 0.03 | 1 | 1091 |
| 13 | db-move | zai/glm-4.5-flash | siblings | 16 | 3/8/4/0 | 0 | 4 | 9 | 10866 / 8026 | 5m12s | 7/0/0 | – | 6/6 | budget | 0 | 0.09 | 0 | 1054 |
| 3 | report-page | ollama-cloud/gemma4:31b | one | 16 | 1/13/1/0 | 0 | 4 | 19 | 27407 / 4166 | 1m52s | 16/0/1 | no_tool_call 2 | 4/16 | budget | 0 | 0.06 | 0 | 1121 |
| 2 | report-page | ollama-cloud/gemma4:31b | siblings | 16 | 3/8/4/0 | 0 | 4 | 8 | 9160 / 3351 | 24s | 6/0/0 | – | 5/5 | budget | 0 | 0.10 | 0 | 936 |
| 11 | report-page | zai/glm-4.5-flash | one | 16 | 1/14/0/0 | 0 | 4 | 18 | 24110 / 11057 | 5m46s | 16/0/0 | – | 12/15 | budget | 0 | 0.01 | 2 | 1150 |
| 1 | report-page | zai/glm-4.5-flash | siblings | 16 | 3/9/3/0 | 0 | 3 | 8 | 9225 / 5882 | 6m13s | 6/0/0 | – | 5/5 | budget | 0 | 0.07 | 0 | 833 |
| 9 | umzug | ollama-cloud/gemma4:31b | one | 16 | 1/13/1/0 | 0 | 4 | 19 | 33623 / 6036 | 1m21s | 13/3/1 | no_tool_call 5 | 8/16 | budget | 0 | 0.05 | 0 | 1196 |
| 8 | umzug | ollama-cloud/gemma4:31b | siblings | 16 | 3/11/1/0 | 0 | 3 | 8 | 9558 / 3901 | 28s | 6/0/0 | – | 5/5 | budget | 0 | 0.10 | 0 | 953 |
| 18 | umzug | zai/glm-4.5-flash | one | 16 | 1/11/3/0 | 0 | 4 | 18 | 27547 / 11518 | 5m57s | 16/0/0 | – | 3/15 | budget | 0 | 0.04 | 0 | 1178 |
| 17 | umzug | zai/glm-4.5-flash | siblings | 16 | 3/10/2/0 | 0 | 3 | 8 | 10028 / 7457 | 3m23s | 6/0/0 | – | 5/5 | budget | 0 | 0.06 | 0 | 874 |
