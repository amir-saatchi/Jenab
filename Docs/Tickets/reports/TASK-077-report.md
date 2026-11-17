# TASK-077 report: the Connect card's refused retry, and a chip's read during its own undo

**Base:** `main` at 35c4c1d. Branch `claude/task-077-connect-retry-and-chip-reads`.
**Model requests:** none. The tests use `statetest`'s fakes. No live check was asked for, and none was run.
**Keys:** none.
**Files:** `internal/app/errors.go`, `internal/native/chat` (`connect.go`, `chips.go`, tests) and `internal/native/state/statetest/fake.go`. TASK-078's files (`internal/secret`, `cmd/desktop/crash.go`, `internal/native/state/hosts.go`) are untouched.

## 1. A refused retry is shown

**Telling the refusals apart.** The native app gets the services' errors as `*app.UIError`, which kept only a kind and a message. Several errors share `KindBusy`, so the kind can't tell "in a turn" from "memory is being refreshed".
- `UIError` now has a `Cause` field (`json:"-"`) and `Unwrap`. `toUI` sets it for the known errors only, to the sentinel itself (`agent.ErrInTurn`, `agent.ErrNoRetry`, `store.ErrNotFound`, …), never to the error that came in. So nothing unredacted rides along, and `errors.Is(err, agent.ErrRefreshing)` works on the UI side.
- The JSON shape is unchanged.
- Errors not in the known list have no cause.

**`sendKept`** now drops only these refusals (`keptGone`):
- `agent.ErrInTurn`: the chat is in a turn, so the message has gone with it.
- `agent.ErrNoRetry`: see below.
- `store.ErrNotFound`: the chat is gone.

Any other refusal, such as `ErrRefreshing` ("Memory is being refreshed. Wait for it to end, or stop it."), shows as an error toast. The message stays sendable: the failed turn's notice is still the chat's last one, and with a provider set up now, the Retry card shows in the Connect card's place. Its *Retry* sends the message once the refresh ends.

**`ErrNoRetry`'s two cases** (`internal/agent/agent.go`, `Retry`):
1. `retryInTurn`: the chat is running or has a turn queued, and no retry wait is pending. A turn is under way, which started from the Retry card or a new message, so the kept message went with it.
2. `!cs.failed`: the chat's last turn didn't stop on an error. `cs.failed` is loaded from the chat, and it is false once a later turn ran or the failed turn was rewound or cleared.

The Retry card and the Connect card show only while the last notice is a failed turn, which is when `cs.failed` is true. So in both cases no message is waiting for *Retry*, and both are dropped. Neither case needs telling apart from the other.

## 2. A chip doesn't read during its own undo

`chip.ask` now returns at once while the chip's own *Undo* or *Redo* runs (`k.busy`). It still notes nothing for that turn undo, so on the first frame after the undo ends it sees a newer turn-undo notice than its last read and reads again. The undo's own answer is taken first, then the read confirms it.

`chip.sent`'s check that an answer is the newest stays as a guard. With no reads while busy, it no longer drops an undo's answer.

## Tests

- `internal/app/errors_test.go`: `TestUIErrorKeepsItsKnownCause`. Four known errors match only themselves after `guard`, the cause is the sentinel alone, and an unknown error has none.
- `internal/native/chat/connect_test.go`:
  - `TestARefusedRetryAfterAConnectSaysSo` (light and dark): the fake's Retry refuses with `ErrRefreshing` as the service would. The error shows, the Retry card shows, and once the refusal stops, *Retry* sends the message.
  - `TestARetryWhoseMessageWentSaysNothing`: `ErrInTurn`, `ErrNoRetry` and a deleted chat. Only the "connected" toast shows.
- `internal/native/chat/chips_test.go`: `TestAChipDoesntReadDuringItsOwnUndo` (light and dark). A memory chip's *Undo* is held (the Fake's new `HoldUndo`). A turn-undo notice shows meanwhile, then the undo lands. The chip must say "· undone" with *Redo*, and offer no *Undo*.
- `statetest.Fake`: `HoldUndo`, and `SetRetryErr` for Retry's answer.

On the old `connect.go` and `chips.go`, `TestARefusedRetryAfterAConnectSaysSo` failed (no error shown) and `TestAChipDoesntReadDuringItsOwnUndo` failed (the chip kept *Undo* for an undone edit). `TestARetryWhoseMessageWentSaysNothing` passes on both, as it should, since the old code dropped those too.

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass. The changed Go files are gofmt-clean.
