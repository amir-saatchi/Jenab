# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A moving average is a calculation used to analyze data points by creating a series of averages of different subsets of the full data set. In finance and crypto tracking, it is primarily used to smooth out short-term price fluctuations (noise) to highlight longer-term trends.

**How it works:**
Instead of looking at a single price, you take the average of prices over a specific window of time (e.g., the last 20 days). As a new day of data arrives, the oldest day is dropped from the calculation, and the "average" moves forward.

**Common Types:**
*   **Simple Moving Average (SMA):** The arithmetic mean of the prices over the chosen period. Every day carries equal weight.
*   **Exponential Moving Average (EMA):** Gives more weight to the most recent prices. It reacts faster to price changes than the SMA.

**Why it's useful:**
*   **Trend Identification:** If the price is consistently above a moving average, the trend is generally considered bullish (upward); if below, bearish (downward).
*   **Support and Resistance:** Traders often view major moving averages (like the 50-day or 200-day) as levels where the price might bounce or struggle to break through.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3026 | 0 | 0 | 305 | end | 3.6 s |  |
