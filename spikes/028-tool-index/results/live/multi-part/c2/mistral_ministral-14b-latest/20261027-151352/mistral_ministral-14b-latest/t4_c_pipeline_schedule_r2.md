# t4_c_pipeline_schedule

What does daily_prices do and when does it run (in the project card)

Good behaviour: Answer from the card (or read_config) in one turn; do not run the pipeline.

## Turn 1

**User:** What does the daily_prices pipeline do, and when does it run?

**Agent:** The **daily_prices** pipeline does the following:
- Fetches today's **OHLC (Open, High, Low, Close) price** for a specified cryptocurrency (default: **BTC**).
- Fetches the **latest news** for the same cryptocurrency.
- Writes the price data to the **`prices`** table.
- Writes the news data to the **`news`** table.

### Inputs:
- **coin**: Either **BTC** or **ETH** (default: **BTC**).
- **currency**: Either **USD** or **EUR** (default: **USD**).

### Schedule:
- Runs **daily at 08:00** (UTC+03:30, Europe/Berlin time).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 56 | 2048 | 0 | 170 | end | 1.8 s |  |
