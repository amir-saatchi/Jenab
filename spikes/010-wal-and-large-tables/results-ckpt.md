modernc.org/sqlite v1.59.0 (SQLite 3.53.4), WAL, `synchronous = NORMAL`, 4 KB pages, default page cache (2 MB) unless noted. 12 CPUs, 2026-09-27.

## 3. Checkpoint modes on a large WAL

Automatic checkpoints are turned off while the WAL is filled with 500-row updates of prices. "Reader holding" means a read transaction was opened before the WAL was filled and is still open.

| WAL | Case | Time | Result (busy, log, checkpointed) | WAL file after |
|---|---|---|---|---|
| 64.0 MB | PASSIVE, no reader | 117 ms | [0 16345 16345] | 64.2 MB |
| 64.0 MB | … then one small write | — | — | 64.2 MB |
| 64.0 MB | PASSIVE with `journal_size_limit = 64 MB`, then one small write | — | — | 64.0 MB |
| 64.0 MB | TRUNCATE, no reader | 113 ms | [0 0 0] | 0.0 MB |
| 64.0 MB | PASSIVE, reader holding | 782 µs | [0 16392 0] | 64.4 MB |
| 64.0 MB | TRUNCATE, reader holding (waits for `busy_timeout`) | 5025 ms | [1 16392 0] | 64.4 MB |
| 64.0 MB | a new read while the WAL is full | 1.54 ms | — | — |
| 64.0 MB | TRUNCATE after the reader closed | 145 ms | [0 0 0] | 0.0 MB |
| 256.0 MB | PASSIVE, no reader | 251 ms | [0 65386 65386] | 256.9 MB |
| 256.0 MB | … then one small write | — | — | 256.9 MB |
| 256.0 MB | PASSIVE with `journal_size_limit = 64 MB`, then one small write | — | — | 64.0 MB |
| 256.0 MB | TRUNCATE, no reader | 303 ms | [0 0 0] | 0.0 MB |
| 256.0 MB | PASSIVE, reader holding | 6.74 ms | [0 65352 0] | 256.8 MB |
| 256.0 MB | TRUNCATE, reader holding (waits for `busy_timeout`) | 5032 ms | [1 65352 0] | 256.8 MB |
| 256.0 MB | a new read while the WAL is full | 1.08 ms | — | — |
| 256.0 MB | TRUNCATE after the reader closed | 303 ms | [0 0 0] | 0.0 MB |
