# SPIKE-028 runs

From results/live. Conditions: 1 every tool native (today), 2 the tool index, 3 the index and history cards (long chats only).
Task: passed without the request and prompt-size checks (the index adds a describe_tool request by design). Done: a task whose needed tools each ran once without an error (the runner's tool_called also counts refused calls). Stopped runs (a quota or the spike's budget) are left out.

## multi-part

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 27 | 26/27 | 26/27 | 26/27 | 0 | 0 | 2.3 | 8791 | 0% | 16 | 0 | 0 | 0 | 0/16 | 0 | 0 → 0 | 0 |
| ollama-cloud/gemma4:31b | 2 | 27 | 27/27 | 27/27 | 27/27 | 0 | 0 | 2.3 | 5454 | 0% | 13 | 13 | 8 | 1 | 0/13 | 1 | 1 → 0 | 0 |
| mistral/ministral-14b-latest | 1 | 27 | 21/27 | 21/27 | 21/27 | 0 | 0 | 2.9 | 10988 | 56% | 36 | 0 | 0 | 0 | 0/36 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 27 | 24/27 | 24/27 | 24/27 | 0 | 0 | 2.6 | 6214 | 56% | 15 | 15 | 6 | 4 | 2/15 | 5 | 6 → 6 | 0 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 1: after_message 1
- mistral/ministral-14b-latest, condition 1: after_message 3, answer_contains 3
- mistral/ministral-14b-latest, condition 2: after_message 2, answer_contains 1

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | ministral-14b-latest c1 | ministral-14b-latest c2 |
|---|--:|--:|--:|--:|
| t3_a_news_add_eth | 3/3 | 3/3 | 3/3 | 3/3 |
| t3_b_price_use_eur | 3/3 | 3/3 | 3/3 | 3/3 |
| t3_d_range_7_days | 2/3 | 3/3 | 0/3 | 1/3 |
| t4_a_what_is_ma | 3/3 | 3/3 | 3/3 | 3/3 |
| t4_b_yesterday_close | 3/3 | 3/3 | 3/3 | 3/3 |
| t4_c_pipeline_schedule | 3/3 | 3/3 | 3/3 | 3/3 |
| t4_d_sma_ema_7day_high | 3/3 | 3/3 | 0/3 | 2/3 |
| t6_e_two_plus_two | 3/3 | 3/3 | 3/3 | 3/3 |
| t6_f_card_schedule | 3/3 | 3/3 | 3/3 | 3/3 |

## skill-loading

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 36 | 33/36 | 33/36 | 33/36 | 0 | 0 | 2.2 | 9249 | 0% | 21 | 0 | 0 | 0 | 0/21 | 0 | 0 → 0 | 0 |
| ollama-cloud/gemma4:31b | 2 | 36 | 18/36 | 18/36 | 18/36 | 0 | 0 | 2.9 | 7817 | 0% | 47 | 47 | 7 | 40 | 15/47 | 4 | 15 → 13 | 0 |
| mistral/ministral-14b-latest | 1 | 36 | 27/36 | 27/36 | 27/36 | 0 | 0 | 2.4 | 10607 | 54% | 24 | 0 | 0 | 0 | 0/24 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 36 | 18/36 | 18/36 | 18/36 | 0 | 0 | 3.4 | 8282 | 68% | 33 | 33 | 12 | 22 | 20/33 | 16 | 16 → 12 | 0 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 1: skill_loaded_before 3
- ollama-cloud/gemma4:31b, condition 2: skill_loaded 3, skill_loaded_before 15
- mistral/ministral-14b-latest, condition 1: skill_loaded_before 9
- mistral/ministral-14b-latest, condition 2: skill_loaded 3, skill_loaded_before 15

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | ministral-14b-latest c1 | ministral-14b-latest c2 |
|---|--:|--:|--:|--:|
| L01_news_view | 3/3 | 0/3 | 3/3 | 0/3 |
| L02_schedule | 1/3 | 0/3 | 0/3 | 0/3 |
| L03_add_column | 3/3 | 3/3 | 3/3 | 3/3 |
| L04_sql_fails | 2/3 | 0/3 | 0/3 | 0/3 |
| L05_design_table | 3/3 | 3/3 | 0/3 | 3/3 |
| L06_etf_news | 3/3 | 0/3 | 3/3 | 0/3 |
| L07_chart | 3/3 | 0/3 | 3/3 | 0/3 |
| L08_form | 3/3 | 0/3 | 3/3 | 0/3 |
| L09_moving_average | 3/3 | 3/3 | 3/3 | 3/3 |
| L10_price_yesterday | 3/3 | 3/3 | 3/3 | 3/3 |
| L11_what_tracked | 3/3 | 3/3 | 3/3 | 3/3 |
| L12_two_plus_two | 3/3 | 3/3 | 3/3 | 3/3 |

