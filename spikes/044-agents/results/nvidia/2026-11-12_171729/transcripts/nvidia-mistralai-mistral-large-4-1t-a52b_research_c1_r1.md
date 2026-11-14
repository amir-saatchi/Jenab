# nvidia-mistralai-mistral-large-4-1t-a52b_research_c1_r1

Researcher, 1 key, rep 1, 389 s.

**Task:** true (read the web true; notes on all three in "notes/btc-price-apis-comparison.md"). Groups [web files memory], recall 1.00, precision 1.00, forbidden 0. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M7B7NABMSASW1DFXCWWAHXX4 · task

8 requests, 63765 prompt tokens, tools 2107 tokens, system 564 tokens.

**User:** Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. Their free limits, how far back the history goes, and whether they need a key. Save your notes.

- `web_search` {"query":"CoinGecko API free tier rate limits API key required 2026 pricing demo plan"}
  → Results for "CoinGecko API free tier rate limits API key required 2026 pricing demo plan": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - …
- `web_search` {"query":"CoinGecko API historical data how far back market_chart/range free tier days limit"}
  → Results for "CoinGecko API historical data how far back market_chart/range free tier days limit": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). D…
- `web_search` {"query":"CoinCap API v3 free rate limit API key required docs 2026"}
  → Results for "CoinCap API v3 free rate limit API key required docs 2026": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API docume…
- `web_search` {"query":"CoinCap API history endpoint how far back historical prices assets bitcoin docs"}
  → Results for "CoinCap API history endpoint how far back historical prices assets bitcoin docs": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da…
- `web_search` {"query":"Binance public API klines candlestick history limit how far back free no API key 2026"}
  → Results for "Binance public API klines candlestick history limit how far back free no API key 2026": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key)…
- `web_search` {"query":"Binance API rate limits weight limits public endpoints free without key 2026 docs"}
  → Results for "Binance API rate limits weight limits public endpoints free without key 2026 docs": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da…
