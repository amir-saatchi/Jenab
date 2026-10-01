# SPIKE-006 — Keychain in background mode (Windows part)

Throwaway code for [SPIKE-006](../../Docs/Tickets/SPIKE-006-keychain-background.md).

```bash
go build -o kc.exe . && go build -ldflags -H=windowsgui -o kc-bg.exe . && go build -o other.exe ./other
./kc.exe run > results.md
./kc-bg.exe watch 30     # reads every 10 s for 30 minutes into watch.log; lock the screen during it
```

- `main.go`: the harness. It stores test secrets with `zalando/go-keyring`, reads them back, starts windowless children three ways, and deletes everything at the end.
- `session.go`: session ID and lock-screen state. The lock test is whether `LogonUI.exe` is running in the session; the WTS lock flag reads wrong on this Windows 11 build.
- `other/`: a separate program that reads a credential with `CredReadW` directly.
- `results.md`: output of the last run. `watch.log`: output of the watch.

Test entries use the service `burrow-spike006`. If a run is interrupted, `./kc.exe cleanup` removes them.

A read that needed a prompt would block, and the child would not report within its 20 s limit. All children reported within a few milliseconds, so no prompt was shown.
