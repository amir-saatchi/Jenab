# SPIKE-028 runs

From results/round2-sets. Conditions: 1 every tool native (today), 2 the tool index, 3 the index and history cards, 4 today's tools and history cards (3 and 4: long chats only).
Task: passed without the request and prompt-size checks (the index adds a describe_tool request by design). Done: a task whose needed tools each ran once without an error (the runner's tool_called also counts refused calls). Stopped runs (a quota or the spike's budget) are left out.

## multi-part

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 2 | 27 | 27/27 | 27/27 | 27/27 | 0 | 0 | 2.5 | 6457 | 0% | 17 | 17 | 11 | 1 | 0/17 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 27 | 21/27 | 21/27 | 21/27 | 0 | 0 | 2.7 | 6969 | 50% | 15 | 15 | 7 | 3 | 1/15 | 5 | 5 | 0 | 6 → 6 | 0 |

Failed checks by type:

- mistral/ministral-14b-latest, condition 2: after_message 3, answer_contains 3

Done by scenario:

| scenario | gemma4:31b c2 | ministral-14b-latest c2 |
|---|--:|--:|
| t3_a_news_add_eth | 3/3 | 3/3 |
| t3_b_price_use_eur | 3/3 | 3/3 |
| t3_d_range_7_days | 3/3 | 0/3 |
| t4_a_what_is_ma | 3/3 | 3/3 |
| t4_b_yesterday_close | 3/3 | 3/3 |
| t4_c_pipeline_schedule | 3/3 | 3/3 |
| t4_d_sma_ema_7day_high | 3/3 | 0/3 |
| t6_e_two_plus_two | 3/3 | 3/3 |
| t6_f_card_schedule | 3/3 | 3/3 |

## skill-loading

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 2 | 36 | 34/36 | 34/36 | 34/36 | 0 | 0 | 3.3 | 13722 | 0% | 41 | 41 | 13 | 27 | 13/41 | 7 | 6 | 1 | 14 → 13 | 0 |
| mistral/ministral-14b-latest | 2 | 36 | 28/36 | 28/36 | 28/36 | 0 | 0 | 3.4 | 12326 | 59% | 19 | 19 | 15 | 2 | 1/19 | 15 | 15 | 0 | 16 → 16 | 0 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 2: skill_loaded_before 2
- mistral/ministral-14b-latest, condition 2: skill_loaded_before 8

Done by scenario:

| scenario | gemma4:31b c2 | ministral-14b-latest c2 |
|---|--:|--:|
| L01_news_view | 3/3 | 3/3 |
| L02_schedule | 3/3 | 0/3 |
| L03_add_column | 3/3 | 3/3 |
| L04_sql_fails | 1/3 | 0/3 |
| L05_design_table | 3/3 | 1/3 |
| L06_etf_news | 3/3 | 3/3 |
| L07_chart | 3/3 | 3/3 |
| L08_form | 3/3 | 3/3 |
| L09_moving_average | 3/3 | 3/3 |
| L10_price_yesterday | 3/3 | 3/3 |
| L11_what_tracked | 3/3 | 3/3 |
| L12_two_plus_two | 3/3 | 3/3 |

## rare-tools

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 2 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.8 | 6702 | 0% | 12 | 12 | 10 | 2 | 0/12 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 3.5 | 7893 | 55% | 15 | 15 | 9 | 6 | 3/15 | 3 | 3 | 0 | 6 → 6 | 0 |

Done by scenario:

| scenario | gemma4:31b c2 | ministral-14b-latest c2 |
|---|--:|--:|
| R1_save_link | 3/3 | 3/3 |
| R2_connect_api | 3/3 | 3/3 |
| R3_read_feed | 3/3 | 3/3 |
| R4_bucket_put | 3/3 | 3/3 |

## Tool definitions in a main chat (estimated tokens)

| set | core tools | core tokens | other tools | their full definitions (c1) | index lines + describe_tool + call_tool (c2) | of that, index lines | saved |
|---|--:|--:|--:|--:|--:|--:|--:|
| multi-part | 6 | 893 | 16 | 1388 | 550 | 379 | 60% |
| skill-loading | 7 | 1114 | 13 | 1227 | 498 | 327 | 59% |
| rare-tools | 6 | 910 | 16 | 1704 | 549 | 378 | 68% |
