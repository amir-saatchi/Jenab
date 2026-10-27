# t4_a_what_is_ma

Simple knowledge question, no tools needed

Good behaviour: Answer directly in one turn; no tools, no background work.

## Turn 1

**User:** What is a moving average?

**Agent:** A **moving average (MA)** is a statistical tool used to analyze data points by creating a series of averages of different subsets of the full data set. It helps smooth out short-term fluctuations and highlight longer-term trends.

### Types of Moving Averages:
1. **Simple Moving Average (SMA):**
   - Calculated by taking the average of a fixed number of past data points (e.g., the last 10 days of closing prices).
   - Formula:
     \[
     \text{SMA} = \frac{\text{Sum of prices for the last } n \text{ days}}{n}
     \]
   - Example: A 10-day SMA for a stock would average the closing prices of the last 10 days.

2. **Exponential Moving Average (EMA):**
   - Gives more weight to recent prices, making it more responsive to new information.
   - Formula:
     \[
     \text{EMA} = (\text{Price today} \times \frac{12}{12 + n}) + (\text{EMA yesterday} \times \frac{n}{12 + n})
     \]
     where \( n \) is the number of periods (e.g., 10 days).
   - Example: A 10-day EMA reacts faster to price changes than an SMA.

### Use in Crypto/Finance:
- **Trend Identification:** Moving averages help identify the direction of the trend (e.g., if the price is above the 50-day MA, the trend may be upward).
- **Support/Resistance:** Moving averages can act as dynamic support or resistance levels.
- **Crossover Strategies:** Traders use crossovers between short-term and long-term MAs (e.g., 50-day and 200-day) to generate buy/sell signals.

Would you like me to calculate a moving average for your BTC price data?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 94 | 3456 | 0 | 399 | end | 15.3 s |  |
