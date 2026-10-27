# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The **daily_prices** pipeline does the following:
1. Fetches today's price data (OHLC: Open, High, Low, Close) for a specified coin (default: **BTC**).
2. Fetches the latest news for the same coin.
3. Writes the price data to the **prices** table and the news to the **news** table.

### Inputs:
- **coin**: Either **BTC** or **ETH** (default: **BTC**).
- **currency**: Either **USD** or **EUR** (default: **USD**).

### Schedule:
It runs **daily at 08:00** (Europe/Berlin time). A single run takes about **30 seconds**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 103 | 3456 | 0 | 161 | end | 2.1 s |  |
