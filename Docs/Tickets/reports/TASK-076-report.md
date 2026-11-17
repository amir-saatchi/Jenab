# TASK-076 report: three leftovers from the review

**Base:** `main` at 8677cc5. **Branch:** `claude/task-076-review-leftovers`.
**Model requests:** none. The tests use fake services and dummy keys.
**Downloads:** none. `github.com/danieljoos/wincred` was already in the module cache and `go.sum`, as go-keyring's dependency on Windows. `go.mod` now lists it as a direct dependency.
**Live check:** none. The real keychain is never touched by a test (Q12). Each fault needs a certain order of answers, which the tests force.
**Docs/:** not touched, as asked. The ticket's Result, status and README row are left for the main session.

## 1. A removed provider's key could stay in a crash log (`cmd/desktop/crash.go`)

Before: `crashRedactor` hid only the secrets of the providers in the settings at the next start. Here is how that failed. A crash came while a provider was set up, and the provider was removed before the next start. Its key was in the crash output but not in the list, so the log showed it.

Now the crash logs hide three kinds of secret:

1. **The settings' providers**, as before: each key and base-URL field.
2. **Every other secret the keychain still holds for Jenab**, found by value. `secret.Store.Stored()` names them, on a keychain that can list its entries (the new `secret.Lister`). The OS keychain can on Windows: it lists Credential Manager's entries under `jenab:*` with wincred's `FilteredList`, the same library go-keyring uses to write them. On macOS and Linux, go-keyring has no way to list, so `Stored` returns `ErrNotListable` and only kinds 1 and 3 apply.
3. **Whatever looks like a key**, by shape (`secret.Shapes`). This is the fallback for a key the keychain no longer has. Removing a provider in Settings deletes its key from the keychain, so this is the usual case. The shapes:
   - `Authorization: Bearer …` (or `Basic`, `Token`), also as a JSON field or a Go map, as `fmt` prints headers;
   - `Bearer …` of 16 characters or more anywhere;
   - a header or field named `x-api-key`, `x-goog-api-key`, `api_key`, `apikey`, `access_token` or `secret_key`, with `:` or `=`;
   - `?key=…`, `&api_key=…`, `&token=…` in a URL, as Gemini takes its key;
   - a token with a known prefix: `sk-` (OpenAI, Anthropic), `gsk_` (Groq), `nvapi-` (NVIDIA), `xai-`, `hf_`, `AIza` (Google), `ghp_`, `github_pat_`, `glpat-`.

   Each value of 8 characters or more becomes `[secret]`. The header or field name stays, so the log still says what was sent. A value already replaced, `[secret:NAME]`, is left as it is, quoted or not (`api_key: [secret:NAME]` too).

What is still not found: a key the keychain no longer has, with no known prefix, sent in a header or field of another name. For example, a bare 32-character key in a custom header. Nothing in the text marks it as a key.

The shapes are used for crash logs only. The chat export and the provider error bodies still use `Redactor` by value, as before.

## 2. Undo turn could open with an older state (`chat/cards.go`)

Before: a click on *Undo turn* read the turn's state and opened the dialog with that read's answer. Here is how that failed. The click's read was still on its way when a new message came; that started another read. If the newer read answered first (say, the turn was undone by a rewind), the click's older answer still opened the dialog. So it asked *Undo turn 1?* about changes already undone.

Now the dialog waits for the newest read started. A click leaves its callback in `undo.then`. Each answer is still kept only if it is newer than the one shown. The waiting dialog opens when the newest read started answers, with that state. If that read fails, the error shows and the button is free again; an older read's failure is ignored. The TASK-072 rewind code in this file is unchanged.

## 3. A number field could stop committing (`kit/number.go`)

Before: after a commit, `sending` held until the value shown changed or the save failed (`Failed`). A save that succeeded without changing the value shown left it set. The next commit of that same text was then dropped as a repeat.

Now `Number.Saved()` ends the commit's save, whatever the value shown. Settings calls it when the save of *Background calls at once* or of a limit ends without an error. Each commit is numbered (`sends`), and `Saved` only ends the newest. So an older commit's save, ending while a newer value is on its way, doesn't let Enter and then Tab send that value twice (TASK-069). The context window field in the Models dialog keeps its value in the dialog and has no save, so it doesn't call `Saved`.

## 4. Tests

| Fault | Test | On the old code |
|---|---|---|
| 1 | `cmd/desktop/crash_test.go`: `TestACrashHidesARemovedProvidersKey`. Settings name one provider; the keychain also holds a removed one's key; a third key, in an `Authorization: Bearer` header, is in neither. | Fails: the log shows both keys. |
| 1 | `internal/secret/shapes_test.go`: `TestShapes`, `TestStoredListsTheKeychain`, `TestNamesUnder`. The shapes, what they leave (`[secret:NAME]` quoted, unquoted, in a Go map or a URL; a branch name with `task-`; a Go stack), and the Credential Manager names. | New API. The review's case, `api_key: [secret:NAME]`, failed on the first commit: it became `api_key: [[secret]]`. |
| 2 | `chat/undo_test.go`: `TestUndoTurnOpensWithTheNewestState`. The click's read is held back (`statetest.Fake.HoldUndoState`); the turn is undone and a message comes, so a newer read answers first. | Fails: "the dialog opened with the older state: Undo turn 1?" |
| 3 | `kit/number_test.go`: `TestNumberInputCommitsAgainAfterASave`. A save that leaves the value shown, then the same text again; then an older save ending while 9 is on its way. | Doesn't build: `Saved` is new. With a do-nothing `Saved` added to the old file, it fails: "after its save, 8 again committed [8], want [8 8]". |

I ran the tests for 1, 2 and 3 against the old `crash.go`, `cards.go` and `number.go`, as the table says.

Other test changes:
- `statetest.Fake` has `HoldUndoState` and `SetTurnUndo`.
- `cmd/desktop`'s test keyring can list its names.

Checks:
- `go build ./...`, `go vet ./...` and `go vet -tags bench ./cmd/desktop` pass. `go vet` of `internal/secret` also passes with `GOOS=linux` and `GOOS=darwin`, for the keychain file that doesn't list.
- `go test ./...` passes in full, every package.
- gofmt is clean on every changed file.
- `-race` still can't run here: there is no C compiler.

## 5. Open points

1. On macOS and Linux the keychain can't be listed through go-keyring. A removed provider's key there is found by shape only. Listing would need the Secret Service search on Linux and the `security` tool on macOS.
2. The shapes could also run on the chat export and on provider error bodies. The ticket asked for crash logs only.
3. Removing a provider in Settings deletes its key and fields from the keychain (`app/connect.go`). So item 2 of section 1 mostly finds keys the keychain kept: a provider removed by editing `config.yaml` by hand, a delete that failed, or a field of a base URL that changed. A removed or replaced key (TASK-074) is found by shape only.
