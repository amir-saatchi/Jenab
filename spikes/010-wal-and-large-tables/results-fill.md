modernc.org/sqlite v1.59.0 (SQLite 3.53.4), WAL, `synchronous = NORMAL`, 4 KB pages, default page cache (2 MB) unless noted. 12 CPUs, 2026-09-27.

## 6. Chunked inserts into a growing indexed table, 3000000 rows

500-row transactions into `trades`: rows arrive in rowid order, but the index on `(coin, date)` is written in random order. Rows per second in each million.

| Writer page cache | 0–1M | 1–2M | 2–3M | Total time |
|---|---|---|---|---|
| 2 MB | 14942 | 9282 | 6911 | 319.4 s |
| 64 MB | 18602 | 16767 | 15794 | 176.7 s |
