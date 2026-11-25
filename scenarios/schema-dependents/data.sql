-- P3-04: the Bitcoin project (SPEC 9.1) of views-and-pages, with a form
-- that edits a news item's title and reason too, run on each run's
-- project database. Synthetic prices and news. The configs are written
-- straight into _jenab_views and _jenab_pages, as the app saved them.
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
  ('https://news.example.test/etf-inflows', '2026-09-24', 'ETF inflows reach a monthly high', 'ETF', 'run-04', '2026-09-24T08:00:00Z'),
  ('https://news.example.test/miner-sales', '2026-09-26', 'Miners sell part of their reserves', 'miners', 'run-06', '2026-09-26T08:00:00Z'),
  ('https://news.example.test/rate-cut', '2026-09-28', 'Central bank hints at a rate cut', 'rates', 'run-08', '2026-09-28T08:00:00Z'),
  ('https://news.example.test/exchange-outage', '2026-09-29', 'Exchange outage delays withdrawals', 'outage', 'run-09', '2026-09-29T08:00:00Z');
INSERT INTO _jenab_views (id, type, title, config, revision, updated_at, updated_by) VALUES
('btc_price_chart', 'chart', 'BTC price', 'version: 1
id: btc_price_chart
title: BTC price
type: chart
chart_type: line
query: |
  SELECT date, price FROM btc_prices WHERE date >= :from_date
filters:
  - { param: from_date, control: date, label: "From", default: "today-30d", required: true }
x: { field: date, label: "Date", format: date }
y:
  - { field: price, label: "USD" }
', 1, '2026-09-30T09:00:00.000Z', 'app'),
('btc_last_price', 'stat', 'Last price', 'version: 1
id: btc_last_price
title: Last price
type: stat
query: |
  SELECT price, price - LAG(price) OVER (ORDER BY date) AS change
  FROM btc_prices ORDER BY date DESC LIMIT 1
value: { field: price, format: currency, format_options: { currency: USD, decimals: 0 } }
change: { field: change, format: number }
caption: "USD, last close"
', 1, '2026-09-30T09:00:00.000Z', 'app'),
('btc_news_list', 'table', 'News', 'version: 1
id: btc_news_list
title: News
type: table
query: |
  SELECT url, date, title, reason FROM btc_news WHERE date >= :from_date
filters:
  - { param: from_date, control: date, label: "From", default: "today-30d", required: true }
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: title, label: "Title", format: text }
  - { field: reason, label: "Reason", format: badge }
  - { field: url, label: "Link", format: url }
', 1, '2026-09-30T09:00:00.000Z', 'app'),
  ('edit_news', 'form', 'Edit news', 'version: 1
id: edit_news
title: Edit news
type: form
load: "SELECT title, reason FROM btc_news WHERE url = :key"
fields:
  - { field: title, label: "Title", control: text, required: true }
  - { field: reason, label: "Why it matters", control: text }
submit: { action: update, table: btc_news, key: url }
', 1, '2026-09-30T09:00:00.000Z', 'app');
INSERT INTO _jenab_pages (id, type, title, config, revision, updated_at, updated_by) VALUES
('bitcoin', 'page', 'Bitcoin', 'version: 1
id: bitcoin
title: Bitcoin
description: Price and news
filters:
  - { param: from_date, control: date, label: "From", default: "today-30d", required: true }
rows:
  - [ { view: btc_last_price, span: 4 }, { view: btc_price_chart } ]
  - [ { view: btc_news_list } ]
', 1, '2026-09-30T09:00:00.000Z', 'app');
