# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The **daily_prices** pipeline:
- Fetches today's **price** (OHLC: open, high, low, close) for a specified **coin** (default: BTC).
- Fetches the **latest news** for the same coin.
- Writes the price data to the **prices** table and the news to the **news** table.

**Inputs**:
- `coin`: BTC or ETH (default: BTC).
- `currency`: USD or EUR (default: USD).

**Schedule**: Runs **daily at 08:00** (UTC+03:30, Europe/Berlin time). A run takes about **30 seconds**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 95 | 2304 | 0 | 149 | end | 3.7 s |  |
