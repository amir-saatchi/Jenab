# TASK-079 report: a card's host lost to a page change

**Base:** `main` at 4b6bed9. **Branch:** `claude/task-079-host-edits-under-the-lock`.
**Model requests:** none. **Downloads:** none. **Live check:** none, as the ticket says; the tests force the order.
**Docs/:** not touched. This report isn't committed; the code is. TASK-078's report was removed from the worktree root, as it's in `Docs/Tickets/reports`.

## What changed

**`project` (`internal/project/settings.go`)**
- New: `EditPrivateHosts(ctx, edit func([]string) []string) ([]string, error)`. It takes `hostsMu`, reads the list, runs `edit` on it, checks the result and saves it, all before letting go of the lock. It returns the list as stored.
- The checks moved into `checkHosts`, which `SetPrivateHosts` and the edit share. Each host gets the spelling requests use; the list is sorted, without repeats or empty ones. A host that isn't one is a `*HostError`, and nothing changes.
- `AddPrivateHost` already held `hostsMu` around its read and save, and still does.
- `SetPrivateHosts` has no read: it checks the hosts, then holds `hostsMu` around its save. So all three are ordered by the one lock. A card's add now lands before or after a page's edit, never between its read and its save.

**`app` (`internal/app/project.go`)**
- New: `ProjectService.EditPrivateHosts(ctx, p, edit)`, guarded like the others. A bad host is `KindInvalid`; a missing project is `KindNotFound`.

**`state` (`internal/native/state/hosts.go`)**
- `HostsAPI` is now `PrivateHosts` and `EditPrivateHosts`.
- `Hosts.Change` makes one `EditPrivateHosts` call where it made `PrivateHosts` and then `SetPrivateHosts`. The page's `next` runs on the list as stored, inside the service's hold. TASK-078's queue and the read after a failed change are unchanged.

## `SetPrivateHosts` on the API

- **Dropped from `state.HostsAPI`.** The store has no use for it, and leaving it there would let a later change go back to a read and then a save.
- **Kept on `app.ProjectService` and `project.Project`.** It replaces the whole list, which is still right where a whole list is meant:
  - the app tests;
  - the agent and export tests, which set a project's hosts up.

  Nothing in the app calls it now.

## Tests

| Test | What it checks | On the old code |
|---|---|---|
| `internal/project/settings_test.go`: `TestAnEditKeepsACardsHost` | An `AddPrivateHost` started inside the edit is let run for 300 ms. It waits for the edit's save, and both hosts are stored. A bad host in an edit is a `*HostError` and changes nothing. | With `EditPrivateHosts` written as the old path (read, then `SetPrivateHosts`), the card's host is saved over: `["127.0.0.1" "nas.local"]`, no `printer.local`. |
| `internal/native/state/stale4_test.go`: `TestACardsHostDuringAPageChangeIsKept` | A fake service whose lock a card's add also takes. The card adds a host while the page change is on its way. The stored list and the page both have the card's host and the page's. | With the old `hosts.go`, the change reads before the card and saves after it: `got [nas.local printer.local], want [nas.local card.local printer.local]`. |
| `internal/app/services_test.go`: `TestPrivateHostsSettings` | It also covers `EditPrivateHosts`: a host is stored in one spelling, and a host with a port is `KindInvalid` and changes nothing. | New API. |

The state test's fake has all three methods, so it builds on both sides. To run it on the old code, I put back the old `hosts.go` and its four test files, which use the old `HostsAPI`.

The host fakes in `hosts_test.go`, `races_test.go`, `stale2_test.go`, `stale3_test.go` and `statetest` now have `EditPrivateHosts`:
- The TASK-072 and TASK-078 fakes still answer the read and the save apart, when the test says, so those tests check the same orders as before.
- `statetest`'s edit runs under the fake's lock, as the service's does.

Checks:
- `go build ./...`, `go vet ./...` and `go vet -tags bench ./cmd/desktop` pass.
- `go test ./...` passes in full, every package.
- gofmt is clean on every changed file.
- `-race` still can't run here: there is no C compiler.

## Open points

None from this ticket. `AddPrivateHost` could be written as an `EditPrivateHosts` too; I left it as it is, since it already reads and saves under the lock.
