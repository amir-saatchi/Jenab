# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The **daily_prices** pipeline:
- Fetches today's price data (OHLC: open, high, low, close) and the latest news for a specified coin (default: **BTC**).
- Supports two currencies: **USD** (default) or **EUR**.
- Writes the data to the **prices** and **news** tables in the local database.

**Schedule**: It runs **daily at 08:00** (UTC+03:30, Europe/Berlin time). A single run takes about **30 seconds**.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 95 | 2304 | 0 | 125 | end | 2.5 s |  |
