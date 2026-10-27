# L11_what_tracked

In two or three sentences, what does this project track?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** In two or three sentences, what does this project track?

**Agent:** This project tracks **Bitcoin news** (stored in the `btc_news` table with URLs, dates, titles, and reasons for tracking) and **Bitcoin prices** (stored in the `btc_prices` table with daily price data). It also maintains a list of **keywords** (in `watch_keywords`) to filter or monitor specific news topics. The pipelines and views are set up to visualize and manage this data.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 115 | 3456 | 0 | 94 | end | 1.6 s |  |
