-- P2-07: the Bitcoin project (SPEC 9.1) after 10 daily runs. Synthetic
-- prices and news. The prices add up to 616100.
CREATE TABLE btc_prices (
  date TEXT PRIMARY KEY NOT NULL,
  price REAL,
  _run_id TEXT,
  _fetched_at TEXT
);
CREATE TABLE btc_news (
  url TEXT PRIMARY KEY NOT NULL,
  date TEXT,
  title TEXT,
  reason TEXT,
  _run_id TEXT,
  _fetched_at TEXT
);
CREATE TABLE watch_keywords (
  id INTEGER PRIMARY KEY,
  keyword TEXT NOT NULL UNIQUE
);
INSERT INTO btc_prices (date, price, _run_id, _fetched_at) VALUES
  ('2026-09-21', 60120, 'run-01', '2026-09-21T08:00:00Z'),
  ('2026-09-22', 60480, 'run-02', '2026-09-22T08:00:00Z'),
  ('2026-09-23', 59870, 'run-03', '2026-09-23T08:00:00Z'),
  ('2026-09-24', 61040, 'run-04', '2026-09-24T08:00:00Z'),
  ('2026-09-25', 61590, 'run-05', '2026-09-25T08:00:00Z'),
  ('2026-09-26', 62210, 'run-06', '2026-09-26T08:00:00Z'),
  ('2026-09-27', 61880, 'run-07', '2026-09-27T08:00:00Z'),
  ('2026-09-28', 62740, 'run-08', '2026-09-28T08:00:00Z'),
  ('2026-09-29', 63150, 'run-09', '2026-09-29T08:00:00Z'),
  ('2026-09-30', 63020, 'run-10', '2026-09-30T08:00:00Z');
INSERT INTO btc_news (url, date, title, reason, _run_id, _fetched_at) VALUES
  ('https://news.example.test/etf-inflows', '2026-09-24', 'ETF inflows reach a monthly high', 'Steady ETF buying adds demand.', 'run-04', '2026-09-24T08:00:00Z'),
  ('https://news.example.test/miner-sales', '2026-09-26', 'Miners sell part of their reserves', 'Miner selling adds supply.', 'run-06', '2026-09-26T08:00:00Z'),
  ('https://news.example.test/rate-cut', '2026-09-28', 'Central bank hints at a rate cut', 'Lower rates favour risk assets.', 'run-08', '2026-09-28T08:00:00Z'),
  ('https://news.example.test/exchange-outage', '2026-09-29', 'Exchange outage delays withdrawals', 'Outages hurt confidence.', 'run-09', '2026-09-29T08:00:00Z');
INSERT INTO watch_keywords (id, keyword) VALUES (1, 'ETF'), (2, 'halving');
