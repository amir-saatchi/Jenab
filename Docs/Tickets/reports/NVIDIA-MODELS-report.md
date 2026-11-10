# NVIDIA models on the scenario suite

**Date:** 2026-11-09 and 2026-11-10 (UTC).
**Models:** `mistralai/mistral-large-4-1t-a52b` and `meta/muse-glimmer-30b`, on NVIDIA's free endpoints (`openai_compatible`, `https://integrate.api.nvidia.com/v1/`, key `NVIDIA_API_KEY`).
**Reference:** `gemini/gemini-3.5-flash-lite`, from its newest existing reports. No Gemini requests were sent.

## Summary

- **mistral-large-4 does Jenab's work better.** It passed 32 of 38 runs at `-reps 1`. muse-glimmer passed 27 of 38.
- mistral-large-4 is also faster. Its typical request takes about 9 s. muse-glimmer's takes about 18 s.
- mistral-large-4 is close to gemini-3.5-flash-lite where both have reports. It beat Gemini on `skill-loading` (12/12) and `missing-features` (6/6).
- muse-glimmer fails the memory set. It saves the preference but doesn't follow it in a new chat, or it searches old chats first.
- No 402 or 429 came back. Two requests stalled and the app retried them. Two runs broke on a slow or stuck model.
- Two more muse-glimmer runs broke because the PC went into standby. That was not the model's fault (see "Hangs and errors").

## Setup

- Runner: `jenab-scenarios run -env <main checkout>/.env -model nvidia/<id> -timeout 8m <sets>`, built once from this worktree.
- `scenarios/models.yaml` (git-ignored, not committed) holds only the NVIDIA provider. Both models are set to a 131,072-token context.
- One model ran at a time. Requests never came close to 40 a minute: each request took 2–90 s.
- Sets at `-reps 1`: `local-basic` (smoke), `skill-loading`, `data-tools`, `missing-features`, `memory`, `project-card`, `grounding-honest`.
- Sets at `-reps 3`, as well: `data-tools`, `memory`, `project-card`.
- Nothing in prompts, skills or code was changed. Failures are recorded as they came.

## Per model

"Requests" counts every provider request, including retries and the schema agent's calls. Pass shares are runs passed of runs.

### mistralai/mistral-large-4-1t-a52b

| Set | Reps | Passed | Requests | Prompt tokens / run | Cache read | Output tokens / run | Time / run |
|---|---|---|---|---|---|---|---|
| local-basic | 1 | 3/3 | 7 | 7,471 | 70 % | 178 | 16 s |
| skill-loading | 1 | 12/12 | 53 | 20,508 | 90 % | 670 | 59 s |
| data-tools | 1 | 3/4 | 10 | 7,812 | 77 % | 218 | 11 s |
| missing-features | 1 | 6/6 | 27 | 15,337 | 85 % | 1,206 | 90 s |
| memory | 1 | 2/2 | 6 | 8,484 | 46 % | 456 | 46 s |
| project-card | 1 | 1/2 | 9 | 15,477 | 38 % | 590 | 80 s |
| grounding-honest | 1 | 5/9 | 30 | 12,302 | 88 % | 996 | 91 s |
| data-tools | 3 | 10/12 | 32 | 8,542 | 90 % | 251 | 32 s |
| memory | 3 | 5/6 | 20 | 9,589 | 72 % | 559 | 66 s |
| project-card | 3 | 3/6 (1 broke) | 27 | 15,034 | 46 % | 433 | 239 s |

- **Requests:** 221 in total. **Tokens:** about 801,000 prompt and 36,700 output.
- **Latency per request:** median 8.6 s, 90th percentile 51 s. 14 requests took over 60 s. The slowest took 232 s: one long answer (1,189 tokens) that streamed slowly, without a stall.
- NVIDIA reports cached tokens for this model, and most of each prompt was read from the cache.
- **Hangs and errors:**
  - 2 streams stalled for 60 s (F3_pipeline, C2_buys_insert r3). The app retried them and the runs went on.
  - C2_buys_insert r2 broke at 8 minutes. The schema agent's request got no reply, and the run never got past `request_schema_change`.
  - No 402, 429 or 5xx came back.

### meta/muse-glimmer-30b

