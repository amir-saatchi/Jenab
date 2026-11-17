# TASK-072 — The native stores: the hosts' counter, the rewind dialog, Ctrl+Tab and the Dev page
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-066

## Goal
Findings from the review of TASK-064 to TASK-070 (2026-11-17), in what TASK-066 built.

## Scope
1. **A removed private host can come back on screen** (Medium). `Hosts.Change` and the list's reload share one counter (`internal/native/state/hosts.go:75`). If a reload starts during a save but reads the list before the save lands, the old list shows until the next load. The save itself works. A reload that read before a save landed doesn't replace the saved list.
2. **The rewind dialog can be built from an older plan** (Low). A dialog opened again, or for another turn, while an earlier `RewindState` is still loading can show the earlier answer (`internal/native/chat/cards.go`). The dialog shows only the answer for its own turn and opening.
3. **Ctrl+Tab may skip a new chat** (Low): the order it cycles through (`internal/native/state/nav.go`) can miss a chat made since it was last read.
4. **The Dev page retries on every frame** (Low). If loading fails, it starts again on each frame with a spinner and never shows the error (`internal/native/dev/dev.go`). Show the error with *Try again*, as Settings does.

## Done when
- A test for each.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done.
1. `Hosts` counts reads, changes started and changes running apart. A read made while a save runs never replaces the save's list.
2. A rewind dialog takes only the plan read for its own click; a late answer for an earlier click is dropped.
3. Ctrl+Tab's order keeps a chat that came in after the chat list's read started.
4. The Dev page keeps a failed read's error and shows it with *Try again* (the code is in `dev/page.go`).

A test for each. No live check: each fault needs answers in a set order, which the tests force. Open points (the screens' `OnPrune` hooks have the same window as Ctrl+Tab had) are in the report: [reports/TASK-072-report.md](reports/TASK-072-report.md).
