# Development setup

How to build, run and test Jenab. The code layout is in [Code-Design/QUESTIONS.md](Code-Design/QUESTIONS.md) (Q1).

## Tools

| Tool | Version | Notes |
|---|---|---|
| Go | 1.26 or newer | the version in `go.mod` |
| Bun | 1.3 or newer | for the frontend; not npm (the lock file is `bun.lock`) |
| Git | any recent | |
| Wails CLI (`wails3`) | `v3.0.0-beta.26` | the same version as the library in `go.mod` |

**Per platform:**

- **Windows 11:** nothing else. WebView2 comes with Windows, and the build needs no C compiler (N-51).
- **macOS:** the Xcode Command Line Tools (`xcode-select --install`). Wails uses cgo for its Cocoa layer.
- **Linux (beta):** a C compiler, `pkg-config`, GTK 4 and WebKitGTK 6.0. On Ubuntu 24.04 or Debian 13:
  ```bash
  sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
  ```
  On Fedora: `gtk4-devel` and `webkitgtk6.0-devel`.

Install the Wails CLI with Go:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
```

It has [Task](https://taskfile.dev) built in, so `wails3 task <name>` runs the tasks in [`Taskfile.yml`](../Taskfile.yml). No separate Task install is needed.

## Build and run

| Command | What it does |
|---|---|
| `wails3 task dev` | runs the app with hot reload: Vite serves the UI, and the Go side rebuilds and restarts on save |
| `wails3 task build` | builds `bin/jenab.exe` (`bin/jenab` on macOS and Linux) for production |
| `wails3 task build DEV=true` | a dev build: no minifying, and the dev tools work |
| `wails3 task run` | runs the built app |
| `wails3 task bindings` | regenerates `frontend/bindings` after a service or event changes |
| `wails3 task check:bindings` | fails if the committed bindings differ from fresh ones (CI runs it) |
| `wails3 task test` | `go vet` and `go test` |

- The Go build embeds `frontend/dist`. `wails3 task build` builds the frontend and the bindings first.
- `frontend/bindings` is generated but committed, so the frontend builds without Go. Commit it with the Go change that made it.
- The frontend uses `@wailsio/runtime`, pinned to the same version as the Wails library. Upgrade both together.
- `wails3 task dev` serves Vite on port 9245; set `WAILS_VITE_PORT` to change it.
- On Windows, the build adds the icon and manifest from `build/windows` through a `.syso` file in `cmd/desktop` (git-ignored).

Without the CLI, `go run ./cmd/desktop` still works after `bun run build` in `frontend`.

## Tests

```bash
go vet ./...
go test ./...
```

- `./...` skips `spikes/` and any `node_modules` (the `ignore` block in `go.mod`). Each spike is its own module; run it from its folder.
- `-race` needs cgo, so it runs in CI on macOS and Linux. On Windows, use WSL or leave it to CI.
- The test layers (unit, fuzz, integration, spike regressions, scenarios) are in QUESTIONS.md Q34–35.

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

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every push to `main` and every pull request, on Windows, macOS and Linux:

1. builds the frontend and runs `bun test`
2. the bindings check, `go fmt` check (Linux), `go vet`
3. `go test`, with `-race` on macOS and Linux
4. builds `cmd/desktop`

On Windows, CI builds with `CGO_ENABLED=0`, so a dependency that needs cgo fails there.

## Keys for development

- In the app, keys go to the OS keychain through *Settings → Models* (SPEC 6.7). They are never kept in files.
- The spikes read keys from `.env` at the repository root. It is git-ignored; never commit it, and never paste a key into an issue or a log.
- Only synthetic prompts go to cloud models during development.

## Commits

- Sign off every commit with `git commit -s` ([CONTRIBUTING](../CONTRIBUTING.md)).
- `.gitattributes` normalizes line endings, so Windows and macOS checkouts give the same diffs.
- Wails is pinned to `v3.0.0-beta.26`. An upgrade touches only `internal/app` and `internal/update` (CODE-OUTLINE 1); test it on all three platforms.
