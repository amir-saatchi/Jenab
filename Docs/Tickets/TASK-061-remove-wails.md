# TASK-061 — Switch to the native app and remove Wails
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-054, TASK-055, TASK-056, TASK-057, TASK-058, TASK-059

## Goal
When the native app has every screen, it becomes Jenab, and the Wails app, the React frontend and Bun leave the build.

## Scope
1. **Proof first:** the native app passes the Phase 2 acceptance scenarios on the reference models (as P2-14 ran them), and a manual run by the user, as the Wails app did.
2. **Remove:** `cmd/desktop`, `internal/webui`, `frontend/` (bindings included), the Wails dependency, and the Wails and Bun tasks in `Taskfile.yml`.
3. **Rename:** `cmd/desktop-native` becomes `cmd/desktop`. The bench keeps its native mode only.
4. **Docs:** `DEVELOPMENT.md`, `CODE-OUTLINE.md` and the SPEC lose the Wails and Bun parts that TASK-053 marked as going away.

## Rules
- One commit for the removal, after the proof, so it is easy to read and to revert.
- Nothing in the services changes.

## Done when
- The acceptance run and the user's manual run pass on the native app.
- `go build ./...` and `go test ./...` pass with no Wails, no frontend and no Bun.

## Result
