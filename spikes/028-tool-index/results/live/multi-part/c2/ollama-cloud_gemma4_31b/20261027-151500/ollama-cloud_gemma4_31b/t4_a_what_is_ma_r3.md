# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A moving average is a calculation used to analyze data points by creating a series of averages of different subsets of the full data set. In finance and crypto, it's primarily used to smooth out short-term price fluctuations (noise) to identify longer-term trends.

**How it works:**
It takes the average of a specific number of previous periods (e.g., 20 days). As a new data point becomes available, the oldest data point in the set is dropped, and the average "moves" forward.

**Common Types:**
*   **Simple Moving Average (SMA):** The arithmetic mean of the price over a specific period.
*   **Exponential Moving Average (EMA):** Gives more weight to recent prices, making it more responsive to new information.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2047 | 0 | 0 | 210 | end | 2.3 s |  |
