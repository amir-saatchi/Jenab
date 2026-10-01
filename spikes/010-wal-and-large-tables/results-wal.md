modernc.org/sqlite v1.59.0 (SQLite 3.53.4), WAL, `synchronous = NORMAL`, 4 KB pages, default page cache (2 MB) unless noted. 12 CPUs, 2026-09-27.

## 1–2. WAL size under constant use

Writer: every 2 s a 500-row pipeline chunk (150 corrections, 350 new rows) and every 0.5 s a chat turn that adds or edits a note, all with change log and before-images.
Readers: 4 goroutines running table pages, charts, 30-day KPIs and the notes list, each followed by a 50–150 ms pause. Automatic checkpoint at the default 1,000 pages.

| Scenario | Duration | Rows written | WAL max | WAL at end | Database at end | Write, 500-row chunk p50 / p99 / max | Chat write p50 / p99 / max | Read p50 / p99 / max |
|---|---|---|---|---|---|---|---|---|
| W1: 4 readers, 1 writer | 1h0m0s | 902663 | 7.8 MB | 7.8 MB | 237.5 MB | 78 ms / 537 ms / 906 ms | 541 µs / 69 ms / 357 ms | 2.46 ms / 28 ms / 301 ms |
| W2: W1 + a reader holding a read transaction for 60 s at a time | 10m0s | 151198 | 267.1 MB | 267.1 MB | 55.7 MB | 41 ms / 210 ms / 534 ms | 559 µs / 8.60 ms / 460 ms | 1.87 ms / 49 ms / 285 ms |
| W3: W2 + `journal_size_limit = 64 MB` | 10m0s | 150694 | 262.1 MB | 262.1 MB | 55.8 MB | 41 ms / 264 ms / 1851 ms | 646 µs / 9.38 ms / 192 ms | 1.69 ms / 42 ms / 433 ms |

WAL file size over time (highest value in each interval):

- **W1**, per 5m0s, MB: 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8
- **W2**, per 1m0s, MB: 81, 100, 114, 243, 251, 251, 251, 251, 251, 267 (10 holds of 60 s)
- **W3**, per 1m0s, MB: 81, 100, 114, 121, 130, 134, 143, 141, 143, 262 (10 holds of 60 s)

## 3a. `wal_checkpoint(TRUNCATE)` when the load stops (idle)

| Scenario | WAL before | Time | Result (busy, log, checkpointed) | WAL after |
|---|---|---|---|---|
| W1 | 7.8 MB | 11 ms | [0 0 0] | 0.0 MB |
| W2 | 267.1 MB | 339 ms | [0 0 0] | 0.0 MB |
| W3 | 262.1 MB | 1148 ms | [0 0 0] | 0.0 MB |
