# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A moving average is a calculation used to analyze data points by creating a series of averages of different subsets of the full data set. In finance and crypto, it is primarily used to smooth out short-term price fluctuations (noise) to identify the underlying trend.

**How it works:**
Instead of looking at a single price, you take the average of the price over a specific period (e.g., 20 days). As a new day of data arrives, the oldest day is dropped, and the new one is added, causing the average to "move" along the timeline.

**Common types:**
*   **Simple Moving Average (SMA):** The arithmetic mean of the prices over the chosen period.
*   **Exponential Moving Average (EMA):** Gives more weight to recent prices, making it react faster to new price changes than the SMA.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3026 | 0 | 0 | 270 | end | 2.5 s |  |
