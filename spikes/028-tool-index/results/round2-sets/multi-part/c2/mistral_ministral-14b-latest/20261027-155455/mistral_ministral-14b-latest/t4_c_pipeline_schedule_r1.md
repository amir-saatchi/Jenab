# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The **daily_prices** pipeline:
- Fetches today's **price** (OHLC: open, high, low, close) and the **latest news** for a specified coin (default: **BTC**).
- Writes the price data to the **prices** table and the news to the **news** table.
- Supports **BTC or ETH** as the coin and **USD or EUR** as the currency (defaults: **BTC, USD**).

It runs **daily at 08:00** (Europe/Berlin time). A single run takes about **30 seconds**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 95 | 2304 | 0 | 134 | end | 2.7 s |  |
