# TASK-059 — Updates and the installer for the native app
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-053

## Goal
The Wails app updates itself through Wails `pkg/updater` (SPEC 2.8, SPIKE-019) and ships with its installer. The native app needs both before it can replace the Wails app.

## Scope
1. **Updates:** MyGo's updater (`mygo.Updater`, signed updates). Check it against SPEC 2.8 and SPIKE-019's findings: the signature, the feed, swapping the running exe on Windows, and rolling back a failed update. Record what differs.
2. **The installer** for `cmd/desktop-native`: the icon, the version info, the Start menu entry, and the app id that notifications use, so a notification click opens Jenab.
3. **A `build:native` task** in `Taskfile.yml` for the release build of `cmd/desktop-native` (found in TASK-053): the `-H windowsgui` and version flags that `DEVELOPMENT.md` now gives by hand, the icon and version info, and the installer step. `DEVELOPMENT.md` then points to the task.
4. **Moving from the Wails app:** the user's settings, projects and keys stay where they are. The first native version arrives as an update to the Wails app.

## Rules
- Windows 11 only; macOS and Linux are TASK-060.
- Ask before downloading any tool, and record its version and SHA-256.
- A test update runs against a local feed, never the public one.

## Done when
- An update from one local native build to the next works, and a broken update is refused.
- The installer installs and removes the native app, and a notification click opens it.
- `task build:native` builds the release exe and the installer.
- An update from the Wails app to the native app keeps the user's data.

## Result
Done. The native app updates itself with MyGo's signed updater (`mygo.Updater`) behind the new `internal/update`, which keeps Jenab's rules from SPEC 2.8: checks at start and every 6 hours, automatic, notify or off, the beta channel, a refused version not tried again on its own, and a restart only when no chat is busy and the window isn't in front. Settings › General has an *Updates* row. `wails3 task build:native` runs `go tool mygo build` from the new `mygo.json` and makes `Jenab.exe` (`-trimpath`, `-H windowsgui`, icon, version info, version flags) and a per-user NSIS installer with a Start menu entry and an Apps entry under the publisher "Jenab". NSIS 3.13 was downloaded once with the user's yes; its version and hashes are in the report. Notifications are tray balloons, so a click needs no app id; it shows the window. Live tests on a scratch profile and a local feed: a wrong key, a flipped bit, a short archive, a manifest without signature and a beta on stable were refused with the files unchanged; a swap that failed half-way was undone; 0.1.1 updated to 0.1.2 and restarted in under a second; automatic mode updated and restarted by itself; the installer installed, upgraded and uninstalled, keeping the data. The run found a lock bug in the restarted copy, fixed with a test. The native installer over the Wails exe kept a project, its chat and the settings. Not checked by hand: a click on a real toast (none showed on this PC; the click message was sent as the shell sends it). Gaps: a new version that crashes at start is not rolled back, and the Apps entry keeps the installed version after an update. `mygo.json` has no update key yet, so release builds can't update themselves until Phase 5. `go build ./...`, `go vet ./...` and `go test ./...` pass. Report: [reports/TASK-059-report.md](reports/TASK-059-report.md).
