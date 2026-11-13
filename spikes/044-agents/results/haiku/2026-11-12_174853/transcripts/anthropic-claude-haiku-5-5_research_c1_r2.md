# anthropic-claude-haiku-5-5_research_c1_r2

Researcher, 1 key, rep 2, 14 s.

**Task:** true (read the web true; notes on all three in "notes/bitcoin_price_apis.md"). Groups [web files memory], recall 1.00, precision 1.00, forbidden 0. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M782KXEAJMSAH4J9PQ4AHRGK · task

4 requests, 24896 prompt tokens, tools 2107 tokens, system 564 tokens.

**User:** Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. Their free limits, how far back the history goes, and whether they need a key. Save your notes.

- `web_search` {"query":"CoinGecko API free plan rate limit history API key required"}
  → Results for "CoinGecko API free plan rate limit history API key required": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API docu…
- `web_search` {"query":"CoinCap API v3 rate limit free API key history"}
  → Results for "CoinCap API v3 rate limit free API key history": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - CoinCap API documentation (ht…
- `web_search` {"query":"Binance public market data API klines rate limit weight API key required"}
  → Results for "Binance public market data API klines rate limit weight API key required": - CoinGecko API documentation (https://docs.coingecko.com/reference/introduction): CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Da… - Coi…
- `fetch_page` {"url":"https://docs.coingecko.com/reference/introduction"}
  → CoinGecko API. Free Demo plan: 30 calls a minute, 10,000 calls a month, a free Demo key required (x-cg-demo-api-key). Daily history back to 2013 for Bitcoin; the free plan returns at most the last 365 days of history. Simple price: GET /simple/price?ids=ethereum,bitcoin&vs_currencies=usd.
- `fetch_page` {"url":"https://docs.coincap.io/"}
  → CoinCap API 3.0. Free tier: 2,500 credits a month, an API key required since 2025. History: /assets/{id}/history with daily intervals back to 2013 for Bitcoin and 2015 for Ethereum. Price: GET /assets/ethereum.
- `fetch_page` {"url":"https://developers.binance.com/docs/binance-spot-api-docs/rest-api"}
  → Binance Spot REST API. Public market data needs no key; weight limit 6,000 a minute per IP. Daily candles (klines, interval 1d) back to 2017 for BTCUSDT and ETHUSDT. Price: GET /api/v3/ticker/price?symbol=ETHUSDT. Prices are in USDT, not USD.
- `bucket_put` {"key":"notes/bitcoin_price_apis.md","content":"# Bitcoin price APIs: CoinGecko vs CoinCap vs Binance\n\nResearched 2026-11-12. Sources: official docs pages listed per row.\n\n| | CoinGecko | CoinCap | Binance |\n|---|---|---|---|\n| Key needed | Yes, free Demo key (header `x-cg-demo-api-key`) | Yes, since 2025 | No, public market data needs no key |\n| Free limit | 30 calls/min, 10,000 calls/month | 2,500 credits/month | 6,000 weight/min per IP |\n| BTC history depth | Daily back to 2013, but f…
  → Stored notes/bitcoin_price_apis.md: version 1, text/markdown, 1.6 KB. Link: jenab://01M782KXA1E77RBXK8X7CFKQNP/notes/bitcoin_price_apis.md
- `update_memory` {"scope":"project","section":"decisions","content":"Bitcoin price API comparison (notes/bitcoin_price_apis.md): Binance needs no key and has daily BTC history from 2017 but quotes USDT; CoinGecko free needs a Demo key and returns only 365 days; CoinCap needs a key with 2,500 credits/month. Binance is the default pick for a keyless daily price feed.","expected_revision":0}
  → Saved project memory, section decisions, revision 1.

**Reply:** | | CoinGecko | CoinCap | Binance |
|---|---|---|---|
| Key needed | Yes, free Demo key | Yes, since 2025 | No |
| Free limit | 30 calls/min, 10,000/month | 2,500 credits/month | 6,000 weight/min per IP |
| BTC history | Daily to 2013, but free plan returns only the last 365 days | Daily to 2013 | Daily klines from 2017 (BTCUSDT) |
| Quote currency | USD (and EUR via vs_currencies) | USD | USDT, not USD |

Sources: [CoinGecko](https://docs.coingecko.com/reference/introduction), [CoinCap](https://docs.coincap.io/), [Binance](https://developers.binance.com/docs/binance-spot-api-docs/rest-api)

**Recommendation:** Use Binance for the daily feed. It needs no key and has the most generous rate limit, but it quotes in USDT and its history starts in 2017. If you need USD or EUR figures directly, use CoinGecko with a free Demo key and store history as you go, since the free plan only covers 365 days. I'd avoid CoinCap for now because its free tier needs a key and has the tightest monthly cap.

Notes saved to `notes/bitcoin_price_apis.md`. I also recorded the one-line conclusion in project memory.

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [notes/bitcoin_price_apis.md].

Memory:

```
[decisions]
Bitcoin price API comparison (notes/bitcoin_price_apis.md): Binance needs no key and has daily BTC history from 2017 but quotes USDT; CoinGecko free needs a Demo key and returns only 365 days; CoinCap needs a key with 2,500 credits/month. Binance is the default pick for a keyless daily price feed.
```
