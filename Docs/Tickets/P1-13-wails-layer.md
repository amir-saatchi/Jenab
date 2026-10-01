# P1-13 — Wails layer
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-10, P1-11

## Goal
The thin `app` package between Go and React (CODE-OUTLINE 9, Q30–Q33).

## Scope
- **Services:** `ProjectService`, `ChatService`, `SettingsService`, `BucketService`, and `DevService` (bound only with the developer tools on).
- **Events:** registered with `RegisterEvent[T]` in the `init()` in `events.go`.
- **Errors:** `UIError` with `MarshalJSON`.
- **Publisher:** `publisher` implements the `Publisher` seams of `agent` and `project`.
- **Snapshots:** snapshot plus sequence (Q32).
- **Objects:** the handler for `/objects/<project_id>/<key>`.
- **Shutdown:** the Q30 order within 10 s.
- **Tooling:**
  - the Wails CLI, the Taskfile and `build/`
  - generated bindings in `frontend/bindings`
  - `wails3 dev` with hot reload
  - DEVELOPMENT.md updated

## Done when
- A Go test drives the services without Wails.
- CI regenerates the bindings and fails if they differ from the committed ones.
- The app quits within 10 s while a turn is running, and the next start needs no recovery.
