# SPIKE-045 results

15 runs, 0 with an error (left out below). Conditions: 1 none, 2 user, 3 agent, 4 publish tool, 5 tool + section + list.

## Subscribe (condition 3)

The subscriptions agents made with `subscribe`. Needed: the docs agent's `schema_changed` and the reviewer's `pipeline_run` (2 per run). Right filter: the needed ones with the key's filter (any table; outcome = failed, any pipeline). Unneeded: neither needed nor allowed (allowed: the price watcher's daily_btc runs, the reporter's weekly_summary runs). At creation: made in the agent's first turn.

| Model | Runs | Made | Needed (recall) | Right filter | Unneeded | Allowed | Precision | At creation | Later | Duplicates | Unsubscribed |
|---|---|---|---|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 | 2 | 2/6 (33%) | 0/2 (0%) | 0 | 0 | 2/2 (100%) | 0 | 2 | 0 | 0 |

Each subscription made, by agent (runs that made it):

- **nvidia/mistralai/mistral-large-4-1t-a52b:** Reviewer: pipeline_run where pipeline = weekly_summary (2);

## Delivery

Notices given per run, and notices skipped because the chat caused the event.

| Model | Condition | Runs | Notices per run | Own events skipped |
|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 none | 3 | 0.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 user | 3 | 3.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 agent | 3 | 0.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 publish tool | 3 | 3.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 5 tool + section + list | 3 | 4.0 | 0 |

## Publish (conditions 4 and 5)

Recall: the price watcher published in message 9's turn, when BTC fell 6.4% (the reporter subscribes to `alerts`). Right channel: on `alerts`. Title: names BTC and the move. Noise: reports published in the 10 routine turns. Extra: reports in turns that changed or found something no subscriber needs (the schema changes, the reviewer's turn after the failed run). Refused: publish calls to a channel that doesn't exist.

| Model | Condition | Runs | Published (recall) | Right channel | Title says what happened | Noise (reports) | Routine turns with a report | Extra | Refused | read_report calls |
|---|---|---|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 publish tool | 3 | 0/3 (0%) | – | – | 0 | 0/30 (0%) | 0 | 0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 5 tool + section + list | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 1 | 1/30 (3%) | 2 | 0 | 3 |

Every report published:

- nvidia/mistralai/mistral-large-4-1t-a52b c5 r1, message 3 (routine), Reviewer on `reviews` (rp_1): "daily_btc r_k3x9 (2026-11-11) succeeded but under-delivered: 1 news item saved instead of 5, and btc_prices.volume is always NULL"
- nvidia/mistralai/mistral-large-4-1t-a52b c5 r1, message 9 (need), Price watcher on `alerts` (rp_2): "BTC dropped 6.40% on 2026-11-12 (71,260 → 66,700 USD), crossing the 5% threshold"
- nvidia/mistralai/mistral-large-4-1t-a52b c5 r2, message 8 (extra), Reviewer on `reviews` (rp_1): "daily_btc run r_q7m2 failed at step price: HTTP 503 from api.example.com"
- nvidia/mistralai/mistral-large-4-1t-a52b c5 r2, message 9 (need), Price watcher on `alerts` (rp_2): "BTC dropped 6.40% day-over-day (2026-11-11 → 2026-11-12), crossing the 5% threshold"
- nvidia/mistralai/mistral-large-4-1t-a52b c5 r3, message 8 (extra), Reviewer on `reviews` (rp_1): "daily_btc run r_q7m2 failed: api.example.com returned HTTP 503 at step price"
- nvidia/mistralai/mistral-large-4-1t-a52b c5 r3, message 9 (need), Price watcher on `alerts` (rp_2): "BTC dropped 6.40% day-over-day (2026-11-11 → 2026-11-12), crossing the 5% threshold"

## Use

At the subscriber's next turn after the event. Docs (7): docs/schema.md shows `volume` after message 7 (schema change at 2). Reviewer (8): looks at the failed run r_q7m2 (run_status, or names it or the 503). Reporter (11): the draft or reply has today's 6.4% drop (also reachable from btc_prices with a query). Docs (13): shows `eth_prices` after message 13 (change at 10). End: the last docs have both. In condition 1 only the project card has the schema changes, and nothing has the runs.

| Model | Condition | Runs | Docs (7) volume | Reviewer (8) r_q7m2 | Reporter (11) drop | Docs (13) eth_prices | Docs at the end |
|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 none | 3 | 0/3 (0%) | 0/3 (0%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 user | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 agent | 3 | 0/3 (0%) | 0/3 (0%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 publish tool | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 5 tool + section + list | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |

## Cost

Prompt tokens per run (as the provider reports them, cached ones included), the event notices' share of them (estimated, summed over every request that carried them), and block 5's (estimated: session notes, subscriptions and, in condition 5, the channels).

| Model | Condition | Runs | Requests per run | Prompt tokens per run | Notices per run | Notices' share | Block 5 per run | Refused calls (groups) |
|---|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 none | 3 | 94.0 | 460871.3 | 0.0 | 0.00% | 0.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 user | 3 | 69.7 | 249521.3 | 577.3 | 0.23% | 2572.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 agent | 3 | 91.0 | 446787.3 | 0.0 | 0.00% | 3003.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 publish tool | 3 | 77.3 | 262016.7 | 666.7 | 0.25% | 2900.0 | 0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 5 tool + section + list | 3 | 77.3 | 309127.0 | 954.0 | 0.31% | 11094.3 | 0 |
