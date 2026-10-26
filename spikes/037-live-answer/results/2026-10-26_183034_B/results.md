# SPIKE-037 runs

## Part B: earlier turns

Prompt tokens as the provider reports them (input + cache read + cache write). Trace tokens are estimated (4 bytes a token) in the last request of the run.

| chat | cond | model | runs | err | requests | prompt / request | prompt / run | largest | cache read | cuts / run | follow-ups right | reads / run | repeated / run | wrong names / run | calls / later turn | fake calls | wrong claims | trace tokens |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| b_jobs | window | ministral-14b-latest | 3 | 0 | 36.0 | 2943 | 105964 | 4806 | 30% | 2.0 | 59% | 6.7 | 0.3 | 3.0 | 1.4 | 0 | 0 | 0 |
| b_jobs | window | gemma4:31b | 3 | 0 | 34.3 | 3281 | 112656 | 6781 | 0% | 2.0 | 78% | 3.3 | 1.3 | 0.0 | 1.2 | 0 | 0 | 0 |
| b_jobs | window | gpt-oss:120b | 3 | 0 | 39.7 | 2741 | 108740 | 6772 | 0% | 2.0 | 81% | 4.0 | 1.3 | 0.0 | 1.3 | 0 | 0 | 0 |
| b_jobs | answers | ministral-14b-latest | 3 | 0 | 37.0 | 2233 | 82625 | 3328 | 36% | 2.0 | 56% | 4.7 | 0.7 | 1.7 | 1.2 | 0 | 0 | 0 |
| b_jobs | answers | gemma4:31b | 3 | 0 | 54.3 | 3713 | 201724 | 18886 | 0% | 2.0 | 89% | 17.0 | 4.3 | 0.0 | 3.1 | 0 | 0 | 0 |
| b_jobs | answers | gpt-oss:120b | 3 | 0 | 60.7 | 2421 | 146903 | 6280 | 0% | 2.0 | 78% | 11.0 | 7.3 | 0.0 | 2.8 | 0 | 0 | 0 |
| b_jobs | trace | ministral-14b-latest | 3 | 0 | 34.0 | 2369 | 80550 | 3684 | 36% | 1.7 | 48% | 5.3 | 0.3 | 1.7 | 1.5 | 10 | 0 | 181 |
| b_jobs | trace | gemma4:31b | 3 | 0 | 36.0 | 2316 | 83384 | 5031 | 0% | 2.0 | 70% | 6.0 | 1.3 | 0.0 | 1.4 | 8 | 0 | 200 |
| b_jobs | trace | gpt-oss:120b | 3 | 0 | 47.0 | 2168 | 101904 | 6061 | 0% | 2.0 | 85% | 10.0 | 1.3 | 0.0 | 1.8 | 0 | 0 | 217 |
| b_trip | window | ministral-14b-latest | 3 | 0 | 34.7 | 3009 | 104319 | 5223 | 29% | 4.0 | 67% | 5.0 | 0.7 | 0.0 | 1.1 | 0 | 0 | 0 |
| b_trip | window | gemma4:31b | 3 | 0 | 38.7 | 3079 | 119048 | 5364 | 0% | 4.0 | 85% | 4.0 | 3.0 | 0.0 | 1.1 | 0 | 0 | 0 |
| b_trip | window | gpt-oss:120b | 3 | 0 | 35.0 | 2495 | 87337 | 5977 | 0% | 4.0 | 78% | 3.0 | 1.7 | 0.0 | 1.2 | 0 | 0 | 0 |
| b_trip | answers | ministral-14b-latest | 3 | 0 | 42.0 | 2061 | 86571 | 4721 | 34% | 4.0 | 33% | 9.3 | 0.0 | 2.3 | 2.2 | 0 | 2 | 0 |
| b_trip | answers | gemma4:31b | 3 | 0 | 63.7 | 3579 | 227837 | 16947 | 0% | 4.0 | 93% | 24.3 | 4.0 | 0.0 | 3.2 | 0 | 0 | 0 |
| b_trip | answers | gpt-oss:120b | 3 | 0 | 56.7 | 3392 | 192213 | 16390 | 0% | 4.7 | 78% | 18.0 | 3.7 | 0.3 | 3.4 | 0 | 0 | 0 |
| b_trip | trace | ministral-14b-latest | 3 | 0 | 35.3 | 2120 | 74892 | 4478 | 33% | 4.0 | 48% | 6.3 | 0.0 | 0.0 | 1.2 | 0 | 0 | 127 |
| b_trip | trace | gemma4:31b | 3 | 0 | 55.3 | 2830 | 156578 | 15398 | 0% | 4.0 | 81% | 13.7 | 3.3 | 0.0 | 1.9 | 2 | 0 | 216 |
| b_trip | trace | gpt-oss:120b | 3 | 0 | 46.7 | 2104 | 98191 | 5117 | 0% | 4.3 | 78% | 8.7 | 1.7 | 0.0 | 2.0 | 0 | 1 | 177 |