| Set | Reps | Passed | Requests | Prompt tokens / run | Cache read | Output tokens / run | Time / run |
|---|---|---|---|---|---|---|---|
| local-basic | 1 | 3/3 | 7 | 8,263 | 27 % | 340 | 31 s |
| skill-loading | 1 | 11/12 (1 broke) | 45 | 17,987 | 30 % | 1,047 | 111 s |
| data-tools | 1 | 4/4 | 14 | 11,639 | 28 % | 648 | 101 s |
| missing-features | 1 | 4/6 | 17 | 10,085 | 17 % | 1,006 | 91 s |
| memory | 1 | 0/2 | 10 | 17,156 | 19 % | 1,247 | 112 s |
| project-card | 1 | 1/2 | 11 | 20,996 | 24 % | 1,596 | 180 s |
| grounding-honest | 1 | 4/9 (1 broke) | 49 | 20,946 | 32 % | 1,856 | 129 s |
| data-tools | 3 | 9/12 (2 broke, standby) | 43 | 12,426 | 40 % | 684 | (standby) |
| memory | 3 | 2/6 | 41 | 26,777 | 42 % | 1,778 | 134 s |
| project-card | 3 | 3/6 | 33 | 21,303 | 10 % | 1,437 | 202 s |

- **Requests:** 270 in total. **Tokens:** about 1,050,000 prompt and 72,100 output: about twice mistral's output, for fewer passes.
- **Latency per request:** median 17.5 s, 90th percentile 54 s. 13 requests took over 60 s. Leaving out standby, the slowest took 163 s.
- **Hangs and errors:**
  - L01_news_view and R1_moving_avg_table broke at 8 minutes. Neither was a hang: the model kept calling tools with slow replies, 30–120 s each. L01 ran 9 `query` calls checking dates; R1 ran 11 requests and also asked the user a question.
  - 1 stream stalled for 2 minutes (D2_eth_spent r1). The app retried it and the run passed.
  - D3_refused_internal r1 and r2 at `-reps 3` broke because the PC went into Modern Standby (Kernel-Power 506 at 21:40 UTC, 507 at 05:27 UTC). One request "took" 18,770 s. These are not model failures. I left them out of the comparison and didn't rerun them, because the request budget was nearly used up.
  - No 402, 429 or 5xx came back.

## Comparison with gemini-3.5-flash-lite

These are the newest reports that include flash-lite. They are older than P2-09 and P2-10, which added `update_memory` and `update_session_notes`, so tools and prompts are a little longer now. The README says such runs aren't strictly comparable. flash-lite has no report for `memory`, `project-card`, `grounding-honest` or `local-basic`.

| Set | gemini-3.5-flash-lite (report date) | mistral-large-4 | muse-glimmer-30b |
|---|---|---|---|
| skill-loading | 32/36, 89 % (2026-10-13, reps 3); 11/12 (2026-10-11, reps 1) | 12/12 | 11/12 (1 broke) |
| data-tools | 12/12, 100 % (2026-10-18, reps 3) | 13/16, 81 % (r1 + r3) | 13/14, 93 % (r1 + r3; 2 standby runs left out) |
| missing-features | 16/18, 89 % (2026-10-21, reps 3) | 6/6 | 4/6 |
| memory | no report | 7/8, 88 % | 2/8, 25 % |
| project-card | no report | 4/8, 50 % (1 broke) | 4/8, 50 % |
| grounding-honest | no report | 5/9 | 4/9 (1 broke) |
| local-basic | no report | 3/3 | 3/3 |
| Time per run | 23 s / 11 s / 17 s (skill-loading / data-tools / missing-features) | 59 s / 11–32 s / 90 s | 111 s / 101 s / 91 s |

For other sets, the newest reference runs are older and on other models. `memory`: gemma4:31b 9/10 (2026-10-25), ministral-14b 5/10 (2026-10-26). `project-card`: gemma4:31b 3/3 (2026-10-21), ministral-14b 5/6 (2026-10-26). `grounding-honest`: gemma4:31b 12/27 and gpt-oss:120b 11/27 (2026-10-20). So `grounding-honest` is hard for every model tried so far.

