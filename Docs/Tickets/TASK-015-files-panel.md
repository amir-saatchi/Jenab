# TASK-015 — The Files panel: browse and open the project's files
**Type:** Task
**Status:** Open
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
