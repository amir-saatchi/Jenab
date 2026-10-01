# Development setup

How to build, run and test Jenab. The code layout is in [Code-Design/QUESTIONS.md](Code-Design/QUESTIONS.md) (Q1).

## Tools

| Tool | Version | Notes |
|---|---|---|
| Go | 1.26 or newer | the version in `go.mod` |
| Bun | 1.3 or newer | for the frontend; not npm (the lock file is `bun.lock`) |
| Git | any recent | |

**Per platform:**

- **Windows 11:** nothing else. WebView2 comes with Windows, and the build needs no C compiler (N-51).
- **macOS:** the Xcode Command Line Tools (`xcode-select --install`). Wails uses cgo for its Cocoa layer.
- **Linux (beta):** a C compiler, `pkg-config`, GTK 4 and WebKitGTK 6.0. On Ubuntu 24.04 or Debian 13:
  ```bash
  sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
  ```
  On Fedora: `gtk4-devel` and `webkitgtk6.0-devel`.

The Wails CLI (`wails3`) is not needed yet. It comes with P1-13, for the generated bindings and the dev mode with hot reload.

## Build and run

The Go build embeds `frontend/dist`, so build the frontend first:

```bash
cd frontend
bun install
bun run build
cd ..
go run ./cmd/desktop
```

On Windows, `go run` also opens a console window. For a build without it:

```bash
go build -ldflags "-H=windowsgui" -o jenab.exe ./cmd/desktop
```

Rebuild the frontend after changing it. Until P1-13, `bun run dev` only serves the UI in a browser, without Go.

## Tests

```bash
go vet ./...
go test ./...
```

- `./...` skips `spikes/` and any `node_modules` (the `ignore` block in `go.mod`). Each spike is its own module; run it from its folder.
- `-race` needs cgo, so it runs in CI on macOS and Linux. On Windows, use WSL or leave it to CI.
- The test layers (unit, fuzz, integration, spike regressions, scenarios) are in QUESTIONS.md Q34–35.

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every push to `main` and every pull request, on Windows, macOS and Linux:

1. builds the frontend
2. `go fmt` check (Linux), `go vet`
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
