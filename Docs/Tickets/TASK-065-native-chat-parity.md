# TASK-065 — The native chat's memory chips, *Refresh memory* and Connect card
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-061

## Goal
The review of the native app (2026-11-17) found features the React chat had and the native chat lost. No ticket deferred them.
- **Memory chip:** `Memory updated: goals [undo] [edit]` (SPEC 3.3) is now plain text. `internal/native/chat/rows.go` (`noticePart`) draws `NoticeMemoryUpdated` and `NoticeDesktopSaved` as a record with *View steps*.
- **Desktop chip:** its *Undo* is gone too.
- **Refresh memory** (SPEC 3.5): there is no button to start it. Its chip (`NoticeMemoryRefreshed`, "3 added, 1 changed · view changes · undo") is a one-line marker.
- **Connect card:** a chat with no provider has no card asking for one (SPEC 3.9). `cards.go` says "not built on MyGo yet".

The services exist (`ChatService.UndoMemory`, `RefreshMemory`, `UndoRefresh`, the desktop save's undo, `SettingsService.Connect`), and `logic/refresh.go` and `logic/provider.go` are ported, but nothing in the native UI calls them. The reference is the removed React code: `git show 490fbdb:frontend/src/chat/parts.tsx`, `refresh.tsx`, `chat-view.tsx`, `provider-card.tsx`.

## Scope
1. **Memory chip:** *Undo* and *Redo* as before, and *Edit*, which opens the memory section (`Stores.OpenMemory`). The line under it when it applies (SPEC 3.3).
2. **Desktop chip:** *Undo* and *Redo*.
3. **Refresh memory:** the button in the chat header with its estimate (`EstimateTitle`, `EstimateText`), and its chip with the edits, *View changes* and a batch *Undo*/*Redo*.
4. **Connect card:** a message sent with no provider is kept, and the card shows the provider list and key field from Settings (`settings/connect.go`), with a running Ollama first. Once connected, the kept message is sent.

## Rules
- The same text and behaviour as the React app unless SPEC says otherwise.
- Right-to-left text as elsewhere (5.8).

## Done when
- Headless tests for each: undo and redo on a memory chip, *Edit* opening the section, a desktop chip's undo, starting *Refresh memory* and undoing its batch, the Connect card sending the kept message.
- Tests the chat screen lacks today: *Undo turn*, the Retry card, a message sent during a turn, *Deny* with a note, an approval after *Stop*.
- Screenshots in light and dark, kept outside the repo.

## Result
Done. Report: [reports/TASK-065-report.md](reports/TASK-065-report.md).
1. The memory chip has *Undo*/*Redo*, *Edit*, which opens the memory section, and its note under it. It asks the backend whether it is undone, so it is right after a reload.
2. The desktop chip has *Undo*/*Redo* and *View steps*, and *Edit*, which opens Settings › Desktop.
3. *Refresh memory* is a bar icon button in the chat header, before *Chat menu*. Its menu has both refreshes, each with its estimate on a second line. A long one asks first with React's dialog. The refresh chip has *View changes* and a batch *Undo*/*Redo*.
4. With no provider set up, the failed turn shows the Connect card with Settings' `ConnectForm`, a running Ollama first. Connecting sends the kept message.

The chat screen also drops a deleted chat's state, through a new `Stores.OnPrune` hook. Each item has headless tests in light and dark, as do *Undo turn*, the Retry card, a message sent during a turn, *Deny* with a note and an approval after *Stop*. A live check on the bench build covered the header button, its estimates, the long-refresh dialog and a refresh run. Screenshots are outside the repo.
