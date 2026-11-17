# TASK-072 report: the hosts' counter, the rewind dialog, Ctrl+Tab and the Dev page

**Base:** `main` at 0e68d3c. **Branch:** `claude/task-072-stores-stale-reads`.
**Model requests:** none. The tests use fake services.
**Downloads:** none.
**Live check:** none. Each fault needs answers to come in a certain order, which the tests force by holding answers back.
**Docs/:** not touched, as asked. The ticket's Result, status and README row are left for the main session.

## 1. A removed private host could come back (`state/hosts.go`)

Before: `Load` and `Change` shared one counter. Take a reload that starts during a save and reads the list before the save lands. It was the last one started, so the save's answer was dropped and the reload's older list showed.

Now there are three counts per project:
- `loads`: reads started;
- `changes`: changes started;
- `changing`: changes running.

A read's list shows only if all three hold:
- it is the last read started;
- no change was running when it started;
- no change started while it read.

So a read made during a save never replaces the save's list. A change's list shows if it is the last change started. `Busy` now clears when the last change running ends, not the first.

A read dropped this way isn't made again. The save's own answer is the stored list, and the next `Load` reads as usual.

## 2. The rewind dialog could show an older plan (`chat/cards.go`)

Before: each click on *Rewind to here* or *Edit and resend* read a `RewindState`, and whichever answer came last opened the dialog. Here is how that failed. A click in chat A, then a click in chat B, while A's answer was still loading. If A's answer came last, it replaced B's question. The TASK-066 check then put it away as another chat's, so B's dialog vanished.

Now a click makes a new `rewindAsk` at once, marked as loading. A plan is taken only by the ask it was read for: if `s.ask` is another ask by then, the plan is dropped. The ask's turn, chat and opening are therefore always its own. A loading ask draws nothing. One for another chat is still put away, as in TASK-066. A failed read clears the ask and shows the error.

The changes in `chat` are in `cards.go` and its test only.

## 3. Ctrl+Tab could skip a new chat (`state/nav.go`)

Before: after each fresh chat list, `Nav.Prune` dropped the project's MRU chats that the list didn't have, except the chat shown. A list read before a chat was made doesn't have that chat. So a chat made and then left before that list came was dropped from the order Ctrl+Tab cycles through.

Now Nav counts the chats that come into MRU and notes when each came (`entered`). `Chats` reads `Nav.Stamp()` as each list's read starts and passes it to `OnList` with the list. `Prune` only drops a chat that was in MRU before the read started. A chat that came in later stays until a list read after it decides. `OnList` gained the stamp argument; only the stores use it. The screens' `OnPrune` hooks are unchanged.

## 4. The Dev page retried every frame (`dev/page.go`)

The ticket names `dev/dev.go`; the code is in `dev/page.go`.

Before: when reading what this start used failed, `View` started the read again on the next frame. Each answer drew a frame, so it read again and again, and showed a spinner but never the error.

Now a failure is kept and shown as Settings shows one:
- a red alert, *What this start used couldn't be read*, with the error;
- a *Try again* button, which reads again.

It doesn't read again until then.

## 5. Tests

| Fault | Test | On the old code |
|---|---|---|
| 1 | `state/stale2_test.go`: `TestAReadDuringASaveDoesNotBringARemovedHostBack`. A save, a reload during it that answers after it, then a read after the save that does show. | The save's list is dropped and the old list shows. |
| 2 | `chat/rewind_test.go`: `TestTheRewindDialogShowsItsOwnPlan`. A's plan is held back (`statetest.Fake.HoldRewindState`) until B's dialog shows. | Fails: "a's late plan put b's dialog away". |
| 3 | `state/stale2_test.go`: `TestCtrlTabKeepsAChatMadeSinceTheListWasRead`. The list is read, c3 is made and left, then the list comes. Ctrl+Tab still reaches c3, and a later list still drops a deleted chat. | c3 is dropped from MRU. |
| 4 | `dev/started_test.go`: `TestAFailedStartReadShowsTryAgain`. It checks 1 read in 10 frames, the error with *Try again*, no spinner, and that *Try again* reads once more and shows the page. | Fails: only "Loading" shows. |

I ran the tests for 2 and 4 against the old `cards.go` and `page.go`, and both fail there. For 1 and 3, the old behavior is reasoned in the table, not run: the new tests use the new API.

Other test changes:
- `statetest.Fake` has `HoldRewindState`, so a test can hold back one chat's plan.
- The TASK-066 test of `OnList` takes the new stamp argument.

Checks:
- `go build ./...` and `go vet ./...` pass.
- In `go test ./...`, every package passed but two that this change doesn't touch:
  - `internal/project`'s `TestCrashExport` ("no kill landed in an export");
  - `internal/scenario`'s `TestRunTimeout` ("0 calls").

  Both packages ran several times slower than usual in that run (319 s and 250 s), with other sessions using the machine. Both tests pass alone. `TestRunTimeout` failed the same way under load in TASK-066 and SPIKE-048 Part A.
- The state, dev and chat rewind tests also pass with `-count=3`.
- `-race` still can't run here: there is no C compiler.

## 6. Open points

1. `Shell.Prune` and the screens' `OnPrune` hooks can still drop the state of a chat made since the list was read, such as a file docked in a chat made and left before the list came. They get the same lists; the stamp could be passed to them too. The ticket asked only for Ctrl+Tab.
2. A list read dropped because a save ran during it isn't made again. The save's answer is the stored list, so this only matters if a card changed the list during the save.
