# TASK-079 — A card's host lost to a page change
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-078

## Goal
The open point in TASK-078's report (Low). A host added by an approval card's *Allow in this project* can be lost to a change made on project settings at the same time.

## Scope
`state.Hosts.Change` (`internal/native/state/hosts.go`) reads the list with `PrivateHosts`, changes it, and saves the whole list with `SetPrivateHosts`. Since TASK-078 a project's page changes run one after another, but a card doesn't go through the store.
- The card calls `project.AddPrivateHost` (`internal/tool/page.go:151`), which reads and writes under the project's `hostsMu` (`internal/project/settings.go`).
- `SetPrivateHosts` takes `hostsMu` only for the save, and the page's read isn't under it at all.
- So a card's host saved between the page's read and its save is overwritten. The page then shows the list as stored, without the card's host. The page shows nothing false, but the user's *Allow in this project* is gone, and the next request asks again.

The change is made where the lock is: the project reads, changes and saves the list in one hold of `hostsMu`.
- Add an edit to `project`, for example `EditPrivateHosts(ctx, func([]string) []string) ([]string, error)`. It checks the hosts as `SetPrivateHosts` does and returns them as stored.
- `AddPrivateHost` and `SetPrivateHosts` take `hostsMu` around their whole read and save, so every path is ordered the same way.
- Expose the edit on `app.ProjectService` and `state.HostsAPI`, and use it in `Hosts.Change`. Keep or drop `SetPrivateHosts` on the API as you see fit; say which in the report.
- An invalid host still changes nothing and is still `KindInvalid`.

## Done when
- A test in `project`: an `AddPrivateHost` that runs while an edit holds the list is kept. It fails on the old read-then-`SetPrivateHosts` path.
- A test in `native/state`: a host the service gains during a page change is in the saved list.
- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass.

## Result
Done. `project.EditPrivateHosts` reads, changes and saves the exceptions in one hold of `hostsMu`; `app.ProjectService` and `state.HostsAPI` have it, and `Hosts.Change` makes one call to it. A card's *Allow in this project* lands before or after a page change, never between its read and its save. `SetPrivateHosts` is gone from `HostsAPI`, so the page can't go back to a read and then a save; `project` and `app` keep it for a whole list, which only tests use now.

A test in `project` and one in `native/state`, both failing on the old code. Report: [reports/TASK-079-report.md](reports/TASK-079-report.md).
