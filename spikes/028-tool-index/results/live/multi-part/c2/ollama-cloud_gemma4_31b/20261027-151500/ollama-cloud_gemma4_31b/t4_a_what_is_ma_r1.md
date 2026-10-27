# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A moving average (MA) is a calculation used to analyze data points by creating a series of averages of different subsets of the full data set. In finance and trading, it is primarily used to smooth out short-term price fluctuations (noise) to identify the underlying trend.

**How it works:**
Instead of looking at a single price point, you take the average of prices over a specific window of time (e.g., the last 10 days). As a new day of data arrives, the oldest day is dropped from the calculation, and the "average" moves forward.

**Common types:**
*   **Simple Moving Average (SMA):** The arithmetic mean of the prices over the period.
*   **Exponential Moving Average (EMA):** Gives more weight to recent prices, making it more responsive to new information.

**Example in your project:**
If you wanted to see the trend for BTC prices in your `prices` table, you could calculate a 7-day moving average of the `close` price to see if the overall direction is up or down, regardless of daily spikes.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2047 | 0 | 0 | 323 | end | 2.9 s |  |
