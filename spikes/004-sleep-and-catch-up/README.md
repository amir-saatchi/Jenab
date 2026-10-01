# SPIKE-004 — Sleep, wake and catch-up (Windows)

Throwaway code for [SPIKE-004](../../Docs/Tickets/SPIKE-004-sleep-and-catch-up.md). It also covers the lock-screen row of [SPIKE-006](../../Docs/Tickets/SPIKE-006-keychain-background.md).

```powershell
go build -o sleepwatch.exe .
# start detached; job times are today, local, HH:MM or HH:MM:SS
Start-Process -WindowStyle Hidden -FilePath .\sleepwatch.exe -ArgumentList 'watch','--jobs','12:40,12:50,13:00'
# ... sleep the machine across a job time, wake it, sign in ...
New-Item stop                                      # graceful stop (or Ctrl+C when run in a console); wait ~3 s
.\sleepwatch.exe analyze sleep.jsonl > results.md
.\sleepwatch.exe cleanup                           # only if the process was killed: deletes the test credential
```

`watch` flags: `--jobs`, `--log` (default `sleep.jsonl` next to the exe; the `stop` file goes next to the log), `--for 3m` (stop after this much wall time).

- `main.go`: the modes; `watch` starts the probes (10 s samples, `time.Ticker(1 min)`, a `time.Sleep(1 min)` loop, `time.AfterFunc(10 min)`, one `time.Timer` per job), two schedulers (SPEC 6.2 with wake events, and tick-only), the keychain reads and the stop handling
- `win.go`: the clocks (`QueryInterruptTime`, `QueryUnbiasedInterruptTime`, `GetTickCount64`), the input desktop name, the hidden window for `WM_POWERBROADCAST`, power settings and WTS session events, and the `PowerRegisterSuspendResumeNotification` callback
- `log.go`: the JSON-lines record; every line carries all clocks and is flushed to disk
- `analyze.go`: turns a log into markdown tables (events, timeline, sleep intervals, what fired after each wake, jobs, keychain reads, clocks)
- `clock.go`: high-resolution timer (from SPIKE-005)
- `results-dryrun.md`: a 3-minute run without sleep, to check the pipeline

The test credential is `burrow-spike004:API_KEY` in Credential Manager; a clean stop deletes it.
