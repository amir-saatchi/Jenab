# TASK-081 — Pruning keeps what came after the chat list's read
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-072

## Goal
Open point 1 in TASK-072's report. TASK-072 made Ctrl+Tab's order keep a chat that came in after the chat list's read started (`Nav.Prune` gets the read's stamp). The other pruning after a chat list still doesn't, and can drop the state of a chat made since the list was read.

## Scope
After each fresh chat list, `Stores.listed` (`internal/native/state/stores.go`) prunes what is kept for the project's chats that the list doesn't have, except the chat shown:
- `Nav.Prune` gets the stamp and keeps a chat that came into its order after the read started.
- `Shell.Prune` (`shell.go`) doesn't: it drops `ChatHidden`, `Drawer` and `Open` (the dock) of every chat not listed.
- The `OnPrune` hooks don't either. The chat screen's `prune` (`internal/native/chat/screen.go`) drops its views, chips, *Refresh memory* cards and open parts.

So a chat made and left while the list was being read loses its docked file, its drawer, and what its screen kept, though it isn't deleted. Give `Shell.Prune` and the `OnPrune` hooks what they need to keep a chat that came in after the read started, as `Nav.Prune` does. That could be the stamp, or a set of chats to keep worked out once in `Stores.listed`. A chat that is really deleted is still pruned at the next list.

## Done when
- Tests: a chat made and left during a chat list's read keeps its docked file (`Shell`) and its screen's state (chat `prune`). Both fail on the old code. A deleted chat's state is still dropped.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done. `Nav.Keep` works out once, in `Stores.listed`, which chats a fresh list leaves: those it lists, the chat shown, and those that came in after the read started. `Threads.Gone`, `Nav.Prune`, `Shell.Prune` and the `OnPrune` hooks all prune by it. A chat made and left during the read keeps its thread, unsent draft, dock, drawer and screen state; a deleted chat loses them at the next list. The review added `Threads.Gone`, which lost the draft (`e7f8cac`).

Tests for the shell, the chat screen and the draft, each failing on the old code. Still open: a chat made during the read and pushed out of the 50-chat MRU before the list answers isn't kept. Report: [reports/TASK-081-report.md](reports/TASK-081-report.md).
