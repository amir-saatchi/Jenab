# TASK-077 — The Connect card's refused retry, and a chip's read during its own undo
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-071

## Goal
Two Low findings from the review of TASK-071 to TASK-076 (2026-11-17), in what TASK-071 built.

## Scope
1. **A refused retry is swallowed** (`internal/native/chat/connect.go`, `sendKept`). It drops every `KindBusy` and `KindNotFound` answer. `KindBusy` also covers `ErrRefreshing` ("Memory is being refreshed") and `ErrNoRetry` (`internal/app/errors.go`). So if *Retry* is refused because memory is being refreshed, the card is already gone and the kept message is lost with no word. Drop only the answers that mean the message went or has nowhere to go: a turn in progress (`ErrInTurn`), nothing to retry because the turn went on, or a deleted chat. Show the rest, and keep the message sendable (the Retry card stays).
2. **A chip reads its state during its own undo** (`internal/native/chat/chips.go`). A chip asks again as soon as a newer *Undo turn* notice shows, even while its own *Undo* or *Redo* is on its way. That read can run before the undo lands, and its answer is taken, so the chip offers the wrong direction. While the chip's own undo runs, it doesn't ask; it asks again once the undo ends if a turn undo came meanwhile.

## Done when
- A test for each, failing on the old code.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done.
1. `app.UIError` keeps the known error it was made from (`Cause`, not sent to the UI), so the native app can tell apart refusals of one kind. `sendKept` drops only `ErrInTurn`, `ErrNoRetry` and a deleted chat; any other refusal, such as a memory refresh, shows as an error, and the Retry card still sends the message.
2. A chip doesn't read its state while its own *Undo* or *Redo* runs; once it ends, it reads again if a turn undo came meanwhile.

A test for each, failing on the old code. Report: [reports/TASK-077-report.md](reports/TASK-077-report.md).
