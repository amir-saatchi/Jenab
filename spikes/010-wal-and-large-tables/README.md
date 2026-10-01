# SPIKE-010 — WAL growth and large tables

Throwaway code for [SPIKE-010](../../Docs/Tickets/SPIKE-010-wal-and-large-tables.md).

```bash
go build -o wb.exe .
./wb.exe big 1000000,5000000 > results-big.md   # 4–5: table rebuild and other migration steps
./wb.exe ckpt > results-ckpt.md                 # 3: checkpoint modes on a 64 MB and 256 MB WAL
./wb.exe wal 60 10 > results-wal.md             # 1–2: W1 for 60 minutes, W2 and W3 for 10 minutes each
./wb.exe fill 3000000 > results-fill.md         # 6: 500-row inserts into a growing indexed table, 2 MB vs 64 MB cache
```

The full set takes about 2.5 hours; most of it is the 60-minute WAL run and filling the 5M-row table.

- `main.go`: connections, helpers
- `wal.go`: the WAL workload (writer, readers, a reader that holds a read transaction) and the checkpoint cases
- `big.go`: the table rebuild and the other migration steps
- `results-*.md`: output of the last run

Test databases go into `work/`, which is deleted at the end. SQLite writes its sort files to the folder named by `TMP`. The spike points `TMP` at a folder on C:, while the databases are on D:, so the drop in free space on C: is the temp space a migration used. Other programs writing to C: at the same time add noise to that number.
