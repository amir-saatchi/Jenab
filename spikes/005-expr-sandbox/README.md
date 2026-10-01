# SPIKE-005 — expr-lang sandbox

Throwaway code for [SPIKE-005](../../Docs/Tickets/SPIKE-005-expr-sandbox.md).

```bash
go run . > results.md
```

- `sandbox.go`: the candidate sandbox on top of `expr-lang/expr`: whitelist check, strict field access, text budget, watchdog
- `cases.go`: 65 cases, some also run with expr's defaults to show what the sandbox adds
- `main.go`: runs the cases and the per-item timing, prints markdown
- `clock.go`: high-resolution timer (Go's `time.Now` moves only every ~0.5 ms on this machine)
- `results.md`: output of the last run
