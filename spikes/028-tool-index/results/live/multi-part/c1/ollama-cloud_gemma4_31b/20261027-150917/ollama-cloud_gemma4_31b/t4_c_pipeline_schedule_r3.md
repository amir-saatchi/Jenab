# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The `daily_prices` pipeline fetches today's price and the latest news for a specific coin (BTC by default, or ETH) in a chosen currency (USD by default, or EUR) and writes the data to the `prices` and `news` tables. It runs daily at 08:00.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3035 | 0 | 0 | 183 | end | 1.6 s |  |
