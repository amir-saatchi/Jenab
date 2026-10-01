# SPIKE-016 — Script sandbox for `script.starlark`

Throwaway code for [SPIKE-016](../../Docs/Tickets/SPIKE-016-starlark-sandbox.md).

```bash
go run . > results.md    # about 3.5 minutes; builds 6 small programs for the size table
```

- `engines.go`: the shared limits (step limit, stack depth, timeout), the `Engine` interface, and how errors are turned into a "stopped by" label
- `eng_starlark.go`, `eng_goja.go`, `eng_lua.go`, `eng_risor.go`, `eng_tengo.go`: one sandbox per candidate, with the best limits the library offers, plus `db.query`, `round`, `add_days` and `format_date`
- `cases.go`: the hostile scripts, the escape probes, the determinism script and the 10,000-row transform, written once per language
- `child.go`: hostile scripts run in a child process (this binary, `child <engine> <case> <mode>`) inside a Windows Job Object with a memory cap. The parent kills the job when Windows reports the cap was hit, or after a 10 s hard timeout
- `watchdog.go`: the in-process fallback: a goroutine that samples the Go heap every 1 ms and cancels the script above 256 MB
- `db.go`: the in-memory modernc database (10,000 rows) and the read-only `db.query`
- `report.go`: runs every section and prints markdown
- `sizes/`: tiny programs for the binary-size table
- `clock.go`: high-resolution timer (copied from SPIKE-005)
- `results.md`: output of the last run

Debug helpers: `go run . one <engine> <case> <mode>` runs one hostile script in a child (`mode` is `lib`, `watchdog` or `job`); `go run . section <name>` prints one section (`hostile`, `probe`, `db`, `transform`, `det`, `cost`).

Memory bombs never run without a cap: every hostile run is inside a Job Object (1 GB safety cap, or 256 MB in section 2b).
