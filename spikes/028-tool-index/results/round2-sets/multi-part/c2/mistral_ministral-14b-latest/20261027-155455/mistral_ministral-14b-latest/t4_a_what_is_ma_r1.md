# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A **moving average (MA)** is a statistical tool used to analyze data points by creating a series of averages of different subsets of the full data set. It is commonly used in finance, economics, and time-series analysis to smooth out short-term fluctuations and highlight longer-term trends.

### Types of Moving Averages:
1. **Simple Moving Average (SMA):**
   - Calculated by taking the arithmetic mean of a given set of prices over a specific period.
   - Example: A 10-day SMA is the average of the last 10 days' closing prices.

2. **Exponential Moving Average (EMA):**
   - Gives more weight to recent prices, making it more responsive to new information.
   - Example: A 20-day EMA assigns higher importance to the most recent days.

### How It Works:
- For a **10-day SMA**, you add the closing prices of the last 10 days and divide by 10.
- For an **EMA**, recent prices are weighted more heavily, so the average reacts faster to changes.

### Use in Crypto/Finance:
- Helps identify trends (e.g., if the price is above the 50-day MA, it may indicate an uptrend).
- Used to generate trading signals (e.g., crossover strategies where a short-term MA crosses above or below a long-term MA).

Would you like an example calculation or how to apply it to the `prices` table in your project?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2390 | 0 | 0 | 310 | end | 5.4 s |  |
