# L11_what_tracked

In two or three sentences, what does this project track?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** In two or three sentences, what does this project track?

**Agent:** This project tracks **Bitcoin-related news and prices**:

- The `btc_news` table stores news articles with URLs, dates, titles, and reasons for tracking, fetched from monitored keywords.
- The `btc_prices` table records daily Bitcoin prices.
- The `watch_keywords` table lists keywords used to filter relevant news.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 37 | 2560 | 0 | 71 | end | 1.3 s |  |
