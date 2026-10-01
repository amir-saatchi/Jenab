# SPIKE-008 — Crash safety

Throwaway code for [SPIKE-008](../../Docs/Tickets/SPIKE-008-crash-safety.md).

```bash
go build -o crash.exe .
./crash.exe 20 > results.md        # 20 hard kills per scenario; add a scenario ID (e.g. S3) to run one
```

The harness starts the same binary as a worker process, kills it at a random moment (TerminateProcess on Windows), then reopens the project, runs recovery and checks that everything is consistent. Test data goes into `work/`, which is deleted at the end.

- `main.go`: harness, kill loop, report
- `scenarios.go`: the eight scenarios (worker, recovery, checks)
- `results.md`: output of the last run

A process kill is not a power cut: the OS still writes out what it has buffered. Power loss is not tested here.
