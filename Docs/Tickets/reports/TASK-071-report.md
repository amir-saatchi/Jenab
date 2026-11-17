# TASK-071 report: the Connect card keeps the message; memory chips' Undo and Redo

**Base:** `main` at 0e68d3c. Branch `claude/task-071-connect-card-and-chips`.
**Model requests:** none paid. The tests use `statetest`'s fakes. The live check used a stand-in OpenAI-compatible server on 127.0.0.1.
**Keys:** none were used or read. The live connect sent no key.
**Real data:** the live check used a scratch profile outside the repo. No system dialogs were opened.
**Files:** only `internal/native/chat` and `internal/native/state/statetest`. Nothing in `internal/native/settings` changed.

## 1. The Connect card sends the kept message

`connectCard` gave its form a `Connected` callback that sent the kept message only if that form was still the one shown. `state.Settings.Connect` keeps the new settings, posting them to the UI, before it reads the model list and calls back. A frame between the two finds a provider, so `keepConnect` drops the card, and the callback then found no form and sent nothing.

Now the callback is `sendKept(p, ch)`, which no longer depends on the card:
- It calls `Retry` for the chat the connect was started from, with the Retry card's busy flag, so the Retry card that shows meanwhile is off while it sends.
- A chat with no message waiting is no error. If the user pressed *Retry* or sent a message in the meantime, or deleted the chat, `Retry` answers busy ("There is nothing to retry") or not found, and that answer is dropped. Any other error shows as before.
- The form is still cleared and dropped as soon as the card stops showing, so a typed key isn't kept. Only the callback outlives it.

## 2. Chips offer an action only once they know which

The chip asked the backend whether its change was undone once, the first time it was drawn. Until the answer came, it offered *Undo*, even for a change that was undone. It never asked again, so after *Undo turn*, which reverts the chip's change with the rest of the turn, it still offered *Undo*.

Now (`chips.go`):
- *Undo*/*Redo* is off until the chip's state is read (`chip.known`).
- The chip reads its state again after each *Undo turn* or *Redo turn* in its chat. The screen keeps the ID of the newest "turn undone" notice (`logic.LastUndo`, computed when the thread changes), and a chip whose last read was before it reads again, with the button off meanwhile.
- A late answer to an older read or undo is dropped (`chip.asks`). So a read sent before a turn undo can't overwrite the newer one, and an undo's own answer is kept unless a turn undo came after it.
- A read that fails leaves the button on with the state it had, as before, so a click shows the error rather than the chip staying off for good.

This applies to all three chips (memory, desktop, Refresh memory), since they share the code. A desktop save and a refresh batch are reverted by *Undo turn* in the same way.

## Tests (headless, light and dark)

- `connect_test.go`: `TestAConnectWhoseModelsAnswerLateSendsTheKeptMessage`. The Settings fake has a new `HoldModels` channel, which holds the model list until it is closed. The test connects, waits until the card is gone with the model list still held, checks nothing was sent yet, releases the list, and waits for the kept message to be sent.
- `chips_test.go`:
  - `TestAChipWaitsForItsState`: an undone memory chip whose read is held (the Fake's new `HoldUndone`). *Undo* is off, and a click does nothing. Once the read answers, the chip says "· undone" with *Redo* on.
  - `TestTheChipsFollowAnUndoOfTheirTurn`: a memory chip and a desktop chip with Persian text. After a "turn undone" notice both read again and say "· undone" with *Redo*. After *Redo turn* both offer *Undo* again, and the chip's own *Undo* still works.
- `logic/chips_test.go`: `TestLastUndoIsTheNewestTurnUndoneNotice`.
- The Fake has `SetUndone` to change the state while the screen runs.

With each fix undone, the new tests failed. Without the Connect fix, the kept message was never sent. Without the chip fix, the held chip's *Undo* was on, and the chips kept *Undo* after the turn undo.

## Live check (Windows 11, bench build, scratch profile)

The bench build ran with `-hold` and was driven through UI Automation and the bench's loopback routes. The stand-in server answered `GET /v1/models` after 5 seconds, and `POST /v1/chat/completions` with a short streamed answer, logging the user messages it got.

| Step | Result |
|---|---|
| Settings › Models › *Remove bench* (the in-app dialog) | "No provider yet. Add one with your own key." |
| "Hello from the live check" sent in a chat | the turn stopped, and the card "Call 1 needs an AI model to answer" showed with "Your message is kept and sent once it is connected." |
| *OpenAI-compatible…*, name "standin", base URL on 127.0.0.1 | "API key (optional)", *Connect* on with no key |
| *Connect* | the card stayed while the model list was held. When it answered 5 s later, the server got the chat request with "Hello from the live check", and the chat showed its answer, "Got it from the stand-in.", with the model "late-model" |

In the real app, the model list is read before the settings are saved, so the frame between the two posts is short and the live run can't force it. The headless test holds the store's model list read to cover that window. The live run checks the whole path with a late provider.

A window-only screenshot of the result (the app's own capture) is kept outside the repo.

**Bench note:** the bench app quit at once, with an empty log, when `XDG_RUNTIME_DIR` was a slightly longer scratch path. It ran with a shorter one. The single-instance socket path seems to hit the AF_UNIX length limit (108 bytes).

## Seen, not changed

- The chat request after the retry also held the turn-failed notice as a user message ("[the turn stopped: the model "default" is not set up. …]"). This is how the backend sends a retried turn and isn't part of this ticket. Whether the model should read that notice may be worth a look.

## Checks

`go build ./...` and `go vet ./...` pass. `go test ./...` passed in every package but one: `internal/project`'s `TestCrashExport` failed once ("no kill landed in an export") while the bench app was building alongside. Run alone, that package passes, and this branch doesn't touch it. The changed Go files are gofmt-clean.
