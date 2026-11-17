# TASK-067 report: crash log, and updates that wait for all work

**Base:** `main` at eb1031d, with MyGo v0.9.1 pinned.
**Model requests:** none. The live check used the bench's local fake model.
**Downloads:** none.
**Real data:** every live run used a scratch profile (`LOCALAPPDATA`, `APPDATA`, `USERPROFILE`, `TEMP`) made by `jenab-bench -hold -keep`, and the bench build keeps keys in memory. The user's own Jenab folders and keychain were not touched. No update was downloaded, no system dialog was opened, and *Open the log* was not clicked.

## What was built

### 1. Crash log

- **`internal/crash`** (new, standard library only):
  - `Start(dir)` sets `debug.SetTraceback("all")` and `debug.SetCrashOutput` to a new file, `logs/crash-<time>-<pid>.raw`. Go then writes every unrecovered panic, on any goroutine, and every fatal error there, with all goroutines' stacks, as well as to stderr. `stop` turns it off and removes the empty file.
  - `Collect(dir, current, redact)` turns the `.raw` files of earlier runs into `.log` files, passing the text through `redact`, removes empty ones (a run that was killed), skips this run's file, and keeps the newest 5 logs.
- **`cmd/desktop`:**
  - `run()` calls `crash.Start(paths.Logs)` right after the app lock, before the settings and the log.
  - Once the secrets exist, it calls `crash.Collect` with `crashRedactor`: every provider's key and base-URL fields, as `exportRedactor` in `internal/app` does. Each new log is also logged as a warning in `jenab.log`.
  - `crashNotice` makes a toast that stays until closed: "Jenab closed unexpectedly", the log's name and "Nothing was sent anywhere.", with *Open the log* (`mygo.Shell.OpenPath`). It is posted to the stores' toasts through the mailbox, so the state package is unchanged.
- **No top-level `recover`.** MyGo has none in its UI loop, and a `recover` in `main` would only cover the main goroutine. The crash output covers every goroutine. After a panic the app's state can't be trusted, so ending the app and keeping the stacks is the right thing.
- **A bug the live check found:** the first version deferred `stop`. A panic runs the defers of its goroutine before the runtime writes it, and MyGo runs posted closures on the goroutine that runs `run()`. So the deferred `stop` turned the crash output off first and removed the empty file: nothing was kept. Now `stop` runs at the end of a normal quit, after `lifecycle.Shutdown`, never in a defer. A start that fails before that leaves an empty `.raw`, which the next start removes. The test's child process has the same shape as `run()` (a defer, the loop on its own goroutine, `stop` at the end), and it fails with a deferred `stop`.
- **`kit.StickyToastAction`** (new) and a line in `shell.View`: `kit.ToastAction` ignored `Sticky` and `Warning`, so a toast with a button closed after the default few seconds. The crash notice is the first sticky toast with a button. Undo toasts are unchanged.

### 2. "Busy" means all work

