# TASK-057 — Project screens in the native app
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-053

## Goal
Port the project screens from `frontend/src/shell` to the native app, after SPIKE-048's go.

## Scope
1. **New project** (`new-project.tsx`) and the **welcome page** (`welcome.tsx`).
2. **Project settings** (`project-settings.tsx`), with TASK-050's changes.
3. **The folder warning** (`folder-warning.tsx`) and the **private hosts** card (`private-hosts.tsx`).
4. **Exports:** a chat (`export-chat.tsx`) and a project (`export-project.tsx`, P2-12).
5. **The chat menu** (`chat-menu.tsx`) and what the rail and chat list still lack after SPIKE-048.

## Rules
- Screens import only the kit and the stores. New components go into the kit with headless tests.
- Port the frontend's tests for these screens to Go.

## Done when
- Each screen works as in the Wails app, with headless tests and screenshots in light and dark.
- A live check: create a project, change its settings, export it and a chat.

## Result
Done on 2026-11-16. Report: [TASK-057-report](reports/TASK-057-report.md).

- **Screens:** a new package, `internal/native/projects`, has the welcome page, *New project*, project settings, the folder warning, private-network exceptions, the project and chat exports, and the project and chat menus. The rail got *Search* (disabled) and the chats while the list is hidden; the chat rows got their ⋯ menu.
- **Stores:** `state.Hosts` and `state.Exports`, with the fake's services in `statetest`. The app's save dialog is `mygo.Dialog.Save`.
- **Kit:** 12 new files: `Field`, `CheckField`, `PageTitle`, `Section`, `DetailRow`, `SegmentGroup`, `Progress`, `ItemRow`, `Path`, `TextLink`, `ActionButton`, `DialogText`, `DialogFooter`, `Locked`, `NavRow`, `MenuTitle`, `TileSpinner` and five icons. Each has a headless test in light and dark.
- **Tests:** the frontend's tests for these screens are ported. 12 screen tests run in light and dark, and save 24 screenshots with `JENAB_SHOTS`. `go vet ./...` passes, and `go test ./...` passes except one timing test in `internal/scenario`, which passes alone.
- **Live check:** a project was created, its level and hosts changed, then it was exported as a zip and as a folder, and its Mother chat as Markdown, through the Windows save dialog.
- **Left over:**
  - Project settings has its own frame until TASK-055's is merged.
  - Right-to-left alignment in `TextInput` waits for a kit change.