- `fetch_page` {"url":"https://docs.coingecko.com/reference/introduction"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `fetch_page` {"url":"https://docs.coingecko.com/reference/price-historical-chart-data-within-range"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `web_search` {"query":"CoinGecko docs market_chart/range \"days\" parameter max \"full\" historical data free demo plan granularity daily data beyond 365 days"}
  → Results for "CoinGecko docs market_chart/range \"days\" parameter max \"full\" historical data free demo plan granularity daily data beyond 365 days": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a m…
- `fetch_page` {"url":"https://docs.coincap.io/"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `web_search` {"query":"docs.coincap.io \"rate limits\" free tier credits per month API key bearer 2026"}
  → Results for "docs.coincap.io \"rate limits\" free tier credits per month API key bearer 2026": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da…
- `web_search` {"query":"docs.coincap.io assets/{id}/history start end interval \"d1\" maximum range how far back bitcoin history"}
  → Results for "docs.coincap.io assets/{id}/history start end interval \"d1\" maximum range how far back bitcoin history": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required …
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `web_search` {"query":"developers.binance.com klines \"1000\" limit max candles per request interval 1d startTime how far back earliest data 2017"}
  → Results for "developers.binance.com klines \"1000\" limit max candles per request interval 1d startTime how far back earliest data 2017": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free …
- `web_search` {"query":"Binance spot REST API rate limits \"6000\" weight per minute \"1200\" request weight limit IP 2026 general info"}
  → Results for "Binance spot REST API rate limits \"6000\" weight per minute \"1200\" request weight limit IP 2026 general info": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key re…
- `web_search` {"query":"\"docs.coingecko.com\" market_chart range endpoint \"365\" free plan only last 365 days paid plans full history granularity automatic"}
  → Results for "\"docs.coingecko.com\" market_chart range endpoint \"365\" free plan only last 365 days paid plans full history granularity automatic": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a mon…
- `web_search` {"query":"CoinGecko API free demo plan 30 calls/minute 10000 calls/month x-cg-demo-api-key 2026 onchain vs public"}
  → Results for "CoinGecko API free demo plan 30 calls/minute 10000 calls/month x-cg-demo-api-key 2026 onchain vs public": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (…
- `web_search` {"query":"CoinCap API 3.0 \"credits\" cost per request rate limit per second free 2500 credits month explorer plan docs.coincap.io reference"}
  → Results for "CoinCap API 3.0 \"credits\" cost per request rate limit per second free 2500 credits month explorer plan docs.coincap.io reference": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month,…
- `web_search` {"query":"CoinCap history endpoint interval m5 h1 d1 start end milliseconds max range \"assets/{assetId}/history\" docs"}
  → Results for "CoinCap history endpoint interval m5 h1 d1 start end milliseconds max range \"assets/{assetId}/history\" docs": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key requ…
- `web_search` {"query":"Binance GET /api/v3/klines parameters limit default 500 max 1000 weight 1-10 depending interval docs developers.binance.com"}
  → Results for "Binance GET /api/v3/klines parameters limit default 500 max 1000 weight 1-10 depending interval docs developers.binance.com": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free…
- `web_search` {"query":"Binance API \"REQUEST_WEIGHT\" limit 6000 per minute \"RAW_REQUESTS\" 1200 per minute IP ban docs 2026"}
  → Results for "Binance API \"REQUEST_WEIGHT\" limit 6000 per minute \"RAW_REQUESTS\" 1200 per minute IP ban docs 2026": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x…
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints#klinecandlestick-data"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `fetch_page` {"url":"https://docs.coingecko.com/reference/coins-id-market-chart"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `web_search` {"query":"Binance klines endpoint documentation \"limit\" \"1000\" maximum 500 default candlestick weight interval 1d startTime endTime"}
  → Results for "Binance klines endpoint documentation \"limit\" \"1000\" maximum 500 default candlestick weight interval 1d startTime endTime": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a fr…
- `web_search` {"query":"CoinCap API pricing plans free 2500 credits Explorer Pro credits cost per endpoint history 1 credit websocket docs.coincap.io reference rate-limiting"}
  → Results for "CoinCap API pricing plans free 2500 credits Explorer Pro credits cost per endpoint history 1 credit websocket docs.coincap.io reference rate-limiting": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10…
- `web_search` {"query":"CoinGecko market_chart vs market_chart/range free plan 365 days limit granularity 5-minute 4-hour daily automatic paid full data since 2013 bitcoin"}
  → Results for "CoinGecko market_chart vs market_chart/range free plan 365 days limit granularity 5-minute 4-hour daily automatic paid full data since 2013 bitcoin": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,0…
- `web_search` {"query":"CoinCap API key required since 2025 free tier bearer token signup coincap.io/api-key 2500 credits per month rate limit requests per second"}
  → Results for "CoinCap API key required since 2025 free tier bearer token signup coincap.io/api-key 2500 credits per month rate limit requests per second": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls …
- `fetch_page` {"url":"https://docs.coincap.io/reference/assetsid_history"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://docs.coincap.io/reference/rate-limits"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api/rate-limits"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `fetch_page` {"url":"https://docs.coingecko.com/reference/coins-id-market-chart-range"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `web_search` {"query":"binance api v3 klines \"limit\" parameter \"If startTime and endTime are not sent\" maximum 1000 default 500"}
  → Results for "binance api v3 klines \"limit\" parameter \"If startTime and endTime are not sent\" maximum 1000 default 500": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key requi…
- `web_search` {"query":"CoinGecko \"market_chart\" granularity \"automatically\" 1 day data 5 minute 4 hours daily data more than 365 days free plan 365 days"}
  → Results for "CoinGecko \"market_chart\" granularity \"automatically\" 1 day data 5 minute 4 hours daily data more than 365 days free plan 365 days": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a mon…
- `bucket_put` {"content":"# Bitcoin price API comparison — CoinGecko vs CoinCap vs Binance\n\nResearched 2026-11-12. All facts from official documentation pages cited per row.\n\n## Comparison\n\n| | CoinGecko | CoinCap | Binance (Spot REST) |\n|---|---|---|---|\n| Free tier limits | Free \"Demo\" plan: 30 calls/min, 10,000 calls/month | Free tier: 2,500 credits/month | Public market data: weight limit 6,000/min per IP (no monthly cap stated) |\n| API key needed? | Yes — free Demo key required, sent as `x…
  → Stored notes/btc-price-apis-comparison.md: version 1, text/markdown, 2.3 KB. Link: jenab://01M7B7NA81Z1EM7XDSYGGHN8SB/notes/btc-price-apis-comparison.md
- `update_memory` {"content":"For daily BTC prices, Binance public API is the only one of CoinGecko/CoinCap/Binance that needs no key (6,000 weight/min per IP, daily klines to 2017, but USDT prices); CoinCap gives the deepest free history (daily to 2013) but needs a key and only 2,500 credits/month; CoinGecko free Demo key caps history at the last 365 days (30 calls/min, 10k calls/month).","expected_revision":0,"scope":"project","section":"decisions"}
  → Saved project memory, section decisions, revision 1.

**Reply:** Done. Comparison of the three Bitcoin price APIs (all facts from official docs, researched 2026-11-12):

| | CoinGecko | CoinCap | Binance (Spot REST) |
|---|---|---|---|
| **Free limits** | Demo plan: 30 calls/min, 10,000 calls/month | Free tier: 2,500 credits/month | 6,000 weight/min per IP, no monthly cap |
| **API key?** | Yes — free Demo key (`x-cg-demo-api-key`) | Yes — key (Bearer) required since 2025 | **No** — public market data needs no key |
| **History depth** | Daily BTC data exists back to 2013, but free plan returns only the **last 365 days** | Daily intervals back to **2013** for BTC (2015 ETH) | Daily klines (`1d`) back to **2017** for BTCUSDT |
| **Caveat** | Deeper history needs a paid plan | Credits consumed per request | Prices in **USDT**, not USD |

**Recommendation for this project:** `daily_btc` needs just one price a day, so all three fit easily. For the deepest free history (backfilling beyond a year), **CoinCap** is best despite the key requirement; if avoiding a key matters most, **Binance** works but gives USDT prices, which complicates the euro conversion in `weekly_summary`. CoinGecko's free tier is the most restrictive on history (365-day cap).

**Saved:**
- Notes: `notes/btc-price-apis-comparison.md` in the bucket (with per-fact source URLs)
- Project memory: one-line conclusion in the `decisions` section

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [notes/btc-price-apis-comparison.md].

Memory:

```
[decisions]
For daily BTC prices, Binance public API is the only one of CoinGecko/CoinCap/Binance that needs no key (6,000 weight/min per IP, daily klines to 2017, but USDT prices); CoinCap gives the deepest free history (daily to 2013) but needs a key and only 2,500 credits/month; CoinGecko free Demo key caps history at the last 365 days (30 calls/min, 10k calls/month).
```
