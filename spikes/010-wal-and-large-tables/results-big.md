modernc.org/sqlite v1.59.0 (SQLite 3.53.4), WAL, `synchronous = NORMAL`, 4 KB pages, default page cache (2 MB) unless noted. 12 CPUs, 2026-09-27.

## 4. Table rebuild, 1000000 rows

Table `trades` (103.9 MB including its index, filled in 25.8 s). The rebuild changes the primary key from `id` to `(coin, date, seq)`: create the new table, copy, drop, rename, create an index, `foreign_key_check`, schema guard. Two readers query the table all the time.

| Variant | Copy | Drop, rename, index | Checks | Commit / rollback | **Writer held** | Temp space, peak | WAL file after | Reads during: p50 / p99 / max (count) | Read errors |
|---|---|---|---|---|---|---|---|---|---|
| old order, default cache, rolled back (try) | 5598 ms | 1328 ms | 0 µs | 5.79 ms | **6932 ms** | 35.5 MB | 120.8 MB | 0 µs / 3.25 ms / 14 ms (670) | 0 |
| new key order, default cache, rolled back (try) | 11.7 s | 1458 ms | 0 µs | 8.57 ms | **13.1 s** | 35.3 MB | 121.1 MB | 0 µs / 3.31 ms / 15 ms (1262) | 0 |
| old order, 64 MB cache, rolled back (try) | 3429 ms | 1032 ms | 0 µs | 43 ms | **4505 ms** | 36.4 MB | 66.4 MB | 0 µs / 5.23 ms / 24 ms (437) | 0 |
| new key order, 64 MB cache, rolled back (try) | 3681 ms | 890 ms | 0 µs | 36 ms | **4608 ms** | 36.4 MB | 66.8 MB | 0 µs / 731 µs / 2.17 ms (450) | 0 |
| old order, 64 MB cache, committed | 2329 ms | 754 ms | 0 µs | 807 ms | **3890 ms** | 36.4 MB | 122.5 MB | 0 µs / 903 µs / 1.52 ms (381) | 0 |

After the commit: WAL file 122.5 MB. `wal_checkpoint(TRUNCATE)` took 18 ms, result [0 0 0].

## 5. Other migration steps, 1000000 rows

Each step is its own transaction, committed. The WAL is truncated before each step. Time includes the commit and the automatic checkpoint that follows it.

| Step | SQL | Writer held | WAL file after |
|---|---|---|---|
| add_column, nullable | `ALTER TABLE trades ADD COLUMN fee REAL` | 3.83 ms | 0.0 MB |
| add_column, NOT NULL DEFAULT 0 | `ALTER TABLE trades ADD COLUMN flag INTEGER NOT NULL DEFAULT 0` | 1.09 ms | 0.0 MB |
| add_column with CHECK | `ALTER TABLE trades ADD COLUMN qty REAL CHECK (qty >= 0)` | 439 ms | 0.0 MB |
| rename_column | `ALTER TABLE trades RENAME COLUMN note TO comment` | 1.27 ms | 0.0 MB |
| create_index, one text column | `CREATE INDEX trades_source ON trades(source)` | 994 ms | 28.1 MB |
| create_index, two columns | `CREATE INDEX trades_coin_price ON trades(coin, price)` | 1440 ms | 18.0 MB |
| drop_index | `DROP INDEX trades_coin_price` | 21 ms | 0.0 MB |
| create_table + copy_data (daily averages) | `CREATE TABLE daily(coin TEXT, date TEXT, avg_price REAL, PRIMARY KEY (coin, date)); INSERT INTO daily SELECT coin, date, avg(price) FROM trades GROUP BY coin, date` | 6147 ms | 53.9 MB |
| drop_column | `ALTER TABLE trades DROP COLUMN comment` | 1263 ms | 77.0 MB |
| drop_table | `DROP TABLE daily` | 78 ms | 0.1 MB |

## 4. Table rebuild, 5000000 rows

Table `trades` (522.3 MB including its index, filled in 2088.1 s). The rebuild changes the primary key from `id` to `(coin, date, seq)`: create the new table, copy, drop, rename, create an index, `foreign_key_check`, schema guard. Two readers query the table all the time.

| Variant | Copy | Drop, rename, index | Checks | Commit / rollback | **Writer held** | Temp space, peak | WAL file after | Reads during: p50 / p99 / max (count) | Read errors |
|---|---|---|---|---|---|---|---|---|---|
| old order, default cache, rolled back (try) | 17.0 s | 4798 ms | 0 µs | 18 ms | **21.8 s** | 176.0 MB | 617.1 MB | 522 µs / 2.06 ms / 17 ms (2086) | 0 |
| new key order, default cache, rolled back (try) | 42.1 s | 5809 ms | 0 µs | 17 ms | **47.9 s** | 176.7 MB | 621.1 MB | 1.05 ms / 2.41 ms / 8.03 ms (4492) | 0 |
| old order, 64 MB cache, rolled back (try) | 15.3 s | 4941 ms | 0 µs | 59 ms | **20.3 s** | 226.0 MB | 562.8 MB | 523 µs / 2.12 ms / 31 ms (1939) | 0 |
| new key order, 64 MB cache, rolled back (try) | 42.1 s | 7012 ms | 0 µs | 153 ms | **49.3 s** | 227.6 MB | 566.8 MB | 533 µs / 2.34 ms / 1778 ms (4420) | 0 |
| old order, 64 MB cache, committed | 36.5 s | 14.8 s | 0 µs | 4677 ms | **56.0 s** | 226.2 MB | 618.8 MB | 3.23 ms / 245 ms / 1788 ms (2992) | 0 |

After the commit: WAL file 618.8 MB. `wal_checkpoint(TRUNCATE)` took 57 ms, result [0 0 0].

## 5. Other migration steps, 5000000 rows

Each step is its own transaction, committed. The WAL is truncated before each step. Time includes the commit and the automatic checkpoint that follows it.

| Step | SQL | Writer held | WAL file after |
|---|---|---|---|
| add_column, nullable | `ALTER TABLE trades ADD COLUMN fee REAL` | 1.02 ms | 0.0 MB |
| add_column, NOT NULL DEFAULT 0 | `ALTER TABLE trades ADD COLUMN flag INTEGER NOT NULL DEFAULT 0` | 1.03 ms | 0.0 MB |
| add_column with CHECK | `ALTER TABLE trades ADD COLUMN qty REAL CHECK (qty >= 0)` | 3501 ms | 0.0 MB |
| rename_column | `ALTER TABLE trades RENAME COLUMN note TO comment` | 1.12 ms | 0.0 MB |
| create_index, one text column | `CREATE INDEX trades_source ON trades(source)` | 5077 ms | 140.4 MB |
| create_index, two columns | `CREATE INDEX trades_coin_price ON trades(coin, price)` | 7057 ms | 90.3 MB |
| drop_index | `DROP INDEX trades_coin_price` | 91 ms | 0.1 MB |
| create_table + copy_data (daily averages) | `CREATE TABLE daily(coin TEXT, date TEXT, avg_price REAL, PRIMARY KEY (coin, date)); INSERT INTO daily SELECT coin, date, avg(price) FROM trades GROUP BY coin, date` | 54.2 s | 92.7 MB |
| drop_column | `ALTER TABLE trades DROP COLUMN comment` | 19.6 s | 391.2 MB |
| drop_table | `DROP TABLE daily` | 283 ms | 0.1 MB |
