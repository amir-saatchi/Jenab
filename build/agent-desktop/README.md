# agent-desktop

The installer puts `agent-desktop.exe` (Windows x64, v1.0.2) next to `jenab.exe`, with this folder's `LICENSE` and `NOTICE`. The binary is not in git (`*.exe` is ignored).

For development, put the binary from the release archive in `bin/` beside the built app. Jenab refuses any file whose SHA-256 is not the one in `internal/desktop/desktop.go`. To update: change `Version` and `SHA256` there, the hash in `NOTICE`, and check the new release's envelope version and commands against the tests.
