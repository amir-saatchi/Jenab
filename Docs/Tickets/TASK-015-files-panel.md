# TASK-015 — The Files panel: browse and open the project's files
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P1-05, P1-14

## Goal
The Files tab lets the user find, open and manage the project's files. Found while testing the app on 2026-10-13.

## Today
- Files lists only the bucket's top folder, and its rows do nothing when clicked (`frontend/src/shell/right-sidebar.tsx:90-115`).
- `cache/` shows like a user folder. It is the app's own cache: fetched pages and long tool results, deleted after 30 days (`internal/store/objects.go:59`).
- The SPEC says only that clicking a `jenab://` link "opens the object in the Files panel" (SPEC 4.2). It doesn't say what the panel does.

## Scope
First write it into SPEC 5.12 and 4.2, then build it:
1. **Folders** open in place, with a path back to the top.
2. **A file** opens in the main area, with the chat in the dock (5.9):
   - text and Markdown shown as in the chat
   - images shown
   - other types: name, type, size and *Open with the system app*
   - its versions (4.3)
3. **`cache/`** is hidden, or shown under a separate *App cache* entry, since the user didn't make it.
4. **Per file:** *Copy link* and *Delete*. A deleted file can still be restored within the undo window (4.4).
5. **`jenab://` links** in the chat open the file in the panel.

## Not in scope
- Upload by drag and drop, unless the SPEC adds it.
- Editing files in the app.

## Done when
- The SPEC describes the panel.
- Each item works in the app on Windows.
- Frontend tests cover folder navigation, hiding `cache/`, and opening a link.

## Result
Done on 2026-11-01. SPEC 4.5, 4.7 and 5.12 describe it.

- **Files tab:** one folder at a time with a path bar, 100 entries a page on scroll (checked with 2,500 files), `cache/` last as *App cache*, and each row's original name or title from its metadata, shown right to left when it is.
- **File view:** text, JSON and CSV (the first 1 MiB, with a note), Markdown as in the chat, images, audio and video; other types show name, type and size. Versions are listed, not opened.
- **Delete** is a change by `user`, with an *Undo* notice. `store.DeleteObjectChange` returns the change's ID; `DeleteObject` and the bucket tools are unchanged.
- **Open with the system app:** a read-only copy in `<temp>/jenab-open/<hash>/`, named with the detected type's extension and marked as downloaded (Zone.Identifier on Windows, quarantine on macOS). The allow-list is one function, `systemExt`, tested as a table with lying names (`evil.pdf` holding a program, `run.txt.exe`). Office files are recognised by their key's extension when detected as ZIP, until TASK-021 detects them properly; `.doc` and `.xls` are refused until then. Copies older than a day go when the app starts.
- **`jenab://` links** in chat Markdown, bare or in inline code, and a tool result's stored ref, open the file in the dock and show its folder. A link to another project or a missing file says so.
- **Dock:** one shared component for files now and pages and pipelines later: chat on the right, swap, hide, close, resize by drag or arrow keys, a drawer below 900 px. While something is docked, the chat list moves to the rail.
- **Open:** the dock's state lives in frontend memory; saving it to `chats.ui_state` comes with pages. The agent isn't told which file is open. *Open with the system app* and *Copy link* were tested in Go and unit tests, not clicked in the app.
- **Checked:** 99 frontend tests (folders and paging, `cache/`, links in Persian text, opening a link, the dock), Go tests for the service, store and allow-list, and every item by hand at 1280 and 760 px in a Wails server-mode build, since WebView2 156 didn't open its debugging port. Merged with TASK-016: one import conflict, then 102 frontend tests pass.
