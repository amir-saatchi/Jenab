# TASK-059 — Updates and the installer for the native app
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-053

## Goal
The Wails app updates itself through Wails `pkg/updater` (SPEC 2.8, SPIKE-019) and ships with its installer. The native app needs both before it can replace the Wails app.

## Scope
1. **Updates:** MyGo's updater (`mygo.Updater`, signed updates). Check it against SPEC 2.8 and SPIKE-019's findings: the signature, the feed, swapping the running exe on Windows, and rolling back a failed update. Record what differs.
2. **The installer** for `cmd/desktop-native`: the icon, the version info, the Start menu entry, and the app id that notifications use, so a notification click opens Jenab.
3. **Moving from the Wails app:** the user's settings, projects and keys stay where they are. The first native version arrives as an update to the Wails app.

## Rules
- Windows 11 only; macOS and Linux are TASK-060.
- Ask before downloading any tool, and record its version and SHA-256.
- A test update runs against a local feed, never the public one.

## Done when
- An update from one local native build to the next works, and a broken update is refused.
- The installer installs and removes the native app, and a notification click opens it.
- An update from the Wails app to the native app keeps the user's data.

## Result
