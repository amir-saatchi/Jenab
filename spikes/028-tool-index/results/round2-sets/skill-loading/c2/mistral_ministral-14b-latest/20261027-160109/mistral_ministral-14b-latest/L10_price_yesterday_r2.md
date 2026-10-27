# L10_price_yesterday

What was the BTC price yesterday?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** What was the BTC price yesterday?

> call query {"sql":"SELECT price FROM btc_prices WHERE date = date('now', '-1 day')"}

> result query:
> price
> (0 rows)

> call query {"sql":"SELECT date, price FROM btc_prices ORDER BY date DESC LIMIT 1"}

> result query:
> date | price
> 2026-10-05 | 60100.00
> (1 rows)

**Agent:** The most recent BTC price in your data is from **October 5, 2026**, at **60,100.00**.

There is no record for yesterday (October 6, 2026). Would you like to fetch today's price or check if a pipeline can update the data?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 33 | 2560 | 0 | 27 | tool_use | 0.9 s |  |
| 2 | 69 | 2560 | 0 | 25 | tool_use | 0.8 s |  |
| 3 | 126 | 2560 | 0 | 70 | end | 1.1 s |  |
