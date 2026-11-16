# Development setup

How to build, run and test Jenab. The code layout is in [CODE-OUTLINE](Code-Design/CODE-OUTLINE.md); [QUESTIONS.md](Code-Design/QUESTIONS.md) (Q1) has the first map, from the Wails days.

The app is `cmd/desktop`, a native app on [MyGo](https://github.com/egoist/mygo) with no webview (SPEC 5.8). Its screens are in `internal/native`.

## Tools

| Tool | Version | Notes |
|---|---|---|
| Go | 1.27.3 or newer | `go.mod` needs Go 1.27.3; with `GOTOOLCHAIN=auto` (the default) Go downloads it on the first build |
| Git | any recent | the build takes the commit from it |

- MyGo comes through `go.mod`, pinned (v0.9.1), and its CLI is a `tool` there, so `go tool mygo` needs no install.
- No C compiler is needed (N-51): MyGo is pure Go. No Node, Bun or other JavaScript tool is needed either.
- The app runs on Windows 11 only for now; macOS and Linux come with TASK-060.

## Build and run

```bash
go run ./cmd/desktop
```

To rebuild and restart the app on every Go change:

```bash
go tool mygo dev
```

- It uses the normal data folder, settings and keychain entries. Only one copy runs at a time (the app lock).
- Closing the window only hides it. Quit from the tray icon's menu or with *Quit Jenab* on the rail.
- For a scratch user folder, point `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` (Windows) at a temporary folder. `USERPROFILE` matters: the default data folder is `%USERPROFILE%\Jenab`, and the start-up scan reads it.

### The release and its installer

```bash
go tool mygo build
```

- It reads `mygo.json` and writes `bin/native/windows-amd64/Jenab.exe` and `jenab-setup-<version>-amd64.exe`.
- The exe is built with `-trimpath` and `-H windowsgui`. MyGo makes its icon (from `build/appicon.png`), version info and manifest from `mygo.json`.
- The first build downloads NSIS into `%LOCALAPPDATA%\mygo`. MyGo pins its version and SHA-256.
- The installer installs per user into `%LOCALAPPDATA%\Programs\Jenab`, adds a Start menu entry and an entry under Apps, and closes a running Jenab first. `/S` installs silently, `/D=<folder>` picks the folder. The uninstaller removes the program only; settings, projects and keys stay.
- `mygo.json` has no `updates` block yet, so release builds can't update themselves; Settings says so. With `updates` (the feed URL and the public key) and the private key in `MYGO_UPDATER_PRIVATE_KEY`, the build also writes the signed update archive and `update-windows-amd64.json`. Never commit a private key.

### The version

- `mygo.json`'s `version` is the one place the version is set (semantic versioning, e.g. `0.1.0-dev`). The root package `jenab` (`jenab.go`) embeds `mygo.json`, and `internal/version` reads the number from it.
- The commit and the "modified" flag come from Go's build info (`vcs.revision`, shortened to 7, and `vcs.modified`). `go build` and `go tool mygo build` stamp them in a git checkout. `go test` builds carry no VCS info, so there the commit is empty.
- `-ldflags -X github.com/amir-saatchi/jenab/internal/version.number=…` (and `.commit`, `.modified`) still override them, e.g. through `MYGO_LDFLAGS`.
- `go test ./internal/version` checks that the version is a semantic version, that `mygo.json`'s identifier matches `internal/appinfo`, and that its publisher is the app's name.
- To bump the version, edit `version` in `mygo.json`, then tag the release commit `vX.Y.Z`.

## Tests

```bash
go vet ./...
go test ./...
```

- `./...` skips `spikes/` (the `ignore` block in `go.mod`). Each spike is its own module; run it from its folder.
- `-race` needs cgo, so it runs in CI on macOS and Linux. On Windows, use WSL or leave it to CI.
- The test layers (unit, fuzz, integration, spike regressions, scenarios) are in QUESTIONS.md Q34–35.

### The native UI's tests

They are ordinary Go tests and need no window:

```bash
go test ./internal/native/...
```

- **Kit components** are tested headless with MyGo's `ui.NewTester` (`kit.NewTester` for the screens' tests, which can't import `ui`). Each component's test builds it in a light and a dark window, finds it by its accessible label, checks that Tab reaches it (or passes over it), and shows right-to-left text in it. A new kit component comes with such a test.
- **The kit's own checks:** the layout isn't mirrored in a right-to-left window, the fonts draw their weights, code tab stops, lists and tables take their direction, the colours' contrast, and `TestOnlyTheKitImportsMyGoUI`, which fails on any import of `github.com/egoist/mygo/ui` outside the kit.
- **The stores** (`internal/native/state`) and the chat's logic (`internal/native/chat/logic`) are tested without a window.
- **Screens** (`shell`, `chat`) are tested headless against `statetest.Fake`, in-memory services.
- On Windows the tester draws text through DirectWrite, as the window does. It sees text and access names, not pixels: a glyph drawn wrong passes.

### The bench

The bench measures the app: start-up, idle memory, opening a chat of 200 messages, streaming while you type, and memory with ten answers at once. Build the app with the `bench` tag, then run the bench (Windows only):

```bash
go build -tags bench -o bin/jenab-bench-app.exe ./cmd/desktop
```

```bash
go run ./cmd/jenab-bench -app bin/jenab-bench-app.exe
```

- It uses its own user folder and a local fake model, so no keys are needed, and writes a report to `bin/bench`. It takes the frame times from MyGo's `MYGO_FRAME_STATS=all`. `-rate` sets the fake model's tokens a second (100 by default).
- Keep the app's window visible and leave the machine alone during a run: a hidden window gets fewer frames.
- The bench build stamps its log in microseconds, adds marks to it, and serves a loopback port the bench drives (`JENAB_BENCH_PORT`). It keeps keys in memory (`JENAB_BENCH_KEY_<PROVIDER>`), never in the keychain. The release build has no bench code.
- `-samples` pictures the 20 text samples (Persian, English, mixed, code, tables, lists).
- `-hold 1m` keeps the app up on a scratch profile, on a chat (`-hold-chat`), for checks by hand or with UI Automation. `-gemini <model>` with `-hold` connects one Gemini model with `GEMINI_API_KEY`, which counts against the key's requests. `-dev-tools` with `-hold` turns the developer tools on, and the fake model answers a message with "use a tool" in it with two `query` calls first, one of them failing. While it holds, the port takes `/state`, `/open`, `/send`, `/key`, `/front`, `/size` (the window's size, for the narrow layout under 900 px), `/shot` and `/quit`, and `POST /file?path=<file>` uploads a file into the open chat's project and opens it, with the chat in the dock.
- Screenshots in `bin/bench` can hold user-name paths; don't commit them.

