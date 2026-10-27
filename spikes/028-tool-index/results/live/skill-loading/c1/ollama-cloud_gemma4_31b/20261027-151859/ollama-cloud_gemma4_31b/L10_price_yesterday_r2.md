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
| 1 | 3061 | 0 | 0 | 109 | tool_use | 1.5 s |  |
| 2 | 3199 | 0 | 0 | 56 | end | 1.0 s |  |
