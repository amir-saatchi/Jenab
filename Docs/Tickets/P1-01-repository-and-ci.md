# P1-01 — Repository skeleton and CI
**Type:** Task
**Status:** In progress (CI runs after the first push)
**Gate:** 3 (Phase 1)
**Requirements:** N-50, N-51

## Goal
The Go module, a first window and CI on all three platforms, so every later ticket starts from a green build.

## Scope
- `go.mod`: module `github.com/amir-saatchi/jenab`, Go 1.26, Wails `v3.0.0-beta.26` pinned; `ignore` keeps `spikes/` and `node_modules` out of `./...`.
- `cmd/desktop`: opens the main window, at least 640 × 480 (5.12).
- `frontend/`: Vite, React and TypeScript, built with Bun and embedded by `frontend/embed.go` (Q1). A placeholder page until P1-14.
- `.github/workflows/ci.yml`: Windows, macOS and Linux (DEVELOPMENT.md).
- [DEVELOPMENT.md](../DEVELOPMENT.md).

## Done when
CI passes on Windows, macOS and Linux, and `go run ./cmd/desktop` opens the window on Windows.

## Result
- 2026-10-01: vet, test and build pass on Windows with `CGO_ENABLED=0`; the window opens.
- Wails uses cgo on macOS (Cocoa) and Linux (GTK 4, WebKitGTK 6.0), so N-51 now reads "no cgo in our code or dependencies; Windows builds need no C compiler".
