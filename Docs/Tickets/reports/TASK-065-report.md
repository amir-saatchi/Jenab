# TASK-065 report: the native chat's memory chips, *Refresh memory* and Connect card

**Base:** `main` at 2109f28, with MyGo v0.9.1 pinned.
**Model requests:** none paid. The tests use `statetest`'s fake services. The live check used the bench build's local fake model.
**Keys:** none were used or read.
**Real data:** the live check used a scratch profile outside the repo. No system dialogs were opened.

The reference was the removed React code (`git show 490fbdb:frontend/src/chat/parts.tsx`, `refresh.tsx`, `chat-view.tsx`, `provider-card.tsx`). The texts are React's.

## 1. Memory chip

`chips.go`'s `memoryChip` draws "Memory updated: preferences" as a record with *Undo* and *Edit*, and its note under it when the notice has one ("From a chat that read web pages", SPEC 3.3).
- The chip asks the backend once whether its change is undone (`MemoryUndone`), so it is right after a reload or after its turn was undone. An undone chip says "· undone" and offers *Redo*.
- *Undo* and *Redo* call `UndoMemory`. One that worked says nothing; one that found the section changed since says so in a toast (`logic.MemoryUndoText`, as React's).
- *Edit* calls `Stores.OpenMemory`: project memory opens the project's settings at *Memory* with that section in focus, user memory the app's settings.
- The button is off while its request runs.

## 2. Desktop chip

`desktopChip` draws "Macro saved: site_search (6 steps)" with *Undo*/*Redo* (`UndoDesktopSave`, `DesktopSaveUndone`) and *View steps* (or *View text* for a recipe) under the row. It also has *Edit*, which opens Settings › Desktop (`Stores.OpenDesktop`, new), as the memory chip does. React's chip had no *Edit*; it costs one button and saves looking for the macro.

## 3. Refresh memory

**The button.** The chat header has a bar icon button, *Refresh memory* (brain icon, square, the bar's size), before *Chat menu*. It opens a menu of the two refreshes, "Refresh memory" and "Full refresh: review the whole chat again", each with its estimate as the item's second line: "12 turns · 3 calls · about 8,000 tokens", or "Nothing new to review" (`logic.EstimateLine`).
- The estimate is asked each time the menu opens, with "Counting the turns" until it answers. A late answer for an earlier opening is dropped.
- The button is off while the chat works, while a refresh runs, and for a chat with no thread yet. It is hidden for a project open read-only after a failed check, since a refresh writes memory.
- This replaces React's split button with one button and a menu, so the bar stays one line (the brief asked for this).

The header is the shell's: `shell.ChatHeaderActions` is a new interface the chat screen implements, and `header()` draws its actions after the spacer. The kit has a new `BarMenuButton` (a `BarIconButton` with a popover menu below its end, which reports whether it is open).

**Starting it.** Choosing an item asks for the estimate. A long one (`Long`) asks first, with React's dialog: "Refresh memory: 100 turns?" and "This makes 21 calls to bench-model, with about 48,552 tokens of input. … Memory changes only when it ends, and Stop cancels it." (`EstimateTitle`, `EstimateText`), with *Cancel* and *Refresh*. A short one starts at once. The dialog is dropped when another chat shows.

**The chip.** A refresh batch is a user message of notices. `refreshChip` draws "Memory refreshed: 3 added, 1 changed" with *Undo*/*Redo* for the whole batch (`UndoRefresh`, `RefreshUndone`), its note, and *View changes*, which shows each section before and after under its title ("PROJECT MEMORY · GOALS"). An undo that found sections changed since undid nothing and names them in a toast (`UndoChangedText`).

## 4. Connect card

A turn that failed because no provider is set up leaves its message kept, as before. When the thread's notice is a failed turn, the chat is idle and Settings has no provider, the Retry card's place shows the Connect card (`connect.go`):
- Title "Notes needs an AI model to answer" (the chat's title, or "This chat"), then "Add a provider with your own key. Your message is kept and sent once it is connected."
- Under it, Settings' `ConnectForm`, which lists the presets with a running Ollama first and shows the key field for the one chosen.

The embedding follows TASK-068's steps: a new form for each card; `View` in each frame it shows; `Clear` and drop when the card goes (another chat, the turn going on, a provider connected, or the chat deleted); and `Connected` acts only if its form is still the one shown. It then calls `Retry`, which sends the kept message, with the Retry card's busy flag.

The form is kept while the user goes elsewhere and comes back to the same chat, so a half-typed key isn't lost by a look at another tab; it is cleared when another chat shows.

## Other changes

- **Pruning.** `Stores` has a new `OnPrune` hook, called where the stores already drop deleted chats. The chat screen uses it to delete a gone chat's views, chips, refresh state, open disclosures, cards and undo dialogs, and its Connect form and refresh dialog. The keys of the per-message maps now hold the chat's ID where they lacked it (the rewind mark).
- `statetest.Fake` gained the undo state, estimates, refreshes, turn undo, live state and `RemoveChat` the tests need. `Sent` records the message ID.
- `failedCard`'s comment no longer says the Connect card isn't built.

## Tests (headless, light and dark)

The four scope items:
- `chips_test.go`: `TestAMemoryChipUndoesRedoesAndEdits` (Undo, Redo with the keyboard, the note, *Edit* opening project memory at the section), `TestAMemoryChipKnowsItIsUndone` (undone before the chat opened; a redo that found the section changed; *Edit* for user memory), `TestADesktopChipUndoes`.
- `refresh_test.go`: `TestRefreshMemoryStartsAndAsksBeforeALongOne` (the menu by keyboard with its estimates, a short refresh, a long one cancelled and then started), `TestRefreshMemoryIsOffWhileTheChatWorks`, `TestARefreshChipUndoesItsBatch` (*View changes*, Undo, Redo, an undo blocked by a changed section).
- `connect_test.go`: `TestTheConnectCardSendsTheKeptMessage` (Ollama first, connect with no key, the kept message sent, the card gone).
- `logic/chips_test.go`, `logic/refresh_test.go` (`TestAnEstimateFitsOneLine`), `kit/titlebar_test.go` (`TestBarMenuButtonSaysItIsOpen`: size, menu position, a Persian item with a subtitle, Enter and Escape, an off button opens nothing), `shell/titlebar_test.go` (the button's size and place in the bar).

The tests the chat screen lacked:
- `connect_test.go`: `TestTheRetryCardRetries`.
- `turns_test.go`: `TestUndoTurnRevertsAndPutsBack`, `TestUndoTurnOfNothingSaysSo`, `TestAMessageSentDuringATurnJoinsIt`, `TestDenyWithANoteSendsIt`, `TestAnApprovalAfterStopIsARecord`.
- `screen_test.go`: `TestADeletedChatsStateGoes`.

Persian text is in each chip, the Connect card's thread and the composer. With the fix undone, the memory chip, Connect card and pruning tests failed.

## Live check (Windows 11, bench build, scratch profile)

The bench build ran with `-hold` and was driven through UI Automation. Screenshots were taken of the app's window only.

| Check | Result |
|---|---|
| The header | *Refresh memory* before *Chat menu*, 28 px square at 125% scaling, the bar one line |
| Its menu | both items, each with the backend's estimate: "100 turns · 21 calls · about 48,552 tokens" |
| *Refresh memory* chosen | the dialog "Refresh memory: 100 turns?", "This makes 21 calls to bench-model, …" |
| *Refresh* | the refresh ran on the bench's fake model, the button off meanwhile. It ended with "Memory refresh failed, nothing was changed: … the answer has no JSON object", since the fake model can't write JSON, and the button came back on |

As in TASK-068, a running Jenab blocks the bench build, so `XDG_RUNTIME_DIR` was set to a scratch folder for the run.

## Screenshots

Kept outside the repo, in light and dark: the memory chip, the desktop chip with its steps, the refresh menu, the long-refresh dialog, the refresh chip with its changes, the Connect card, the *Undo turn* dialog, a message joining a turn, and the bar. Plus three from the live check. The tests write them when `JENAB_SHOTS` names a folder.

## Not done, and gaps

- A real refresh chip and Connect card weren't seen live: the bench's model can't produce a batch, and the bench has a provider. Both are covered headless.

## Follow-ups

- `cmd/jenab-bench` could set `XDG_RUNTIME_DIR` itself (as TASK-068 noted).
- A live check of a refresh chip needs a model that writes JSON, such as one of the reference models, which costs requests.

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass. The changed Go files are gofmt-clean.
