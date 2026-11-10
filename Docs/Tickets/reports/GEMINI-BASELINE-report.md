# gemini-3.5-flash-lite baseline on the NVIDIA scenario sets

**Date:** 2026-11-10, 06:14 to 06:35 UTC.
**Model:** `gemini/gemini-3.5-flash-lite` (kind `gemini`, key `GEMINI_API_KEY`, the first key only).
**Compared with:** `mistralai/mistral-large-4-1t-a52b` and `meta/muse-glimmer-30b`, from the NVIDIA session's report (2026-11-09 and 11-10).

## Summary

- flash-lite passed 31 of 38 runs at `-reps 1`. mistral-large-4 passed 32, muse-glimmer 27.
- Over all runs (reps 1 and reps 3), flash-lite and mistral-large-4 tie at 50 of 62 (81 %). muse-glimmer passed 41 of 60 (68 %).
- flash-lite is strongest on `data-tools` (16/16) and `project-card` (6/8). It is weak on `memory` (4/8): in a new chat it looks through old chats before it answers.
- flash-lite is fast per request: median 2.0 s, against 8.6 s for mistral and 17.5 s for muse.
- No run broke. No 402 came back, and no 429 said the daily quota was used up.
- The free tier's per-minute limit (15 requests a minute) sent back 14 short 429s. The app waited and retried each one, and the runs went on.
- 264 Gemini requests in total, under the 300 cap.

## Setup

- Runner: `jenab-scenarios run -env <main checkout>/.env -model gemini/gemini-3.5-flash-lite -timeout 8m <sets>`, built once from this worktree at commit 38dd3bb.
- `scenarios/models.yaml` (git-ignored, not committed) holds only the Gemini provider, with flash-lite at a 1,048,576-token context, as in `models.example.yaml`. The main checkout's models.yaml has no flash-lite entry, so it wasn't copied.
- Sets at `-reps 1`: `local-basic`, `skill-loading`, `data-tools`, `missing-features`, `memory`, `project-card`, `grounding-honest`.
- Sets at `-reps 3`, as well: `data-tools`, `memory`, `project-card`.
- Nothing in prompts, skills or code was changed. Failures are recorded as they came.
- The PC was kept awake for the whole run. No request took longer than 37 s, so standby played no part.

## flash-lite per set

"Requests" counts every provider request, including the 429 retries and the schema agent's calls. Tokens and time are per run. Time per run includes the waits after the per-minute 429s.

| Set | Reps | Passed | Requests | Prompt tokens / run | Cache read | Output tokens / run | Time / run |
|---|---|---|---|---|---|---|---|
| local-basic | 1 | 3/3 | 8 | 9,028 | 7 % | 406 | 7 s |
| skill-loading | 1 | 12/12 | 62 | 28,130 | 37 % | 1,216 | 28 s |
| data-tools | 1 | 4/4 | 16 | 10,923 | 0 % | 535 | 19 s |
| missing-features | 1 | 5/6 | 21 | 12,984 | 31 % | 1,321 | 12 s |
| memory | 1 | 1/2 | 9 | 11,365 | 0 % | 1,419 | 39 s |
| project-card | 1 | 2/2 | 8 | 12,922 | 15 % | 784 | 10 s |
| grounding-honest | 1 | 4/9 | 41 | 17,227 | 17 % | 1,451 | 20 s |
| data-tools | 3 | 12/12 | 44 | 11,169 | 0 % | 605 | 13 s |
| memory | 3 | 3/6 | 25 | 10,786 | 0 % | 1,075 | 22 s |
| project-card | 3 | 4/6 | 30 | 15,964 | 15 % | 1,004 | 34 s |

- **Requests:** 264 in total. 14 of them were logged per-minute 429s, and about 13 were the schema agent's. In C1_keywords_query r1 the providers got one request more than the transcript lists (5 against 3 + 1), after a 1-minute wait: most likely a schema-agent retry.
- **Tokens:** about 984,000 prompt and 63,100 output. 20 % of the prompt tokens were read from Gemini's cache.
- **Latency per request** (the 237 answered main-chat requests): median 2.0 s, 90th percentile 4.2 s, slowest 37 s. None took over 60 s.
- **Errors:** 14 × `rate_limited (429)`, "limit: 15 requests" a minute. They came in 9 runs (L02, L03, L08, D4, D2 r2, R2, M2 ×2, C2 r2). No 402, 5xx or stalled stream.
- L03_add_column took 27 requests, as in older flash-lite reports (26). mistral took 25 there. The fake migration doesn't change the fixture, so models check again and again.

## Comparison

flash-lite is today's run. The NVIDIA numbers are taken from the NVIDIA report. The two muse-glimmer `data-tools` runs lost to standby (D3_refused_internal r1 and r2 at reps 3) are left out.

