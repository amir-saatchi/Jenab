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
| 1 | 2425 | 0 | 0 | 113 | tool_use | 1.7 s |  |
| 2 | 2567 | 0 | 0 | 58 | end | 1.1 s |  |
