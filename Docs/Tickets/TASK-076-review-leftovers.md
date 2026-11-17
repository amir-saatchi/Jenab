# TASK-076 — Three small leftovers from the review: crash redaction, Undo turn's state, number fields
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-072, TASK-073

## Goal
Three Low findings from the review of TASK-064 to TASK-070 (2026-11-17) that TASK-071 to TASK-075 didn't cover.

## Scope
1. **A removed provider's key isn't redacted from a crash log.** `crashRedactor` (`cmd/desktop/crash.go`) builds its list of secrets from the providers in the settings at the next start. A provider removed between the crash and that start isn't there, so its key, if the crash output holds it, goes into the `.log` as it is. Redact every secret Jenab's keychain still holds, not only the current providers'. Where the value is gone from the keychain too, it can't be matched by value; say in the report what is done for that case, for example common key shapes such as an `Authorization: Bearer` value.
2. ***Undo turn* can be opened with an older state** (`internal/native/chat/cards.go`, `undoTurn`). `read(then)` gives `then` the answer to its own read, even when a newer read has answered already (`u.shown`). So the dialog can be built from an older plan than the one on screen, as the rewind dialog could before TASK-072. The dialog uses the newest state read, or waits for it.
3. **A number field can stop committing** (`internal/native/kit/number.go`). After a commit, `sending` stays set until the value shown changes or the save fails. If the save succeeds but the value shown stays the same, for example when the service keeps or rounds it, the next commit of that same text is dropped. `sending` ends when the save ends, whatever the value shown.

## Rules
- Keys are never logged or shown (6.7). Tests use dummy keys.

## Done when
- A test for each, failing on the old code.
- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass.

## Result
Done.
1. Crash logs hide the settings' providers' secrets as before, then every other secret the keychain still holds for Jenab (`secret.Store.Stored`; Windows only, since go-keyring can't list on macOS or Linux), then whatever looks like a key (`secret.Shapes`): an `Authorization` or bearer value, a field or header named like a key, a key in a URL, or a known prefix such as `sk-` or `AIza`. A key with none of these shapes, in a header of another name, is still not found. The shapes are used for crash logs only.
2. *Undo turn* opens with the state of the newest read started, and waits for it.
3. `Number.Saved` ends a commit's save whatever the value shown; Settings calls it after a save without an error.

A test for each, failing on the old code. The review found an unquoted `[secret:NAME]` losing its name to the shapes; fixed in `b47c175`. Report: [reports/TASK-076-report.md](reports/TASK-076-report.md).