## rare-tools

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | failed → recovered | history calls |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.2 | 7600 | 0% | 15 | 0 | 0 | 0 | 3/15 | 0 | 3 → 3 | 0 |
| ollama-cloud/gemma4:31b | 2 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.8 | 6024 | 0% | 12 | 12 | 10 | 2 | 0/12 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 1 | 12 | 12/12 | 12/12 | 12/12 | 0 | 0 | 2.0 | 7826 | 39% | 12 | 0 | 0 | 0 | 0/12 | 0 | 0 → 0 | 0 |
| mistral/ministral-14b-latest | 2 | 12 | 9/12 | 9/12 | 9/12 | 0 | 0 | 3.0 | 6226 | 42% | 15 | 15 | 6 | 9 | 6/15 | 3 | 9 → 6 | 0 |

Failed checks by type:

- mistral/ministral-14b-latest, condition 2: answer_contains 3

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | ministral-14b-latest c1 | ministral-14b-latest c2 |
|---|--:|--:|--:|--:|
| R1_save_link | 3/3 | 3/3 | 3/3 | 3/3 |
| R2_connect_api | 3/3 | 3/3 | 3/3 | 3/3 |
| R3_read_feed | 3/3 | 3/3 | 3/3 | 0/3 |
| R4_bucket_put | 3/3 | 3/3 | 3/3 | 3/3 |

## long-chats

| model | cond | runs | passed | task | done | broke | stopped | requests/run | prompt tok/run | cache read | rare calls | via call_tool | describes | blind | arg errors | unknown name | failed → recovered | history calls | card had the needed turn | cards added |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| ollama-cloud/gemma4:31b | 1 | 9 | 7/9 | 7/9 | 7/9 | 0 | 0 | 4.0 | 20039 | 0% | 23 | 0 | 0 | 0 | 6/23 | 0 | 6 → 6 | 14 | – | – |
| ollama-cloud/gemma4:31b | 2 | 9 | 4/9 | 4/9 | 4/9 | 0 | 0 | 4.8 | 17477 | 0% | 24 | 24 | 3 | 21 | 8/24 | 6 | 12 → 7 | 20 | – | – |
| ollama-cloud/gemma4:31b | 3 | 9 | 7/9 | 7/9 | 7/9 | 0 | 0 | 3.9 | 12247 | 0% | 16 | 16 | 4 | 12 | 3/16 | 2 | 6 → 3 | 9 | 3/6 | 3 |
| mistral/ministral-14b-latest | 1 | 9 | 6/9 | 6/9 | 6/9 | 0 | 0 | 3.3 | 18754 | 29% | 15 | 0 | 0 | 0 | 0/15 | 3 | 3 → 0 | 6 | – | – |
| mistral/ministral-14b-latest | 2 | 9 | 3/9 | 3/9 | 3/9 | 0 | 0 | 3.3 | 10069 | 33% | 5 | 5 | 3 | 3 | 0/5 | 7 | 4 → 2 | 0 | – | – |
| mistral/ministral-14b-latest | 3 | 9 | 6/9 | 6/9 | 6/9 | 0 | 0 | 2.4 | 6273 | 68% | 6 | 6 | 3 | 3 | 0/6 | 3 | 3 → 3 | 0 | 3/6 | 3 |

Failed checks by type:

- ollama-cloud/gemma4:31b, condition 1: tool_called 4
- ollama-cloud/gemma4:31b, condition 2: tool_called 15
- ollama-cloud/gemma4:31b, condition 3: tool_called 6
- mistral/ministral-14b-latest, condition 1: tool_called 9
- mistral/ministral-14b-latest, condition 2: tool_called 15
- mistral/ministral-14b-latest, condition 3: tool_called 9

Done by scenario:

| scenario | gemma4:31b c1 | gemma4:31b c2 | gemma4:31b c3 | ministral-14b-latest c1 | ministral-14b-latest c2 | ministral-14b-latest c3 |
|---|--:|--:|--:|--:|--:|--:|
| H1_avg_chart | 1/3 | 1/3 | 2/3 | 3/3 | 0/3 | 3/3 |
| H2_drop_alert | 3/3 | 0/3 | 2/3 | 0/3 | 0/3 | 0/3 |
| H3_in_window | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |

## Tool definitions in a main chat (estimated tokens)

| set | core tools | core tokens | other tools | their full definitions (c1) | index lines + describe_tool + call_tool (c2) | of that, index lines | saved |
|---|--:|--:|--:|--:|--:|--:|--:|
| multi-part | 4 | 595 | 18 | 1686 | 601 | 430 | 64% |
| skill-loading | 5 | 816 | 15 | 1525 | 548 | 377 | 64% |
| rare-tools | 4 | 612 | 18 | 2002 | 599 | 428 | 70% |
| long-chats | 4 | 612 | 15 | 1560 | 553 | 382 | 65% |
