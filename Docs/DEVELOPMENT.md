# Development setup

How to build, run and test Jenab. The code layout is in [Code-Design/QUESTIONS.md](Code-Design/QUESTIONS.md) (Q1) and [CODE-OUTLINE](Code-Design/CODE-OUTLINE.md).

**Two desktop apps until TASK-061** (SPEC 5.8):
- `cmd/desktop-native`, the native app on [MyGo](https://github.com/egoist/mygo), with no webview. New screens and features are built here.
- `cmd/desktop`, the Wails app with the React frontend in `frontend/`. It still ships, and it gets bug fixes only.

Both build the same services and take the same lock, so only one runs at a time. TASK-061 switches to the native app and removes Wails, `frontend/` and Bun. Every step below marked *(Wails)* goes then.

## Tools

| Tool | Version | Notes |
|---|---|---|
| Go | 1.27.3 or newer | `go.mod` needs Go 1.27.3; with `GOTOOLCHAIN=auto` (the default) Go downloads it on the first build |
| Bun | 1.3 or newer | *(Wails)* for the frontend; not npm (the lock file is `bun.lock`) |
| Git | any recent | |
| Wails CLI (`wails3`) | `v3.0.0` | *(Wails)* the same version as the library in `go.mod` |

**Per platform:**

- **Windows 11:** nothing else. Neither app needs a C compiler (N-51); MyGo is pure Go, and WebView2 comes with Windows.
- **macOS** *(Wails)*: the Xcode Command Line Tools (`xcode-select --install`). Wails uses cgo for its Cocoa layer.
- **Linux (beta)** *(Wails)*: a C compiler, `pkg-config`, GTK 4 and WebKitGTK 6.0. On Ubuntu 24.04 or Debian 13:
  ```bash
  sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
  ```
  On Fedora: `gtk4-devel` and `webkitgtk6.0-devel`.

MyGo comes through `go.mod`, pinned (v0.9.1); there is nothing to install. The native app runs on Windows only for now; macOS and Linux come with TASK-060.

*(Wails)* Install the Wails CLI with Go:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
```

It has [Task](https://taskfile.dev) built in, so `wails3 task <name>` runs the tasks in [`Taskfile.yml`](../Taskfile.yml). No separate Task install is needed.

## Build and run

### The native app

```bash
go run ./cmd/desktop-native
```

- It builds the services as the Wails app does and uses the same data folder, settings and keychain entries. The Wails app takes the same lock, so quit one before starting the other.
- Closing the window only hides it. Quit from the tray icon's menu or with *Quit Jenab* on the rail.
- To build it with the version stamped in, and as a window app with no console:

  ```bash
  go build -trimpath -ldflags="-H windowsgui -X $(go list -m)/internal/version.number=$(cat VERSION)" -o bin/jenab-native.exe ./cmd/desktop-native
  ```

  The Taskfile has no task for it yet; the installer and the release build come with TASK-059.
- For a scratch user folder, point `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` (Windows) at a temporary folder. `USERPROFILE` matters: the default data folder is `%USERPROFILE%\Jenab`, and the start-up scan reads it.

### The Wails app *(Wails)*

| Command | What it does |
|---|---|
| `wails3 task dev` | runs the app with hot reload: Vite serves the UI, and the Go side rebuilds and restarts on save |
| `wails3 task build` | builds `bin/jenab.exe` (`bin/jenab` on macOS and Linux) for production |
| `wails3 task build DEV=true` | a dev build: no minifying, and the dev tools work |
| `wails3 task run` | runs the built app |
| `wails3 task bindings` | regenerates `frontend/bindings` after a service or event changes |
| `wails3 task check:bindings` | fails if the committed bindings differ from fresh ones (CI runs it) |
| `wails3 task test` | `go vet` and `go test` |
| `wails3 task bench` | Windows: builds the app with the `bench` tag and measures it (`cmd/jenab-bench`, P1-18) |

- The Go build embeds `frontend/dist`. `wails3 task build` builds the frontend and the bindings first.
- `frontend/bindings` is generated but committed, so the frontend builds without Go. Commit it with the Go change that made it.
- The frontend uses `@wailsio/runtime`, pinned to the same version as the Wails library. Upgrade both together.
- `wails3 task dev` serves Vite on port 9245; set `WAILS_VITE_PORT` to change it.
- On Windows, the build adds the icon and manifest from `build/windows` through a `.syso` file in `cmd/desktop` (git-ignored).
- **Version:** set in `VERSION` at the repository root (semantic versioning, e.g. `0.1.0-dev`). `wails3 task build`, `dev` and `bench` stamp it into `internal/version` with the git short commit, and with "modified" when tracked files had changes. A build without the Taskfile, such as `go run ./cmd/desktop`, says `dev`. `build/config.yml`, `build/windows/info.json` (four numbers for Windows' fixed versions, e.g. `0.1.0.0`) and `frontend/package.json` carry the same number; `go test ./internal/version` fails when one differs. To bump it, edit `VERSION` and those three files, then tag the release commit `vX.Y.Z`.

Without the CLI, `go run ./cmd/desktop` still works after `bun run build` in `frontend`.

## Tests

```bash
go vet ./...
go test ./...
```

- `./...` skips `spikes/` and any `node_modules` (the `ignore` block in `go.mod`). Each spike is its own module; run it from its folder.
- `-race` needs cgo, so it runs in CI on macOS and Linux. On Windows, use WSL or leave it to CI.
- The test layers (unit, fuzz, integration, spike regressions, scenarios) are in QUESTIONS.md Q34–35.

### The native UI's tests

They are ordinary Go tests and need no window:

```bash
go test ./internal/native/...
```

- **Kit components** are tested headless with MyGo's `ui.NewTester` (`kit.NewTester` for the screens' tests, which can't import `ui`). Each component's test builds it in a light and a dark window, finds it by its accessible label, checks that Tab reaches it (or passes over it), and shows right-to-left text in it. A new kit component comes with such a test.
- **The kit's own checks:** the layout isn't mirrored in a right-to-left window, the fonts draw their weights, code tab stops, lists and tables take their direction, and `TestOnlyTheKitImportsMyGoUI`, which fails on any import of `github.com/egoist/mygo/ui` outside the kit.
- **The stores** (`internal/native/state`) and the chat's logic (`internal/native/chat/logic`) are tested without a window. Their tests are the TypeScript tests, ported.
- **Screens** (`shell`, `chat`) are tested headless against `statetest.Fake`, in-memory services.
- On Windows the tester draws text through DirectWrite, as the window does. It sees text and access names, not pixels: a glyph drawn wrong passes.

### The Wails app's tests *(Wails)*

The frontend tests (the stores, the formatting and the contrast check) run with Bun, in `frontend`:

```bash
bun test
```

To check the UI in a normal browser against the real Go side, build the server mode and open `http://127.0.0.1:9310`:

```bash
go build -tags server -o bin/jenab-server.exe ./cmd/desktop
WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT=9310 bin/jenab-server.exe
```

It uses your real data folder. For a scratch one, point `LOCALAPPDATA` and `USERPROFILE` (Windows) or `HOME` somewhere else.

Windows reserves blocks of ports for Hyper-V, WSL and Docker, and they can move after a restart. If the server says "An attempt was made to access a socket in a way forbidden by its access permissions", the port is in one: list them with `netsh interface ipv4 show excludedportrange protocol=tcp` and pick another.

### The bench

`wails3 task bench` *(Wails)* measures the built app: start-up, idle memory, opening a chat of 200 messages, streaming while you type, and memory with ten answers at once. It uses its own user folder and a local fake model, so no keys are needed, and writes a report to `bin/bench`. A build with the `bench` tag opens WebView2's DevTools port when `JENAB_DEVTOOLS_PORT` is set; shipped builds don't. Keep the app's window visible and leave the machine alone during a run (about 3 minutes).

**The native app (`-native`):** build `cmd/desktop-native` with the `bench` tag, then point the bench at it:

```bash
go build -tags bench -o bin/jenab-native-bench.exe ./cmd/desktop-native
```

```bash
go run ./cmd/jenab-bench -native bin/jenab-native-bench.exe
```

- It uses the same seed, user folder and fake model as the Wails runs, and takes the frame times from MyGo's `MYGO_FRAME_STATS=all`. `-rate` sets the fake model's tokens a second (100 by default).
- The bench build stamps its log in microseconds, adds marks to it, and serves a loopback port the bench drives. It keeps keys in memory (`JENAB_BENCH_KEY_<PROVIDER>`), never in the keychain. The release build has no bench code.
- `-samples` pictures the 20 text samples (Persian, English, mixed, code, tables, lists) in both apps, for comparing with Chromium; it needs `bin/jenab-bench-app.exe` from `wails3 task bench` too.
- `-hold 10m` keeps the native app open on a chat (`-hold-chat`) for checks by hand or with UI Automation. `-gemini <model>` with `-hold` connects one Gemini model with `GEMINI_API_KEY`, which counts against the key's requests.
- Screenshots in `bin/bench` can hold user-name paths; don't commit them.

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

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every push to `main` and every pull request, on Windows, macOS and Linux:

1. builds the frontend and runs `bun test` *(Wails)*
2. the bindings check *(Wails)*, `go fmt` check (Linux), `go vet`
3. `go test`, with `-race` on macOS and Linux
4. builds `cmd/desktop` *(Wails)*

`go test` covers the native UI's tests. CI doesn't build `cmd/desktop-native` yet; TASK-060 adds it, with macOS and Linux.

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
- MyGo is pinned in `go.mod` (v0.9.1). It is pre-v1 and releases often, so a move to a new version is its own change: it touches only `internal/native/kit` and `cmd/desktop-native` (CODE-OUTLINE 10), and the kit's tests must pass. Ask before downloading a new version.
- *(Wails)* Wails is pinned to `v3.0.0`. Since v3.0.0 Wails releases weekly (v3.1.0, v3.2.0, ...). An upgrade touches only `internal/webui` and `internal/update` (CODE-OUTLINE 1); test it on all three platforms.
