# SPIKE-045 results

15 runs, 0 with an error (left out below). Conditions: 1 none, 2 user, 3 agent, 4 publish tool, 5 tool + section + list.

## Subscribe (condition 3)

The subscriptions agents made with `subscribe`. Needed: the docs agent's `schema_changed` and the reviewer's `pipeline_run` (2 per run). Right filter: the needed ones with the key's filter (any table; outcome = failed, any pipeline). Unneeded: neither needed nor allowed (allowed: the price watcher's daily_btc runs, the reporter's weekly_summary runs). At creation: made in the agent's first turn.

| Model | Runs | Made | Needed (recall) | Right filter | Unneeded | Allowed | Precision | At creation | Later | Duplicates | Unsubscribed |
|---|---|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 3 | 2 | 2/6 (33%) | 1/2 (50%) | 0 | 0 | 2/2 (100%) | 0 | 2 | 0 | 0 |

Each subscription made, by agent (runs that made it):

- **gemini/gemini-3.5-flash-lite:** Docs agent: schema_changed (any) (1); Reviewer: pipeline_run (any) (1);

## Delivery

Notices given per run, and notices skipped because the chat caused the event.

| Model | Condition | Runs | Notices per run | Own events skipped |
|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 1 none | 3 | 0.0 | 0 |
| gemini/gemini-3.5-flash-lite | 2 user | 3 | 3.0 | 0 |
| gemini/gemini-3.5-flash-lite | 3 agent | 3 | 0.3 | 0 |
| gemini/gemini-3.5-flash-lite | 4 publish tool | 3 | 3.7 | 0 |
| gemini/gemini-3.5-flash-lite | 5 tool + section + list | 3 | 4.0 | 0 |

## Publish (conditions 4 and 5)

Recall: the price watcher published in message 9's turn, when BTC fell 6.4% (the reporter subscribes to `alerts`). Right channel: on `alerts`. Title: names BTC and the move. Noise: reports published in the 10 routine turns. Extra: reports in turns that changed or found something no subscriber needs (the schema changes, the reviewer's turn after the failed run). Refused: publish calls to a channel that doesn't exist.

| Model | Condition | Runs | Published (recall) | Right channel | Title says what happened | Noise (reports) | Routine turns with a report | Extra | Refused | read_report calls |
|---|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 4 publish tool | 3 | 2/3 (67%) | 2/2 (100%) | 2/2 (100%) | 0 | 0/30 (0%) | 1 | 3 | 2 |
| gemini/gemini-3.5-flash-lite | 5 tool + section + list | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 4 | 4/30 (13%) | 9 | 0 | 3 |

Every report published:

