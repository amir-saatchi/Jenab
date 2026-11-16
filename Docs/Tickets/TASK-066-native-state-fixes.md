# TASK-066 — The native stores: lost updates and stale reads
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-061

## Goal
The review of the native app (2026-11-17) found ways the UI's stores can show the wrong state.

## Scope
1. **A failed re-read loses updates** (`internal/native/state/threads.go`, `read` and `readOnce`). When a read is already running, `read` calls `then(nil)` at once, before the new snapshot is in, so `Reload`'s "once it is applied" isn't true. If the running read then fails, the queued events are dropped and the extra read is skipped. A queued `Status{Idle}` can be lost, and the chat stays on "working" with *Stop* after it has finished. Fix: call `then` when the read it waited for is applied; on a failure, keep the queue and read again (with a limit), or mark the thread stale so the next event reads it.
2. **Stale answers win:** `Chats.Load`, `Chats.LoadWaiting`, `Hosts.Load` and `Settings.loadModels` have no sequence guard, so an older answer posted last replaces a newer one (the model picker or chat list shows data from before a change). Use a counter as `Pages.LoadUsage` does.
3. **A dropped edit:** `Settings.Edit` reports success (sends nil) and does nothing when settings aren't loaded yet, so `SetTheme` can be lost silently. Queue the edit or report it.
4. **A forgotten chat comes back:** `Threads.Forget`, `Gone` and the 20-chat eviction don't stop a read in flight, which writes the thread back, so the cap leaks.
5. **Off-thread writes and wrong targets:**
   - `internal/native/dev/page.go`: a goroutine sets `s.starting = false` off the UI thread. Post it.
   - `internal/native/chat/cards.go`: the rewind dialog keeps only the turn, so it can rewind another chat if the place changes while it is open. Keep the project and chat in the ask. The undo state reads (`undoTurn`) can land out of order; keep the newest.
6. **Small:** `Stores.Close` panics on a second call. `Nav.MRU` and the shell's per-chat maps keep deleted chats; prune them when a chat is gone.

## Rules
- Store fields are touched only on the UI thread (`post`), as now.

## Done when
- Tests: a failed re-read with a queued `Status{Idle}` ends idle; `Reload`'s callback comes after the snapshot is applied; two overlapping loads keep the newer answer; an edit before settings load isn't lost; a forgotten chat's read doesn't bring it back; the rewind dialog acts on its own chat.

## Result
Done. Report: [reports/TASK-066-report.md](reports/TASK-066-report.md).
1. A read that waits on a running read gets its callback once the next snapshot is applied. A failed re-read keeps its events and tries again twice. If every try fails, the events are applied to the thread as it is, so a turn's end still ends it, and the thread is read again at its next event.
2. The chat list, the waiting list, the private hosts and the model list keep the answer of the read started last.
3. An edit made before the settings are read is queued: the settings are read first, then the edit is saved on top. If that read fails, the edit is reported.
4. Forgetting a chat drops its read in flight, so the 20-chat limit and deleted chats hold.
5. The developer tools take their read's result on the UI thread. The rewind dialog acts on its own chat and doesn't open over another one. The undo state keeps the newest answer.
6. `Stores.Close` can be called twice. Deleted chats leave the Ctrl+Tab list and the shell's per-chat state.
7. `-race` couldn't run here: there is no C compiler.
