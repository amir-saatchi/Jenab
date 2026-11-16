# TASK-057 — Project screens in the native app
**Type:** Task
**Status:** Open
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
