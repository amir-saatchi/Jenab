# TASK-061 — Switch to the native app and remove Wails
**Type:** Task
**Status:** In progress (proof passed; removal next)
**Gate:** 3
**Needs:** TASK-054 (not its item 3, the MyGo release with the harakat fix), TASK-055, TASK-056, TASK-057, TASK-058, TASK-059, TASK-062

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

### Proof
Part 1, scope item 1: **passed.** The acceptance runs pass, and the user's manual run of the native app passed on 2026-11-17. Nothing was removed or renamed yet; items 2 to 4 come next. Report: [reports/TASK-061-proof-report.md](reports/TASK-061-proof-report.md).

- **Acceptance runs (2026-11-16, main at `ae309c1`):** `scenarios/phase2-acceptance`, `P2_schema_memory`, `-reps 3`, as P2-14 ran it, on the two models the user picked.

  | Model | Passed | Requests a run | Time a run | Cost |
  |---|---|---|---|---|
  | Anthropic `claude-haiku-5-5` | 3/3 | 16 | 26–29 s | $0.015 for all 3 |
  | Gemini `gemini-3.5-flash-lite` | 3/3 | 15–18 | 39–149 s | free |

  - In every run the undo took out exactly turn 7's 3 rows, and turn 9 named only 2026-11-13.
  - No quota error; the reserve Gemini key wasn't needed.
  - Against P2-14: no run tried `sqlite_master` (P2-14: 7 of 9), which fits TASK-051. Haiku still called `describe_table` in turn 5 every time; flash-lite skipped it in 2 of 3. Flash-lite needed fewer requests (P2-14: 21–24). Nothing regressed.
- **Go test:** `TestPhase2Acceptance` passes.
- **Native release:** `wails3 task build:native` built `Jenab.exe` and `jenab-setup-0.1.0-dev-amd64.exe` into `bin/native/windows-amd64` (nothing downloaded). The bench build started on a scratch profile, loaded, showed the welcome screen and quit through the bench port, with no system dialog.
- **Manual run (2026-11-17):** passed. The user ran the report's steps with Haiku 5.5 on their own data folder, using the release built from main at `ae309c1` with `wails3 task build:native`. The user's words: "All passed". The screenshots stay with the user.
- **For item 2:** removing Wails removes `wails3 task`, which runs the Taskfile today. The tasks then need another runner.
