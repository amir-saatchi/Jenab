# SPIKE-001 — SQLite driver capabilities

Throwaway code for [SPIKE-001](../../Docs/Tickets/SPIKE-001-sqlite-driver.md).

```bash
go run . modernc
go run . ncruces
```

Each run creates a temporary database, runs every check, prints a markdown table and deletes the database.

- `main.go`: checks shared by both drivers, plus the driver-specific authorizer and column-info checks
- `followup.go`: EXPLAIN-based table detection and the snapshot test
- `mem_windows.go`: private-memory measurement (Windows only)
- `cmd/modernc-guard/`: quick check of the SQL guards on modernc, a first step for SPIKE-007 (`go run ./cmd/modernc-guard`)
