# TASK-056 — The files panel, the file view and the dock in the native app
**Type:** Task
**Status:** Done
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

Done on 2026-11-16, on `main` at 1f581f4. Report: [reports/TASK-056-report.md](reports/TASK-056-report.md). No model requests, no downloads.

- **Reading files:** `app.NativeBucket.Read` reads a file's bytes in the same process. The stores (`state.Files`, `OpenFile`, `OpenLink`, `DeleteFile` with *Undo*, `OpenWithSystem`, `Bar`) are ported from `frontend/src/shell` and `lib/files.ts` with their tests.
- **The kit:** `Tree`, `Split`, `Tabs`, `PathBar`, `StatusBar`, `EndPanel`, `Picture`, `LinkText`/`LinkBubble`, `Item`, `ToastAction` and `CopyText`, each in its own file. Each has a headless test in light and dark, covering the keyboard, its label and right-to-left text.
- **The screens:**
  - `files.Screen` is the Files tab and the file view.
  - `shell/dock.go` is the dock, the right sidebar, the bottom bar and the project's chats in the rail.
  - In the chat, `jenab://` links open the file next to the chat: in answers as a link, bare or in inline code, and in the user's message when bare. "Stored as …" opens the stored output.
- **Tests:** `shell/files_test.go` covers the Files tab, the dock, the links, delete and undo, the Word file, the picture and the bottom bar, in light and dark. Its screenshots are in [reports/TASK-056/](reports/TASK-056/).
- **The live check:** a scratch project with made-up Markdown (a Persian file name), Word and PNG files, driven with UI Automation. A link opened the note in the dock, the picture and the Word file opened, and delete and undo worked.
- **Gaps:**
  - audio and video open with the system app (MyGo has no player);
  - MyGo names the divider "Divider";
  - the narrow layout under 900 px isn't done;
  - the composer's pickers are cut off in a narrow dock.
- **Docs:** SPEC 4.5 and 5.12, and CODE-OUTLINE section 10.
