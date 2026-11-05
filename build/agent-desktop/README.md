# agent-desktop

The installer puts `agent-desktop.exe` (Windows x64, v1.2.0) next to `jenab.exe`, with this folder's `LICENSE` and `NOTICE`. The binary is not in git (`*.exe` is ignored).

For development, put the binary from the release archive in `bin/` beside the built app. Jenab refuses any file whose SHA-256 is not the one in `internal/desktop/desktop.go`. To update: change `Version` and `SHA256` there, the hash in `NOTICE`, and check the new release's envelope version (`envelopeVersion`; 1.2.0's is 2.6), error codes and commands against the tests.
