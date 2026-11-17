# TASK-070 — Remove the code and text the Wails app left behind
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-061, TASK-069

## Goal
TASK-061 removed the Wails app but, by its rule, left the services unchanged. Some code in them only ever served Wails' web view, and nothing in the native app calls it now. TASK-069 fixed the comments and listed the code ([its report](reports/TASK-069-report.md), "Not done, and notes"). The user chose (2026-11-17) to remove it. Phase 3 pages are native too (SPEC 5), so nothing needs files served over HTTP.

## Scope
1. **Serving files over HTTP:** `app.ObjectHandler` (`internal/app/bucket.go`) and `bucket.Handler`, `bucket.Route` and their types (`internal/bucket/serve.go`), with their tests. The app reads files in its own process (`app.NativeBucket`). Keep what `NativeBucket` and the file view still use, such as the type detection and the checks on what may be shown. Move any of those checks' tests that only went through the handler, so they still cover the same cases.
2. **Errors as JSON:** `UIError.MarshalJSON` (`internal/app/errors.go`) and its test, if nothing in the app marshals errors now. The exports and the logs must not depend on it; check them first.
3. **A sweep of `internal/app`, `internal/bucket` and `internal/project`** for other things only the Wails bindings or events used: routes, event names and payload shapes, frontend-only DTOs and helpers. Remove what nothing outside its own tests calls. List each in the report, with what was kept and why. Keep JSON tags that files on disk or exports use.
4. **The skills' text:** the twelve `.claude/skills/golang-*/SKILL.md` files say Jenab is "a Go + Wails v3 desktop app". Make it "a Go desktop app with a native UI on MyGo". Change nothing else in them. The `shadcn` skill stays, since `mockups/` still uses it.
5. **Docs:** CODE-OUTLINE and SPEC lose anything that describes the removed code as current. Decision records and history (QUESTIONS, the Gates, PROPOSAL's roadmap, SPEC's notes on the move from Wails, PORTFOLIO) stay as they are.

## Rules
- Nothing the native app does changes: no change in behaviour, only removal of code that nothing outside tests calls.
- No new tools or dependencies. Use `go build`, `go vet`, grep and the compiler to find callers. If a dead-code tool would help, ask Main first, since it is a download.
- The removal is one commit, so it is easy to read and to revert.

## Done when
- The report lists everything removed and everything kept, with the reason.
- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass.
- The bench build still starts on a scratch profile, opens a file in the file view and quits through the bench port.
