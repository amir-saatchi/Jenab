# L11_what_tracked

In two or three sentences, what does this project track?

Good behaviour: Answer without loading a skill.

## Turn 1

**User:** In two or three sentences, what does this project track?

**Agent:** This project tracks **Bitcoin news** (stored in the `btc_news` table) and **Bitcoin prices** (stored in the `btc_prices` table). It also maintains a list of **keywords** (in the `watch_keywords` table) to filter or monitor relevant news topics. The data is fetched via pipelines and displayed in views like tables, charts, and forms.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 37 | 2560 | 0 | 86 | end | 1.7 s |  |
