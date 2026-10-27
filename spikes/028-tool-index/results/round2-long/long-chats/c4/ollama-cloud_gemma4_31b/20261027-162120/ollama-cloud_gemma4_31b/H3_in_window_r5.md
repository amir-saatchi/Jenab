# H3_in_window

The same chart for ETH, from a turn in the window (control)

Good behaviour: Turn 9, inside the window, built btc_price_news: a scatter chart of the price against the number of news items a day. The follow-up needs no older turn; cards should not get in the way.

## Turn 1

**User:** How many days of prices do we have in btc_prices?

> call query {"sql":"SELECT COUNT(*) AS days, MIN(date) AS first, MAX(date) AS last FROM btc_prices"}

> result query:
> 1 row:
> days | first | last
> 10 | 2026-09-26 | 2026-10-05

**Agent:** Ten days, from 2026-09-26 to 2026-10-05.

## Turn 2

**User:** What was the highest BTC price in that range?

> call query {"sql":"SELECT date, price FROM btc_prices ORDER BY price DESC LIMIT 1"}

> result query:
> 1 row:
> date | price
> 2026-09-26 | 61000

**Agent:** The highest was 61,000 USD on 2026-09-26; it has gone down a little every day since.

## Turn 3

**User:** Add 'halving' to the news keywords.

> call insert {"rows":[{"keyword":"halving"}],"table":"watch_keywords"}

> result insert:
> inserted 1 row into watch_keywords.

**Agent:** Done. The news keywords are now ETF, regulation and halving.

## Turn 4

**User:** Save the Bitcoin whitepaper to our resources, https://example.org/bitcoin.pdf

> call save_link {"title":"Bitcoin whitepaper","url":"https://example.org/bitcoin.pdf"}

> result save_link:
> saved link 1 to Resources.

**Agent:** Saved "Bitcoin whitepaper" to Resources.

## Turn 5

**User:** When does daily_btc run?

**Agent:** Every day at 08:00 UTC, and you can start it by hand. It has not run yet.

## Turn 6

**User:** Which BTC news did we store on 2026-10-04?

> call query {"sql":"SELECT title, url FROM btc_news WHERE date = '2026-10-04'"}

> result query:
> 2 rows:
> title | url
> Seed story 2 | https://news.example.com/b/2
> Seed story 3 | https://news.example.com/b/3

**Agent:** Two stories, "Seed story 2" and "Seed story 3".

## Turn 7

**User:** What is the average BTC price over the ten days?

> call query {"sql":"SELECT ROUND(AVG(price), 1) AS avg FROM btc_prices"}

> result query:
> 1 row:
> avg
> 60550.0

**Agent:** 60,550 USD.

## Turn 8

**User:** And how much did ETH move over the same days?

> call query {"sql":"SELECT MIN(price) AS low, MAX(price) AS high FROM eth_prices"}

> result query:
> 1 row:
> low | high
> 2299 | 2410

**Agent:** ETH ranged from 2,299 to 2,410 USD, about 5% from low to high.

## Turn 9

**User:** Make a scatter chart of the BTC price against the number of news items each day, titled 'Price vs news'.

> call save_view {"yaml":"version: 1\nid: btc_price_news\ntitle: Price vs news\ntype: chart\nchart_type: scatter\nquery: |\n  SELECT p.date, p.price, COUNT(n.url) AS news\n  FROM btc_prices p LEFT JOIN btc_news n ON n.date = p.date\n  GROUP BY p.date\nx: { field: news, label: \"News items\" }\ny:\n  - { field: price, label: \"Price (USD)\" }\n"}

> result save_view:
> saved view (revision 1).

**Agent:** Saved btc_price_news, "Price vs news", a scatter chart with one point per day.

## Turn 10

**User:** Thanks. Which views do we have now?

**Agent:** btc_avg14_chart, btc_news_table, btc_price_chart, btc_price_news and btc_weekly_range.

## Turn 11

**User:** Now the same chart for ETH, from eth_prices and eth_news.

> call save_view {"yaml":"version: 1\nid: eth_price_news\ntitle: Price vs news (ETH)\ntype: chart\nchart_type: scatter\nquery: |\n  SELECT p.date, p.price, COUNT(n.url) AS news\n  FROM eth_prices p LEFT JOIN eth_news n ON n.date = p.date\n  GROUP BY p.date\nx: { field: news, label: \"News items\" }\ny:\n  - { field: price, label: \"Price (USD)\" }\n"}

*Skill loaded: config-guide*

> result save_view:
> saved view (revision 1).

**Agent:** Saved eth_price_news, "Price vs news (ETH)", a scatter chart with one point per day.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3233 | 0 | 0 | 450 | tool_use | 2.1 s |  |
| 2 | 6313 | 0 | 0 | 78 | end | 1.2 s |  |
