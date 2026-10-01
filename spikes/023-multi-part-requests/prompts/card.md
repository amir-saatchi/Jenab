Project card
Project: Crypto watch. Today is 2026-09-29; the time zone is Europe/Berlin; prices are stored in USD unless a run says otherwise.
Tables:
- prices(coin TEXT, date TEXT, open REAL, high REAL, low REAL, close REAL, currency TEXT), primary key (coin, date). BTC only so far: 60 rows, 2026-07-31 to 2026-09-28.
- news(url TEXT primary key, coin TEXT, title TEXT, source TEXT, published_at TEXT). 12 rows, BTC news.
Pipelines:
- daily_prices: fetches today's price and the latest news for one coin and writes them to prices and news. Inputs: coin (BTC or ETH, default BTC), currency (USD or EUR, default USD). Schedule: daily at 08:00. A run takes about 30 s.
- backfill_prices: fetches daily OHLC prices for a coin over past days. Inputs: coin (BTC or ETH), days (1-365), currency (USD or EUR). No schedule. A run takes about 2 min for 90 days.
- fear_greed: new, saved today at 11:40, not reviewed yet. Fetches the crypto fear and greed index and writes a one-line summary. Schedule: daily at 09:00.
Views: price_chart (line chart of close, 90 days), news_table (latest news).
Connections: coingecko (https://api.coingecko.com/api/v3), for GET /simple/price?ids=bitcoin,ethereum&vs_currencies=usd,eur&include_24hr_change=true.