- gemini/gemini-3.5-flash-lite c4 r1, message 8 (extra), Reviewer on `general` (refused): "Pipeline daily_btc failed (r_q7m2)"
- gemini/gemini-3.5-flash-lite c4 r1, message 8 (extra), Reviewer on `reviews` (rp_2): "Failed pipeline run r_q7m2 (daily_btc)"
- gemini/gemini-3.5-flash-lite c4 r2, message 9 (need), Price watcher on `updates` (refused): "BTC dropped by 6.4% on 2026-11-12"
- gemini/gemini-3.5-flash-lite c4 r2, message 9 (need), Price watcher on `alerts` (rp_2): "BTC dropped by 6.4% on 2026-11-12"
- gemini/gemini-3.5-flash-lite c4 r3, message 9 (need), Price watcher on `bitcoin` (refused): "BTC price dropped by 6.40% on 2026-11-12"
- gemini/gemini-3.5-flash-lite c4 r3, message 9 (need), Price watcher on `alerts` (rp_2): "BTC price dropped by 6.40% on 2026-11-12"
- gemini/gemini-3.5-flash-lite c5 r1, message 2 (extra), Schema agent on `data` (rp_1): "Added volume column to btc_prices"
- gemini/gemini-3.5-flash-lite c5 r1, message 3 (routine), Reviewer on `reviews` (rp_2): "daily_btc run r_k3x9 inserted zero news items due to duplicate feed URLs"
- gemini/gemini-3.5-flash-lite c5 r1, message 8 (extra), Reviewer on `reviews` (rp_3): "daily_btc run r_q7m2 failed due to HTTP 503 from api.example.com"
- gemini/gemini-3.5-flash-lite c5 r1, message 9 (need), Price watcher on `alerts` (rp_4): "BTC dropped by 6.40% on 2026-11-12"
- gemini/gemini-3.5-flash-lite c5 r1, message 10 (extra), Schema agent on `data` (rp_5): "Created eth_prices table"
- gemini/gemini-3.5-flash-lite c5 r2, message 2 (extra), Schema agent on `data` (rp_1): "Added volume column to btc_prices"
- gemini/gemini-3.5-flash-lite c5 r2, message 3 (routine), Reviewer on `reviews` (rp_2): "Review of daily_btc run r_k3x9"
- gemini/gemini-3.5-flash-lite c5 r2, message 8 (extra), Reviewer on `reviews` (rp_3): "Review of failed daily_btc run r_q7m2"
- gemini/gemini-3.5-flash-lite c5 r2, message 9 (need), Price watcher on `alerts` (rp_4): "BTC dropped 6.40% on 2026-11-12 (crossed 5%)"
- gemini/gemini-3.5-flash-lite c5 r2, message 10 (extra), Schema agent on `data` (rp_5): "Added table eth_prices"
- gemini/gemini-3.5-flash-lite c5 r3, message 2 (extra), Schema agent on `data` (rp_1): "Added volume column to btc_prices"
- gemini/gemini-3.5-flash-lite c5 r3, message 7 (routine), Docs agent on `data` (rp_2): "Updated docs/schema.md with intro and volume column"
- gemini/gemini-3.5-flash-lite c5 r3, message 8 (extra), Reviewer on `reviews` (rp_3): "daily_btc run r_q7m2 failed due to HTTP 503"
- gemini/gemini-3.5-flash-lite c5 r3, message 9 (need), Price watcher on `alerts` (rp_4): "BTC dropped 6.40% to 66,700"
- gemini/gemini-3.5-flash-lite c5 r3, message 10 (extra), Schema agent on `data` (rp_5): "Created eth_prices table"
- gemini/gemini-3.5-flash-lite c5 r3, message 13 (routine), Docs agent on `data` (rp_6): "Added eth_prices table to docs/schema.md"

## Use

At the subscriber's next turn after the event. Docs (7): docs/schema.md shows `volume` after message 7 (schema change at 2). Reviewer (8): looks at the failed run r_q7m2 (run_status, or names it or the 503). Reporter (11): the draft or reply has today's 6.4% drop (also reachable from btc_prices with a query). Docs (13): shows `eth_prices` after message 13 (change at 10). End: the last docs have both. In condition 1 only the project card has the schema changes, and nothing has the runs.

| Model | Condition | Runs | Docs (7) volume | Reviewer (8) r_q7m2 | Reporter (11) drop | Docs (13) eth_prices | Docs at the end |
|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 1 none | 3 | 0/3 (0%) | 0/3 (0%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |
| gemini/gemini-3.5-flash-lite | 2 user | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |
| gemini/gemini-3.5-flash-lite | 3 agent | 3 | 0/3 (0%) | 0/3 (0%) | 3/3 (100%) | 1/3 (33%) | 0/3 (0%) |
| gemini/gemini-3.5-flash-lite | 4 publish tool | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |
| gemini/gemini-3.5-flash-lite | 5 tool + section + list | 3 | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |

## Cost

Prompt tokens per run (as the provider reports them, cached ones included), the event notices' share of them (estimated, summed over every request that carried them), and block 5's (estimated: session notes, subscriptions and, in condition 5, the channels).

| Model | Condition | Runs | Requests per run | Prompt tokens per run | Notices per run | Notices' share | Block 5 per run | Refused calls (groups) |
|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 1 none | 3 | 60.3 | 288616.3 | 0.0 | 0.00% | 0.0 | 0 |
| gemini/gemini-3.5-flash-lite | 2 user | 3 | 57.7 | 251462.3 | 244.0 | 0.10% | 2097.0 | 0 |
| gemini/gemini-3.5-flash-lite | 3 agent | 3 | 56.3 | 322353.3 | 24.0 | 0.01% | 1865.0 | 0 |
| gemini/gemini-3.5-flash-lite | 4 publish tool | 3 | 70.3 | 358755.0 | 462.3 | 0.13% | 2735.3 | 0 |
| gemini/gemini-3.5-flash-lite | 5 tool + section + list | 3 | 61.3 | 324682.7 | 495.0 | 0.15% | 10003.0 | 0 |
