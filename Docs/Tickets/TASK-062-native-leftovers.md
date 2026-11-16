# TASK-062 — The native app's leftovers before the switch
**Type:** Task
**Status:** Open
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