### Follow-ups right, per kind

Each cell is right/asked, and in brackets how many of those turns read the history, a ref, a table or a page.

| chat | cond | model | table | detail | source | task | ref | more | fact | claims |
|---|---|---|---|---|---|---|---|---|---|---|
| b_jobs | window | ministral-14b-latest | 6/6 (4) | 3/3 (0) | 2/3 (3) | 3/3 (0) | 0/3 (3) | 0/3 (0) | 1/3 (3) | 1/3 (0) |
| b_jobs | window | gemma4:31b | 6/6 (3) | 2/3 (0) | 2/3 (0) | 3/3 (0) | 3/3 (3) | 3/3 (1) | 2/3 (2) | 0/3 (1) |
| b_jobs | window | gpt-oss:120b | 6/6 (5) | 3/3 (0) | 3/3 (1) | 3/3 (0) | 3/3 (3) | 1/3 (0) | 3/3 (3) | 0/3 (0) |
| b_jobs | answers | ministral-14b-latest | 5/6 (3) | 0/3 (1) | 2/3 (1) | 3/3 (0) | 1/3 (3) | 0/3 (0) | 3/3 (3) | 1/3 (0) |
| b_jobs | answers | gemma4:31b | 6/6 (3) | 3/3 (3) | 3/3 (1) | 3/3 (3) | 3/3 (3) | 3/3 (0) | 3/3 (3) | 0/3 (3) |
| b_jobs | answers | gpt-oss:120b | 6/6 (4) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 0/3 (0) | 3/3 (3) | 0/3 (1) |
| b_jobs | trace | ministral-14b-latest | 5/6 (3) | 1/3 (2) | 2/3 (1) | 3/3 (0) | 0/3 (2) | 1/3 (2) | 0/3 (3) | 1/3 (2) |
| b_jobs | trace | gemma4:31b | 6/6 (3) | 0/3 (0) | 3/3 (0) | 3/3 (0) | 3/3 (3) | 1/3 (0) | 3/3 (3) | 0/3 (1) |
| b_jobs | trace | gpt-oss:120b | 6/6 (4) | 3/3 (3) | 3/3 (2) | 3/3 (0) | 3/3 (3) | 2/3 (0) | 3/3 (3) | 0/3 (1) |
| b_trip | window | ministral-14b-latest | 4/6 (3) | 2/3 (3) | 3/3 (2) | 3/3 (0) | 2/3 (3) | 3/3 (3) | 1/3 (1) | 0/3 (0) |
| b_trip | window | gemma4:31b | 5/6 (6) | 3/3 (3) | 3/3 (1) | 3/3 (0) | 3/3 (3) | 3/3 (3) | 3/3 (1) | 0/3 (0) |
| b_trip | window | gpt-oss:120b | 3/6 (3) | 3/3 (3) | 3/3 (3) | 3/3 (0) | 3/3 (3) | 3/3 (3) | 3/3 (1) | 0/3 (0) |
| b_trip | answers | ministral-14b-latest | 0/6 (3) | 0/3 (3) | 0/3 (3) | 3/3 (0) | 0/3 (3) | 3/3 (3) | 3/3 (3) | 0/3 (3) |
| b_trip | answers | gemma4:31b | 6/6 (6) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 1/3 (3) |
| b_trip | answers | gpt-oss:120b | 5/6 (3) | 2/3 (2) | 2/3 (3) | 3/3 (2) | 3/3 (3) | 3/3 (3) | 2/3 (2) | 1/3 (2) |
| b_trip | trace | ministral-14b-latest | 4/6 (4) | 0/3 (3) | 2/3 (3) | 3/3 (0) | 0/3 (3) | 3/3 (3) | 1/3 (3) | 0/3 (2) |
| b_trip | trace | gemma4:31b | 6/6 (6) | 3/3 (3) | 1/3 (1) | 3/3 (0) | 3/3 (3) | 3/3 (3) | 3/3 (3) | 0/3 (3) |
| b_trip | trace | gpt-oss:120b | 5/6 (5) | 3/3 (3) | 3/3 (3) | 3/3 (0) | 2/3 (3) | 3/3 (2) | 2/3 (1) | 0/3 (3) |

