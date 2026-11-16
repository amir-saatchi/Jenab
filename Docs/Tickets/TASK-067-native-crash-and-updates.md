# TASK-067 — Crash log, and updates that wait for all work
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-061

## Goal
The review of the native app (2026-11-17) found that a crash leaves no trace, and that the automatic update can cut off work.

## Scope
1. **Crash log:** `cmd/desktop/main.go` has no top-level recover and doesn't set `debug.SetCrashOutput`. The release build has no console, so a panic in a view or a posted closure ends the app with no message. Write crash output (the panic and the stacks) to a file in the app's log folder, keep the last few, and on the next start tell the user that Jenab closed unexpectedly, with *Open the log*. Secrets are redacted as in other logs (6.7). No crash report is sent anywhere.
2. **"Busy" means all work:** the update's `CanRestart` asks `stores.Busy()`, which counts only chats working or waiting. A pipeline run or a background task (8.3, 8.6) can be cut off after the 5 s shutdown wait. Count every running turn, background task, subagent and pipeline run (from the orchestrator and the scheduler, not the UI stores).
3. **The updater:**
   - A failed re-check after a version is installed shows "failed" and hides "restart to update" (`update.Service.check`). Keep the installed version shown.
   - Switching updates off waits for a download to finish and then offers the restart anyway. Make the download cancellable and stop it.
4. **Quit and a second start:** MyGo releases its single-instance socket before Jenab's shutdown has released `app.lock`. A second start during a slow quit shows "Jenab is already running". Have the second copy wait briefly for the lock, as `--after-update` does.

## Rules
- Nothing leaves the machine.

## Done when
- Tests: a panic in a posted closure writes the crash file; the next start shows the notice; `CanRestart` is false while a pipeline run or background task is going; a failed re-check keeps the installed version; switching off cancels a download.
