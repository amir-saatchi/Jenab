# SPIKE-009 — Change log and undo cost

Throwaway code for [SPIKE-009](../../Docs/Tickets/SPIKE-009-change-log-cost.md).

```bash
go build -o cl.exe .
./cl.exe > results.md        # about 6 minutes
```

For each `_burrow_change_rows` layout (A: `WITHOUT ROWID`, B: rowid table), it measures:
1. write cost of a 5,000-row upsert: plain, with row keys, and with row keys and before-images
2. undo of that upsert: revert, redo, and revert with skipped conflicts, checked against a snapshot of the table
3. the conflict check with 1 million row entries in the log
4. database size after one simulated year, with three cleanup rules

Files:
- `changelog.go`: schema, change-log writes, conflict check, undo
- `main.go`: benchmarks and report
- `results.md`: output of the last run

Test data goes into `work/`, which is deleted at the end.
