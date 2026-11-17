# TASK-074 — Replace the token of a provider on this computer
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-068

## Goal
A finding from the review of TASK-064 to TASK-070 (2026-11-17), in what TASK-068 built (Medium).

## Scope
*Add provider* lets a local server such as LM Studio have a token, but the provider's row in *Settings → Models* hides *Replace key* for anything on this computer (`internal/native/settings/models.go:115`, `!pr.Local()`). The only way to change the token is to remove the provider and add it again. A local provider gets *Replace key* too, with the field optional, so its token can be changed or removed. One with no token still says "No key needed".

## Rules
- Keys go only to the keychain (6.7).

## Done when
- Tests: a local provider's token is replaced, and removed.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done. Every provider row has *Replace key*. For a provider on this computer the field is optional: a typed token is checked and saved in the keychain, and an empty field (*Remove key*) checks the server without one and deletes the stored token. The row says "On this computer · key in the keychain" or "· no key needed" (`SettingsView.LocalKeys`, which checks only that a key exists).

Tests in `provider`, `app` and `native/settings`. No live check, which would need a local server that asks for a token. Report: [reports/TASK-074-report.md](reports/TASK-074-report.md).
