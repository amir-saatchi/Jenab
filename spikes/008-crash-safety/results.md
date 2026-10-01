# SPIKE-008 results

modernc.org/sqlite v1.59.0, WAL, `synchronous = NORMAL`. 20 hard kills per scenario (TerminateProcess on Windows). S3c crashes at a fixed point instead of a random one.

## S1 — chunked pipeline write (100k rows in 500-row transactions)

| Measure | Value |
|---|---|
| Killed during | chunk commit 3, chunk write 17 |
| Recovery actions | runs marked interrupted 20 |
| Open + quick_check + recovery | avg 291ms, max 646ms |
| project.db size at the end | 147.4 MB (with -wal) |
| Problems after recovery | 0 |

## S2 — migration with a table rebuild (200k rows)

| Measure | Value |
|---|---|
| Killed during | commit 5, copy 14, drop 1 |
| Open + quick_check + recovery | avg 55ms, max 94ms |
| project.db size at the end | 20.7 MB (with -wal) |
| Problems after recovery | 0 |

## S3 — bucket put of 50 MB files, with deletes

| Measure | Value |
|---|---|
| Killed during | fsync 17, stream to tmp 3 |
| Recovery actions | half-written tmp files removed 20, orphan object files swept 0 |
| Open + quick_check + recovery | avg 28ms, max 36ms |
| project.db size at the end | 0.1 MB (with -wal) |
| Problems after recovery | 0 |

## S3c — crash exactly after the rename into objects/, before the index row

| Measure | Value |
|---|---|
| Killed during | index 20 |
| Recovery actions | half-written tmp files removed 0, orphan object files swept 20 |
| Open + quick_check + recovery | avg 18ms, max 38ms |
| project.db size at the end | 0.1 MB (with -wal) |
| Problems after recovery | 0 |

## S4a — VACUUM INTO straight to the final file name

*Demonstration without the rule: problems are expected here.*

| Measure | Value |
|---|---|
| Killed during | vacuum into 20 |
| Recovery actions | snapshots with a hot -journal next to them 20 |
| Open + quick_check + recovery | avg 58ms, max 71ms |
| project.db size at the end | 16.3 MB (with -wal) |
| Problems after recovery | 20 |

- iteration 1: snapshot snap-1790525857709313500.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 2: snapshot snap-1790525859704507800.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 3: snapshot snap-1790525860877912300.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 4: snapshot snap-1790525862290313700.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 5: snapshot snap-1790525863711546100.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 6: snapshot snap-1790525865229659000.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 7: snapshot snap-1790525866745606300.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 8: snapshot snap-1790525867650026400.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 9: snapshot snap-1790525868415843000.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)
- iteration 10: snapshot snap-1790525870463414600.db (0.0 MB) is broken: SQL logic error: no such table: prices (1)

... and 10 more

## S4b — VACUUM INTO a .partial file, fsync, rename

| Measure | Value |
|---|---|
| Killed during | fsync 2, vacuum into 18 |
| Recovery actions | partial snapshots removed 20, their -journal files removed 18 |
| Open + quick_check + recovery | avg 58ms, max 72ms |
| project.db size at the end | 16.4 MB (with -wal) |
| Problems after recovery | 0 |

## S5a — chat message written to chats.db before the project change that refers to it

| Measure | Value |
|---|---|
| Killed during | message 9, project change 11 |
| Open + quick_check + recovery | avg 51ms, max 76ms |
| project.db size at the end | 17.4 MB (with -wal) |
| Problems after recovery | 0 |

## S5b — project change written before its chat message

*Demonstration without the rule: problems are expected here.*

| Measure | Value |
|---|---|
| Killed during | message 6, project change 14 |
| Open + quick_check + recovery | avg 21ms, max 35ms |
| project.db size at the end | 2.6 MB (with -wal) |
| Problems after recovery | 11 |

- iteration 1: 1 change-log entries point to a message that does not exist
- iteration 2: 1 change-log entries point to a message that does not exist
- iteration 5: 1 change-log entries point to a message that does not exist
- iteration 8: 1 change-log entries point to a message that does not exist
- iteration 11: 1 change-log entries point to a message that does not exist
- iteration 13: 1 change-log entries point to a message that does not exist
- iteration 14: 1 change-log entries point to a message that does not exist
- iteration 15: 1 change-log entries point to a message that does not exist
- iteration 17: 1 change-log entries point to a message that does not exist
- iteration 18: 1 change-log entries point to a message that does not exist

... and 1 more

## Summary

| Scenario | Problems after recovery | Result |
|---|---|---|
| S1 — chunked pipeline write (100k rows in 500-row transactions) | 0 | OK |
| S2 — migration with a table rebuild (200k rows) | 0 | OK |
| S3 — bucket put of 50 MB files, with deletes | 0 | OK |
| S3c — crash exactly after the rename into objects/, before the index row | 0 | OK |
| S4a — VACUUM INTO straight to the final file name | 20 | problems shown, as expected |
| S4b — VACUUM INTO a .partial file, fsync, rename | 0 | OK |
| S5a — chat message written to chats.db before the project change that refers to it | 0 | OK |
| S5b — project change written before its chat message | 11 | problems shown, as expected |
