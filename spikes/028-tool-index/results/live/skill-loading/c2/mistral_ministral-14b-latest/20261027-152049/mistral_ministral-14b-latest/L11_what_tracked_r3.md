# L11_what_tracked

In two or three sentences, what does this project track?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** In two or three sentences, what does this project track?

**Agent:** This project tracks **Bitcoin-related data**, including:
- **News articles** about Bitcoin (stored in the `btc_news` table) with keywords for filtering.
- **Bitcoin price history** (stored in the `btc_prices` table) over time.
- **Keywords** used to monitor specific topics in Bitcoin news (stored in the `watch_keywords` table).

It also includes views and pipelines to visualize and manage this data.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 16 | 2176 | 0 | 97 | end | 2.8 s |  |