### Testing updates and the installer

Test updates only against a local feed, never the public one, and keep test keys outside the repo.

1. Make a key pair in a scratch folder: `go tool mygo keygen -o <scratch>/keys`.
2. Copy `mygo.json` into a folder under `bin/` and give it a test version, `"main": "github.com/amir-saatchi/jenab/cmd/desktop"`, an absolute `icon`, and `"updates": {"publicKey": "<the .pub>", "url": "https://updates.invalid/jenab", "deltas": 0}`.
3. Build each version with the bench tag and the private key:

   ```bash
   GOFLAGS=-tags=bench MYGO_UPDATER_PRIVATE_KEY="$(cat <scratch>/keys/mygo-update.key)" MYGO_LDFLAGS="-X github.com/amir-saatchi/jenab/internal/version.number=0.1.2" go tool mygo build -skip-installer -o <scratch>/out/v2 bin/<folder>
   ```

4. Install the first version with its installer (`/S /D=<scratch>\install\Jenab`) and start it with `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` in a scratch folder, `JENAB_BENCH_PORT` set, and `JENAB_UPDATE_FEED` set to a folder (or a loopback URL) that holds the next version's `update-windows-amd64.json` and archive. The bench build reads the feed from there instead of the URL in the manifest. Run the installer and the uninstaller with the same `APPDATA`: the Start menu entry goes under it, and an uninstaller with another one leaves the entry behind.
5. Drive it on the bench port: `POST /update?do=check|install|restart`, `POST /hide`, `POST /notify` (a test notification), and `GET /state` for the version, the projects, whether the window is shown and focused, and the updater's state.

