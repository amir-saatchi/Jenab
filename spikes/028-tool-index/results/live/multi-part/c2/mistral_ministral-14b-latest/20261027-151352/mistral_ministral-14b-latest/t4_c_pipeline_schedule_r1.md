# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The **daily_prices** pipeline does the following:
- Fetches today's price data (OHLC: Open, High, Low, Close) for a specified cryptocurrency (default: **BTC**).
- Fetches the latest news related to the specified cryptocurrency.
- Writes the price data to the **prices** table and the news to the **news** table.

### When it runs:
- **Daily at 08:00** (UTC+03:30, Europe/Berlin time).

### Inputs:
- **coin**: BTC or ETH (default: BTC).
- **currency**: USD or EUR (default: USD).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 56 | 2048 | 0 | 147 | end | 1.9 s |  |