Where flash-lite failed before: L04_sql_fails (0/3, sql-queries not loaded first), F4_view (1/3), and once each L02_schedule and F2/F3. Both NVIDIA models passed L04. mistral-large-4 also passed F4; muse-glimmer failed it.

## Failures

### mistral-large-4

- **D3_refused_internal (3 of 4 runs failed):** told to run a query on `_jenab_columns` "as it is", it doesn't run it. It predicts the refusal and goes straight to `describe_table`. The answer it gives (`trades.receipt`) is right. flash-lite also skipped the query in 2026-10-18, when that check was info only.
- **C1_keywords_query (3 of 4 runs failed):** it calls `describe_table` on `btc_news` before making the keyword table, though the project card already lists the columns.
- **C2_buys_insert r2:** broke. The schema agent's request got no reply (an NVIDIA hang).
- **R1_moving_avg_table:** wrong numbers. It computed the 7-day average only from rows inside the asked range, so the first six days are averages of fewer than 7 days.
- **R4_worked_example, R5_supply_cap, R7_saved_config:** the number check fired. It wrote worked examples and the supply cap from its own arithmetic, with no `query` behind them (11 of 16, 19 of 21, and 3 of 5 numbers had no source).
- **M2_tldr r3:** in the new chat it called `list_chats` and `search_history` before answering. It still followed the TL;DR rule.
- **L03_add_column:** passed, but took 25 requests. The fake migration doesn't change the fixture, so it checked again and again for the new column and finally asked the user.
- **L06_etf_news (info):** called `web_search` before loading `web-research`.

### muse-glimmer-30b

- **M1_persian (2 of 4 runs failed):** it saves "answer in Persian" to user memory (in Persian), then answers in English in the new chat.
- **M2_tldr (4 of 4 runs failed):** in the new chat it calls `search_history`, `list_chats` and `read_messages` before answering, up to 17 requests in one run. The answer does start with "TL;DR:".
- **C1_keywords_query (4 of 4 runs failed):** calls `describe_table` on `btc_news`, as mistral does.
- **F3_pipeline:** doesn't say pipelines aren't available yet. Instead it fetched a price page, made a `daily_prices` table, inserted a row it wasn't asked for, and said a pipeline "still needs to be set up".
- **F4_view:** asks the user for the table and column names instead of looking at the database.
- **L01_news_view, R1_moving_avg_table:** broke at 8 minutes. These were long, slow tool loops, not hangs. In R1 it also asked the user a question.
- **R3_what_if, R4_worked_example, R5_supply_cap, R7_saved_config:** the number check fired (18 of 32, 11 of 16, 6 of 7, and 3 of 6 numbers had no source).
- **D2_eth_spent r2:** the total, 6,254.40, was right. It wrote it with a narrow no-break space (U+202F) as the thousands separator, which the check's regex doesn't accept. This is a format failure, recorded as it is.
- **L06_etf_news (info):** called `web_search` before loading `web-research`.

## Recommendation

**Keep `mistralai/mistral-large-4-1t-a52b` as the second test model.** Reasons:

- It passed more runs (32/38 against 27/38 at reps 1) and matched or beat flash-lite on the sets flash-lite has reports for.
- It follows user memory across chats (7/8). muse-glimmer doesn't (2/8).
- It's about twice as fast per request, writes about half the output tokens, and NVIDIA serves most of its prompt from the cache.
- Its failures are small and steady (an unneeded `describe_table`, skipping a query it expects to be refused). They show up again across reps, which makes them useful to track.

Drawbacks: it can use many requests in a loop (25 in L03), and NVIDIA sometimes hangs on it, so keep a timeout.

muse-glimmer is not worth keeping as a test model. It's slow, it often turns a simple turn into a long tool loop, and it breaks the memory feature.

## Totals

| Model | Requests sent |
|---|---|
| mistralai/mistral-large-4-1t-a52b | 221 |
| meta/muse-glimmer-30b | 270 |
| **Total** | **491** |

Both models stayed under 300 requests each, and the total under 600. NVIDIA_API_KEY was the only key used.

## Files

- Reports (`.md` and `.json`) and transcripts are in each set's `scenarios/<set>/results/` in this worktree, under the times 20261109-185924 to 20261110-054329.
- `scenarios/models.yaml` is git-ignored and was not committed.
