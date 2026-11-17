# TASK-073 — Updates during a download, and the crash log's raw file
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-067

## Goal
Findings from the review of TASK-064 to TASK-070 (2026-11-17), in what TASK-067 built. All Low.

## Scope
1. ***Restart now* is ignored during a download.** While a newer version downloads, a click on *Restart now* for the version already installed does nothing (`internal/update/update.go:296`: every request but a settings change waits for the download's end). It restarts into the installed version, or the button isn't offered while it can't act.
2. **Off and on again skips a check.** Turning updates off and back on during a download leaves the next check to the 6-hour timer. Turning them on again checks as a start does.
3. **The raw crash file stays.** If writing the redacted log fails, the unredacted `.raw` file, which may hold keys, stays on disk (`internal/crash/crash.go`). It's readable only by the user and retried at each start, but never cleaned up. After a few failed tries, or past an age, delete it and say the crash log couldn't be kept.

## Done when
- A test for each.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done.
1. *Restart now* during a download stops the download and restarts into the installed version, once the stopped download has returned.
2. Updates turned off and on again during a download check at once, as a start does.
3. `crash.Collect` counts failed tries next to the raw file. At the third, or at the first once the file is 7 days old, it deletes the raw file, and the start shows a sticky notice that the crash log couldn't be kept.

A test for each. SPEC 2.1 and 2.8 have the new rules. Report: [reports/TASK-073-report.md](reports/TASK-073-report.md).
