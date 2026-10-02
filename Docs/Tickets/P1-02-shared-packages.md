# P1-02 — Shared packages: id, config, secret, limit, logfile
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-01
**Requirements:** R-91, N-21, N-43

## Goal
The bottom row of the import graph (CODE-OUTLINE 1, 3), which every other package uses.

## Scope
- **`id`:** typed IDs, ULIDs from `oklog/ulid/v2` with `crypto/rand`, and `Source` with `SourceOf`, `Kind` and `ID` (SPEC 2.5).
- **`config`:** `DefaultPaths` (2.1), and `config.yaml` read into typed settings with defaults: `context` (3.6), `llm`, `scheduler`, `approvals`, `ui`, the data folder. Saving goes through `go.yaml.in/yaml/v3` nodes, so the user's comments stay. View and pipeline configs and JSON Schema (10) come with Phase 3.
- **`secret`:** `zalando/go-keyring` under the service `jenab`, keys as `jenab:provider:<id>`, 1–2,560 bytes, read when needed and never cached (6.7). `secret.Value` prints `[secret:NAME]`; only the provider's auth header calls `Reveal()` (Q36).
- **`limit`:** `Fair` (the 10:1 rule) and `Gate` with two priorities (Q19), for `max_parallel_calls` (7.6).
- **`logfile`:** `slog` text handler to `<data>/logs/`, 10 MB × 5, Info by default and Debug with the developer tools (Q36).

## Done when
- Unit tests for each package; `synctest` tests for `Gate` and `Fair` (Q35).
- Saving a setting keeps a comment the user wrote in `config.yaml`, and an unknown key (for example from a newer version) is logged with its line and ignored, so the app still starts.
- A test shows a `secret.Value` as `[secret:NAME]` through `%v`, `%#v`, `slog` and JSON.
- A keychain round trip works on Windows; macOS and Linux run it in CI. The test needs `JENAB_KEYCHAIN_TEST=1`, so it never touches a keychain by surprise.

## Result
- `internal/id`, `config`, `secret`, `limit` and `logfile`, with tests. `go vet` and `go test ./...` pass on Windows; the keychain round trip passed there too.
- The `secret` test caught a leak: `%v` on a struct with an unexported `secret.Value` field printed the key, because fmt skips `String()` on unexported fields. The value now sits in a closure, which fmt can't look into.
- App files (`config.yaml`, `registry.db`, `logs/`) stay in `<user data dir>/Jenab`; only `projects/` moves with the data folder (SPEC 2.1).
- `id` first had its own ULID generator; it now uses `oklog/ulid/v2` (picked in SPIKE-002), so there is less code to keep. The library's default random source is `math/rand`, so `id` passes `crypto/rand` instead.
- Debug logging follows `dev_tools`; there is no separate debug setting.
- `updates.mode` defaults to `notify`.
- `limit.Gate` follows the new SPEC 7.6: chat calls never wait, background calls wait in order, and `SetSize` applies a changed limit at once. New settings: `max_parallel_calls` 8, `provider_max_parallel_calls` (`ollama: 1`) and `max_subagents_per_chat` 5. Saving keeps comments on map entries too.
