# L10_price_yesterday

What was the BTC price yesterday?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** What was the BTC price yesterday?

> call query {"sql":"SELECT price FROM btc_prices WHERE date = date('now', '-1 day')"}

> result query:
> price
> (0 rows)

> call query {"sql":"SELECT date FROM btc_prices ORDER BY date DESC LIMIT 1"}

> result query:
> date
> 2026-10-05
> (1 rows)

**Agent:** The latest BTC price in your database is from **2026-10-05** (today). There is no price recorded for yesterday (2026-10-04).

Would you like me to check if the pipeline that fetches BTC prices is set up correctly or if it needs to be updated?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 33 | 2560 | 0 | 27 | tool_use | 1.0 s |  |
| 2 | 69 | 2560 | 0 | 23 | tool_use | 0.9 s |  |
| 3 | 112 | 2560 | 0 | 72 | end | 1.2 s |  |