| Set | flash-lite (today) | mistral-large-4 | muse-glimmer-30b |
|---|---|---|---|
| local-basic | 3/3 | 3/3 | 3/3 |
| skill-loading | 12/12 | 12/12 | 11/12 (1 broke) |
| data-tools (r1 + r3) | 16/16 | 13/16 | 13/14 |
| missing-features | 5/6 | 6/6 | 4/6 |
| memory (r1 + r3) | 4/8 | 7/8 | 2/8 |
| project-card (r1 + r3) | 6/8 | 4/8 (1 broke) | 4/8 |
| grounding-honest | 4/9 | 5/9 | 4/9 (1 broke) |
| **Reps 1 only** | **31/38** | **32/38** | **27/38** |
| **All runs** | **50/62, 81 %** | **50/62, 81 %** | **41/60, 68 %** |
| Requests | 264 (14 were 429s) | 221 | 270 |
| Prompt tokens | ~984,000 | ~801,000 | ~1,050,000 |
| Output tokens | ~63,100 | ~36,700 | ~72,100 |
| Median request | 2.0 s | 8.6 s | 17.5 s |
| 90th percentile | 4.2 s | 51 s | 54 s |
| Broke | 0 | 1 (NVIDIA hang) | 2 slow loops, + 2 standby (left out) |

## flash-lite's failures

"Shared" means all three models failed this scenario the same way, or nearly. Those may point at the scenario or the app rather than a model.

- **M2_tldr (4 of 4 runs failed). Shared.** In the new chat it calls `list_chats`, and in 2 runs `read_messages` too, before answering. The "TL;DR:" line is there in 3 runs. In r2 it sent an empty answer after `list_chats`. mistral failed this once (r3, the same calls), muse 4 of 4. Since all three reach for old chats here, the new chat's context may invite it.
- **C1_keywords_query (2 of 4 runs failed). Shared in part.**
  - r2 called `describe_table` on `btc_news`, though the card lists the columns. mistral (3/4) and muse (4/4) did the same.
  - r3 put its final answer into the `insert` call before the join ran. The answer then said it "listed the matching news items" without listing them. Only flash-lite did this.
- **R1_moving_avg_table. Shared.** Its query filtered the dates in `WHERE` before the window, so the first six averages use fewer than 7 days (63,298.68 on 09-07, not 63,050.59). mistral made the same mistake. muse broke on R1 after 8 minutes.
- **R4_worked_example, R5_supply_cap, R7_saved_config. Shared.** The number check fired: worked examples, the supply cap and a config's constants come from its own head, with no tool behind them (11 of 17, 5 of 5, 3 of 4 numbers). All three models failed these three.
- **R3_what_if.** The number check fired (4 of 21 numbers without a source). muse failed it too; mistral passed.
- **F4_view.** Asked for a chart, it doesn't say views aren't available yet. It made a `bitcoin_prices` table, inserted 30 days of closes it made up, and drew an ASCII chart "stored in your database". This is the worst failure of the run: invented data saved as real. muse failed F4 differently (it asked the user for names); mistral passed.
- **Info only:** C2_buys_insert r1 made 2 calls where 1 is expected.

## Last report against today

The older reports are from before P2-09 and P2-10, so tools and prompts are a little longer now. Today's reps-1 runs are small samples.

| Set | Older flash-lite report | Today |
|---|---|---|
| skill-loading | 32/36, 89 % (2026-10-13, reps 3); 11/12 (2026-10-11) | 12/12 |
| data-tools | 12/12 (2026-10-18, reps 3) | 16/16 (r1 + r3) |
| missing-features | 16/18, 89 % (2026-10-21, reps 3) | 5/6 |

- **skill-loading:** better. L04_sql_fails failed in every older run (0/3, 0/1, 0/1): `sql-queries` wasn't loaded first. Today it passed. L02_schedule (2/3 before) passed too. Prompt tokens per run are the same (28,170 → 28,130); output fell (1,761 → 1,216); requests per run fell (5.8 → 5.2).
- **data-tools:** the same pass share. On 2026-10-18, D3_refused_internal skipped the refused query in 2 of 3 runs (then info only). Today it ran the query in all 4 runs. Prompt tokens per run rose about 30 % (8,568 → about 11,100), from the new tools.
- **missing-features:** about the same. F4_view still fails (1/3 before, 0/1 today), and it still makes up prices. Prompt tokens per run barely moved (12,816 → 12,984).

So the new tools didn't make flash-lite worse on these sets, and skill-loading looks better.

## Recommendation

**Yes, mistral-large-4 still looks like a good second test model next to flash-lite.**

- Over the same 62 runs they tie at 81 %, so it is a peer, not a weaker model.
- Their weak spots differ, so together they cover more:
  - mistral follows memory across chats (7/8; flash-lite 4/8) and says views aren't there yet (F4; flash-lite makes up data).
  - flash-lite is steadier on `data-tools` (16/16 against 13/16) and on `project-card` (6/8 against 4/8).
- Where both fail the same way (M2_tldr, C1's `describe_table`, R1, R4, R5, R7), it's likely a scenario or app issue, which is useful to know.
- Drawbacks: mistral is about 4 times slower per request and NVIDIA sometimes hangs. flash-lite's free tier caps at 15 requests a minute, which slowed some runs here.

## Totals

| Key | Requests sent |
|---|---|
| GEMINI_API_KEY | 264 (incl. 14 logged per-minute 429s and the schema agent's calls) |

- All 264 fell before 08:00 UTC, so they count on the quota day that began 2026-11-09 08:00 UTC. Other work's use of the key that day isn't known here.
- No other key was used.

## Files

- Reports (`.md` and `.json`) and transcripts are in each set's `scenarios/<set>/results/` in this worktree, at the times 20261110-061411 to 20261110-063251.
- `scenarios/models.yaml` is git-ignored and was not committed. Nothing was committed.
