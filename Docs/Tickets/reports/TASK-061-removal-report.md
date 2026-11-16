# TASK-061 removal report: Wails, the frontend and the Taskfile leave

Part 2 of [TASK-061](../TASK-061-remove-wails.md), scope items 2 to 5. Branch `claude/task-061-remove-wails-34b1e5`, from main at `2370044`. Work on 2026-11-17, Windows 11. Part 1, the proof, is in [TASK-061-proof-report.md](TASK-061-proof-report.md).

**Status:** done. The native app is Jenab. No Wails module, no `frontend/` and no Bun are left in the build.

## Commits

| Commit | What |
|---|---|
| `282dd2a` | The removal: files, the Wails module, the rename, the bench, CI. One commit, so it is easy to read and revert. |
| `c7380af` | The version stamp, the docs, and comments that named removed paths. |
| `9f02ceb` | Settings' update line named `task build:native`. The release check below found it. |

Git shows `cmd/desktop/main.go` and `bench.go` as changed, not renamed: the Wails app had files of the same names there.

## What went

- `cmd/desktop` (the Wails app), `internal/webui`, `frontend/` (React, Vite, the bindings, Bun's lockfile).
- The Wails module. `go mod tidy` needed a few `go.mod` files that weren't in the local cache; the user allowed the download. `go.mod` lost Wails and its own dependencies; `gopkg.in/yaml.v2` and `github.com/dlclark/regexp2` are now listed as indirect, which Wails' graph pinned before.
- `build/config.yml` and `build/windows/` (`icon.ico`, `info.json`, `wails.exe.manifest`). Only Wails' syso step read them. MyGo makes the exe's icon, version info and manifest from `mygo.json` and `build/appicon.png`, which stays. `build/agent-desktop/` stays too.
- `Taskfile.yml` and `VERSION`.
- The bench's WebView2 mode: the DevTools client, its probes, memory dumps and Markdown report, and the `-native` flag. The bench now drives only the bench build of `cmd/desktop`, named with `-app` (default `bin/jenab-bench-app.exe`). Its memory sums the app's process tree, without the WebView2 split.
- CI: the Bun setup, the frontend build and tests, the bindings check, and the Wails Linux packages. MyGo builds without cgo. CI still can't run (the billing lock), so the workflow is untested.
- `.gitattributes` lost the bindings rule, `.gitignore` lost `.task/`. `node_modules/` and `dist/` stay ignored for `mockups/`.

## The rename

`cmd/desktop-native` is `cmd/desktop`. `mygo.json`'s `main`, the bench, the docs and comments follow.

## The version

- `mygo.json`'s `version` is the one place. A root package, `jenab` (`jenab.go`), embeds `mygo.json`, and `internal/version` reads the number from it. So `go run`, the bench build and tests show the same number as the release.
- The commit and the modified flag come from `debug.ReadBuildInfo`: `vcs.revision`, cut to 7, and `vcs.modified`. `-X …/internal/version.number`, `.commit` and `.modified` still override, for a build outside a git checkout.
- `Info.Wails` is gone; *About*'s second line is the Go version and the OS.
- Tests: `mygo.json`'s version is a semantic version, its identifier is `appinfo.Identifier`, its publisher is `appinfo.Name` (TASK-059's check compared it with `build/windows/info.json`, now gone), and `Get` returns that version. `vcs` and `configVersion` have table tests. A test binary has no VCS stamp, so `Get` there has no commit.
- The bench build's `/state` has a new field, `build`: the version with the commit, as *About* shows it.

## Checks

All on this branch, on Windows 11.

