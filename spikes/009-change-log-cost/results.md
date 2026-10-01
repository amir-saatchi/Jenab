# SPIKE-009 results

modernc.org/sqlite v1.59.0, WAL, `synchronous = NORMAL`. Before-images are stored per row in `_burrow_change_rows.before` as JSON.

# Layout A: WITHOUT ROWID, key (table_name, row_key, change_id)

## 1. Write cost: 5,000-row upsert in 500-row chunks, default page cache (2 MB)

Table `prices` with 100,000 rows. Median of 7 runs; the modes are interleaved.

| Rows that update existing rows | Plain upsert | + row keys | + row keys and before-images | Before-images vs plain |
|---|---|---|---|---|
| 0% | 26 ms | 27 ms | 34 ms | +29% |
| 50% | 103 ms | 191 ms | 250 ms | +143% |
| 100% | 158 ms | 556 ms | 722 ms | +358% |

## 1. Write cost: 5,000-row upsert in 500-row chunks, writer page cache 64 MB

Table `prices` with 100,000 rows. Median of 7 runs; the modes are interleaved.

| Rows that update existing rows | Plain upsert | + row keys | + row keys and before-images | Before-images vs plain |
|---|---|---|---|---|
| 0% | 31 ms | 30 ms | 41 ms | +29% |
| 50% | 88 ms | 253 ms | 359 ms | +306% |
| 100% | 163 ms | 565 ms | 859 ms | +428% |

## 2. Undo of that upsert (50% updates)

| Case | Conflict check | Undo | Rows restored correctly |
|---|---|---|---|
| Revert run, no conflicts | 7.88 ms (0 found) | 394 ms | yes |
| Redo (undo the undo), then undo again | — | 322 ms | yes |
| Revert run, 100 rows changed later (skipped) | 9.70 ms (100 found) | 402 ms | yes |
| Undo turn, 3 rows (median of 50) | 0.00 ms | 0.54 ms | yes |

## 3. Conflict check with 1000300 row entries in the change log

(Filling the history took 2.43s.)

| Case | Conflict check (median of 5) |
|---|---|
| Revert a 5,000-row run, no conflicts | 8.06 ms |
| Revert a 5,000-row run, 100 conflicts | 8.40 ms (100 found) |
| Undo a 3-row turn (average of 200) | 0.05 ms |

Size at this point:

| Table (with its indexes) | Size | Share |
|---|---|---|
| _burrow_change_rows | 117.6 MB | 73% |
| prices | 42.3 MB | 26% |
| _burrow_changes | 0.1 MB | 0% |
| **Change log total** | **117.7 MB** | **74%** |
| Database (used pages) / free pages | 160.0 MB / 0.0 MB | |

Change entries: 2077. Row entries: 1005403, of which 240352 still hold a before-image.

## 4. One simulated year: no cleanup

Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).
Simulated in 23.898s.

| Table (with its indexes) | Size | Share |
|---|---|---|
| _burrow_change_rows | 19.0 MB | 46% |
| prices | 17.6 MB | 42% |
| _burrow_changes | 2.9 MB | 7% |
| notes | 2.2 MB | 5% |
| **Change log total** | **21.8 MB** | **52%** |
| Database (used pages) / free pages | 41.7 MB / 0.0 MB | |

Change entries: 7665. Row entries: 194860, of which 19295 still hold a before-image.

## 4. One simulated year: after 90 days, drop before-images, keep row entries

Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).
Simulated in 37.162s. Daily cleanup: median 3.34 ms, max 29 ms.

| Table (with its indexes) | Size | Share |
|---|---|---|
| _burrow_change_rows | 18.2 MB | 44% |
| prices | 17.6 MB | 43% |
| _burrow_changes | 2.9 MB | 7% |
| notes | 2.2 MB | 5% |
| **Change log total** | **21.1 MB** | **51%** |
| Database (used pages) / free pages | 40.9 MB / 0.0 MB | |

Change entries: 7665. Row entries: 194860, of which 4823 still hold a before-image.

## 4. One simulated year: after 90 days, drop before-images and row entries, keep a summary on the change

Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).
Simulated in 1m47.273s. Daily cleanup: median 18 ms, max 2233 ms.

| Table (with its indexes) | Size | Share |
|---|---|---|
| prices | 17.6 MB | 64% |
| _burrow_change_rows | 4.9 MB | 18% |
| _burrow_changes | 2.9 MB | 10% |
| notes | 2.2 MB | 8% |
| **Change log total** | **7.7 MB** | **28%** |
| Database (used pages) / free pages | 27.6 MB / 0.0 MB | |

