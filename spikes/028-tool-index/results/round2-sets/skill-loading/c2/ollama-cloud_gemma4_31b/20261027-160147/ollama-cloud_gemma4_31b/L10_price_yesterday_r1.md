# L10_price_yesterday

What was the BTC price yesterday?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** What was the BTC price yesterday?

> call query {"sql":"SELECT price FROM btc_prices WHERE date = '2026-10-05'"}

> result query:
> price
> 60100.00
> (1 rows)

**Agent:** The BTC price yesterday (2026-10-05) was 60,100.00.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2425 | 0 | 0 | 149 | tool_use | 2.0 s |  |
| 2 | 2603 | 0 | 0 | 55 | end | 0.9 s |  |