- `go build ./...`, `go vet ./...` and `go test ./...`: pass.
- `go tool mygo build` (MyGo 0.9.1): `Jenab.exe` (39.3 MB) and `jenab-setup-0.1.0-dev-amd64.exe` (10.2 MB) in `bin/native/windows-amd64`, from `cmd/desktop`, in 20–27 s. NSIS was already in `%LOCALAPPDATA%\mygo`, so nothing was downloaded.
- **VCS stamping:** MyGo's build runs `go build -trimpath -ldflags …` with no `-buildvcs=false` (read in its source, `cmd/mygo/build.go`). `go version -m` on the release exe: `path github.com/amir-saatchi/jenab/cmd/desktop`, `vcs.revision=9f02ceb…`, `vcs.modified=false`, and no Wails module.
- ***About* in a release build:** the release exe has no bench port, so a second build ran through the same pipeline with the bench tag (`GOFLAGS=-tags=bench go tool mygo build -skip-installer -o <scratch>`). It started with `LOCALAPPDATA`, `APPDATA`, `USERPROFILE` and `TEMP` in a scratch folder. `GET /state` gave `"build": "0.1.0-dev (9f02ceb)"`. UI Automation pressed the rail's *Settings* button, and `/shot` showed *About*: "Jenab 0.1.0-dev (9f02ceb)" and "Go 1.27.3, windows/amd64". `POST /quit` closed it. No system dialog opened. The first run of this check, on `c7380af`, showed the stale `task build:native` line that `9f02ceb` fixed.
- **`go tool mygo dev`:** with the bench tag, a scratch profile, Go's caches pinned to their usual folders and `GOPROXY=off`, it built in 4.1 s and started the app, which loaded with `build` `0.1.0-dev (9f02ceb)`. `POST /quit` closed the app, and `mygo dev` ended with it. A first try without pinned caches put Go's build cache in the scratch profile and rebuilt everything; it was stopped. The module cache stayed in place, so nothing was downloaded.
- **The bench:** `go build -tags bench -o bin/jenab-bench-app.exe ./cmd/desktop`, then `go run ./cmd/jenab-bench -app bin/jenab-bench-app.exe -hold 10s`: the app opened "History 200" and held for 10 s, and the bench ended with exit 0.

## The docs

- `Docs/DEVELOPMENT.md`: the plain commands (`go tool mygo build`, `go tool mygo dev`, `go vet ./...`, `go test ./...`), the version, the bench build, and testing updates with `cmd/desktop`.
- `CODE-OUTLINE.md` and `SPEC.md`: the Wails, React and Bun parts TASK-053 marked are gone; where a part described the app, it now describes the native one.
- `README.md`, `PROPOSAL.md`, `PORTFOLIO.md`, Gates 0, 2 and 4, and `app_structure.svg`: current facts updated.
- Comments in `internal/native`, `internal/lifecycle` and `internal/update` name "the React app's" files instead of `frontend/src/…`, so the provenance stays.

## What the grep still finds

`git grep -i` for `wails`, `bun ` and `frontend/`, outside `Docs/Tickets` and `spikes`:

- **History:** `QUESTIONS.md` (decision records), Gates 1–3, PROPOSAL's Phase 1 roadmap and its decided question 4, PORTFOLIO's past experience, and SPEC's notes on SPIKE-019 and on moving from the Wails app's data. `DEVELOPMENT.md` says "No Node, Bun or other JavaScript tool is needed" and points to Q1's map "from the Wails days".
- **The services:** comments in `internal/app` (8 files), `internal/bucket/serve.go` and `internal/project/events.go` still name Wails. The rule kept the services unchanged. Rewording them is a comment-only follow-up.
- `internal/agent/desktop_macro_live_test.go`: a search query in test data ("Wails v3 Go desktop framework").
- `.claude/skills/golang-*`: third-party skill docs.
- `mockups/` still uses Vite, React and Bun. It is a design project, not part of the build. `skills-lock.json` and the `.gitignore` entry for `.claude/skills/shadcn/` belong to it and to the old frontend; they stay until the user decides.

## Notes

- Nothing in `internal/agent`, `internal/app`, the stores or the providers changed.
- One Jenab process that the user had started was running during the checks; it was left alone. Every copy started here ran from the scratch folder and quit.
- Nothing was pushed or merged.