### A fake price site for manual runs

Manual runs that fetch daily crypto pages, such as P2-14's, can use fixed pages served on this machine instead of the internet. From the repository root:

```bash
go run ./cmd/jenab-fakesite
```

- It serves `bench-bitcoin`'s pages, 19 days from 5 to 23 October 2026, at `http://127.0.0.1:8099/`: an index at `/` and one day at `/crypto/YYYY-MM-DD`, e.g. `http://127.0.0.1:8099/crypto/2026-10-05`. Give the agent that pattern. Other days and paths are 404.
- Each request is logged with its time, path and status, so the terminal shows what the agent fetched. Stop it with Ctrl+C.
- The pages are the scenario files in `scenarios/bench-bitcoin/pages` and `scenarios/bench-bitcoin-long/pages`, read at start. A day in both must have the same text, or it names the files and exits. `-pages <folder>` (repeatable) serves other folders; `-addr` must stay on a loopback address.
- The app asks before reaching loopback addresses (SPEC 6.7): the first fetch shows a card; choose *Allow in this project*, or add `127.0.0.1` under *Private-network exceptions* in the project settings first.

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every push to `main` and every pull request, on Windows, macOS and Linux: the `go fmt` check (Linux), `go vet`, and `go test`, with `-race` on macOS and Linux. `go test` covers the native UI's tests.

On Windows, CI builds with `CGO_ENABLED=0`, so a dependency that needs cgo fails there.

## Keys for development

- In the app, keys go to the OS keychain through *Settings → Models* (SPEC 6.7). They are never kept in files.
- The spikes read keys from `.env` at the repository root. It is git-ignored; never commit it, and never paste a key into an issue or a log.
- Only synthetic prompts go to cloud models during development.

## Reference models

Spikes and tests run on three models (the first decided in SPIKE-030, the second on 2026-11-10, the third on 2026-11-15):
- **Gemini `gemini-3.5-flash-lite`**, the main one. It is fast and has a full JSON-schema mode. The free tier allows 500 requests a day, reset at midnight Pacific time, and Google may use free-tier prompts.
- **NVIDIA `mistralai/mistral-large-4-1t-a52b`**, for the `openai_compatible` kind, which most providers use. NVIDIA's free endpoints allow about 40 requests a minute; requests are slower (about 9 s) and sometimes hang, so runs need a timeout. On the scenario suite it passed as many runs as `gemini-3.5-flash-lite` ([GEMINI-BASELINE-report](Tickets/reports/GEMINI-BASELINE-report.md)).
- **Anthropic `claude-haiku-5-5`**, for the `anthropic` kind. It is fast (about 2 s a request) and thinks by default. It is paid: a full suite pass cost about $0.11 ([HAIKU-PASS-report](Tickets/reports/HAIKU-PASS-report.md)), so live runs need the user's OK and a cap.

Ollama Cloud's `gemma4:31b` was the third, for the Ollama backend, until 2026-11-15: Ollama removed its free tier ([MODELS](MODELS.md#not-working-now)). The Ollama backend is now checked with local Ollama only (SPIKE-038's `lfm3:1.7b`). Older reports still have gemma4:31b's numbers.

A failure on any other model is recorded, not worked around. Fixes are made the same way for every model (SPEC 1).

[MODELS](MODELS.md) lists every model the development keys can reach, with its limits and when it was last checked.

## Commits

- Sign off every commit with `git commit -s` ([CONTRIBUTING](../CONTRIBUTING.md)).
- `.gitattributes` normalizes line endings, so Windows and macOS checkouts give the same diffs.
- MyGo is pinned in `go.mod` (v0.9.1). It is pre-v1 and releases often, so a move to a new version is its own change: it touches only `internal/native/kit` and `cmd/desktop` (CODE-OUTLINE 10), and the kit's tests must pass. Ask before downloading a new version.
