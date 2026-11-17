# TASK-078 — Key shapes in escaped JSON, the crash redactor at start, and overlapping host changes
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-072, TASK-076

## Goal
Three Low findings from the review of TASK-071 to TASK-076 (2026-11-17), in what TASK-072 and TASK-076 built.

## Scope
1. **Shapes miss escaped JSON** (`internal/secret/shapes.go`). The field patterns need a bare `"`, `:` or `=` after the name, so JSON printed with `%q` (`\"api_key\":\"…\"`) isn't matched. An unprefixed key of a removed provider then stays in the crash log. On macOS and Linux the shapes are the only cover for removed keys. Match the escaped forms too.
2. **The crash redactor runs at every start** (`cmd/desktop/crash.go`, `main.go`). `crashRedactor` lists and reads the whole keychain at every start, before the window shows, even when there is no crash output. Build it only when a `.raw` file is there.
3. **Host changes and reads that overlap** (`internal/native/state/hosts.go`). A `Load` that overlaps a `Change` is dropped; if the `Change` then fails, nothing reads the list again, so the page can show a list that isn't stored. Two `Change`s that overlap each read, change and write the list without order and can lose an edit. Check whether `Busy` (`internal/native/projects/settings.go`) keeps changes from overlapping on every path; if not, run a project's changes one after another. After a failed change, read the list again.

## Rules
- Keys are never logged or shown (6.7). Tests use dummy keys.

## Done when
- A test for each, failing on the old code.
- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass.

## Result
Done.
1. The key shapes also match quotes escaped by backslashes, as in JSON printed with `%q` once or twice, and a value stops at a backslash, so the redacted text still unquotes.
2. `crashRedactor` reads the keychain at its first call only, and `crash.Collect` calls it only for a crash file with text in it.
3. A project's host changes run one after another, so none saves over another's edit. A failed change reads the list again. `Busy` didn't keep changes apart on every path (*Remove* has no second check, and two events in one frame could start two).

A test for each, failing on the old code. Open: a card's *Allow in this project* can still be overwritten between a page change's read and its save; the fix is a service-level edit under the project's lock. Report: [reports/TASK-078-report.md](reports/TASK-078-report.md).
