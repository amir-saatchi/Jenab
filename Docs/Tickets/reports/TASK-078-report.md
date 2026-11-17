# TASK-078 report: key shapes in escaped JSON, the crash redactor at start, and overlapping host changes

**Base:** `main` at 35c4c1d. **Branch:** `claude/task-078-shapes-crash-start-and-hosts`.
**Model requests:** none. The tests use fakes and dummy keys.
**Downloads:** none.
**Live check:** none. The real keychain is never touched by a test (Q12), and the host faults need an order of answers that the tests force.
**Docs/:** not touched, as asked. This report isn't committed; the code is.

## 1. Shapes missed escaped JSON (`internal/secret/shapes.go`)

Before: the field shapes took a bare `"`, `'`, `:` or `=` after the name. JSON printed with `%q` has `\"api_key\":\"…\"`, so an unprefixed key in it wasn't found.

Now every quote the shapes allow may be escaped, by any number of backslashes, so `%q` once or twice is matched. This covers the `Authorization` field and the key-named fields (`api_key`, `x-api-key` and the rest). A value also stops at a backslash, so the escaped closing quote stays: the redacted text still unquotes (`strconv.Unquote`). The `?key=` shape already matched in escaped text; its value now also stops at a backslash. A `[secret:NAME]` is still left as it is, escaped or not.

## 2. The crash redactor ran at every start (`cmd/desktop/crash.go`)

Before: `crashRedactor` listed and read the whole keychain when it was made, at every start, before the window showed, even with no crash to collect.

Now `crashRedactor` returns a function that reads the keychain at its first call (`sync.Once`). `crash.Collect` calls it only for a crash file with text in it: no `.raw` file, or an empty one, reads nothing. `main.go` and the `crash` package are unchanged.

## 3. Host changes and reads that overlapped (`internal/native/state/hosts.go`)

Before:
- A `Load` made while a `Change` ran was dropped, rightly, as it may have read before the save. But if the `Change` then failed, nothing read the list again. The page could keep showing a list that isn't stored, such as one without a host a card had just added.
- Two `Change`s at once each read the list, changed it and saved it, in no set order. One could save over the other's edit.

Now:
- A project's changes run one after another. A `Change` waits in a queue while another of the project's changes runs, and starts when it ends, so it reads what the earlier one saved. `Busy` holds while one runs or waits.
- Each successful change shows the list it saved.
- A failed change reads the list again when no other change of the project waits. If one waits, its answer becomes the list, or its failure reads the list again.
- A `Load` still shows only if no change ran while it read (TASK-072).

### Did `Busy` keep changes apart?

On the page, mostly, but not on every path:
- *Add* is disabled while busy and checks `!busy` again.
- *Remove* is disabled while busy, but has no second check.
- Both read `busy` once per frame. Two input events handled in one frame, such as Enter in the field and a click on *Remove*, could start two changes.

Cards don't go through the store: *Allow in this project* calls `project.AddPrivateHost`, which reads and writes under the project's `hostsMu`. The store's queue now covers every page path. The card path is open point 1.

## 4. Tests

| Fault | Test | On the old code |
|---|---|---|
| 1 | `internal/secret/shapes_test.go`: `TestShapesInEscapedJSON`. Four JSON bodies printed with `%q` once and twice: `api_key`, `x-api-key`, `Authorization: Bearer`, `?key=`. The key, with no known prefix, is gone, and the text still unquotes. `[secret:NAME]` and a field of another name stay as they are. | Fails: the `api_key` and `x-api-key` keys stay, and the `?key=` text no longer unquotes. |
| 2 | `cmd/desktop/crash_test.go`: `TestTheCrashRedactorReadsTheKeychainForACrashOnly`. A keychain that counts its reads: none with no crash file or an empty one, then reads and redaction with a crash. | Fails: "a start with no crash read the keychain 12 times". |
| 3 | `internal/native/state/stale3_test.go`: `TestAFailedHostChangeReadsTheListAgain`. A change fails after a card added a host and a read made during the save was dropped; the list is read again and shows the card's host. | Fails: times out, as no read comes. |
| 3 | `stale3_test.go`: `TestHostChangesRunOneAfterAnother`. Two changes at once: add one host, remove another. Both edits are stored, and `Busy` holds until the second ends. | Fails: "got [], want [printer.local]": the second save lost the first's edit. |

I ran each test against the old `shapes.go`, `crash.go` and `hosts.go`, with the results above.

Checks:
- `go build ./...`, `go vet ./...` and `go vet -tags bench ./cmd/desktop` pass, and `go vet` of `internal/secret` with `GOOS=linux`.
- `go test ./...` passes in full, every package. The state package's tests also pass with `-count=3`.
- gofmt is clean on every changed file.
- `-race` still can't run here: there is no C compiler.

## 5. Open points

1. **A card's host can still be lost.** The store's change reads the list, then saves the whole list with `SetPrivateHosts`. A card's `AddPrivateHost` that lands between the read and the save is overwritten. The page then shows the saved list, without the card's host, so it doesn't show a list that isn't stored. But the card's host is gone.
   The fix is at the service: an edit that reads and saves under `hostsMu`, such as `EditPrivateHosts(ctx, p, func([]string) []string)` in `project`, `app.ProjectService` and `HostsAPI`, or *add* and *remove* calls. That changes the service API, so I left it for a ticket.
2. As in TASK-076, the shapes don't find a key with no known prefix sent in a header or field of another name.
