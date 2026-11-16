# TASK-056 — The files panel, the file view and the dock in the native app
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-053

## Goal
Port the files panel, the file view and the dock (SPEC 5.9 and 5.12) to the native app, after SPIKE-048's go.

## Scope
1. **The files panel** (`files-panel.tsx`, `right-sidebar.tsx`) on MyGo's `Tree`.
2. **The file view** (`file-view.tsx`, `open-file.ts`): the kinds the Wails app shows, and opening the rest in the system app.
3. **The dock** (`dock.tsx`, `bottom-bar.tsx`) on MyGo's `Split`, with `state.Shell` for its size and what is open.
4. **References** that open a file (SPEC 4.5), as TASK-053 describes them for the native app.

## Rules
- Screens import only the kit and the stores. The tree and the split go through kit components with headless tests.
- Port the frontend's tests for these parts to Go.

## Done when
- The panel, the view and the dock work as in the Wails app, with headless tests and screenshots in light and dark.
- A live check with a project that has Markdown, Word and image files, and a Persian file name.

## Result
