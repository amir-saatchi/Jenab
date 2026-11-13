# anthropic-claude-haiku-5-5_research_c4_r2

Researcher, 4 unasked, rep 2, 26 s.

**Task:** true (read the web true; notes on all three in "research/bitcoin_price_apis_2026-11-12.md"). Groups [files web], recall 0.67, precision 1.00, forbidden 0. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

**create_chat** (task) → 01M782MMDHK24Q7W97TD0C1TMV "Research: Bitcoin price APIs", picks [web files] → groups [files web], 111 tokens

> Role: You are a research assistant for the Bitcoin project. You look up facts on the web for the user and save them as notes they can come back to. Use only what your sources say, and cite the URL for each fact. Say when a source is unclear or can't be confirmed. Save the final notes with bucket_put under research/ in the project files, using a short descriptive name with the date. Report back briefly with the saved file name and the key findings.

> Message: Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. For each, find: (1) free tier limits (rate limits, call quotas), (2) how far back the historical price data goes, and (3) whether an API key is needed, for the free tier and for historical data. Use the official docs and pricing pages. Put the comparison in a short table plus source URLs and the date you checked. Save the notes as a file under research/ with bucket_put, and tell me the file name.

## mother · plain

1 requests, 5172 prompt tokens, tools 2597 tokens, system 649 tokens.

**User:** What does the daily_btc pipeline do?


**Reply:** The `daily_btc` pipeline runs every day at 08:00 (cron `0 8 * * *`). It fetches the current BTC price and the 5 news items most likely to affect it. Its results go into the project's `btc_prices` and `btc_news` tables.

## mother · ask

1 requests, 5390 prompt tokens, tools 2597 tokens, system 649 tokens.

**User:** I often need things looked up on the web for this project, like comparing data sources, with notes I can come back to later.


**Reply:** That fits a dedicated research chat. I can set one up with web search, page reading and the project's files. For each lookup it would save a dated note in the project files, so you can come back to it later. For example, a comparison of BTC price sources would be stored as a note you can reopen. Want me to create it?

## mother · task

2 requests, 12264 prompt tokens, tools 2597 tokens, system 649 tokens.

**User:** Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. Their free limits, how far back the history goes, and whether they need a key. Save your notes.

- `create_chat` {"title":"Research: Bitcoin price APIs","role":"You are a research assistant for the Bitcoin project. You look up facts on the web for the user and save them as notes they can come back to. Use only what your sources say, and cite the URL for each fact. Say when a source is unclear or can't be confirmed. Save the final notes with bucket_put under research/ in the project files, using a short descriptive name with the date. Report back briefly with the saved file name and the key findings.","mess…
  → Started the chat 01M782MMDHK24Q7W97TD0C1TMV "Research: Bitcoin price APIs" as task t_1. Its reply comes in a finish notice; there is no need to poll.

**Reply:** I've started a research chat, "Research: Bitcoin price APIs", to look into CoinGecko, CoinCap and Binance. It will check their official docs for free-tier limits, how far back historical data goes, and whether a key is needed. It will save the notes to the project files under research/ and send me the file name. I'll pass on the results when they arrive.

## 01M782MMDHK24Q7W97TD0C1TMV · task:start

4 requests, 22392 prompt tokens, tools 1629 tokens, system 582 tokens.

**User:** [Message from Mother]  
Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. For each, find: (1) free tier limits (rate limits, call quotas), (2) how far back the historical price data goes, and (3) whether an API key is needed, for the free tier and for historical data. Use the official docs and pricing pages. Put the comparison in a short table plus source URLs and the date you checked. Save the notes as a file under research/ with bucket_put, and tell me the file name.

- `fetch_page` {"url":"https://www.coingecko.com/en/api/pricing"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `fetch_page` {"url":"https://docs.coingecko.com/v3.0.1/reference/introduction"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `fetch_page` {"url":"https://docs.coincap.io/"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api/general-api-information"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `fetch_page` {"url":"https://docs.coincap.io/#rate-limits"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://docs.coingecko.com/reference/coins-id-market-chart"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `bucket_put` {"key":"research/bitcoin_price_apis_2026-11-12.md","content":"# Bitcoin price APIs: CoinGecko vs CoinCap vs Binance\n\nChecked: 2026-11-12 (UTC)\n\n| | CoinGecko | CoinCap | Binance |\n|---|---|---|---|\n| (1) Free tier limits | Demo (free) plan: 30 calls/min, 10,000 calls/month | 2,500 credits/month | Public market data: weight limit 6,000/min per IP |\n| (2) Historical depth (BTC, daily) | Back to 2013, but the free plan returns at most the last 365 days | Back to 2013 (BTC), daily interval, v…
  → Stored research/bitcoin_price_apis_2026-11-12.md: version 1, text/markdown, 1.7 KB. Link: jenab://01M782MBAN52K7V4GMV5SYVZT8/research/bitcoin_price_apis_2026-11-12.md

**Reply:** I saved the comparison as **research/bitcoin_price_apis_2026-11-12.md**. I checked the official docs and pricing pages on 2026-11-12.

| | CoinGecko | CoinCap | Binance |
|---|---|---|---|
| Free tier limits | 30 calls/min, 10,000/month | 2,500 credits/month | 6,000 weight/min per IP |
| BTC history (daily) | Back to 2013; free plan limited to last 365 days | Back to 2013 | Back to 2017 (BTCUSDT) |
| Key, free tier | Yes (free Demo key) | Yes (required since 2025) | No |
| Key, historical data | Yes; free plan capped at 365 days | Yes | No (public klines) |

Caveats:
- Binance prices are in USDT, not USD.
- The CoinCap docs don't say how many credits each call uses, so that is unconfirmed.
- The pages I could fetch were short summaries, not full pricing tables. Recheck the live pages before relying on the limits.

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [research/bitcoin_price_apis_2026-11-12.md].
