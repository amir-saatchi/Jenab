# TASK-066 report: lost updates and stale reads in the native stores

**Base:** `main` at eb1031d. **Branch:** `claude/task-066-state-fixes`.
**Model requests:** none. The tests use fake services.
**Downloads:** none.
**Live check:** none. Each fix is about timing: two answers coming in the wrong order, a read failing, or a chat going away while it is read. A person can't make these happen on purpose in the app. The tests below hold each answer back and let it go in the order that broke things.

## 1. A failed re-read lost the turn's end (`state/threads.go`)

Before: when a read of a chat was already running, `read` called `then(nil)` at once. So `Reload`'s callback came before the new snapshot was in. If the running read then failed, the events queued during it were dropped and the extra read was skipped. A queued `Status{Idle}` was lost, and the chat stayed on "working" with *Stop*.

Now:
- A `then` waiting on a running read is called when the read after it is applied, or when it fails.
- A failed re-read of a chat that shows keeps its queue and tries again after `retryWaits` (500 ms, then 2 s), as a first read does in `Open`.
- When every try fails, the queued events are applied to the thread as it is, so a turn's end still ends it. The thread is marked stale, and the next event for it starts a read.
- A first read of a chat not shown yet is still tried again by `Open` (`tryOpen`), not twice.

## 2. Older answers replaced newer ones

`Chats.Load`, `Chats.LoadWaiting` and `Hosts.Load` count the reads they start, as `Pages.LoadUsage` does. An answer is applied only if no read has started since. The caller's channel still gets the answer's error, or nil.
- A dropped `Chats.Load` answer doesn't call `Threads.Gone`, so an old list can't drop the threads of chats made since.
- `Hosts.Change` counts too, so a `Load` that started before a change can't put back the list from before it.
- `Settings.loadModels` runs on the settings queue and on `LoadModels`' goroutine, so its counter is atomic. On the UI thread, an answer replaces `Models` only if it started after the one shown.

## 3. An edit before the settings loaded was dropped (`state/settings.go`)

`Edit` used to send nil and do nothing when the settings weren't read yet, so an early `SetTheme` was lost. Now the edit is kept as pending and queued like any other. If its turn comes and the settings still aren't read, the job reads them first, then saves the edit on top. If that read fails, the edit is taken out and its channel gets the error.

## 4. A forgotten chat came back

`Threads.Forget` also drops the chat's read in flight and its stale mark. `Gone` and the 20-chat limit both call `Forget`.
- When a dropped read's answer comes, it is ignored, and its callbacks get nil.
- `tryOpen`'s next try stops if the chat is no longer among the chats opened lately.

## 5. Off-thread write and wrong targets

- **`dev/page.go`:** a goroutine set `s.starting = false` off the UI thread. Now `starting` holds the channel from `LoadStarted`. The stores fill that channel on the UI thread, and `View` takes it there. No goroutine is left.
- **The rewind dialog (`chat/cards.go`):** the ask now holds the project and chat of the message it came from, and *Rewind* or *Edit and resend* acts on them. The dialog shows only over its own chat. If the answer to *Rewind to here* or *Edit and resend* comes after the user moved to another chat, the dialog is put away and not opened there. A rewind already running finishes in its own chat: the composer it fills and the thread it reloads are that chat's.
- **The undo reads (`undoTurn`):** each read is counted. An answer replaces the state shown only if it started after the one shown, so an older answer that comes last is dropped. The callback of a click (`open`) still gets its own answer.

## 6. Small fixes

- `Stores.Close` runs once (`sync.Once`), so a second call does nothing.
- `Chats` has an `OnList` hook. The stores use it, after a fresh chat list, to prune the project's deleted chats:
  - from `Nav.MRU` (`Nav.Prune`);
  - from the shell's per-chat maps, `Open`, `ChatHidden` and `Drawer` (`Shell.Prune`).

  The chat shown stays, as in `Threads.Gone`, because the list may be older than a chat just made. During a Ctrl+Tab cycle, `Nav.Prune` waits for the next list, so the cycle's place holds.

## 7. Tests

New, in `state/races_test.go` and `chat/rewind_test.go`:

| Done when | Test |
|---|---|
| A failed re-read with a queued `Status{Idle}` ends idle | `TestAFailedReReadStillEndsTheTurn`. It also checks that the stale thread is read at the next event. `TestAFailedReReadIsTriedAgainWithItsEvents` checks a second try that works. |
| `Reload`'s callback comes after the snapshot is applied | `TestReloadAnswersAfterTheSnapshotIsApplied` |
| Two overlapping loads keep the newer answer | `TestOverlappingChatListsKeepTheNewer` (chat list and waiting list), `TestOverlappingHostReadsKeepTheNewer`, `TestOverlappingModelReadsKeepTheNewer` |
| An edit before settings load isn't lost | `TestAnEditBeforeTheSettingsLoadIsKept` |
| A forgotten chat's read doesn't bring it back | `TestAForgottenChatsReadDoesNotBringItBack` (a re-read and a first read) |
| The rewind dialog acts on its own chat | `TestTheRewindDialogActsOnItsOwnChat`. It fails on the old `cards.go`, where the dialog opens over the other chat. |
| (small) | `TestStoresCloseTwice`, `TestADeletedChatLeavesNavAndShell` |

`statetest.Fake` now records which chat each rewind was for (`Rewound`). gofmt also lined up two of its old lines.

Checks:
- `go build ./...`, `go vet ./...` and `go test ./...` pass. In the full run, `internal/scenario`'s `TestRunTimeout` failed once ("0 calls" in a refresh's merge). That package isn't touched here, and the test passed 3 times out of 3 alone. It failed the same way under load in SPIKE-048 Part A.
- The state tests also pass with `-count=10`, and the chat rewind tests with `-count=5`.
- `-race` couldn't run: it needs cgo, and this machine has no C compiler (`CGO_ENABLED=0`, no gcc). It was the same in SPIKE-048 Part A. Installing one would be a download, so I didn't.

## 8. Open points

1. Run the state and chat tests with `-race` on a machine with a C compiler, or after one is installed here.
2. The chat screen's own per-chat maps (`views`, `cards`, `undos`, `open` in `chat/screen.go`) still keep deleted chats. They are the screen's state, not the stores', and `cards.go` is TASK-065's next. They could take the same `OnList` signal.
3. `dev/page.go` reads `Started` again on the next frame after a failed read, as it did before. A failing read isn't reported there; the shell's own `LoadStarted` reports it.
