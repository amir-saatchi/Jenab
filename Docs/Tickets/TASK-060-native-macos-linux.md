# TASK-060 — The native app on macOS and Linux
**Type:** Task
**Status:** Open (waits for CI and a Mac)
**Gate:** 3
**Needs:** TASK-054, CI on GitHub Actions

## Goal
SPIKE-048 built and measured the native app on Windows 11 only. MyGo has macOS and Linux backends. Before Jenab ships there again, the native app must build and work on both.

## Scope
1. **Builds** for macOS and Linux in CI, next to Windows. CI also builds the Windows exe of `cmd/desktop-native`, which it doesn't yet (found in TASK-053): `go test ./...` compiles its packages but not the binary.
2. **The checks from SPIKE-048 item 9** on each system: text (the 20 samples), Persian input, streaming, memory, and the screen reader (VoiceOver, Orca).
3. **What differs:** fonts (MyGo's text path on each system), the tray, notifications, hiding on close, single instance.

## Rules
- Runs when GitHub Actions can run again and a Mac is at hand.
- Fixes go into the kit or into small per-system files, not into screens.

## Done when
- CI builds `cmd/desktop-native` on Windows, macOS and Linux, and the headless tests pass on each.
- The checks above are recorded for each system, with what failed.

## Result
