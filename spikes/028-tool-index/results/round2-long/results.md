# SPIKE-028 runs

From results/live, results/round2-sets, results/round2-long. Conditions: 1 every tool native (today), 2 the tool index, 3 the index and history cards, 4 today's tools and history cards (3 and 4: long chats only).
Task: passed without the request and prompt-size checks (the index adds a describe_tool request by design). Done: a task whose needed tools each ran once without an error (the runner's tool_called also counts refused calls). Stopped runs (a quota or the spike's budget) are left out.

## multi-part

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 27 | 26/27 | 26/27 | 26/27 | 0 | 0 | 2.3 | 8791 | 0% | 16 | 0 | 0 | 0 | 0/16 | 0 | 0 | 0 | 0 → 0 | 0 |
| ollama-cloud/gemma4:31b | 2 | 27 | 27/27 | 27/27 | 27/27 | 0 | 0 | 2.5 | 6457 | 0% | 17 | 17 | 11 | 1 | 0/17 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 1 | 27 | 21/27 | 21/27 | 21/27 | 0 | 0 | 2.9 | 10988 | 56% | 36 | 0 | 0 | 0 | 0/36 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 27 | 21/27 | 21/27 | 21/27 | 0 | 0 | 2.7 | 6969 | 50% | 15 | 15 | 7 | 3 | 1/15 | 5 | 5 | 0 | 6 → 6 | 0 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 1: after_message 1
- mistral/ministral-14b-latest, condition 1: after_message 3, answer_contains 3
- mistral/ministral-14b-latest, condition 2: after_message 3, answer_contains 3

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | ministral-14b-latest c1 | ministral-14b-latest c2 |
|---|--:|--:|--:|--:|
| t3_a_news_add_eth | 3/3 | 3/3 | 3/3 | 3/3 |
| t3_b_price_use_eur | 3/3 | 3/3 | 3/3 | 3/3 |
| t3_d_range_7_days | 2/3 | 3/3 | 0/3 | 0/3 |
| t4_a_what_is_ma | 3/3 | 3/3 | 3/3 | 3/3 |
| t4_b_yesterday_close | 3/3 | 3/3 | 3/3 | 3/3 |
| t4_c_pipeline_schedule | 3/3 | 3/3 | 3/3 | 3/3 |
| t4_d_sma_ema_7day_high | 3/3 | 3/3 | 0/3 | 0/3 |
| t6_e_two_plus_two | 3/3 | 3/3 | 3/3 | 3/3 |
| t6_f_card_schedule | 3/3 | 3/3 | 3/3 | 3/3 |

## skill-loading

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 36 | 33/36 | 33/36 | 33/36 | 0 | 0 | 2.2 | 9249 | 0% | 21 | 0 | 0 | 0 | 0/21 | 0 | 0 | 0 | 0 → 0 | 0 |
| ollama-cloud/gemma4:31b | 2 | 36 | 34/36 | 34/36 | 34/36 | 0 | 0 | 3.3 | 13722 | 0% | 41 | 41 | 13 | 27 | 13/41 | 7 | 6 | 1 | 14 → 13 | 0 |
| mistral/ministral-14b-latest | 1 | 36 | 27/36 | 27/36 | 27/36 | 0 | 0 | 2.4 | 10607 | 54% | 24 | 0 | 0 | 0 | 0/24 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 36 | 28/36 | 28/36 | 28/36 | 0 | 0 | 3.4 | 12326 | 59% | 19 | 19 | 15 | 2 | 1/19 | 15 | 15 | 0 | 16 → 16 | 0 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 1: skill_loaded_before 3
- ollama-cloud/gemma4:31b, condition 2: skill_loaded_before 2
- mistral/ministral-14b-latest, condition 1: skill_loaded_before 9
- mistral/ministral-14b-latest, condition 2: skill_loaded_before 8

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | ministral-14b-latest c1 | ministral-14b-latest c2 |
|---|--:|--:|--:|--:|
| L01_news_view | 3/3 | 3/3 | 3/3 | 3/3 |
| L02_schedule | 1/3 | 3/3 | 0/3 | 0/3 |
| L03_add_column | 3/3 | 3/3 | 3/3 | 3/3 |
| L04_sql_fails | 2/3 | 1/3 | 0/3 | 0/3 |
| L05_design_table | 3/3 | 3/3 | 0/3 | 1/3 |
| L06_etf_news | 3/3 | 3/3 | 3/3 | 3/3 |
| L07_chart | 3/3 | 3/3 | 3/3 | 3/3 |
| L08_form | 3/3 | 3/3 | 3/3 | 3/3 |
| L09_moving_average | 3/3 | 3/3 | 3/3 | 3/3 |
| L10_price_yesterday | 3/3 | 3/3 | 3/3 | 3/3 |
| L11_what_tracked | 3/3 | 3/3 | 3/3 | 3/3 |
| L12_two_plus_two | 3/3 | 3/3 | 3/3 | 3/3 |

