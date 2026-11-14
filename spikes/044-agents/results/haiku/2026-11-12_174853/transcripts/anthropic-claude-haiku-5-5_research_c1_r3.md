# anthropic-claude-haiku-5-5_research_c1_r3

Researcher, 1 key, rep 3, 24 s.

**Task:** true (read the web true; notes on all three in "notes/bitcoin_price_apis.md"). Groups [web files memory], recall 1.00, precision 1.00, forbidden 0. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M782WHQG5E2H728Q9SDB606F · task

4 requests, 24600 prompt tokens, tools 2107 tokens, system 564 tokens.

**User:** Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. Their free limits, how far back the history goes, and whether they need a key. Save your notes.

- `web_search` {"query":"CoinGecko API free plan rate limit historical data API key"}
  → Results for "CoinGecko API free plan rate limit historical data API key": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API docum…
- `web_search` {"query":"CoinCap API v3 rate limit API key historical data"}
  → Results for "CoinCap API v3 rate limit API key historical data": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API documentation …
- `web_search` {"query":"Binance public market data API klines rate limits no API key historical"}
  → Results for "Binance public market data API klines rate limits no API key historical": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - Coin…
- `fetch_page` {"url":"https://docs.coingecko.com/reference/introduction"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `fetch_page` {"url":"https://docs.coincap.io/"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `bucket_put` {"key":"notes/bitcoin_price_apis.md","content":"# Bitcoin price APIs: CoinGecko vs CoinCap vs Binance\n\nResearched 2026-11-12. Sources are the official docs pages named in each row.\n\n| | CoinGecko | CoinCap | Binance |\n|---|---|---|---|\n| Free limit | 30 calls/min, 10,000 calls/month (Demo plan) [1] | 2,500 credits/month [2] | 6,000 weight/min per IP [3] |\n| Key needed | Yes, free Demo key (`x-cg-demo-api-key`) [1] | Yes, since 2025 [2] | No, for public market data [3] |\n| BTC history | D…
  → Stored notes/bitcoin_price_apis.md: version 1, text/markdown, 1.5 KB. Link: jenab://01M782WHNRA9EBMQQF7N26S2QH/notes/bitcoin_price_apis.md
- `update_memory` {"scope":"project","section":"decisions","content":"Bitcoin price API research (notes/bitcoin_price_apis.md): Binance BTCUSDT needs no key and has the most free headroom but quotes USDT; CoinGecko free history is limited to 365 days; CoinCap needs a key.","expected_revision":0}
  → Saved project memory, section decisions, revision 1.

**Reply:** | | CoinGecko | CoinCap | Binance |
|---|---|---|---|
| Free limit | 30 calls/min, 10,000/month [1] | 2,500 credits/month [2] | 6,000 weight/min per IP [3] |
| Key needed | Yes, free Demo key [1] | Yes, since 2025 [2] | No, for public data [3] |
| BTC history | Daily back to 2013; free plan limited to last 365 days [1] | Daily back to 2013 [2] | Daily klines back to 2017 (BTCUSDT) [3] |
| Quote | USD | USD | USDT, not USD [3] |

**Recommendation:** Use Binance BTCUSDT daily klines for the daily_btc pipeline. It needs no key and has the most free headroom. Convert USDT to USD if you need exact USD. For deep history, CoinCap or paid CoinGecko, since free CoinGecko stops at 365 days.

Notes saved to `notes/bitcoin_price_apis.md`. Project memory now records the conclusion.

Sources:
1. https://docs.coingecko.com/reference/introduction
2. https://docs.coincap.io/
3. https://developers.binance.com/docs/binance-spot-api-docs/rest-api

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [notes/bitcoin_price_apis.md].

Memory:

```
[decisions]
Bitcoin price API research (notes/bitcoin_price_apis.md): Binance BTCUSDT needs no key and has the most free headroom but quotes USDT; CoinGecko free history is limited to 365 days; CoinCap needs a key.
```
