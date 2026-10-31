# TASK-027 dev sweep: SPIKE-028's long chats

Each chat has 10 earlier turns; the follow-up's first request cuts, so the window starts at turn 8.

- H1_avg_chart (needs turn 2, past the cut): "Make that 14-day average chart for ETH too. eth_prices has the same columns as btc_prices."
  scores: t1 2.30/1w t2 7.13/4w t3 3.22/2w t6 1.61/1w t8 1.61/1w t9 4.61/2w t10 1.61/1w
- H2_drop_alert (needs turn 3, past the cut): "Could ETH get the early warning as well? Its prices are in eth_prices."
  scores: t1 3.22/2w t2 1.61/1w t3 1.61/1w t4 2.30/1w t9 2.30/1w
- H3_in_window (needs turn 9, in the window): "Now the same chart for ETH, from eth_prices and eth_news."
  scores: t1 3.91/2w t3 1.61/1w t8 6.21/3w t9 1.61/1w t10 3.22/2w
- H4_week_range (needs turn 5, past the cut): "Can I see the per-week spread for Ether as well? It's in eth_prices."
  scores: t1 3.91/2w t2 1.61/1w t5 6.21/3w t6 2.30/1w t7 2.30/1w t9 2.30/1w t10 1.61/1w
- H5_etf_digest (needs turn 4, past the cut): "Could Ether get that weekly roundup too? Store it in eth_news."
  scores: t2 2.30/1w t4 1.61/1w t7 1.61/1w t9 2.30/1w t10 2.30/1w

| min score | margin | needed turn carded | wrong cards | cards on the control |
|---|---|---|---|---|
| 2.0 | 1.00 | 2/4 | 2 | 0 |
| 2.0 | 1.25 | 2/4 | 2 | 0 |
| 2.0 | 1.50 | 2/4 | 2 | 0 |
| 2.0 | 2.00 | 1/4 | 2 | 0 |
| 3.0 | 1.00 | 2/4 | 2 | 0 |
| 3.0 | 1.25 | 2/4 | 2 | 0 |
| 3.0 | 1.50 | 2/4 | 2 | 0 |
| 3.0 | 2.00 | 1/4 | 2 | 0 |
| 4.0 | 1.00 | 2/4 | 0 | 0 |
| 4.0 | 1.25 | 2/4 | 0 | 0 |
| 4.0 | 1.50 | 2/4 | 0 | 0 |
| 4.0 | 2.00 | 1/4 | 0 | 0 |
| 5.0 | 1.00 | 2/4 | 0 | 0 |
| 5.0 | 1.25 | 2/4 | 0 | 0 |
| 5.0 | 1.50 | 2/4 | 0 | 0 |
| 5.0 | 2.00 | 1/4 | 0 | 0 |
| 6.0 | 1.00 | 2/4 | 0 | 0 |
| 6.0 | 1.25 | 2/4 | 0 | 0 |
| 6.0 | 1.50 | 2/4 | 0 | 0 |
| 6.0 | 2.00 | 1/4 | 0 | 0 |
| 8.0 | 1.00 | 0/4 | 0 | 0 |
| 8.0 | 1.25 | 0/4 | 0 | 0 |
| 8.0 | 1.50 | 0/4 | 0 | 0 |
| 8.0 | 2.00 | 0/4 | 0 | 0 |

The app's PickCards: H1_avg_chart [2]; H2_drop_alert []; H3_in_window []; H4_week_range [5]; H5_etf_digest [].

Frozen (TASK-027): min score 5.0 and margin 1.25, the middle of the range
that cards the needed turn of H1 and H4 with no wrong card and none on
the control H3 (min score above 3.91 up to 6.21; margin 1 to 1.5). H2 and
H5 paraphrase their turn and share no rare word with it, so no threshold
cards them. Set before the history-cards set was scored.