### Repeated work, wrong names, fake calls and wrong claims

- b_jobs_window_mistral-ministral-14b-latest_r1 repeated: t15 fetch_page jobs.example.com/zalando-jda again
- b_jobs_window_mistral-ministral-14b-latest_r1 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"remote"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r1 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"work from home"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r1 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"hybrid"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r1 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"office"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r2 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"remote"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r2 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"work"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r3 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"remote"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r3 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"work from home"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_mistral-ministral-14b-latest_r3 wrong name: t13 search_ref {"ref":"cache/pages/jobs.example.com/zalando-jda.txt","query":"hybrid"}: no object at cache/pages/jobs.example.com/zalando-jda.txt
- b_jobs_window_ollama-cloud-gemma4-31b_r1 repeated: t4 subagent linkedin again
- b_jobs_window_ollama-cloud-gemma4-31b_r1 repeated: t8 subagent web again
- b_jobs_window_ollama-cloud-gemma4-31b_r1 repeated: t14 subagent web again
- b_jobs_window_ollama-cloud-gemma4-31b_r1 repeated: t14 subagent web again
- b_jobs_window_ollama-cloud-gpt-oss-120b_r1 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_window_ollama-cloud-gpt-oss-120b_r2 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_window_ollama-cloud-gpt-oss-120b_r2 repeated: t15 fetch_page jobs.example.com/n26-risk again
- b_jobs_window_ollama-cloud-gpt-oss-120b_r3 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_mistral-ministral-14b-latest_r1 repeated: t15 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_mistral-ministral-14b-latest_r1 wrong name: t11 task_status {"id":"task-7XyXJQJQJQ"}: no task "task-7XyXJQJQJQ" (tasks: t_1, t_2)
- b_jobs_answers_mistral-ministral-14b-latest_r2 wrong name: t8 search_ref {"ref":"ref_12345","query":"team size"}: no object at ref_12345
- b_jobs_answers_mistral-ministral-14b-latest_r3 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_mistral-ministral-14b-latest_r3 wrong name: t11 task_status {"id":"stepstone-search-2026-10-26"}: no task "stepstone-search-2026-10-26" (tasks: t_1, t_2)
- b_jobs_answers_mistral-ministral-14b-latest_r3 wrong name: t13 search_ref {"ref":"ref_5h3m9xn7p","query":"remote"}: no object at ref_5h3m9xn7p
- b_jobs_answers_mistral-ministral-14b-latest_r3 wrong name: t13 read_ref {"ref":"ref_5h3m9xn7p","offset":0}: no object at ref_5h3m9xn7p
- b_jobs_answers_ollama-cloud-gemma4-31b_r1 repeated: t2 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gemma4-31b_r1 repeated: t2 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gemma4-31b_r1 repeated: t8 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gemma4-31b_r1 repeated: t9 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gemma4-31b_r1 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gemma4-31b_r2 repeated: t2 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gemma4-31b_r2 repeated: t2 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gemma4-31b_r2 repeated: t8 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gemma4-31b_r2 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gemma4-31b_r3 repeated: t14 subagent web again
- b_jobs_answers_ollama-cloud-gemma4-31b_r3 repeated: t14 subagent web again
- b_jobs_answers_ollama-cloud-gemma4-31b_r3 repeated: t8 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gemma4-31b_r3 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t2 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t2 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t9 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t10 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t14 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t11 subagent stepstone again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t2 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t2 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t8 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t9 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t15 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t15 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t15 subagent web again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t15 subagent web again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t2 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t2 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t3 web_search "zalando junior data analyst berlin" again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t8 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t9 fetch_page jobs.example.com/n26-risk again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t16 fetch_page jobs.example.com/n26-risk again
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t7 [insert_rows`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t7 [full steps`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t13 [search_ref`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t13 [read_ref`
- b_jobs_trace_mistral-ministral-14b-latest_r2 repeated: t14 fetch_page jobs.example.com/zalando-jda again
- b_jobs_trace_mistral-ministral-14b-latest_r2 wrong name: t10 task_status {"id":"req_67k2j2k2j2k2"}: no task "req_67k2j2k2j2k2" (tasks: none)
- b_jobs_trace_mistral-ministral-14b-latest_r2 fake call: `t4 [subagent`
- b_jobs_trace_mistral-ministral-14b-latest_r2 fake call: `t6 [task_status`
- b_jobs_trace_mistral-ministral-14b-latest_r2 fake call: `t6 [full steps`
- b_jobs_trace_mistral-ministral-14b-latest_r2 fake call: `t9 [subagent`
- b_jobs_trace_mistral-ministral-14b-latest_r3 wrong name: t6 task_status {"id":"task_5f8d3e9a6"}: no task "task_5f8d3e9a6" (tasks: none)
- b_jobs_trace_mistral-ministral-14b-latest_r3 wrong name: t10 task_status {"id":"subagent_background"}: no task "subagent_background" (tasks: t_1)
- b_jobs_trace_mistral-ministral-14b-latest_r3 wrong name: t11 search_ref {"ref":"ref_3Z8XJQZ9","query":"remote"}: no object at ref_3Z8XJQZ9
- b_jobs_trace_mistral-ministral-14b-latest_r3 wrong name: t11 read_ref {"ref":"ref_3Z8XJQZ9","offset":0}: no object at ref_3Z8XJQZ9
- b_jobs_trace_mistral-ministral-14b-latest_r3 fake call: `t4 [subagent`
- b_jobs_trace_mistral-ministral-14b-latest_r3 fake call: `t9 [subagent`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t8 [search_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t8 [read_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t17 [read_messages`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t17 [full steps`
- b_jobs_trace_ollama-cloud-gemma4-31b_r2 repeated: t2 subagent web again
- b_jobs_trace_ollama-cloud-gemma4-31b_r2 repeated: t14 subagent web again
- b_jobs_trace_ollama-cloud-gemma4-31b_r2 repeated: t14 subagent web again
- b_jobs_trace_ollama-cloud-gemma4-31b_r2 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_trace_ollama-cloud-gemma4-31b_r2 fake call: `t8 [search_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r2 fake call: `t8 [read_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r3 fake call: `t8 [search_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r3 fake call: `t8 [read_ref`
- b_jobs_trace_ollama-cloud-gpt-oss-120b_r1 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_trace_ollama-cloud-gpt-oss-120b_r2 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_trace_ollama-cloud-gpt-oss-120b_r3 repeated: t13 fetch_page jobs.example.com/zalando-jda again
- b_jobs_trace_ollama-cloud-gpt-oss-120b_r3 repeated: t15 fetch_page jobs.example.com/n26-risk again
- b_trip_window_mistral-ministral-14b-latest_r1 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_window_mistral-ministral-14b-latest_r2 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gemma4-31b_r1 repeated: t4 subagent web again
- b_trip_window_ollama-cloud-gemma4-31b_r1 repeated: t6 subagent web again
- b_trip_window_ollama-cloud-gemma4-31b_r1 repeated: t14 subagent web again
- b_trip_window_ollama-cloud-gemma4-31b_r1 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gemma4-31b_r2 repeated: t6 subagent web again
- b_trip_window_ollama-cloud-gemma4-31b_r2 repeated: t8 subagent web again
- b_trip_window_ollama-cloud-gemma4-31b_r2 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gemma4-31b_r3 repeated: t6 subagent web again
- b_trip_window_ollama-cloud-gemma4-31b_r3 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gpt-oss-120b_r1 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gpt-oss-120b_r1 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_window_ollama-cloud-gpt-oss-120b_r2 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gpt-oss-120b_r2 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_window_ollama-cloud-gpt-oss-120b_r3 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_answers_mistral-ministral-14b-latest_r1 wrong name: t11 task_status {"id":"task_5XZ8LmQ7p"}: no task "task_5XZ8LmQ7p" (tasks: t_1, t_2)
- b_trip_answers_mistral-ministral-14b-latest_r1 wrong claim: booking\.com
- b_trip_answers_mistral-ministral-14b-latest_r1 wrong claim: tripadvisor
- b_trip_answers_mistral-ministral-14b-latest_r2 wrong name: t11 task_status {"id":"task_5hZ8XJQ7XJ"}: no task "task_5hZ8XJQ7XJ" (tasks: t_1, t_2)
- b_trip_answers_mistral-ministral-14b-latest_r2 wrong name: t13 search_ref {"ref":"ref_20261026_12345","query":"elevator"}: no object at ref_20261026_12345
- b_trip_answers_mistral-ministral-14b-latest_r2 wrong name: t13 read_ref {"ref":"ref_20261026_12345","offset":0}: no object at ref_20261026_12345
- b_trip_answers_mistral-ministral-14b-latest_r3 wrong name: t11 task_status {"id":"fado_search_2027"}: no task "fado_search_2027" (tasks: t_1, t_2)
- b_trip_answers_mistral-ministral-14b-latest_r3 wrong name: t13 search_ref {"ref":"ref_56789","query":"elevator"}: no object at ref_56789
- b_trip_answers_mistral-ministral-14b-latest_r3 wrong name: t13 read_ref {"ref":"ref_56789","offset":0}: no object at ref_56789
- b_trip_answers_ollama-cloud-gemma4-31b_r1 repeated: t4 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r1 repeated: t5 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r1 repeated: t6 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r1 repeated: t5 web_search "must-see sights in belém lisbon" again
- b_trip_answers_ollama-cloud-gemma4-31b_r2 repeated: t5 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r2 repeated: t6 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r2 repeated: t5 web_search "must-see sights in belém lisbon" again
- b_trip_answers_ollama-cloud-gemma4-31b_r3 repeated: t4 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r3 repeated: t5 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r3 repeated: t6 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r3 repeated: t6 subagent web again
- b_trip_answers_ollama-cloud-gemma4-31b_r3 repeated: t5 web_search "must-see sights in belém lisbon" again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t3 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t3 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t9 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t10 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r1 repeated: t14 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r1 wrong name: t12 task_status {"id":"t_5"}: no task "t_5" (tasks: t_1, t_2, t_3, t_4)
- b_trip_answers_ollama-cloud-gpt-oss-120b_r2 repeated: t14 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t2 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t2 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t9 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_answers_ollama-cloud-gpt-oss-120b_r3 repeated: t16 fetch_page travel.example.com/hotels/cais-sodre-loft again
- b_trip_trace_ollama-cloud-gemma4-31b_r1 repeated: t4 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r1 repeated: t4 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r1 repeated: t5 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r1 repeated: t6 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r2 repeated: t4 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r2 repeated: t6 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r2 repeated: t6 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r2 repeated: t7 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r2 fake call: `t5 [subagent`
- b_trip_trace_ollama-cloud-gemma4-31b_r3 repeated: t6 subagent web again
- b_trip_trace_ollama-cloud-gemma4-31b_r3 repeated: t6 web_search "must-see sights in belém lisbon" again
- b_trip_trace_ollama-cloud-gemma4-31b_r3 fake call: `t5 [web_search`
- b_trip_trace_ollama-cloud-gpt-oss-120b_r1 repeated: t9 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_trace_ollama-cloud-gpt-oss-120b_r1 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_trace_ollama-cloud-gpt-oss-120b_r2 wrong claim: booking\.com
- b_trip_trace_ollama-cloud-gpt-oss-120b_r3 repeated: t8 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_trace_ollama-cloud-gpt-oss-120b_r3 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again
- b_trip_trace_ollama-cloud-gpt-oss-120b_r3 repeated: t14 fetch_page travel.example.com/hotels/baixa-house again

