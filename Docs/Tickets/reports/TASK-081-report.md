# TASK-081 report: pruning keeps what came after the chat list's read

- **Base:** main at ecdf2ef
- **Branch:** `claude/task-081-prune-hooks-stamp`, commits 7ab75ea and e7f8cac, the review fix for drafts (not merged, not pushed)
- **Model requests:** none
- **Downloads:** none
- **Live check:** none needed
- **Docs:** not edited

## What changed

Of the two options in the ticket, I chose the keep set worked out once in `Stores.listed`.

- **`Nav.Keep(p, listed, shown, stamp)`** (`internal/native/state/nav.go`, new) returns the chats a fresh list leaves:
  - the chats it lists;
  - the chat shown;
  - the project's chats that came into MRU after the read started. These are the same chats TASK-072's stamp kept in `Nav.Prune`.
- **`Nav.Prune(p, keep)`** now drops the project's chats that aren't in `keep`. The stamp rule moved into `Keep`, so it works the same as before. It still waits during a Ctrl+Tab cycle.
- **`Shell.Prune(p, keep)`** (`shell.go`) drops `ChatHidden`, `Drawer` and `Open` only for the project's chats that aren't in `keep`.
- **`OnPrune`** (`stores.go`) is now `func(p id.Project, keep []id.Chat)`.
  - The chat screen's `prune` (`internal/native/chat/screen.go`) keeps the views, chips, *Refresh memory* cards and open parts of every chat in `keep`.
  - Nothing else registers an `OnPrune` hook.
- **`Stores.listed`** works out `keep` once and passes it to `Threads.Gone`, `Nav.Prune`, `Shell.Prune` and each hook.
- **`Threads.Gone(p, keep)`** (review fix, `threads.go`): `Stores.listed` now calls it with the keep set, so a chat made and left during the read keeps its thread and its unsent composer draft (`Drafts`).
  - `Chats.Load` calls `Threads.Gone` with the list's chats only when `OnList` is unset. When `OnList` is set, as the stores set it, `listed` does it.
  - `Threads.Gone` still also keeps the chat shown last (the last in `Threads.recent`), as before.
  - A deleted chat came in before the next list's read, so that list forgets its thread and draft.
- **A deleted chat is still pruned:** it came into Nav before the next list's read started, so that list's `keep` leaves it out.
- **Test fixture:** in `chat/screen_test.go`, `openWith` is split into `newFixture` and `show`. This lets a test start a list read before the fixture's chat is made. `openWith` works as before for every other test.

## Tests

| Test | What it checks | Old code |
|---|---|---|
| `state.TestPruneKeepsAChatMadeSinceTheListWasRead` | c3 is made, docked and hidden, then left, while the list is read. It keeps its dock and `ChatHidden`, and the hooks are told to keep it. c2, which was deleted, loses its dock. At the next list, c3 (now deleted) loses its dock and `ChatHidden`, and leaves MRU. | **Fails:** `got { }, want {file new.csv}` |
| `state.TestAChatMadeSinceTheListWasReadKeepsItsDraft` (review fix) | c3 is made, given a thread and a draft, then left, while the list is read. It keeps both. c2, which was deleted, loses its draft. At the next list, c3 (now deleted) loses its thread and draft. | **Fails on 7ab75ea:** `got , want half a thought` |
| `chat.TestAChatMadeSinceTheListWasReadKeepsItsState` | A list read is held while the chat is made, given state, and left. The chat screen still keeps its 6 things (view, chip, card input, unfolded parts). The next list, without the chat, drops all of them. | **Fails:** `keeps 0 things, was 6` |

How I ran them against the old code:
- I restored `nav.go`, `shell.go`, `stores.go` and `chat/screen.go` from HEAD.
- In the state test, the hook got the old three-argument signature so it would compile.

For the draft test I restored `chats.go`, `threads.go` and `stores.go` from 7ab75ea.

The TASK-072 test `TestCtrlTabKeepsAChatMadeSinceTheListWasRead`, the earlier `TestADeletedChatsStateGoes` and `TestAChatGoneFromItsListIsForgottenButTheOneShown` still pass.

## Checks

- `go build ./...`: pass.
- `go vet ./...`: pass.
- `go vet -tags bench ./cmd/desktop`: pass.
- `go test ./...`: pass, 43 packages ok, exit 0.
- gofmt: clean on every changed file.
- `-race`: not run, because there is no C compiler on this machine.

## Open points

1. **Fixed in the review commit:** `Threads.Gone` used to prune without the stamp, which lost a just-made chat's unsent draft. It now uses the keep set (see above).
2. **A rare edge case:** a chat made during the read that is pushed out of the 50-chat MRU before the list answers loses its entry time. `Keep` then doesn't keep it. In practice that needs 50 chats opened during one list read.
