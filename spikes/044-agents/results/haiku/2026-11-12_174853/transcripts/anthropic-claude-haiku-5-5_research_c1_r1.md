# anthropic-claude-haiku-5-5_research_c1_r1

Researcher, 1 key, rep 1, 13 s.

**Task:** true (read the web true; notes on all three in "notes/bitcoin_price_apis.md"). Groups [web files memory], recall 1.00, precision 1.00, forbidden 0. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M7826SZESDKH1A87NATDYH2P · task

4 requests, 24797 prompt tokens, tools 2107 tokens, system 564 tokens.

**User:** Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. Their free limits, how far back the history goes, and whether they need a key. Save your notes.

- `web_search` {"query":"CoinGecko API free plan rate limit historical data how far back"}
  → Results for "CoinGecko API free plan rate limit historical data how far back": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API …
- `web_search` {"query":"CoinCap API v3 rate limit API key required history"}
  → Results for "CoinCap API v3 rate limit API key required history": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API documentation…
- `web_search` {"query":"Binance public market data API klines rate limits no API key history limit"}
  → Results for "Binance public market data API klines rate limits no API key history limit": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - C…
- `fetch_page` {"url":"https://docs.coingecko.com/reference/introduction"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `fetch_page` {"url":"https://docs.coincap.io/"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `bucket_put` {"key":"notes/bitcoin_price_apis.md","content":"# Bitcoin price APIs: CoinGecko vs CoinCap vs Binance\n\nResearched 2026-11-12 from the providers' documentation pages.\n\n| | CoinGecko | CoinCap | Binance |\n|---|---|---|---|\n| Free limit | 30 calls/min, 10,000 calls/month (Demo plan) | 2,500 credits/month | 6,000 weight/min per IP |\n| API key | Yes, free Demo key (`x-cg-demo-api-key`) | Yes, required since 2025 | No, public market data needs no key |\n| BTC history depth | Daily back to 2013,…
  → Stored notes/bitcoin_price_apis.md: version 1, text/markdown, 1.6 KB. Link: jenab://01M7826SXSYNZGN8RHRVGF86GQ/notes/bitcoin_price_apis.md
- `update_memory` {"scope":"project","section":"decisions","content":"Price API research (2026-11-12, notes/bitcoin_price_apis.md): Binance is the preferred no-key source for daily BTC history (back to 2017, USDT quote); CoinGecko free plan is capped at 365 days of history; CoinCap needs a key.","expected_revision":0}
  → Saved project memory, section decisions, revision 1.

**Reply:** | | CoinGecko | CoinCap | Binance |
|---|---|---|---|
| Free limit | 30 calls/min, 10,000/month | 2,500 credits/month | 6,000 weight/min per IP |
| API key | Yes (free Demo key) | Yes (required since 2025) | No |
| BTC history | Daily to 2013, but free plan only last 365 days | Daily to 2013 | Daily klines to 2017 (BTCUSDT) |
| Quote | USD | USD | USDT, not USD |

Sources: docs.coingecko.com/reference/introduction, docs.coincap.io, developers.binance.com/docs/binance-spot-api-docs/rest-api.

**Recommendation:** Binance for the daily price history. It needs no key and goes back furthest on the free tier. Its prices are in USDT, so convert if you need USD. Use CoinGecko only if you need USD directly and can accept the 365-day history cap.

The CoinCap and Binance doc pages returned short summaries, so check exact credit costs and kline limits before relying on them.

Notes saved to `notes/bitcoin_price_apis.md` in the bucket. I also added the one-line conclusion to project memory.

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [notes/bitcoin_price_apis.md].

Memory:

```
[decisions]
Price API research (2026-11-12, notes/bitcoin_price_apis.md): Binance is the preferred no-key source for daily BTC history (back to 2017, USDT quote); CoinGecko free plan is capped at 365 days of history; CoinCap needs a key.
```
