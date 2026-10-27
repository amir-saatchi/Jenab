# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A moving average is a calculation used to analyze data points by creating a series of averages of different subsets of the full data set. In finance and crypto, it's primarily used to smooth out short-term price fluctuations (noise) to identify longer-term trends.

**How it works:**
Instead of looking at a single price point, you take the average price over a specific window of time (e.g., 10 days, 50 days, or 200 days). As a new data point becomes available, the oldest one is dropped, and the "window" moves forward—hence the name "moving average."

**Common types:**
*   **Simple Moving Average (SMA):** The arithmetic mean of the prices over the chosen period.
*   **Exponential Moving Average (EMA):** Gives more weight to recent prices, making it react faster to new price changes than the SMA.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2242 | 0 | 0 | 252 | end | 3.3 s |  |
