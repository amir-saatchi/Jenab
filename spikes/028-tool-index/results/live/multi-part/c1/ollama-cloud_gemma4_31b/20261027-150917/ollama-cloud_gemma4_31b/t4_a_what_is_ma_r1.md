# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A moving average is a calculation used to analyze data points by creating a series of averages of different subsets of the full data set. In finance and data analysis, it is primarily used to smooth out short-term fluctuations (noise) to highlight longer-term trends.

**How it works:**
It "moves" because as new data becomes available, the oldest data point in the window is dropped, and the newest one is added, shifting the average forward in time.

**Common Types:**
*   **Simple Moving Average (SMA):** The arithmetic mean of a set of prices over a specific number of periods. All data points are weighted equally.
*   **Exponential Moving Average (EMA):** Gives more weight to recent prices, making it more responsive to new information than the SMA.

**Example:**
In your "Crypto watch" project, a 7-day moving average of BTC prices would average the closing price of the last 7 days. Each day, the oldest price is replaced by today's price, showing you the general direction of the price without the daily "zigzag."

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3026 | 0 | 0 | 321 | end | 2.7 s |  |
