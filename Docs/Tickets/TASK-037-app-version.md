# TASK-037 — One version number for the app
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** none

## Goal
Jenab has no version yet: `0.0.0` is a placeholder in `build/config.yml`, `build/windows/info.json` and `frontend/package.json`, with no tags. The updater (SPEC 2.8) will compare versions, and bug reports need one. Give the app one version, set in one place, shown in the app and stamped into the build.

## Scope
1. **The number:** semantic versioning. Now `0.1.0-dev`: working towards 0.1.0, which comes when Phase 1 and Phase 2 acceptance (P1-18, P2-14) pass.
2. **One source:** a `VERSION` file at the repository root. Everything else comes from it or is checked against it:
   - The Go side reads it at build time through `-ldflags -X` in `Taskfile.yml` (`wails3 task build`, `dev`, `run`), into a small package such as `internal/version`, with the git commit (short hash) and a mark when the tree had changes. `go run ./cmd/desktop` without the flags shows `dev`.
   - `build/config.yml`, `build/windows/info.json` and `frontend/package.json` carry the same number. Windows' file version needs four numbers (`0.1.0.0`); the product version can keep `-dev`.
   - A Go test fails when any of them differs from `VERSION`, so they can't drift.
3. **Shown in the app:** in Settings, General, an "About" line: `Jenab 0.1.0-dev (d449b7e)`, with a copy button. A binding returns version, commit, and Go and Wails versions.
4. **In logs:** the version and commit in the first log line at start.
5. **Releases:** a short note in the report on how a release would bump it (edit `VERSION`, tag `v0.1.0`). No release, tag or packaging work here.

## Not in scope
- The updater, installers, signing, release notes, tags.

## Done when
- `wails3 task build` gives an exe whose Settings show `0.1.0-dev` and the commit, and whose Windows file properties show the version.
- The drift test passes, and fails when one file is changed by hand.
- `go run ./cmd/desktop` still builds and shows `dev`.

## Result
Done on 2026-11-02. SPEC 2.8 and 5.12, and DEVELOPMENT.md, describe it.

- **Built:** `VERSION` (`0.1.0-dev`); the Taskfile's `build` (also `DEV=true`) and `bench` stamp the number, the short commit and a *modified* mark into `internal/version`. `build/config.yml`, `build/windows/info.json` (`0.1.0.0` fixed, `0.1.0-dev` as strings) and `frontend/package.json` match it, and `TestFilesMatchVERSION` names the field that differs. `SystemService.Version`, an *About* row in Settings → General, and `app: start version=… commit=…` as the first log line.
- **Checked:** the exe's file properties show 0.1.0.0 and 0.1.0-dev; the built app logged the version and commit; the About line in the server build; a build without the flags says `dev`. The drift test failed when `package.json` and `info.json` were edited by hand.
- **Not checked:** the copy click (the browser pane blocks the clipboard; it is the chat's `CopyButton`).
