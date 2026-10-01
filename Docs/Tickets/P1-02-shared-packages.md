# P1-02 — Shared packages: id, config, secret, limit, logfile
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-01
**Requirements:** R-91, N-21, N-43

## Goal
The bottom row of the import graph (CODE-OUTLINE 1, 3), which every other package uses.

## Scope
- **`id`:** typed IDs, our own ULID generator, and `Source` with `SourceOf`, `Kind` and `ID` (SPEC 2.5).
- **`config`:** `DefaultPaths` (2.1), and `config.yaml` read into typed settings with defaults: `context` (3.6), `llm`, `scheduler`, `approvals`, `ui`, the data folder. Saving goes through `go.yaml.in/yaml/v3` nodes, so the user's comments stay. View and pipeline configs and JSON Schema (10) come with Phase 3.
- **`secret`:** `zalando/go-keyring` under the service `jenab`, keys as `jenab:provider:<id>`, 1–2,560 bytes, read when needed and never cached (6.7). `secret.Value` prints `[secret:NAME]`; only the provider's auth header calls `Reveal()` (Q36).
- **`limit`:** `Fair` (the 10:1 rule) and `Gate` with two priorities (Q19), for `max_parallel_calls` (7.6).
- **`logfile`:** `slog` text handler to `<data>/logs/`, 10 MB × 5, Info by default and Debug with the developer tools (Q36).

## Done when
- Unit tests for each package; `synctest` tests for `Gate` and `Fair` (Q35).
- Saving a setting keeps a comment the user wrote in `config.yaml`, and an unknown key (for example from a newer version) is logged with its line and ignored, so the app still starts.
- A test shows a `secret.Value` as `[secret:NAME]` through `%v`, `%#v`, `slog` and JSON.
- A keychain round trip works on Windows; macOS and Linux run it in CI.
