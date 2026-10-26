# SPIKE-037 runs

## Part B: earlier turns

Prompt tokens as the provider reports them (input + cache read + cache write). Trace tokens are estimated (4 bytes a token) in the last request of the run.

| chat | cond | model | runs | err | requests | prompt / request | prompt / run | largest | cache read | cuts / run | follow-ups right | reads / run | repeated / run | wrong names / run | calls / later turn | fake calls | wrong claims | trace tokens |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| b_jobs | trace | ministral-14b-latest | 1 | 0 | 30.0 | 2481 | 74423 | 3906 | 27% | 0.0 | 44% | 2.0 | 0.0 | 0.0 | 0.9 | 7 | 0 | 0 |
| b_jobs | trace | gemma4:31b | 1 | 0 | 46.0 | 3305 | 152016 | 10070 | 0% | 0.0 | 67% | 19.0 | 0.0 | 0.0 | 2.8 | 4 | 0 | 0 |
| b_jobs | trace | gpt-oss:120b | 1 | 0 | 48.0 | 2106 | 101107 | 3430 | 0% | 0.0 | 67% | 5.0 | 1.0 | 0.0 | 1.8 | 0 | 0 | 0 |

### Follow-ups right, per kind

Each cell is right/asked, and in brackets how many of those turns read the history, a ref, a table or a page.

| chat | cond | model | table | detail | source | task | ref | more | fact | claims |
|---|---|---|---|---|---|---|---|---|---|---|
| b_jobs | trace | ministral-14b-latest | 2/2 (1) | 1/1 (1) | 0/1 (0) | 1/1 (0) | 0/1 (0) | 0/1 (0) | 0/1 (1) | 0/1 (0) |
| b_jobs | trace | gemma4:31b | 2/2 (1) | 0/1 (0) | 1/1 (0) | 1/1 (0) | 0/1 (0) | 0/1 (0) | 1/1 (1) | 1/1 (1) |
| b_jobs | trace | gpt-oss:120b | 2/2 (1) | 1/1 (1) | 1/1 (1) | 1/1 (0) | 0/1 (1) | 0/1 (0) | 1/1 (1) | 0/1 (1) |

### Repeated work, wrong names, fake calls and wrong claims

- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t5 [insert_rows`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t5 [full steps`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t7 [insert_rows`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t7 [full steps`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t13 [search_ref`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t13 [search_ref`
- b_jobs_trace_mistral-ministral-14b-latest_r1 fake call: `t13 [read_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t8 [search_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t8 [read_ref`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t13 [search_history`
- b_jobs_trace_ollama-cloud-gemma4-31b_r1 fake call: `t13 [read_ref`
- b_jobs_trace_ollama-cloud-gpt-oss-120b_r1 repeated: t13 fetch_page jobs.example.com/zalando-jda again

