# TASK-062 — The native app's leftovers before the switch
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-055, TASK-056, TASK-057, TASK-059

## Goal
TASK-055 to TASK-059 left a few gaps in the native app, listed in their reports. Close them before TASK-061's proof, so the acceptance run and the user's manual run test the app as it will ship.

## Scope
1. **One settings frame:** project settings (`internal/native/projects`) still draws its own page frame. Use TASK-055's (`SettingsLayout` and the pieces beside it), so project settings and the app's settings look and move the same, as `settings-view.tsx` shows them (TASK-057 report).
2. **Right-to-left text in inputs:** a Persian name typed into the kit's `TextInput` is aligned left. The Wails input uses `dir="auto"`: the first strong letter decides. Do the same in the kit, for `TextInput` and `TextArea`, without mirroring the layout (TASK-057 report).
3. **The narrow layout, under 900 px:** the drawer and the overlays of SPEC 5.12 and `frontend/src/shell` (`dock.tsx` and the shell around it). The native frame has none yet (TASK-056 report).
4. **The composer in a narrow dock:** in a dock about 370 px wide, the composer's row of pickers is cut off at *Approvals: Standard*. It should fit or wrap as the Wails composer does (TASK-056 report).
5. **The Apps entry's version after an update:** it keeps the installed version, because MyGo's updater doesn't write it. Update it after an update, in Jenab or upstream in MyGo (TASK-059 report).

## Rules
- Screens import only the kit and the stores. Kit changes get headless tests in light and dark, for the keyboard, the label and right-to-left text.
- If a fix needs MyGo, check its latest release and its main branch first. If it is still missing, draft an issue for the user and don't file it; the user posts upstream.
- Live checks use a scratch profile and no real system dialogs.

## Done when
- Each item works as in the Wails app, or as its report asks, with headless tests and screenshots in light and dark, kept outside the repo unless a report needs them.
- A live check of items 1 to 4 at 1280 and at 800 px wide, and of item 5 with a local update.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done.
1. Project settings uses Settings' frame, `kit.SettingsLayout`, and has the project's Memory section, as the Wails app does. `kit/page.go` is gone.
2. `TextInput`, `TextArea` and the composer take their direction from the first strong letter. The layout isn't mirrored.
3. Below 900 px, the rail and the chat list open as an overlay from ☰. The right sidebar opens from *Pages*. A docked chat opens as a 448 px drawer over the file. Pages get a header with ☰, the settings list narrows to 176 px, and the bottom bar is hidden. The new kit pieces are `Narrow` and `Sheet`.
4. In a narrow dock, the model label is cut and the approval level shows only its icon, so the picker row fits in 320 px.
5. At each start on Windows, the app writes its version into the Apps entry, if the entry points at its own folder. MyGo's updater doesn't do this (checked in v0.9.1, v0.9.2 and main). Filed upstream as [egoist/mygo#1950](https://github.com/egoist/mygo/issues/1950) on 2026-11-17.

Each item has headless tests in light and dark. Items 1 to 4 were checked live with the bench build at 1280 and 800 px. A new bench route, `POST /file`, docks a file. Item 5 was checked live with a local update from 0.1.1 to 0.1.2: the Apps entry went from 0.1.1 to 0.1.2, and the uninstaller removed it.

Gaps:
- An overlay gives the focus to its first control as it opens, as MyGo's dialogs do.
- There is no activity dot on the chat button. The Wails app has none either.
- The chat row's ⋯ isn't always shown below 900 px.

`go build ./...`, `go vet ./...` and `go test ./...` pass. Report: [reports/TASK-062-report.md](reports/TASK-062-report.md).
