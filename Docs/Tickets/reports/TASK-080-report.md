# TASK-080 report: what the model reads of a failed turn's notice

**Branch:** `claude/task-080-retried-turn-notice`, cut from `main` at ecdf2ef.
**Commit:** b8b5627. It is not merged and not pushed.
**Model requests:** none. The test uses the agent harness's fake provider.

## What the model reads now

When a turn fails, the app stores a user-role `turn_failed` notice. Its text is written for the user, for example "[the turn stopped: p says the quota or credit is used up: daily limit. Add credit, or pick another model for this chat.]". Before this change every provider sent that text to the model unchanged. After a Retry it was the last message of the request.

The model now reads one of two fixed lines in place of the notice:

| The notice's place in the history | What the model reads |
|---|---|
| The turn was retried: a message of the same turn follows the notice, or nothing follows it | `[the turn stopped here on an error, and the user retried it]` |
| The turn was not retried: the next message belongs to a later turn | `[the turn stopped here on an error]` |

The stored notice keeps its text, so the user still sees the same advice and the same Retry card.

## Where it is decided

The rule is in one place, `shape` in `internal/agent/context.go`. That function already decides what each stored message becomes in a request: it hides marks and cards and turns quotes into text.
- `failedPart` writes the line. It copies the notice and replaces only the copy's text.
- No provider changed. All five provider encodings still send `Notice.Text` as plain text, so the rule is the same for every model and provider.
- Nothing is stored or migrated. Old notices and new ones get the same line.

## Why a line and not nothing

- **A retried turn.** If the notice were dropped, the request could end with the user's question or with a tool result, and nothing would show that the earlier attempt broke off. With the line, the model knows the turn stopped and that the user asked for it to go on, so it carries on with the turn. It doesn't apologise for an error it can't see.
- **An earlier failure that was not retried.** The line shows why that turn has no answer, so the model doesn't treat the user's next message as a reply to an answer it gave.
- **Why the line names no cause.** None of the causes is something the model can act on:
  - Quota, a refused request, a request that's too large and a model that isn't set up are about the user's settings.
  - "Tool calls that couldn't run, 3 times in a row" is already in front of the model: each bad call's error is stored as its tool result, before the notice.

  So the ticket's `<what>` is left out. This also needs no new stored field, and old notices need no special case.

## How "retried" is told apart

Retry continues the same turn (8.3), so the turn number is enough:
- A message after the notice in the same turn means the turn went on.
- No message after the notice means this is the request that Retry started.
- A message of a later turn means the user asked something new without retrying.

The prefix sent to the provider stays stable, which keeps the cache valid:
- A notice that was retried reads the "retried" line in every later request.
- A notice that was not retried is first sent once the next turn has begun, so it always reads the plain line.

The only place the rule can be wrong is the cut check's token estimate (`cutIfDue`), which sees the history only up to the previous turn. There, a failed last turn reads the "retried" line, about five tokens longer. That estimate only counts tokens and is never sent.

## Test

`TestWhatTheModelReadsOfAFailedTurn` in `internal/agent/retry_test.go`:
1. A turn fails on a quota error and the user presses Retry. The retried request is exactly `["user: price?", "user: [the turn stopped here on an error, and the user retried it]"]`.
2. Another turn fails, and the user sends a new message without retrying. That request holds the earlier retried line, the answer "ten", the question, `[the turn stopped here on an error]`, and the new message.
3. Both stored notices still hold the user's advice, "Add credit, or pick another model".

`TestTurnGivesUpAfterTenMinutes` asserted that the retried request ends with the user's notice text. It now expects the line.

With `context.go` put back to ecdf2ef, both tests fail: the requests hold "… Add credit, or pick another model for this chat." and "… Use Retry to try again.".

## SPEC wording (for 8.3; Docs/ not edited)

> A failed turn's notice is for the user. The model reads, in its place, "[the turn stopped here on an error, and the user retried it]" when Retry continued that turn, and "[the turn stopped here on an error]" when the user went on without it. The line names no cause: a tool call's error is already in its result, and the other causes are the user's settings. This is decided where the history is shaped for a request, the same for every provider. The stored notice is unchanged.

## Checks

The following all pass:
- `go build ./...`
- `go vet ./...`
- `go vet -tags bench ./cmd/desktop`
- `go test ./...`

The changed Go files are gofmt-clean.

## Follow-ups

None needed. If a cause is ever wanted in the line, it would need a stored field on the notice; notices written before that field existed would keep the plain line.
