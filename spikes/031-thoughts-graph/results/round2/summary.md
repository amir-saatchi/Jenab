# SPIKE-031 round 2: graphs

One row per graph. *Valid* counts define and expand calls: on the first try / after the retry / not at all. *Next* is how often the model's `next` named a thought that could be expanded. *Spread* is the mean standard deviation of sibling weights. *Dups* are pairs of titles with mostly the same words. *Index* is the size of the index in the last expand call, in characters.

| Graph | Problem | Model | Mode | Thoughts | Kinds (sol/step/crit/merge) | Dead ends | Depth | Calls | Tokens in / out | Time | Valid | Failures | Next | Finish | Reweights | Spread | Dups | Index |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 9 | b1-exam | gemini/gemini-3.5-flash-lite | index | 16 | 2/10/3/0 | 0 | 4 | 9 | 14444 / 3616 | 44s | 7/1/0 | schema 1 | 6/7 | budget | 2 | 0.03 | 2 | 1255 |
| 11 | b1-exam | gemini/gemini-3.5-flash-lite | path | 16 | 3/10/2/0 | 0 | 3 | 8 | 7284 / 2923 | 15s | 7/0/0 | – | 0/6 | budget | 0 | 0.08 | 0 | 0 |
| 10 | b1-exam | ollama-cloud/gemma4:31b | index | 16 | 3/8/3/1 | 0 | 3 | 10 | 30931 / 10915 | 59s | 1/5/3 | schema 11 | 5/8 | budget | 2 | 0.06 | 0 | 940 |
| 12 | b1-exam | ollama-cloud/gemma4:31b | path | 16 | 3/8/4/0 | 0 | 3 | 7 | 6437 / 3082 | 19s | 6/0/0 | – | 0/5 | budget | 0 | 0.10 | 3 | 0 |
| 17 | bakery | gemini/gemini-3.5-flash-lite | index | 16 | 3/7/5/0 | 1 | 4 | 9 | 12138 / 3755 | 19s | 8/0/0 | – | 6/7 | budget | 4 | 0.06 | 0 | 1585 |
| 19 | bakery | gemini/gemini-3.5-flash-lite | path | 16 | 3/8/4/0 | 0 | 4 | 9 | 8591 / 3481 | 36s | 8/0/0 | – | 0/7 | budget | 0 | 0.07 | 1 | 0 |
| 18 | bakery | ollama-cloud/gemma4:31b | index | 16 | 3/8/4/0 | 0 | 3 | 8 | 22981 / 6814 | 33s | 1/5/1 | schema 7 | 5/6 | budget | 2 | 0.12 | 0 | 1299 |
| 20 | bakery | ollama-cloud/gemma4:31b | path | 16 | 3/8/4/0 | 0 | 3 | 7 | 7202 / 3358 | 25s | 6/0/0 | – | 0/5 | budget | 0 | 0.10 | 0 | 0 |
| 5 | db-move | gemini/gemini-3.5-flash-lite | index | 16 | 3/9/3/0 | 0 | 4 | 10 | 12992 / 2889 | 17s | 9/0/0 | – | 7/8 | budget | 1 | 0.04 | 0 | 1165 |
| 7 | db-move | gemini/gemini-3.5-flash-lite | path | 16 | 3/8/4/0 | 1 | 4 | 7 | 6324 / 2644 | 41s | 6/0/0 | – | 0/5 | budget | 0 | 0.11 | 0 | 0 |
| 6 | db-move | ollama-cloud/gemma4:31b | index | 16 | 3/7/5/0 | 0 | 3 | 9 | 21014 / 5683 | 32s | 3/4/1 | schema 5, check 1 | 6/7 | budget | 0 | 0.10 | 0 | 1117 |
| 8 | db-move | ollama-cloud/gemma4:31b | path | 16 | 3/7/5/0 | 1 | 3 | 7 | 6425 / 2711 | 19s | 6/0/0 | – | 0/5 | budget | 0 | 0.13 | 1 | 0 |
| 2 | report-page | gemini/gemini-3.5-flash-lite | index | 16 | 3/7/4/1 | 0 | 4 | 9 | 11089 / 2748 | 18s | 8/0/0 | – | 6/7 | budget | 4 | 0.06 | 0 | 1090 |
| 3 | report-page | gemini/gemini-3.5-flash-lite | path | 16 | 3/7/5/0 | 0 | 3 | 8 | 7960 / 2611 | 27s | 6/1/0 | schema 1 | 0/6 | budget | 0 | 0.09 | 0 | 0 |
| 1 | report-page | ollama-cloud/gemma4:31b | index | 16 | 3/8/4/0 | 0 | 3 | 7 | 12697 / 3717 | 23s | 4/2/0 | schema 2 | 5/5 | budget | 0 | 0.11 | 0 | 934 |
| 4 | report-page | ollama-cloud/gemma4:31b | path | 16 | 3/8/4/0 | 0 | 3 | 7 | 6472 / 2734 | 15s | 6/0/0 | – | 0/5 | budget | 0 | 0.11 | 0 | 0 |
| 13 | umzug | gemini/gemini-3.5-flash-lite | index | 16 | 3/7/5/0 | 0 | 4 | 10 | 13615 / 3490 | 43s | 9/0/0 | – | 6/8 | budget | 5 | 0.04 | 0 | 1330 |
| 15 | umzug | gemini/gemini-3.5-flash-lite | path | 16 | 3/8/4/0 | 0 | 4 | 8 | 7798 / 3070 | 51s | 7/0/0 | – | 0/6 | budget | 0 | 0.06 | 0 | 0 |
| 14 | umzug | ollama-cloud/gemma4:31b | index | 16 | 3/9/3/0 | 0 | 3 | 7 | 17826 / 5338 | 29s | 2/4/0 | schema 4 | 5/5 | budget | 0 | 0.11 | 0 | 1058 |
| 16 | umzug | ollama-cloud/gemma4:31b | path | 16 | 3/8/4/0 | 0 | 3 | 7 | 6716 / 3297 | 19s | 6/0/0 | – | 0/5 | budget | 0 | 0.10 | 0 | 0 |