- **`Orchestrator.Busy()`** (new): reports `o.busy > 0`. That counter already counts every runner (a turn, queued, running or waiting for the user's card, with its subagents, which run inside its tool calls, and its title request), every memory refresh, and every call such as Send or Undo. It is what `Orchestrator.Wait` waits for at shutdown.
- **`CanRestart`** in `main.go` is now `!appBusy() && window not in front`, with `appBusy = orch.Busy() || projects.Busy()`. `project.Manager.Busy` reports a project with leased work. `Stores.Busy` (chats only) is no longer used for the restart; it stays for the UI, and the state package (TASK-066's) is unchanged.
- **Pipeline runs and background tasks:** there is no scheduler, pipeline runner or background-task code yet (only `scheduler` settings exist). Their runs must join `appBusy` when they are built (Phase 3). SPEC 2.8 and CODE-OUTLINE say so.
- Checked live: with a project open and no turn, `busy` is false, so the UI holds no project lease while idle. During a reply it is true.

### 3. The updater

- **A failed re-check keeps the installed version.** Once a version is installed, `check` no longer shows *checking* and, on an error, logs it and keeps *ready* with the installed version and its notes, so Settings keeps *Restart now*. A failed install of a newer version does the same. The automatic restart still happens.
- **Switching off stops a download.** `install` runs `Install` on its own goroutine with a context it can cancel, and keeps reading requests. Switching to off cancels it. The app stays as it was, the status is *off* (or *ready*, if an earlier version is installed), no restart is offered, and the version is not marked as refused: turned on again, it is offered again. If the swap had already finished, it counts as installed. Checks, installs and restarts asked for during a download wait for its end.

### 4. Quit and a second start

- `lockApp` now waits up to `quitWait` (`lifecycle.ShutdownTimeout` + 5 s = 15 s) on a plain start too, as `--after-update` waits up to a minute. Once MyGo's single-instance check has passed, a copy that still holds `app.lock` is one that is quitting.
- `run()` logs "app: waited for a copy that was quitting" when the lock took longer than 200 ms.
- If the lock is still held after 15 s, the start says "Jenab is already running", as before.

### The bench build

`GET /state` adds `busy` (`appBusy`). `POST /panic?text=` posts a closure that panics with the text. DEVELOPMENT.md has both, and a note that only one bench app runs per Windows account (below).

## Tests

| Done when | Test |
|---|---|
| A panic in a posted closure writes the crash file | `internal/crash`: `TestAPanicInAPostedClosureWritesTheCrashFile` runs the test binary again as a child that starts the crash output, runs a UI loop and posts a closure that panics with a fake key. It checks exit status 2, one `.raw`, and after `Collect` one `.log` with the panic, the stacks and `[secret:provider:test]`, without the key. `TestStopRemovesTheEmptyFile`, `TestCollectKeepsTheLastFew`. |
| The next start shows the notice | `cmd/desktop`: `TestTheNextStartShowsTheCrash` (the real `secret.Store` redactor over a test keyring, the toast's text and *Open the log* opening the newest log). `internal/native/kit`: `TestStickyToastActionStays` (the notice stays a minute, an Undo toast doesn't; the button runs and closes it). |
| `CanRestart` is false while work goes on | `internal/agent`: `TestBusyWhileATurnOrARefreshRuns` (a hanging turn, then a hanging memory refresh). Pipeline runs and background tasks don't exist yet (above). |
| A failed re-check keeps the installed version | `internal/update`: `TestFailedRecheckKeepsTheInstalledVersion` (ready with 1.1.0 and its notes after the failed check, and the automatic restart still comes). |
| Switching off cancels a download | `internal/update`: `TestSwitchingOffStopsTheDownload` (the download's context ends, off, no restart, and turned on again the version is offered). |
| A second start during a quit | `cmd/desktop`: `TestASecondStartWaitsForAQuittingCopy`; `TestTheRestartedCopyLocksOnce` now shortens `quitWait`. |

`go build ./...`, `go vet ./...` (and `go vet -tags bench ./cmd/desktop`) and `go test ./...` pass.

## Live check

`jenab-bench -hold -keep -rate 2` with the bench build, on a scratch profile with the fake local model and a fake key in `JENAB_BENCH_KEY_BENCH`:

1. **Busy:** with the *Busy* chat open and idle, `busy` was false. Two seconds after `/send`, with the reply streaming, it was true.
2. **Second start:** `/quit` during the reply, then a second start at once. The quit took 40 ms, so the second copy met no lock. To test the wait, PowerShell held the same byte range of `app.lock` for 4 s while the app started: it came up after 4.7 s and logged "waited for a copy that was quitting" (3.7 s), with no "already running" box.
3. **Crash:** `POST /panic` with the fake key in the text. The app ended, and `logs` had `crash-…-8740.raw` (6.5 KB) with the panic, `goroutine 1 [running, locked to thread]` in the posted closure under MyGo's `mainLoop.drain`, and three more goroutines.
4. **Next start:** the `.raw` became `crash-…-8740.log`, whose first line reads `panic: bench: the key [secret:provider:bench] leaked`. The key isn't in it. `jenab.log` has "app: an earlier run crashed". After the kit fix, a screenshot 10 s after the start showed the warning toast at the bottom right: "Jenab closed unexpectedly", the log's name, and *Open the log*.
5. **Normal quit:** `/quit` left no `.raw`.

The first two crash runs found the deferred-`stop` bug and the toast that didn't stay; both are fixed above.

## Notes

- **One bench app per Windows account:** MyGo's single-instance check is per user, not per profile. While another session's bench build was running (TASK-068's, from its own scratch folder), my bench app handed its command line to that one and exited; that likely showed the other app's window once. I waited for it to quit before going on. DEVELOPMENT.md now says to check for a running `jenab-bench-app` first.
- **Follow-up:** `crashRedactor` repeats `exportRedactor`'s list of secret names. A shared function (in `provider` or `app`) would keep the two from drifting. I left `internal/app` alone, which TASK-069 is editing.
- Go writes stack traces with the build's source paths. `go tool mygo build` builds with `-trimpath`, so a release's crash logs hold module paths, not the builder's folders. A crash log stays on the user's machine anyway.