Change entries: 7665. Row entries: 48594, of which 4823 still hold a before-image.

# Layout B: rowid table + index (table_name, row_key, change_id)

## 1. Write cost: 5,000-row upsert in 500-row chunks, default page cache (2 MB)

Table `prices` with 100,000 rows. Median of 7 runs; the modes are interleaved.

| Rows that update existing rows | Plain upsert | + row keys | + row keys and before-images | Before-images vs plain |
|---|---|---|---|---|
| 0% | 21 ms | 31 ms | 57 ms | +175% |
| 50% | 151 ms | 231 ms | 274 ms | +82% |
| 100% | 288 ms | 641 ms | 770 ms | +167% |

## 1. Write cost: 5,000-row upsert in 500-row chunks, writer page cache 64 MB

Table `prices` with 100,000 rows. Median of 7 runs; the modes are interleaved.

| Rows that update existing rows | Plain upsert | + row keys | + row keys and before-images | Before-images vs plain |
|---|---|---|---|---|
| 0% | 23 ms | 34 ms | 62 ms | +171% |
| 50% | 269 ms | 703 ms | 772 ms | +187% |
| 100% | 222 ms | 611 ms | 744 ms | +235% |

## 2. Undo of that upsert (50% updates)

| Case | Conflict check | Undo | Rows restored correctly |
|---|---|---|---|
| Revert run, no conflicts | 8.35 ms (0 found) | 261 ms | yes |
| Redo (undo the undo), then undo again | — | 318 ms | yes |
| Revert run, 100 rows changed later (skipped) | 10 ms (100 found) | 288 ms | yes |
| Undo turn, 3 rows (median of 50) | 0.00 ms | 0.52 ms | yes |

## 3. Conflict check with 1000300 row entries in the change log

(Filling the history took 2.629s.)

| Case | Conflict check (median of 5) |
|---|---|
| Revert a 5,000-row run, no conflicts | 7.55 ms |
| Revert a 5,000-row run, 100 conflicts | 9.37 ms (100 found) |
| Undo a 3-row turn (average of 200) | 0.05 ms |

Size at this point:

| Table (with its indexes) | Size | Share |
|---|---|---|
| _burrow_change_rows | 127.0 MB | 75% |
| prices | 43.1 MB | 25% |
| _burrow_changes | 0.1 MB | 0% |
| **Change log total** | **127.2 MB** | **75%** |
| Database (used pages) / free pages | 170.3 MB / 0.0 MB | |

Change entries: 2077. Row entries: 1005403, of which 240352 still hold a before-image.

## 4. One simulated year: no cleanup

Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).
Simulated in 25.774s.

| Table (with its indexes) | Size | Share |
|---|---|---|
| _burrow_change_rows | 21.7 MB | 49% |
| prices | 17.6 MB | 40% |
| _burrow_changes | 2.9 MB | 6% |
| notes | 2.2 MB | 5% |
| **Change log total** | **24.6 MB** | **55%** |
| Database (used pages) / free pages | 44.5 MB / 0.0 MB | |

Change entries: 7665. Row entries: 194860, of which 19295 still hold a before-image.

## 4. One simulated year: after 90 days, drop before-images, keep row entries

Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).
Simulated in 31.513s. Daily cleanup: median 1.04 ms, max 77 ms.

| Table (with its indexes) | Size | Share |
|---|---|---|
| _burrow_change_rows | 21.7 MB | 49% |
| prices | 17.6 MB | 40% |
| _burrow_changes | 2.9 MB | 6% |
| notes | 2.2 MB | 5% |
| **Change log total** | **24.6 MB** | **55%** |
| Database (used pages) / free pages | 44.4 MB / 0.0 MB | |

Change entries: 7665. Row entries: 194860, of which 4823 still hold a before-image.

## 4. One simulated year: after 90 days, drop before-images and row entries, keep a summary on the change

Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).
Simulated in 1m3.528s. Daily cleanup: median 8.97 ms, max 81 ms.

| Table (with its indexes) | Size | Share |
|---|---|---|
| prices | 17.6 MB | 62% |
| _burrow_change_rows | 5.8 MB | 20% |
| _burrow_changes | 2.9 MB | 10% |
| notes | 2.2 MB | 8% |
| **Change log total** | **8.7 MB** | **30%** |
| Database (used pages) / free pages | 28.6 MB / 0.0 MB | |

Change entries: 7665. Row entries: 48594, of which 4823 still hold a before-image.
