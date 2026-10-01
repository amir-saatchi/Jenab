# SPIKE-007 — SQL guard without an authorizer

Throwaway code for [SPIKE-007](../../Docs/Tickets/SPIKE-007-sql-guard.md).

```bash
go run . modernc > results-modernc.md
go run . ncruces > results-ncruces.md
```

- `guard.go`: layer 1 (text check with a small SQLite lexer) and layer 2 (EXPLAIN check)
- `cases.go`: the 63 agent query cases
- `schema.go`: the schema guard for migrations and its 19 cases
- `main.go`: drivers, connection limits, the runner and the cost measurement
- `results-*.md`: output of the last run
