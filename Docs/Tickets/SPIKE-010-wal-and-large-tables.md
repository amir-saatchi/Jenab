# SPIKE-010 — WAL growth and large tables
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Does the WAL file stay small under constant use, and how long do large migrations hold the writer (SPEC 7.2, 7.5)?

1. One hour of 4 readers and 1 writer: WAL size over time with the default `wal_autocheckpoint`
2. The same with one reader that holds a read transaction for 60 s: does the WAL keep growing?
3. `wal_checkpoint(PASSIVE)` vs `TRUNCATE` on idle: time and effect
4. Rebuilding a 1-million-row table (change primary key, copy data): how long is the writer blocked?
5. `ALTER TABLE ADD COLUMN` and `CREATE INDEX` on the same table: time

## Done when
- Checkpoint rules are chosen and written into SPEC 7.2
- Rebuild times are known; if a rebuild blocks the writer for more than 5 s, the spec gets a rule (for example, a progress notice and pausing background writes)

## Result
Run on 2026-09-27 with modernc.org/sqlite v1.59.0 (SQLite 3.53.4) on Windows 11 with 12 CPUs, one NVMe SSD (C: and D: are partitions of it), 4 KB pages, WAL and `synchronous = NORMAL`. All writes include the change log with before-images (SPIKE-009).
- **Code and full output:** [spikes/010-wal-and-large-tables](../../spikes/010-wal-and-large-tables/) (`results-*.md`).
- **Extra runs:**
  - tables of 5 million rows as well as 1 million
  - a 64 MB writer page cache as well as the 2 MB default
  - insert speed into a growing table (item 6)

| # | Question | Result |
|---|---|---|
| 1 | 1 hour, 4 readers + 1 writer | **The WAL stayed at 7.8 MB for the whole hour** (903,000 rows written). 500-row chunk p99 537 ms; chat write p99 69 ms; read p99 28 ms, max 301 ms. |
| 2 | A reader holding a read transaction for 60 s at a time | **The WAL grew to 267 MB in 10 minutes.** `journal_size_limit = 64 MB` did not help (262 MB), because the WAL can't restart while the reader holds its transaction. |
| 3 | PASSIVE vs TRUNCATE | See the checkpoint table below. |
| 4 | Rebuild, 1M and 5M rows | See the rebuild table below. **Over 5 s at 1M rows with the default cache**, and 20–56 s at 5M rows. Readers kept working: no errors; read p99 at most 5 ms, except 245 ms (max 1.8 s) during the 5M commit. |
| 5 | Other migration steps | See the migration steps table below. |
| 6 | 500-row inserts into a growing table with an index | With the 2 MB cache, speed falls from 14,900 to 6,900 rows/s by 3M rows. With a 64 MB cache: 18,600 to 15,800 rows/s. |

**Checkpoints (item 3)**

| WAL | PASSIVE | TRUNCATE | With a reader holding |
|---|---|---|---|
| 64 MB | 117 ms; the file stays 64 MB | 113 ms; the file becomes 0 | PASSIVE copies nothing. **TRUNCATE waits the full `busy_timeout` (5 s), then fails**, and the writer is blocked for those 5 s. |
| 256 MB | 251 ms; the file stays 257 MB. With `journal_size_limit = 64 MB`, the next write cuts it to 64 MB. | 303 ms; the file becomes 0 | same |

After load stops (idle), TRUNCATE took 11 ms to 1.1 s.

**Rebuild of `trades` (item 4)**

The rebuild changes the primary key and then runs: copy, drop, rename, index, `foreign_key_check` and the schema guard. Times are how long the writer is held.

| Variant | 1M rows (104 MB) | 5M rows (522 MB) |
|---|---|---|
| try, same key order, 2 MB cache | 6.9 s | 21.8 s |
| try, new key order, 2 MB cache | 13.1 s | 47.9 s |
| try, same key order, 64 MB cache | 4.5 s | 20.3 s |
| try, new key order, 64 MB cache | 4.6 s | 49.3 s |
| committed, same key order, 64 MB cache | 3.9 s | 56.0 s |
| Temp space, peak | 36 MB | 226 MB |
| WAL file after | 67–123 MB | 563–621 MB |
| TRUNCATE after commit | 18 ms | 57 ms |

**Other migration steps (item 5)**

| Step | 1M rows | 5M rows |
|---|---|---|
| `ADD COLUMN` (nullable or `NOT NULL DEFAULT`), `RENAME COLUMN` | ~1 ms | ~1 ms |
| `ADD COLUMN … CHECK` (scans the table) | 0.4 s | 3.5 s |
| `CREATE INDEX` | 1.0–1.4 s | 5.1–7.1 s |
| `DROP COLUMN` (rewrites the table) | 1.3 s | 19.6 s |
| `create_table` + `copy_data` with `GROUP BY` | 6.1 s | 54.2 s |
| `DROP INDEX`, `DROP TABLE` | 21–78 ms | 91–283 ms |

**Findings**
- **Short read transactions keep the WAL small.** The existing rule (no read transaction longer than the query timeout) is what matters. `journal_size_limit` only shrinks the file after a checkpoint that could finish.
- **TRUNCATE can block the writer.** When a reader holds a transaction, TRUNCATE waits `busy_timeout` and then fails. It needs a short timeout of its own.
- **Big migrations leave a big WAL,** about 1.2× the data they rewrote. TRUNCATE right after commit takes under 0.1 s.
- **Tries cost as much as real migrations,** because the copy is the expensive part. Rolling back is fast (under 0.2 s).
- **A 64 MB writer cache** makes a 1M-row rebuild up to 3× faster, and keeps chunked inserts about twice as fast past 2M rows. At 5M rows it barely helps.
- **Temp files go to the OS temp folder,** which is on C: by default and not the project drive. Peak use was about 0.4× the table size.
- **`copy_data` time depends on the query:** 6 s for 1M rows with `GROUP BY`.

**Limits:**
- One machine with a fast SSD. Slow disks will be slower.
- Temp space was measured as the drop in free space on C:, so other programs writing at the same time add noise.
- The 5M "committed" variant ran after four tries, with background checkpoints, so it is slower than the tries.

## Decision
**Decided (2026-09-27):**

SPEC 7.2:
- The automatic checkpoint stays at the default.
- The `project.db` writer also sets `journal_size_limit = 64 MB` and `cache_size = 64 MB`.
- TRUNCATE runs at idle, at close and after a long migration. It runs with `busy_timeout = 100 ms`; if it is busy, it is retried at the next idle.

SPEC 7.5, **long migrations:** a migration is long when its steps rewrite or scan more than 500,000 rows in total. Go estimates this from row counts.
- **Before:**
  - The approval card shows the estimated time.
  - Go checks free space: at least 2× the rewritten tables on the project drive, and 0.5× on the temp folder's drive.
- **During:**
  - The project shows a progress notice.
  - Reads continue.
  - Project writes wait in the queue: background runs pause at their next write step, and the time spent waiting doesn't count toward step timeouts.
  - Chat keeps working (`chats.db` has its own writer).
- **After:** TRUNCATE.
- **Try:** a long try runs on a `VACUUM INTO` copy on its own connection, so it doesn't hold the writer. The copy is deleted afterwards. *Not measured here: the time to make the copy.*
