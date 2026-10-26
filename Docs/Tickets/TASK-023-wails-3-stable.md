# TASK-023 — Move to Wails v3.0.0 and Go 1.26.10
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** —

## Goal
Wails v3.0.0, the first stable v3, came out on 2026-10-26. Jenab is on `v3.0.0-beta.26`. Move to v3.0.0, and build with Go 1.26.10.

## Why
- **Security:** the release fixes the Wails v3 advisories and asks everyone on beta.30 or older to upgrade. One stays partly open: any iframe in a window can call the bound Go methods, so Jenab must never embed untrusted pages.
- **Windows:** two crashes are fixed, one painting the menu bar and one in dark mode.
- **Go 1.26.10** has a `crypto/x509` fix (GO-2026-6809). Wails recommends it.
- The notes for beta.27 to v3.0.0 list no breaking changes. Only Go 1.26 is now required, which we already use.

## Scope
1. `go.mod`: `github.com/wailsapp/wails/v3 v3.0.0`, and `toolchain go1.26.10`, so every build uses it without a system install. Keep `go 1.26.0`. Other modules may move up with Wails; list them.
2. `frontend/package.json`: `@wailsio/runtime` `3.0.0`, with `bun install`.
3. The `wails3` CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`.
4. Regenerate the bindings (`wails3 task bindings`). Any change in them is listed.
5. Fix what the upgrade breaks. Code changes stay in `internal/app` and `internal/update` (CODE-OUTLINE 1); anything else is reported first.
6. The spikes keep their own versions.

## Done when
- `go build`, `go vet` and `go test` pass on Go 1.26.10; the frontend typecheck and tests pass; `wails3 task check:bindings` passes.
- `wails3 task build` builds `bin/jenab.exe`, and the user starts it and checks a chat.
- DEVELOPMENT.md shows the new versions.

## Result
Done on 2026-10-27 (commit 6dfb15f).

- **Versions:** Wails v3.0.0 in `go.mod`, `@wailsio/runtime` 3.0.0, and the `wails3` CLI v3.0.0. `go.mod` has `toolchain go1.26.10`. The Go install in `C:\Program Files\Go` stays at 1.26.0.
- **Other modules:** only `golang.org/x/net` moved, to v0.60.0. `klauspost/compress` 1.18.7 and `x/image` 0.45.0, the fixed versions, come in through Wails; `go.mod` doesn't list them because Jenab doesn't build their packages.
- **Bindings:** one new file, `frontend/bindings/events/index.ts`, with our 7 event names as constants. Nothing uses it yet.
- **Code:** no changes needed.
- **Tests:** Go build, vet and tests, the frontend typecheck and 79 tests, and `check:bindings` pass. `wails3 task build` builds the app, and the user started it and used a chat.
- **Not tried:** macOS and Linux.
