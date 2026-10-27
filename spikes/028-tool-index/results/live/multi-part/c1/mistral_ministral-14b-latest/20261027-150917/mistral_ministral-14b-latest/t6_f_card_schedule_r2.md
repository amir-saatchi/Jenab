# t6_f_card_schedule

"When does daily_prices run?" (in the project card) -> Mother answers itself

Good behaviour: Answer 08:00 from the project card; no delegation.

## Turn 1

**User:** When does the daily_prices pipeline run?

**Agent:** The `daily_prices` pipeline runs **daily at 08:00** (Europe/Berlin time). It fetches today's price and the latest news for the specified coin (default: BTC) and writes them to the `prices` and `news` tables.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 4023 | 128 | 0 | 63 | end | 1.3 s |  |