## rare-tools

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.2 | 7600 | 0% | 15 | 0 | 0 | 0 | 3/15 | 0 | 0 | 0 | 3 → 3 | 0 |
| ollama-cloud/gemma4:31b | 2 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.8 | 6702 | 0% | 12 | 12 | 10 | 2 | 0/12 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 1 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.0 | 7826 | 39% | 12 | 0 | 0 | 0 | 0/12 | 0 | 0 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 3.5 | 7893 | 55% | 15 | 15 | 9 | 6 | 3/15 | 3 | 3 | 0 | 6 → 6 | 0 |

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | ministral-14b-latest c1 | ministral-14b-latest c2 |
|---|--:|--:|--:|--:|
| R1_save_link | 3/3 | 3/3 | 3/3 | 3/3 |
| R2_connect_api | 3/3 | 3/3 | 3/3 | 3/3 |
| R3_read_feed | 3/3 | 3/3 | 3/3 | 3/3 |
| R4_bucket_put | 3/3 | 3/3 | 3/3 | 3/3 |

## long-chats

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | index tool called natively | nested call_tool | failed → recovered | history calls | card had the needed turn | cards added |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 25 | 19/25 | 19/25 | 19/25 | 0 | 0 | 4.8 | 25460 | 0% | 25 | 0 | 0 | 0 | 0/25 | 0 | 0 | 0 | 19 → 19 | 52 | – | – |
| ollama-cloud/gemma4:31b | 2 | 25 | 18/25 | 18/25 | 18/25 | 0 | 0 | 5.2 | 24706 | 0% | 22 | 22 | 8 | 18 | 3/22 | 8 | 0 | 7 | 30 → 20 | 52 | – | – |
| ollama-cloud/gemma4:31b | 3 | 25 | 22/25 | 22/25 | 22/25 | 0 | 0 | 3.3 | 14170 | 0% | 22 | 22 | 7 | 15 | 0/22 | 3 | 0 | 3 | 14 → 6 | 16 | 15/20 | 20 |
| ollama-cloud/gemma4:31b | 4 | 25 | 25/25 | 25/25 | 25/25 | 0 | 0 | 3.1 | 14801 | 0% | 25 | 0 | 0 | 0 | 0/25 | 0 | 0 | 0 | 8 → 8 | 17 | 15/20 | 20 |
| mistral/ministral-14b-latest | 1 | 25 | 10/25 | 10/25 | 10/25 | 0 | 0 | 5.4 | 31152 | 66% | 27 | 0 | 0 | 0 | 0/27 | 13 | 13 | 0 | 10 → 0 | 39 | – | – |
| mistral/ministral-14b-latest | 2 | 25 | 10/25 | 10/25 | 10/25 | 0 | 0 | 4.7 | 22792 | 59% | 29 | 29 | 13 | 17 | 3/29 | 16 | 16 | 0 | 22 → 13 | 13 | – | – |
| mistral/ministral-14b-latest | 3 | 25 | 15/25 | 15/25 | 15/25 | 0 | 0 | 3.8 | 18185 | 67% | 25 | 25 | 15 | 10 | 0/25 | 15 | 15 | 0 | 15 → 15 | 0 | 15/20 | 20 |
| mistral/ministral-14b-latest | 4 | 25 | 17/25 | 17/25 | 17/25 | 0 | 0 | 2.9 | 15950 | 48% | 29 | 0 | 0 | 0 | 0/29 | 1 | 1 | 0 | 1 → 0 | 5 | 15/20 | 20 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 1: tool_called 11
- ollama-cloud/gemma4:31b, condition 2: tool_called 21
- ollama-cloud/gemma4:31b, condition 3: tool_called 9
- mistral/ministral-14b-latest, condition 1: tool_called 40
- mistral/ministral-14b-latest, condition 2: tool_called 40
- mistral/ministral-14b-latest, condition 3: tool_called 30
- mistral/ministral-14b-latest, condition 4: tool_called 24

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | gemma4:31b c3 | gemma4:31b c4 | ministral-14b-latest c1 | ministral-14b-latest c2 | ministral-14b-latest c3 | ministral-14b-latest c4 |
|---|--:|--:|--:|--:|--:|--:|--:|--:|
| H1_avg_chart | 4/5 | 3/5 | 5/5 | 5/5 | 5/5 | 5/5 | 5/5 | 5/5 |
| H2_drop_alert | 5/5 | 5/5 | 5/5 | 5/5 | 0/5 | 0/5 | 0/5 | 0/5 |
| H3_in_window | 5/5 | 5/5 | 5/5 | 5/5 | 5/5 | 5/5 | 5/5 | 5/5 |
| H4_week_range | 1/5 | 2/5 | 4/5 | 5/5 | 0/5 | 0/5 | 5/5 | 5/5 |
| H5_etf_digest | 4/5 | 3/5 | 3/5 | 5/5 | 0/5 | 0/5 | 0/5 | 2/5 |

## Tool definitions in a main chat (estimated tokens)

| set | core tools | core tokens | other tools | their full definitions (c1) | index lines + describe_tool + call_tool (c2) | of that, index lines | saved |
|---|--:|--:|--:|--:|--:|--:|--:|
| multi-part | 6 | 893 | 16 | 1388 | 550 | 379 | 60% |
| skill-loading | 7 | 1114 | 13 | 1227 | 498 | 327 | 59% |
| rare-tools | 6 | 910 | 16 | 1704 | 549 | 378 | 68% |
| long-chats | 6 | 910 | 13 | 1262 | 503 | 332 | 60% |
