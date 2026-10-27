# L11_what_tracked

In two or three sentences, what does this project track?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** In two or three sentences, what does this project track?

**Agent:** This project tracks **Bitcoin news** (stored in the `btc_news` table) by monitoring keywords and fetching relevant articles, along with **Bitcoin price data** (in the `btc_prices` table) over time. It also maintains a list of keywords (`watch_keywords`) to filter or trigger news updates. The views and pipelines are set up to display this data in tables, charts, and forms.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 115 | 3456 | 0 | 90 | end | 1.8 s |  |
